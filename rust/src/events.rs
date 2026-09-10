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

/// The canonical name of the failed-lookup interception event, mirroring
/// `internal/get` upstream. It runs as a waterfall around every failed
///
/// [`Context::get_named`] with the arguments `[name, GetError]` followed by
/// the `next` continuation; the chain's result unwraps into a [`GetResult`],
/// so a listener may supply a fallback service by calling `next` with one
/// (or returning it directly).
pub const EVENT_GET: &str = "internal/get";

/// The canonical name of the service registration interception event,
/// mirroring `internal/set` upstream. It runs as a waterfall around every
///
/// [`Context::provide_named`] with the arguments `[name, value]` followed by
/// the `next` continuation. A listener may rewrite the value before calling
/// `next`, veto the registration by returning [`Rc<Error>`](crate::sync::Rc),
/// or replace the registration by returning an [`SetOutcome`] cell holding
/// its own disposer; the framework terminal stores its own outcome in a
/// fresh cell and returns it.
pub const EVENT_SET: &str = "internal/set";

/// The canonical name of the listener registration interception event,
/// mirroring `internal/listener` upstream. It runs as a bail around every
///
/// [`Context::on_named`] with the arguments `[name, ListenerRef, prepend]`;
/// the first non-`None` result must be an [`SetOutcome`] cell holding the
/// replacement registration's disposer (or an error), and the original
/// registration never happens.
pub const EVENT_LISTENER: &str = "internal/listener";

/// The canonical name of the dispatch observation event, mirroring
/// `internal/dispatch` upstream. It is emitted (as an ordinary event, with
///
/// the arguments `[mode, name, DispatchArgs]`) before every non-`internal/`
/// dispatch through emit, parallel, serial/bail and waterfall.
pub const EVENT_DISPATCH: &str = "internal/dispatch";

/// Describes a failed service lookup handed to [`EVENT_GET`] listeners.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct GetError {
    /// The unresolved service name.
    pub name: String,
    /// The framework's failure message.
    pub message: String,
}

/// The terminal value of the [`EVENT_GET`] waterfall: the fallback a
/// listener supplied, if any.
#[derive(Debug, Clone, Default)]
pub struct GetResult {
    /// The fallback value.
    pub value: Option<Value>,
    /// Whether the fallback resolves the lookup.
    pub ok: bool,
}

/// The disposer carrier of the [`EVENT_SET`] and [`EVENT_LISTENER`]
/// interception contracts: a shared cell whose entry the interception
///
/// listener fills with the outcome of the (possibly replaced) registration.
/// Taking the entry transfers the disposer's ownership back to the caller.
pub struct SetOutcome {
    /// The registration outcome: a disposer on success, an error on veto or
    /// failure, `None` while nothing has been decided yet.
    pub entry: Option<crate::Result<Disposer>>,
}

/// The listener handed to [`EVENT_LISTENER`] interception listeners,
/// carrying the listener about to be registered.
pub struct ListenerRef(pub Listener);

/// The argument slice of a dispatch, handed to [`EVENT_DISPATCH`] listeners.
pub struct DispatchArgs(pub Vec<Value>);

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

        // The EVENT_LISTENER interception can replace the registration
        // entirely: the first non-none bail result must be a SetOutcome cell.
        if let Some(result) = self.bail(
            EVENT_LISTENER,
            &[
                value(name.to_string()),
                value(ListenerRef(Rc::clone(&listener))),
                value(options.prepend),
            ],
        ) {
            return result.downcast::<RefCell<SetOutcome>>().map_or_else(
                |_| {
                    Err(crate::Error::Interception(
                        "internal/listener interception returned an unexpected value".to_string(),
                    ))
                },
                |cell| {
                    let entry = cell.borrow_mut().entry.take();
                    match entry {
                        Some(Ok(disposer)) => Ok(disposer),
                        Some(Err(err)) => Err(err),
                        None => Err(crate::Error::Interception(
                            "internal/listener interception returned no disposer".to_string(),
                        )),
                    }
                },
            );
        }

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
        self.notify_dispatch("emit", name, args);
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
        self.notify_dispatch("parallel", name, args);
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
        self.notify_dispatch("serial", name, args);
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

    /// Run the [`EVENT_GET`] waterfall around a failed lookup, mirroring
    /// `interceptGet` in the Go port: a no-op passthrough when no listener
    /// is registered.
    pub(crate) fn intercept_get(&self, name: &str) -> Option<Value> {
        let has_listeners = {
            let core = self.core.borrow();
            core.hooks.get(EVENT_GET).is_some_and(|hooks| !hooks.is_empty())
        };
        if !has_listeners {
            return None;
        }
        let terminal: Next = Rc::new(|args: &[Value]| {
            if let Some(first) = args.first()
                && first.clone().downcast::<GetResult>().is_ok()
            {
                return Some(first.clone());
            }
            Some(value(GetResult::default()))
        });
        let result = self.waterfall(
            EVENT_GET,
            vec![
                value(name.to_string()),
                value(GetError {
                    name: name.to_string(),
                    message: format!("cannot get property {name:?} without inject"),
                }),
            ],
            &terminal,
        );
        match result {
            Some(v) => {
                if let Ok(result) = v.clone().downcast::<GetResult>()
                    && result.ok
                {
                    return result.value.clone();
                }
                Some(v)
            }
            None => None,
        }
    }

    /// Run the [`EVENT_SET`] waterfall around a service registration,
    /// mirroring `Provide` in the Go port: a direct store when no listener
    /// is registered.
    pub(crate) fn intercept_set(&self, name: &str, v: Value) -> crate::Result<Disposer> {
        let outcome: Rc<RefCell<SetOutcome>> = Rc::new(RefCell::new(SetOutcome { entry: None }));
        let terminal_outcome = Rc::clone(&outcome);
        let ctx = self.clone();
        let key = name.to_string();
        let terminal: Next = Rc::new(move |args: &[Value]| {
            let value = args.get(1)?;
            let result = ctx.provide_inner(&key, Rc::clone(value));
            terminal_outcome.borrow_mut().entry = Some(result);
            let carrier: Rc<RefCell<SetOutcome>> = Rc::clone(&terminal_outcome);
            Some(carrier)
        });
        match self.waterfall(EVENT_SET, vec![value(name.to_string()), v], &terminal) {
            Some(v) => match v.downcast::<RefCell<SetOutcome>>() {
                Ok(cell) => {
                    let entry = cell.borrow_mut().entry.take();
                    match entry {
                        Some(Ok(disposer)) => Ok(disposer),
                        Some(Err(err)) => Err(err),
                        None => Err(crate::Error::Interception(
                            "internal/set interception returned no disposer".to_string(),
                        )),
                    }
                }
                Err(other) => {
                    if let Ok(err) = other.downcast::<crate::Error>() {
                        return Err((*err).clone());
                    }
                    Err(crate::Error::Interception(
                        "internal/set interception returned an unexpected value".to_string(),
                    ))
                }
            },
            None => Err(crate::Error::Interception(
                "internal/set interception returned no disposer".to_string(),
            )),
        }
    }

    /// Emit [`EVENT_DISPATCH`] for non-internal dispatches when an observer
    /// is registered, mirroring `notifyDispatch` in the Go port.
    fn notify_dispatch(&self, mode: &str, name: &str, args: &[Value]) {
        if name.starts_with("internal/") {
            return;
        }
        let has_observers = {
            let core = self.core.borrow();
            core.hooks
                .get(EVENT_DISPATCH)
                .is_some_and(|hooks| !hooks.is_empty())
        };
        if !has_observers {
            return;
        }
        self.emit_named(
            EVENT_DISPATCH,
            &[
                value(mode.to_string()),
                value(name.to_string()),
                value(DispatchArgs(args.to_vec())),
            ],
        );
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
        self.notify_dispatch("waterfall", name, &args);
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
