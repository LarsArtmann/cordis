//! Event dispatch: listeners, registration scopes and the five dispatch
//! modes (emit, parallel, serial, bail, waterfall).

use std::any::Any;
use crate::sync::RefCell;
use crate::sync::Rc;
#[cfg(feature = "thread-safe")]
use crate::sync::BorrowExt as _;

use crate::context::{Context, Disposer};
use crate::core::{self, Bag, Core};

/// A type erased event argument or service value.
#[cfg(not(feature = "thread-safe"))]
pub type Value = Rc<dyn Any>;
#[cfg(feature = "thread-safe")]
pub type Value = std::sync::Arc<dyn Any + Send + Sync>;

/// Build a Value from any payload.
pub fn value<T: crate::sync::Shared>(v: T) -> Value {
    Rc::new(v)
}

/// The canonical name of the typed service `T`.
///
/// It is the `type_name`. The typed
/// service API stores services under this name, so lookups resolve by type
/// identity instead of hand written strings. Pass it to `FnPlugin::inject`
/// and `Context::isolate` to depend on, or isolate, a typed service.
#[must_use]
pub fn service_name<T: ?Sized + Any>() -> &'static str {
    std::any::type_name::<T>()
}

/// The canonical name of the typed event `E`. Typed events dispatch under
/// this name; string event names remain for the framework's internal
/// namespace.
#[must_use]
pub fn event_name<E: ?Sized + Any>() -> &'static str {
    std::any::type_name::<E>()
}

/// A chain continuation for the waterfall dispatch mode.
#[cfg(not(feature = "thread-safe"))]
pub type Next = Rc<dyn Fn(&[Value]) -> Option<Value>>;
#[cfg(feature = "thread-safe")]
pub type Next = std::sync::Arc<dyn Fn(&[Value]) -> Option<Value> + Send + Sync>;

/// An event listener. The return value matters for the bail, serial and
/// waterfall modes and is ignored by emit and parallel.
#[cfg(not(feature = "thread-safe"))]
pub type Listener = Rc<dyn Fn(&[Value]) -> Option<Value>>;
#[cfg(feature = "thread-safe")]
pub type Listener = std::sync::Arc<dyn Fn(&[Value]) -> Option<Value> + Send + Sync>;

pub struct Hook {
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
    /// Subscribe to the string event `name`. Prefer the typed [`Context::on`]
    /// for application events; string names remain for the framework's
    /// internal namespace and dynamic event names. The subscription is bound
    /// to this context's fiber and rolls back with it; the returned Disposer
    /// removes it early.
    ///
    /// # Errors
    ///
    /// Returns [`crate::Error::InactiveEffect`] if this context's fiber is
    /// not active or has no effect bag to attach the subscription to.
    pub fn on_named(&self, name: &str, listener: Listener, options: EventOptions) -> crate::Result<Disposer> {
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
            drop(core);
        }

        let Some(bag) = self.bag() else {
            // Roll back the hook insertion.
            let mut core = self.core.borrow_mut();
            remove_hook(&mut core, name, &hook);
            return Err(crate::Error::InactiveEffect);
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

    /// Subscribe to the string event `name`, removing the listener after the
    /// first delivery.
    ///
    /// # Errors
    ///
    /// Returns [`crate::Error::InactiveEffect`] under the same conditions as
    /// [`Context::on_named`].
    pub fn once_named(&self, name: &str, listener: Listener, options: EventOptions) -> crate::Result<Disposer> {
        let holder: Rc<RefCell<Option<Disposer>>> = Rc::new(RefCell::new(None));
        let fired = Rc::new(std::sync::atomic::AtomicBool::new(false));
        let disposer = self.on_named(
            name,
            Rc::new({
                let holder = Rc::clone(&holder);
                let fired = Rc::clone(&fired);
                move |args| {
                    if fired.swap(true, std::sync::atomic::Ordering::SeqCst) {
                        return None;
                    }
                    // The guard must not span dispose(): a re-entrant lock
                    // of the same cell would deadlock the Mutex build.
                    let pending = holder.borrow_mut().take();
                    if let Some(d) = pending {
                        d.dispose();
                    }
                    listener(args)
                }
            }),
            options,
        )?;
        *holder.borrow_mut() = Some(disposer);
        Ok(Disposer::new(move || {
            let pending = holder.borrow_mut().take();
            if let Some(d) = pending {
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

    /// Deliver the string event `name` synchronously to every matching
    /// listener in registration order.
    pub fn emit_named(&self, name: &str, args: &[Value]) {
        for hook in self.resolve_hooks(name) {
            (hook.listener)(args);
        }
    }

    /// Deliver `name` to every matching listener and join all errors,
    /// mirroring ctx.parallel upstream. The current implementation runs
    /// listeners sequentially because the crate is single-threaded; error
    /// aggregation semantics are identical.
    ///
    /// # Errors
    ///
    /// Returns [`crate::Error::Validation`] joining every listener failure:
    /// a returned error payload or a listener panic.
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

    /// Subscribe to the event type `E`. Typed events are the primary event
    /// API: the event name derives from the type, so emitters and listeners
    /// cannot drift apart on a hand written string, and the payload arrives
    /// fully typed. The subscription is bound to this context's fiber and
    /// rolls back with it; the returned Disposer removes it early.
    ///
    /// # Panics
    /// When a listener for `E` receives an argument of another type; this
    /// indicates mixed typed and untyped use of the same event name.
    ///
    /// # Examples
    ///
    /// ```
    /// use cordis::{Context, EventOptions};
    ///
    /// struct Ping(u32);
    ///
    /// let ctx = Context::new();
    /// let _hook = ctx.on(
    ///     |ping: &Ping| assert_eq!(ping.0, 42),
    ///     EventOptions::default(),
    /// );
    /// ctx.emit(Ping(42));
    /// ```
    ///
    /// # Errors
    ///
    /// Returns [`crate::Error::InactiveEffect`] under the same conditions as
    /// [`Context::on_named`].
    pub fn on<E: crate::sync::Shared>(&self, listener: impl Fn(&E) + crate::sync::MaybeSendSync + 'static, options: EventOptions) -> crate::Result<Disposer> {
        self.on_named(event_name::<E>(), typed_listener(listener), options)
    }

    /// Subscribe to the event type `E`, removing the listener after the
    /// first delivery.
    ///
    /// # Errors
    ///
    /// Returns [`crate::Error::InactiveEffect`] under the same conditions as
    /// [`Context::on`].
    pub fn once<E: crate::sync::Shared>(&self, listener: impl Fn(&E) + crate::sync::MaybeSendSync + 'static, options: EventOptions) -> crate::Result<Disposer> {
        self.once_named(event_name::<E>(), typed_listener(listener), options)
    }

    /// Deliver `event` synchronously to every listener registered for its
    /// type `E`, in registration order, applying this context's emission
    /// filter.
    pub fn emit<E: crate::sync::Shared>(&self, event: E) {
        self.emit_named(event_name::<E>(), &[value(event)]);
    }

    /// Compose listeners around a terminal function, mirroring
    /// ctx.waterfall upstream. Each listener receives the arguments followed
    /// by a `next` continuation; not calling `next` short-circuits the chain.
    pub fn waterfall(&self, name: &str, args: Vec<Value>, terminal: &Next) -> Option<Value> {
        fn call(hooks: &[Rc<Hook>], args: Vec<Value>, terminal: &Next) -> Option<Value> {
            let Some((hook, tail)) = hooks.split_first() else {
                return terminal(&args);
            };
            let tail: Vec<Rc<Hook>> = tail.to_vec();
            let terminal = Rc::clone(terminal);
            let next: Next = Rc::new(move |next_args| call(&tail, next_args.to_vec(), &terminal));
            let mut rest = args;
            rest.push(Rc::new(next));
            (hook.listener)(&rest)
        }
        let hooks = self.resolve_hooks(name);
        call(&hooks, args, terminal)
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

/// Wrap a typed listener into the type erased Listener shape.
// A typed/untyped mix on one event name is a programmer error; the
// framework's own dispatch always passes exactly one payload of `E`.
#[allow(clippy::panic)]
fn typed_listener<E: crate::sync::Shared>(listener: impl Fn(&E) + crate::sync::MaybeSendSync + 'static) -> Listener {
    Rc::new(move |args: &[Value]| {
        let first = args
            .first()
            .unwrap_or_else(|| panic!("cordis: typed event {} expects one argument", event_name::<E>()));
        let typed = first.clone().downcast::<E>().unwrap_or_else(|_| {
            panic!(
                "cordis: typed event {} received an argument of another type",
                event_name::<E>()
            )
        });
        listener(&typed);
        None
    })
}
