//! Context tree: scopes carrying services, listeners and effects.

use std::cell::RefCell;
use std::rc::Rc;

use crate::core::{Bag, Core, IsolateKey};

/// Restricts which listeners receive events emitted through a context,
/// mirroring Context.filter upstream.
pub type Filter = Rc<dyn Fn(&Context) -> bool>;

/// A scope in the context tree. Clone freely; clones share the same scope.
///
/// See the crate documentation for the big picture.
#[derive(Clone)]
pub struct Context {
    pub(crate) core: Rc<RefCell<Core>>,
    pub(crate) data: Rc<ContextData>,
}

pub(crate) struct ContextData {
    pub parent: Option<Rc<ContextData>>,
    pub fiber: crate::fiber::FiberId,
    pub isolate: Option<Vec<(String, IsolateKey)>>,
    pub filter: Option<Filter>,
    /// The effect bag collecting registrations while an effect body runs.
    pub collect: Option<Rc<RefCell<Bag>>>,
}

impl Context {
    /// Create a root context with its own registry, event bus and service
    /// store. The root fiber is always active.
    pub fn new() -> Context {
        let core = Core::new();
        let ctx = Context {
            core,
            data: Rc::new(ContextData {
                parent: None,
                fiber: crate::fiber::FiberId(0),
                isolate: None,
                filter: None,
                collect: None,
            }),
        };
        let root = crate::fiber::FiberData::new_root(ctx.clone());
        ctx.core.borrow_mut().alloc_fiber(root);
        ctx
    }

    /// A plain child scope, mirroring ctx.extend() upstream.
    pub fn extend(&self) -> Context {
        Context {
            core: Rc::clone(&self.core),
            data: Rc::new(ContextData {
                parent: Some(Rc::clone(&self.data)),
                fiber: self.data.fiber,
                isolate: None,
                filter: None,
                collect: None,
            }),
        }
    }

    /// A child scope with its own service realm for `name`, mirroring
    /// ctx.isolate(name).
    pub fn isolate(&self, name: &str) -> Context {
        let key = self.core.borrow_mut().fresh_key();
        self.isolate_with(name, key)
    }

    /// A child scope sharing a named realm with every other scope created
    /// with the same label, mirroring ctx.isolate(name, label).
    pub fn isolate_shared(&self, name: &str, label: &str) -> Context {
        let synthetic = format!("{name}\0{label}");
        let key = self.core.borrow_mut().root_key(&synthetic);
        self.isolate_with(name, key)
    }

    fn isolate_with(&self, name: &str, key: IsolateKey) -> Context {
        Context {
            core: Rc::clone(&self.core),
            data: Rc::new(ContextData {
                parent: Some(Rc::clone(&self.data)),
                fiber: self.data.fiber,
                isolate: Some(vec![(name.to_string(), key)]),
                filter: None,
                collect: None,
            }),
        }
    }

    /// A child scope with an event emission filter.
    pub fn with_filter(&self, filter: Filter) -> Context {
        Context {
            core: Rc::clone(&self.core),
            data: Rc::new(ContextData {
                parent: Some(Rc::clone(&self.data)),
                fiber: self.data.fiber,
                isolate: None,
                filter: Some(filter),
                collect: None,
            }),
        }
    }

    /// A filter matching listeners whose realm key for `name` equals this
    /// context's key, the building block for realm scoped events.
    pub fn realm_filter(&self, name: &str) -> Filter {
        let key = self.isolate_key(name);
        let name = name.to_string();
        Rc::new(move |listener: &Context| listener.isolate_key(&name) == key)
    }

    /// Resolve the realm key of `name` through the scope chain, falling back
    /// to the root realm.
    pub fn isolate_key(&self, name: &str) -> IsolateKey {
        self.find_isolate_override(name)
            .unwrap_or_else(|| self.core.borrow_mut().root_key(name))
    }

    /// Walk the scope chain for a realm override without touching the core.
    pub(crate) fn find_isolate_override(&self, name: &str) -> Option<IsolateKey> {
        let mut data = Some(Rc::clone(&self.data));
        while let Some(d) = data {
            if let Some(isolate) = &d.isolate {
                for (n, key) in isolate {
                    if n == name {
                        return Some(*key);
                    }
                }
            }
            data = d.parent.clone();
        }
        None
    }

    /// The fiber owning this context.
    pub fn fiber(&self) -> crate::fiber::Fiber {
        crate::fiber::Fiber {
            core: Rc::clone(&self.core),
            id: self.data.fiber,
        }
    }

    /// Errors reported by failing cleanups and plugin bodies.
    pub fn logged_errors(&self) -> Vec<String> {
        self.core.borrow().errors.clone()
    }

    /// Run `f` as one framework transaction: fiber transitions triggered
    /// inside are coalesced and settle after `f` returns.
    pub fn batch<R>(&self, f: impl FnOnce(&Context) -> R) -> R {
        crate::core::enter(&self.core);
        let result = f(self);
        crate::core::leave(&self.core);
        result
    }

    pub(crate) fn with_collect(&self, bag: Rc<RefCell<Bag>>) -> Context {
        Context {
            core: Rc::clone(&self.core),
            data: Rc::new(ContextData {
                parent: Some(Rc::clone(&self.data)),
                fiber: self.data.fiber,
                isolate: None,
                filter: None,
                collect: Some(bag),
            }),
        }
    }
}

impl Default for Context {
    fn default() -> Self {
        Context::new()
    }
}

/// Removes a registration ahead of time. Disposing twice is a no-op.
pub struct Disposer {
    inner: Option<Box<dyn FnOnce()>>,
}

impl Disposer {
    pub(crate) fn new(f: impl FnOnce() + 'static) -> Disposer {
        Disposer {
            inner: Some(Box::new(f)),
        }
    }

    /// Dispose the registration. Consumes the disposer; cloning is not
    /// possible, so double disposal is unrepresentable.
    pub fn dispose(mut self) {
        if let Some(f) = self.inner.take() {
            f();
        }
    }
}

impl std::fmt::Debug for Disposer {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Disposer").finish_non_exhaustive()
    }
}

impl Drop for Disposer {
    fn drop(&mut self) {
        // Leaking on drop is intentional: dropping a Disposer without
        // calling dispose keeps the registration alive for the fiber's
        // lifetime, matching upstream semantics.
    }
}
