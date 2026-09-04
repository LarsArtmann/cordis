//! Event dispatch: listeners, registration scopes and the five dispatch
//! modes (emit, parallel, serial, bail, waterfall).

use std::any::Any;
use std::cell::RefCell;
use std::rc::Rc;

use crate::context::{Context, Disposer};
use crate::core::{self, Bag, Core};

/// A type erased event argument or service value.
pub type Value = Rc<dyn Any>;

/// Build a Value from any payload.
pub fn value<T: Any>(v: T) -> Value {
    Rc::new(v)
}

/// A chain continuation for the waterfall dispatch mode.
pub type Next = Rc<dyn Fn(&[Value]) -> Option<Value>>;

/// An event listener. The return value matters for the bail, serial and
/// waterfall modes and is ignored by emit and parallel.
pub type Listener = Rc<dyn Fn(&[Value]) -> Option<Value>>;

pub(crate) struct Hook {
    pub owner: Context,
    pub listener: Listener,
    pub global: bool,
}

/// Listener registration options.
#[derive(Default, Clone, Copy)]
pub struct EventOptions {
    /// Register before existing listeners for the same event.
    pub prepend: bool,
    /// Exempt from emission filters.
    pub global: bool,
}

impl Context {
    /// Subscribe to `name`. The subscription is bound to this context's
    /// fiber and rolls back with it; the returned Disposer removes it early.
    pub fn on(&self, name: &str, listener: Listener, options: EventOptions) -> crate::Result<Disposer> {
        core::enter(&self.core);
        let result = self.on_inner(name, listener, options);
        core::leave(&self.core);
        result
    }

    fn on_inner(&self, name: &str, listener: Listener, options: EventOptions) -> crate::Result<Disposer> {
        self.fiber().assert_active()?;
        let hook = Rc::new(Hook {
            owner: self.clone(),
            listener,
            global: options.global,
        });
        {
            let mut core = self.core.borrow_mut();
            let hooks = core.hooks.entry(name.to_string()).or_default();
            if options.prepend {
                hooks.insert(0, Rc::clone(&hook));
            } else {
                hooks.push(Rc::clone(&hook));
            }
        }

        let bag = match self.bag() {
            Some(bag) => bag,
            None => {
                // Roll back the hook insertion.
                let mut core = self.core.borrow_mut();
                remove_hook(&mut core, name, &hook);
                return Err(crate::Error::InactiveEffect);
            }
        };
        let entry = Bag::push(
            &bag,
            format!("ctx.on({name:?})"),
            Box::new({
                let core = Rc::clone(&self.core);
                let name = name.to_string();
                let hook = Rc::clone(&hook);
                move || {
                    let mut core = core.borrow_mut();
                    remove_hook(&mut core, &name, &hook);
                }
            }),
        );
        Ok(Disposer::new({
            let core = Rc::clone(&self.core);
            move || Bag::dispose_entry(&core, &bag, &entry)
        }))
    }

    /// Subscribe to `name`, removing the listener after the first delivery.
    pub fn once(&self, name: &str, listener: Listener, options: EventOptions) -> crate::Result<Disposer> {
        let holder: Rc<std::cell::RefCell<Option<Disposer>>> = Rc::new(std::cell::RefCell::new(None));
        let fired = Rc::new(std::cell::Cell::new(false));
        let disposer = self.on(
            name,
            Rc::new({
                let holder = Rc::clone(&holder);
                let fired = Rc::clone(&fired);
                move |args| {
                    if fired.replace(true) {
                        return None;
                    }
                    if let Some(d) = holder.borrow_mut().take() {
                        d.dispose();
                    }
                    listener(args)
                }
            }),
            options,
        )?;
        *holder.borrow_mut() = Some(disposer);
        Ok(Disposer::new(move || {
            if let Some(d) = holder.borrow_mut().take() {
                d.dispose();
            }
        }))
    }

    fn resolve_hooks(&self, name: &str) -> Vec<Rc<Hook>> {
        let (hooks, filter) = {
            let core = self.core.borrow();
            (
                core.hooks.get(name).cloned().unwrap_or_default(),
                self.data.filter.clone(),
            )
        };
        hooks
            .into_iter()
            .filter(|hook| hook.global || filter.as_ref().is_none_or(|f| f(&hook.owner)))
            .collect()
    }

    /// Deliver `name` synchronously to every matching listener in
    /// registration order.
    pub fn emit(&self, name: &str, args: &[Value]) {
        for hook in self.resolve_hooks(name) {
            (hook.listener)(args);
        }
    }

    /// Deliver `name` to every matching listener and join all errors,
    /// mirroring ctx.parallel upstream. The current implementation runs
    /// listeners sequentially because the crate is single-threaded; error
    /// aggregation semantics are identical.
    pub fn parallel(&self, name: &str, args: &[Value]) -> crate::Result<()> {
        let mut errors = Vec::new();
        for hook in self.resolve_hooks(name) {
            let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| (hook.listener)(args)));
            match result {
                Ok(Some(v)) => {
                    if let Some(err) = v.downcast::<String>().ok().map(|e| e.to_string()) {
                        errors.push(err);
                    }
                }
                Ok(None) => {}
                Err(_) => errors.push("listener panicked".to_string()),
            }
        }
        if errors.is_empty() {
            Ok(())
        } else {
            Err(crate::Error::Validation(errors.join("\n")))
        }
    }

    /// Return the first non-none listener result, mirroring ctx.serial and
    /// ctx.bail upstream (identical in a synchronous runtime).
    pub fn bail(&self, name: &str, args: &[Value]) -> Option<Value> {
        for hook in self.resolve_hooks(name) {
            if let Some(result) = (hook.listener)(args) {
                return Some(result);
            }
        }
        None
    }

    /// Alias of [`Context::bail`].
    pub fn serial(&self, name: &str, args: &[Value]) -> Option<Value> {
        self.bail(name, args)
    }

    /// Compose listeners around a terminal function, mirroring
    /// ctx.waterfall upstream. Each listener receives the arguments followed
    /// by a `next` continuation; not calling `next` short-circuits the chain.
    pub fn waterfall(&self, name: &str, args: Vec<Value>, terminal: Next) -> Option<Value> {
        fn call(hooks: &[Rc<Hook>], args: Vec<Value>, terminal: &Next) -> Option<Value> {
            let Some(hook) = hooks.first() else {
                return terminal(&args);
            };
            let tail: Vec<Rc<Hook>> = hooks[1..].to_vec();
            let terminal = Rc::clone(terminal);
            let next: Next = Rc::new(move |next_args| call(&tail, next_args.to_vec(), &terminal));
            let mut rest = args;
            rest.push(Rc::new(next));
            (hook.listener)(&rest)
        }
        let hooks = self.resolve_hooks(name);
        call(&hooks, args, &terminal)
    }

    /// The current cleanup collection target: the enclosing effect bag while
    /// an effect body runs, otherwise the fiber's own bag.
    pub(crate) fn bag(&self) -> Option<Rc<RefCell<Bag>>> {
        if let Some(bag) = &self.data.collect {
            return Some(Rc::clone(bag));
        }
        let fiber = self.core.borrow().fiber(self.data.fiber);
        let f = fiber.borrow();
        if f.disposed {
            return None;
        }
        match f.state {
            crate::fiber::FiberState::Active | crate::fiber::FiberState::Loading => f.bag.clone(),
            _ => None,
        }
    }
}

fn remove_hook(core: &mut Core, name: &str, hook: &Rc<Hook>) {
    if let Some(hooks) = core.hooks.get_mut(name) {
        hooks.retain(|candidate| !Rc::ptr_eq(candidate, hook));
    }
}
