//! Registry snapshots and their restore: a snapshot captures every runtime
//! with its fibers, and restoring makes the registry match the snapshot
//! again by disposing the delta and restarting what went missing.

use crate::core::{Core, RuntimeData};
use crate::fiber::{Fiber, FiberState};
use crate::sync::{Rc, RefCell};
#[cfg(feature = "thread-safe")]
use crate::sync::BorrowExt as _;

/// One live fiber as seen by a snapshot.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FiberSnapshot {
    /// The framework-wide unique id of the fiber.
    pub uid: i64,
    /// The lifecycle state at snapshot time.
    pub state: FiberState,
}

/// One plugin runtime as seen by a snapshot: the plugin identity plus the
/// fibers it currently powers.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RuntimeSnapshot {
    /// The plugin name.
    pub name: String,
    /// The runtime's registry id.
    pub id: u64,
    /// The runtime's live fibers, in registration order.
    pub fibers: Vec<FiberSnapshot>,
}

/// A point-in-time view of the registry: every runtime with its fibers.
/// Produce one with [`crate::Context::snapshot`]; bring it back with
/// [`crate::Context::restore`].
#[derive(Debug, Clone, Default)]
pub struct RegistrySnapshot {
    /// Every runtime live at snapshot time.
    pub runtimes: Vec<RuntimeSnapshot>,
}

fn fiber_snapshot(core: &Core, runtime: &RuntimeData) -> Vec<FiberSnapshot> {
    runtime
        .fibers
        .iter()
        .filter_map(|fid| {
            let data = core.fiber(*fid);
            let f = data.borrow();
            if f.state == FiberState::Disposed {
                return None;
            }
            Some(FiberSnapshot {
                uid: f.uid,
                state: f.state,
            })
        })
        .collect()
}

impl crate::context::Context {
    /// Capture the current registry: every runtime with its fibers.
    ///
    /// # Examples
    ///
    /// ```
    /// use cordis::{plugin, start, Context};
    ///
    /// let ctx = Context::new();
    /// let snapshot = ctx.snapshot();
    /// assert!(snapshot.runtimes.is_empty());
    /// ```
    #[must_use]
    pub fn snapshot(&self) -> RegistrySnapshot {
        let core = self.core.borrow();
        let mut runtimes: Vec<(u64, &RuntimeData)> = core
            .runtimes
            .iter()
            .map(|(id, runtime)| (*id, runtime))
            .collect();
        runtimes.sort_by_key(|(id, _)| *id);
        RegistrySnapshot {
            runtimes: runtimes
                .into_iter()
                .map(|(id, runtime)| RuntimeSnapshot {
                    name: runtime.name.clone(),
                    id,
                    fibers: fiber_snapshot(&core, runtime),
                })
                .collect(),
        }
    }

    /// Make the registry match `snapshot` again. Runtimes that appeared
    /// since the snapshot are disposed; runtimes that went missing are
    /// restarted from their stashed bodies with their last config, on the
    /// context this method is called on. Fibers the snapshot recorded as
    /// active but that are down now restart through their runtime.
    ///
    /// # Panics
    /// When a runtime's fiber slot is missing from the arena, which can
    /// only happen if the core was mutated concurrently.
    pub fn restore(&self, snapshot: &RegistrySnapshot) {
        let (delta, dispose, restart, requeue) = {
            let core = self.core.borrow();
            let delta: Vec<u64> = core
                .runtimes
                .keys()
                .filter(|id| !snapshot.runtimes.iter().any(|snap| snap.id == **id))
                .copied()
                .collect();
            let mut dispose = Vec::new();
            for id in &delta {
                if let Some(runtime) = core.runtimes.get(id) {
                    dispose.extend(runtime.fibers.iter().copied());
                }
            }
            let restart: Vec<u64> = snapshot
                .runtimes
                .iter()
                .filter(|snap| !core.runtimes.contains_key(&snap.id))
                .map(|snap| snap.id)
                .collect();
            let mut requeue = Vec::new();
            for snap in &snapshot.runtimes {
                let Some(runtime) = core.runtimes.get(&snap.id) else {
                    continue;
                };
                let live = fiber_snapshot(&core, runtime);
                if live.len() < snap.fibers.len() {
                    requeue.push(snap.id);
                }
            }
            (delta, dispose, restart, requeue)
        };

        // Dispose the delta first so restarted runtimes see a clean realm.
        // Explicit removals are reversible: keep the body so a later
        // restore can bring the runtime back.
        for id in delta {
            let entry = {
                let core = self.core.borrow();
                let Some(runtime) = core.runtimes.get(&id) else {
                    continue;
                };
                runtime
                    .fibers
                    .last()
                    .and_then(|fid| core.fibers.get(fid.0))
                    .and_then(|slot| slot.as_ref())
                    .and_then(|data| {
                        let f = data.borrow();
                        if f.state == FiberState::Disposed {
                            return None;
                        }
                        Some(f.config.clone())
                    })
                    .map(|config| (runtime.base.clone(), config))
            };
            let mut core = self.core.borrow_mut();
            if let Some(runtime) = core.runtimes.remove(&id) {
                if let Some(entry) = entry {
                    core.stash.insert(id, entry);
                } else {
                    core.runtimes.insert(id, runtime);
                }
            }
        }
        for fid in dispose {
            Fiber::from_parts(Rc::clone(&self.core), fid).dispose();
        }

        // Restart the missing runtimes from their stashed bodies, on the
        // live context calling restore, with no core borrow held while
        // plugin bodies run.
        let restart_bodies: Vec<_> = {
            let core = self.core.borrow();
            restart
                .iter()
                .filter_map(|id| core.stash.get(id).cloned())
                .collect()
        };
        for (base, config) in restart_bodies {
            let _ = crate::plugin::start_base(self, base, config);
        }
        // Fibers of surviving runtimes that died since the snapshot come
        // back through a restart request; the state machine re-runs the
        // body with the current config.
        let requeue_fibers = {
            let core = self.core.borrow();
            let mut out = Vec::new();
            for id in requeue {
                let Some(runtime) = core.runtimes.get(&id) else {
                    continue;
                };
                for fid in runtime.fibers.iter().copied() {
                    let data = core.fiber(fid);
                    let f = data.borrow();
                    if f.state == FiberState::Pending {
                        out.push(fid);
                    }
                }
            }
            out
        };
        let mut core = self.core.borrow_mut();
        for fid in requeue_fibers {
            let data = core.fiber(fid);
            data.borrow_mut().restart_requested = true;
            core.queue(fid);
        }
    }
}

impl Fiber {
    pub(crate) const fn from_parts(core: Rc<RefCell<Core>>, id: crate::fiber::FiberId) -> Self {
        Self { core, id }
    }
}
