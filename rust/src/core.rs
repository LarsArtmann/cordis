//! Shared mutable state of one context tree, the effect bag tree, and the
//! drain queue that coalesces fiber state transitions.

use std::cell::RefCell;
use std::collections::{HashMap, VecDeque};
use std::rc::Rc;

use crate::context::Context;
use crate::events::{Hook, Value};
use crate::fiber::{FiberData, FiberId};

/// Realm keys are the Rust counterpart of the per-realm symbols upstream.
pub(crate) type IsolateKey = u64;

/// A service instance living in one realm.
pub(crate) struct Impl {
    pub fiber: FiberId,
    pub value: Value,
}

/// The type erased plugin body.
pub(crate) type ApplyFn = Rc<dyn Fn(&Context, Value) -> crate::Result<()>>;

/// The shared runtime identity and body of a plugin, used by both the
/// closure form (FnPlugin) and the trait form (Plugin).
pub(crate) struct PluginBase {
    pub id: u64,
    pub name: String,
    pub inject: Vec<String>,
    pub apply: ApplyFn,
}

/// A back-reference from a fiber to its registration entry in the parent
/// fiber's effect bag.
pub(crate) type BagEntry = (Rc<RefCell<Bag>>, Rc<RefCell<Entry>>);

/// One plugin runtime: every live fiber of a plugin.
pub(crate) struct RuntimeData {
    pub name: String,
    pub apply: ApplyFn,
    pub fibers: Vec<FiberId>,
}

/// A cleanup releasing one resource.
pub(crate) type Cleanup = Box<dyn FnMut()>;

/// An ordered collection of disposables owned by a fiber or by one effect
/// inside a fiber. Disposal is always last in, first out.
pub(crate) struct Bag {
    pub items: Vec<Rc<RefCell<Entry>>>,
}

pub(crate) enum EntryKind {
    Leaf(Option<Cleanup>),
    Node(Rc<RefCell<Bag>>),
}

pub(crate) struct Entry {
    pub label: String,
    pub kind: EntryKind,
    pub done: bool,
}

impl Bag {
    pub fn new() -> Rc<RefCell<Bag>> {
        Rc::new(RefCell::new(Bag { items: Vec::new() }))
    }

    pub fn push(bag: &Rc<RefCell<Bag>>, label: String, cleanup: Cleanup) -> Rc<RefCell<Entry>> {
        let entry = Rc::new(RefCell::new(Entry {
            label,
            kind: EntryKind::Leaf(Some(cleanup)),
            done: false,
        }));
        bag.borrow_mut().items.push(Rc::clone(&entry));
        entry
    }

    pub fn push_node(bag: &Rc<RefCell<Bag>>, label: String, node: Rc<RefCell<Bag>>) -> Rc<RefCell<Entry>> {
        let entry = Rc::new(RefCell::new(Entry {
            label,
            kind: EntryKind::Node(node),
            done: false,
        }));
        bag.borrow_mut().items.push(Rc::clone(&entry));
        entry
    }

    /// Detach without executing, used when a fiber disposes itself.
    pub fn detach(bag: &Rc<RefCell<Bag>>, entry: &Rc<RefCell<Entry>>) {
        entry.borrow_mut().done = true;
        bag.borrow_mut().items.retain(|item| !Rc::ptr_eq(item, entry));
    }

    /// Execute an entry (children first, then its own cleanup) exactly once.
    pub fn execute(core: &Rc<RefCell<Core>>, entry: &Rc<RefCell<Entry>>) {
        {
            let mut e = entry.borrow_mut();
            if e.done {
                return;
            }
            e.done = true;
        }
        let kind = {
            let mut e = entry.borrow_mut();
            std::mem::replace(&mut e.kind, EntryKind::Leaf(None))
        };
        match kind {
            EntryKind::Leaf(Some(cleanup)) => run_cleanup(core, cleanup),
            EntryKind::Leaf(None) => {}
            EntryKind::Node(bag) => Bag::drain(core, &bag),
        }
    }

    /// Detach and execute an entry.
    pub fn dispose_entry(core: &Rc<RefCell<Core>>, bag: &Rc<RefCell<Bag>>, entry: &Rc<RefCell<Entry>>) {
        {
            let e = entry.borrow_mut();
            if e.done {
                return;
            }
        }
        bag.borrow_mut().items.retain(|item| !Rc::ptr_eq(item, entry));
        Bag::execute(core, entry);
    }

    /// Drain the bag: detach all items and execute them in reverse order.
    pub fn drain(core: &Rc<RefCell<Core>>, bag: &Rc<RefCell<Bag>>) {
        let mut items: Vec<Rc<RefCell<Entry>>> = {
            let mut b = bag.borrow_mut();
            std::mem::take(&mut b.items)
        };
        for entry in items.drain(..).rev() {
            Bag::execute(core, &entry);
        }
    }

    /// The introspection view, mirroring Fiber.getEffects() upstream.
    pub fn meta(bag: &Rc<RefCell<Bag>>) -> Vec<crate::fiber::EffectMeta> {
        let b = bag.borrow();
        b.items
            .iter()
            .map(|entry| {
                let e = entry.borrow();
                let children = match &e.kind {
                    EntryKind::Node(node) => Bag::meta(node),
                    EntryKind::Leaf(_) => Vec::new(),
                };
                crate::fiber::EffectMeta {
                    label: e.label.clone(),
                    children,
                }
            })
            .collect()
    }
}

pub(crate) struct Core {
    pub hooks: HashMap<String, Vec<Rc<Hook>>>,
    pub store: HashMap<IsolateKey, Impl>,
    pub props: HashMap<String, ()>,
    pub keys: HashMap<String, IsolateKey>,
    pub last_key: IsolateKey,
    pub fibers: Vec<Option<Rc<RefCell<FiberData>>>>,
    pub runtimes: HashMap<u64, RuntimeData>,
    pub counter: usize,

    /// Re-entrant API depth: transitions drain when the outermost public
    /// call returns.
    pub depth: usize,
    pub draining: bool,
    pub dirty: VecDeque<FiberId>,

    /// Errors reported by failing cleanups and plugin bodies, mirroring the
    /// logger error channel upstream.
    pub errors: Vec<String>,
}

impl Core {
    pub fn new() -> Rc<RefCell<Core>> {
        Rc::new(RefCell::new(Core {
            hooks: HashMap::new(),
            store: HashMap::new(),
            props: HashMap::new(),
            keys: HashMap::new(),
            last_key: 0,
            fibers: Vec::new(),
            runtimes: HashMap::new(),
            counter: 0,
            depth: 0,
            draining: false,
            dirty: VecDeque::new(),
            errors: Vec::new(),
        }))
    }

    pub fn next_uid(&mut self) -> i64 {
        self.counter += 1;
        self.counter as i64
    }

    pub fn root_key(&mut self, name: &str) -> IsolateKey {
        if let Some(key) = self.keys.get(name) {
            return *key;
        }
        self.last_key += 1;
        self.keys.insert(name.to_string(), self.last_key);
        self.last_key
    }

    /// Allocate a fresh realm key that can never collide with a named realm.
    pub fn fresh_key(&mut self) -> IsolateKey {
        self.last_key += 1;
        self.last_key
    }

    pub fn alloc_fiber(&mut self, data: FiberData) -> FiberId {
        let id = FiberId(self.fibers.len());
        self.fibers.push(Some(Rc::new(RefCell::new(data))));
        self.fibers[id.0].as_ref().unwrap().borrow_mut().id = id;
        id
    }

    pub fn fiber(&self, id: FiberId) -> Rc<RefCell<FiberData>> {
        Rc::clone(self.fibers[id.0].as_ref().expect("fiber arena entry"))
    }

    /// Queue a fiber for state transition evaluation.
    pub fn queue(&mut self, id: FiberId) {
        let fiber = self.fiber(id);
        let mut f = fiber.borrow_mut();
        if !f.queued {
            f.queued = true;
            self.dirty.push_back(id);
        }
    }

    /// Queue every fiber injecting one of `names` in the realm of `from`,
    /// mirroring ReflectService.notify upstream.
    pub fn notify_dependents(&mut self, from: &Context, names: &[String]) {
        let mut checks = Vec::new();
        for (index, slot) in self.fibers.iter().enumerate() {
            let Some(fiber) = slot else { continue };
            let f = fiber.borrow();
            if f.runtime.is_none() {
                continue;
            }
            for name in names {
                if !f.inject.contains(name) {
                    continue;
                }
                checks.push((
                    index,
                    f.ctx.find_isolate_override(name),
                    from.find_isolate_override(name),
                    name.clone(),
                ));
                break;
            }
        }
        let mut targets = Vec::new();
        for (index, fiber_override, from_override, name) in checks {
            let fiber_key = fiber_override.unwrap_or_else(|| self.root_key(&name));
            let from_key = from_override.unwrap_or_else(|| self.root_key(&name));
            if fiber_key == from_key {
                targets.push(FiberId(index));
            }
        }
        for id in targets {
            self.queue(id);
        }
    }

    pub fn log_error(&mut self, name: &str, message: &str) {
        self.errors.push(format!("<{name}> {message}"));
    }
}

/// Enter a public API boundary.
pub(crate) fn enter(core: &Rc<RefCell<Core>>) {
    core.borrow_mut().depth += 1;
}

/// Leave a public API boundary, draining pending fiber transitions when the
/// outermost call returns.
pub(crate) fn leave(core: &Rc<RefCell<Core>>) {
    let should_drain = {
        let mut c = core.borrow_mut();
        c.depth -= 1;
        if c.depth == 0 && !c.draining {
            c.draining = true;
            true
        } else {
            false
        }
    };
    if !should_drain {
        return;
    }
    loop {
        let next = core.borrow_mut().dirty.pop_front();
        match next {
            Some(id) => {
                let fiber = core.borrow().fiber(id);
                fiber.borrow_mut().queued = false;
                crate::fiber::transition(core, id);
            }
            None => break,
        }
    }
    core.borrow_mut().draining = false;
}

/// Run a user cleanup, catching panics into the error log and draining any
/// transitions it triggered.
pub(crate) fn run_cleanup(core: &Rc<RefCell<Core>>, cleanup: Box<dyn FnMut()>) {
    enter(core);
    if std::panic::catch_unwind(std::panic::AssertUnwindSafe(cleanup)).is_err() {
        core.borrow_mut().log_error("root", "cleanup panicked");
    }
    leave(core);
}
