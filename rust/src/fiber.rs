//! Fibers: effect scopes with a lifecycle, and the transition state machine.

use crate::sync::RefCell;
use std::collections::HashSet;
use crate::sync::Rc;
#[cfg(feature = "thread-safe")]
use crate::sync::BorrowExt as _;

use crate::context::{Context, ContextData, Disposer};
use crate::core::{Bag, Core};

/// The lifecycle state of a fiber, mirroring FiberState upstream.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FiberState {
    /// Waiting for injected services.
    Pending,
    /// The plugin body is executing.
    Loading,
    /// The plugin body completed and its effects are live.
    Active,
    /// The plugin body failed; partial effects were rolled back.
    Failed,
    /// Permanently disposed.
    Disposed,
    /// Effects are being rolled back.
    Unloading,
}

/// Arena index of a fiber inside its core.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub(crate) struct FiberId(pub usize);

/// Introspection view of one registered effect.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct EffectMeta {
    pub label: String,
    pub children: Vec<EffectMeta>,
}

pub(crate) struct FiberData {
    pub id: FiberId,
    pub uid: i64,
    pub ctx: Context,
    pub parent: Context,
    pub config: crate::events::Value,
    pub inject: HashSet<String>,
    pub runtime: Option<u64>,
    pub state: FiberState,
    pub disposed: bool,
    pub restart_requested: bool,
    pub queued: bool,
    pub executing: bool,
    pub bag: Option<Rc<RefCell<Bag>>>,
    pub entry: Option<crate::core::BagEntry>,
}

impl FiberData {
    pub fn new_root(ctx: Context) -> FiberData {
        FiberData {
            id: FiberId(0),
            uid: 0,
            ctx: ctx.clone(),
            parent: ctx,
            config: crate::events::value(()),
            inject: HashSet::new(),
            runtime: None,
            state: FiberState::Active,
            disposed: false,
            restart_requested: false,
            queued: false,
            executing: false,
            bag: Some(Bag::new()),
            entry: None,
        }
    }
}

/// A handle to a fiber: one instance of a running plugin.
#[derive(Clone)]
pub struct Fiber {
    pub(crate) core: Rc<RefCell<Core>>,
    pub(crate) id: FiberId,
}

impl std::fmt::Debug for Fiber {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Fiber")
            .field("uid", &self.uid())
            .field("state", &self.state())
            .finish()
    }
}

impl Fiber {
    fn data(&self) -> Rc<RefCell<FiberData>> {
        self.core.borrow().fiber(self.id)
    }

    /// The current lifecycle state.
    pub fn state(&self) -> FiberState {
        self.data().borrow().state
    }

    /// The framework-wide unique id: 0 for root, -1 after disposal.
    pub fn uid(&self) -> i64 {
        self.data().borrow().uid
    }

    /// The plugin name, resolved through the parent chain like upstream.
    pub fn name(&self) -> String {
        // Lock order: Core before FiberData, everywhere.
        let core = self.core.borrow();
        let data = self.data();
        let f = data.borrow();
        if let Some(runtime_id) = f.runtime
            && let Some(runtime) = core.runtimes.get(&runtime_id)
            && !runtime.name.is_empty()
        {
            return runtime.name.clone();
        }
        let is_root = f.runtime.is_none();
        let parent = f.parent.clone();
        drop(f);
        drop(data);
        drop(core);
        if is_root {
            "root".to_string()
        } else {
            parent.fiber().name()
        }
    }

    /// The context owned by this fiber.
    pub fn context(&self) -> Context {
        self.data().borrow().ctx.clone()
    }

    /// The introspection tree of live effects.
    pub fn effects(&self) -> Vec<EffectMeta> {
        let data = self.data();
        let f = data.borrow();
        match &f.bag {
            Some(bag) => Bag::meta(bag),
            None => Vec::new(),
        }
    }

    pub(crate) fn assert_active(&self) -> crate::Result<()> {
        if self.data().borrow().disposed {
            return Err(crate::Error::InactiveEffect);
        }
        Ok(())
    }

    /// Permanently dispose the fiber: effects roll back, it leaves its
    /// plugin runtime and can never activate again. Idempotent. Disposing
    /// the root fiber restarts it instead.
    pub fn dispose(&self) {
        core_enter_leave(self, |fiber| {
            {
                let data = fiber.data();
                let mut f = data.borrow_mut();
                if f.runtime.is_none() {
                    drop(f);
                    fiber.restart_root();
                    return;
                }
                if f.disposed {
                    return;
                }
                f.disposed = true;
            }
            let runtime_id = {
                let data = fiber.data();
                let f = data.borrow();
                f.runtime.unwrap()
            };
            {
                let mut core = fiber.core.borrow_mut();
                if let Some(runtime) = core.runtimes.get_mut(&runtime_id) {
                    runtime.fibers.retain(|id| *id != fiber.id);
                    if runtime.fibers.is_empty() {
                        core.runtimes.remove(&runtime_id);
                    }
                }
            }
            // Detach from the parent bag without executing the entry,
            // disposal is already happening here.
            let entry = fiber.data().borrow_mut().entry.take();
            if let Some((bag, entry)) = entry {
                Bag::detach(&bag, &entry);
            }
            fiber.core.borrow_mut().queue(fiber.id);
        });
    }

    /// Root fiber disposal: roll back every root scope effect and start over.
    fn restart_root(&self) {
        let bag = {
            let data = self.data();
            let mut f = data.borrow_mut();
            f.bag.take()
        };
        if let Some(bag) = bag {
            Bag::drain(&self.core, &bag);
        }
        self.data().borrow_mut().bag = Some(Bag::new());
    }

    /// Unload and reload the fiber with its current config.
    pub fn restart(&self) -> crate::Result<()> {
        self.assert_active()?;
        core_enter_leave(self, |fiber| {
            {
                let data = fiber.data();
                let mut f = data.borrow_mut();
                f.restart_requested = true;
            }
            fiber.core.borrow_mut().queue(fiber.id);
        });
        Ok(())
    }

    /// Replace the fiber's config and restart it. The restart settles through
    /// the drain queue, so cascading dependency updates never observe torn
    /// states.
    /// Typed variant of [`Fiber::update`]: replaces the config with `C`.
    pub fn update_config<C: crate::sync::Shared>(&self, config: C) -> crate::Result<()> {
        self.update(crate::events::value(config))
    }

    pub fn update(&self, config: crate::events::Value) -> crate::Result<()> {
        self.assert_active()?;
        core_enter_leave(self, |fiber| {
            {
                let data = fiber.data();
                let mut f = data.borrow_mut();
                f.config = config;
                f.restart_requested = true;
            }
            fiber.core.borrow_mut().queue(fiber.id);
        });
        Ok(())
    }
}

fn core_enter_leave(fiber: &Fiber, f: impl FnOnce(&Fiber)) {
    crate::core::enter(&fiber.core);
    f(fiber);
    crate::core::leave(&fiber.core);
}

impl Context {
    /// Attach a cleanup to the current effect scope: the enclosing effect
    /// body while one runs, otherwise the fiber itself. Cleanups run on
    /// rollback, last in, first out.
    pub fn attach(&self, cleanup: impl FnMut() + crate::sync::MaybeSend + 'static) -> crate::Result<Disposer> {
        crate::core::enter(&self.core);
        let result = (|| {
            self.fiber().assert_active()?;
            let bag = self.bag().ok_or(crate::Error::InactiveEffect)?;
            let entry = Bag::push(&bag, "ctx.attach()".to_string(), Box::new(cleanup));
            Ok(Disposer::new({
                let core = Rc::clone(&self.core);
                move || Bag::dispose_entry(&core, &bag, &entry)
            }))
        })();
        crate::core::leave(&self.core);
        result
    }

    /// Execute `f` within an effect scope: registrations made through the
    /// context passed to `f` become children of this effect and roll back
    /// together, last in, first out. On error or panic everything `f`
    /// registered rolls back immediately.
    pub fn effect(&self, label: &str, f: impl FnOnce(&Context) -> crate::Result<()>) -> crate::Result<Disposer> {
        crate::core::enter(&self.core);
        let result = self.effect_inner(label, f);
        crate::core::leave(&self.core);
        result
    }

    fn effect_inner(&self, label: &str, f: impl FnOnce(&Context) -> crate::Result<()>) -> crate::Result<Disposer> {
        self.fiber().assert_active()?;
        let parent = match self.bag() {
            Some(bag) => bag,
            None => return Err(crate::Error::InactiveEffect),
        };
        let child = Bag::new();
        let entry = Bag::push_node(&parent, label.to_string(), Rc::clone(&child));
        let sub = self.with_collect(child);
        let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| f(&sub)));
        match outcome {
            Ok(Ok(())) => {}
            Ok(Err(err)) => {
                Bag::dispose_entry(&self.core, &parent, &entry);
                return Err(err);
            }
            Err(_) => {
                Bag::dispose_entry(&self.core, &parent, &entry);
                return Err(crate::Error::PluginFailed {
                    name: label.to_string(),
                    source: "effect panicked".to_string(),
                });
            }
        }
        Ok(Disposer::new({
            let core = Rc::clone(&self.core);
            move || Bag::dispose_entry(&core, &parent, &entry)
        }))
    }
}

/// Create the fiber data for a plugin instance.
pub(crate) fn new_fiber(
    parent: &Context,
    config: crate::events::Value,
    inject: &[String],
    runtime: u64,
) -> FiberData {
    let mut set = HashSet::new();
    for name in inject {
        set.insert(name.clone());
    }
    let id = FiberId(usize::MAX); // assigned by alloc_fiber
    let ctx = Context {
        core: Rc::clone(&parent.core),
        data: Rc::new(ContextData {
            parent: Some(Rc::clone(&parent.data)),
            fiber: id,
            isolate: None,
            intercept: None,
            filter: None,
            collect: None,
        }),
    };
    let core = Rc::clone(&parent.core);
    let uid = core.borrow_mut().next_uid();
    FiberData {
        id,
        uid,
        ctx,
        parent: parent.clone(),
        config,
        inject: set,
        runtime: Some(runtime),
        state: FiberState::Pending,
        disposed: false,
        restart_requested: false,
        queued: false,
        executing: false,
        bag: None,
        entry: None,
    }
}

/// Fix up the fiber context's back-reference after arena allocation.
pub(crate) fn link_fiber_ctx(core: &Rc<RefCell<Core>>, id: FiberId) {
    let fiber = core.borrow().fiber(id);
    let mut f = fiber.borrow_mut();
    f.id = id;
    f.ctx = Context {
        core: Rc::clone(core),
        data: Rc::new(ContextData {
            parent: Some(Rc::clone(&f.ctx.data)),
            fiber: id,
            isolate: None,
            intercept: None,
            filter: None,
            collect: None,
        }),
    };
}

/// Does every injected service currently resolve for this fiber?
fn deps_ready(core: &Rc<RefCell<Core>>, id: FiberId) -> bool {
    let fiber = core.borrow().fiber(id);
    let f = fiber.borrow();
    if f.disposed {
        return false;
    }
    let c = f.ctx.clone();
    let inject: Vec<String> = f.inject.iter().cloned().collect();
    drop(f);

    let core_ref = core.borrow();
    for name in &inject {
        let key = c
            .find_isolate_override(name)
            .unwrap_or_else(|| core_ref.keys.get(name).copied().unwrap_or(0));
        let Some(imp) = core_ref.store.get(&key) else {
            return false;
        };
        let provider = core_ref.fiber(imp.fiber);
        if provider.borrow().state != FiberState::Active {
            return false;
        }
    }
    true
}

/// One step of the fiber state machine, driven by the drain queue. User
/// code always runs without borrows held.
pub(crate) fn transition(core: &Rc<RefCell<Core>>, id: FiberId) {
    enum Action {
        None,
        Deactivate,
        Activate,
        Restart,
    }

    let action = {
        let fiber = core.borrow().fiber(id);
        let mut f = fiber.borrow_mut();
        if f.executing {
            return;
        }
        let restart = std::mem::take(&mut f.restart_requested);
        let disposed = f.disposed;
        let state = f.state;
        drop(f);

        let ready = deps_ready(core, id);
        let want_active = ready && !disposed;

        let fiber = core.borrow().fiber(id);
        let mut f = fiber.borrow_mut();
        if f.executing {
            return;
        }
        match (state, want_active, restart, disposed) {
            (_, _, _, true) if state != FiberState::Active => {
                f.state = FiberState::Disposed;
                f.uid = -1;
                Action::None
            }
            (FiberState::Active, true, true, false) => {
                f.executing = true;
                f.state = FiberState::Unloading;
                Action::Restart
            }
            (FiberState::Active, false, _, _) => {
                f.executing = true;
                f.state = FiberState::Unloading;
                Action::Deactivate
            }
            (FiberState::Pending | FiberState::Failed, true, _, false) => {
                f.executing = true;
                Action::Activate
            }
            _ => Action::None,
        }
    };

    match action {
        Action::None => {}
        Action::Deactivate => {
            unload(core, id);
            finish_transition(core, id, false);
        }
        Action::Activate => {
            load(core, id);
            finish_transition(core, id, true);
        }
        Action::Restart => {
            unload(core, id);
            let fiber = core.borrow().fiber(id);
            fiber.borrow_mut().state = FiberState::Loading;
            load(core, id);
            finish_transition(core, id, true);
        }
    }
}

fn finish_transition(core: &Rc<RefCell<Core>>, id: FiberId, activated: bool) {
    let fiber = core.borrow().fiber(id);
    let mut f = fiber.borrow_mut();
    f.executing = false;
    if f.disposed {
        f.state = FiberState::Disposed;
        f.uid = -1;
    } else if f.state != FiberState::Failed {
        f.state = if activated {
            FiberState::Active
        } else {
            FiberState::Pending
        };
    }
}

/// Roll back every effect of the current activation, last in, first out.
fn unload(core: &Rc<RefCell<Core>>, id: FiberId) {
    let bag = {
        let fiber = core.borrow().fiber(id);
        let mut f = fiber.borrow_mut();
        f.bag.take()
    };
    if let Some(bag) = bag {
        Bag::drain(core, &bag);
    }
}

/// Execute the plugin body, collecting its effects. On failure the partial
/// effects roll back and the fiber enters StateFailed.
fn load(core: &Rc<RefCell<Core>>, id: FiberId) {
    let (ctx, config, apply, name) = {
        // Lock order: Core before FiberData, everywhere.
        let c = core.borrow();
        let fiber = c.fiber(id);
        let mut f = fiber.borrow_mut();
        f.bag = Some(Bag::new());
        f.state = FiberState::Loading;
        let Some(runtime_id) = f.runtime else {
            f.state = FiberState::Disposed;
            f.uid = -1;
            return;
        };
        let Some(runtime) = c.runtimes.get(&runtime_id) else {
            // The runtime was removed concurrently; the fiber is stale and
            // will settle as disposed on its next transition.
            f.state = FiberState::Disposed;
            f.uid = -1;
            return;
        };
        let (runtime_name, apply) = (runtime.name.clone(), Rc::clone(&runtime.apply));
        (f.ctx.clone(), f.config.clone(), apply, runtime_name)
    };

    let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| apply(&ctx, config)));
    let error = match outcome {
        Ok(Ok(())) => None,
        Ok(Err(err)) => Some(err.to_string()),
        Err(_) => Some("plugin panicked".to_string()),
    };

    if let Some(message) = error {
        let bag = {
            let fiber = core.borrow().fiber(id);
            let mut f = fiber.borrow_mut();
            f.bag.take()
        };
        if let Some(bag) = bag {
            Bag::drain(core, &bag);
        }
        core.borrow_mut().log_error(&name, &message);
        let fiber = core.borrow().fiber(id);
        fiber.borrow_mut().state = FiberState::Failed;
    }
}
