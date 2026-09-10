//! # cordis
//!
//! Rust port of [Cordis](https://github.com/cordiverse/cordis), a
//! meta-framework of spatiotemporal composability.
//!
//! An application is a tree of [`Context`] scopes. Every plugin instance runs
//! inside a [`Fiber`], an effect scope with a lifecycle: when a fiber leaves
//! the active state, everything it registered (listeners, provided services,
//! nested plugins, plain cleanups) rolls back in reverse order. Fibers
//! declare service dependencies via `inject` and are activated, unloaded and
//! reloaded as dependencies appear and disappear.
//!
//! The port mirrors the Go implementation's architecture:
//!
//! * State transitions are coalesced through a drain queue and settle before
//!   the outermost framework call returns, so no torn intermediate states
//!   are observable.
//! * User callbacks never run while internal state is borrowed, so listeners
//!   and plugins may freely call back into the framework.
//!
//! The default build is single-threaded (`Rc`/`RefCell` based), matching
//! the execution model of the TypeScript original. The `thread-safe` feature
//! swaps the [`sync`] aliases for `Arc`/`Mutex` with identical semantics:
//! user callbacks still run with no borrows held, and the core is only ever
//! locked for the duration of one framework call.
//!
//! # Internal events
//!
//! Two event names extend upstream's `internal/update` contract, both fired
//! on the owning fiber's context like ordinary events. [`EVENT_STATUS`]
//! reports every fiber state change (payload: a [`StatusChange`]).
//! [`EVENT_PLUGIN`] brackets a plugin fiber's life: once when the fiber is
//! created and once when it is disposed (payload: the [`Fiber`] handle).
//! [`EVENT_UPDATE`] intercepts config updates as a waterfall whose listeners
//! may rewrite the config before calling `next`, or veto by ending the chain.
//!
//! The root fiber deliberately stands outside this machinery: it owns no
//! plugin runtime, so disposing it just rolls back and restarts the root
//! scope (firing no [`EVENT_PLUGIN`]), and updating its config returns
//! [`Error::RootUpdate`] instead of running the [`EVENT_UPDATE`] waterfall.

mod context;

pub mod sync;
mod core;
mod events;
mod fiber;
mod plugin;
mod service;
mod snapshot;

pub use context::{Context, Disposer, Filter, Guard};
pub use events::{event_name, service_name, value, EventOptions, Listener, Next, Value};
pub use fiber::{EffectMeta, Fiber, FiberState, StatusChange, EVENT_PLUGIN, EVENT_STATUS, EVENT_UPDATE};
pub use plugin::{plugin, plugin_type_id, start, start_fn, FnPlugin, Plugin, Registry, Runtime};
pub use snapshot::{FiberSnapshot, RegistrySnapshot, RuntimeSnapshot};

use std::error::Error as StdError;
use std::fmt;

/// The framework error type.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Error {
    /// An effect, listener, service or plugin was registered on a context
    /// whose fiber is no longer active.
    InactiveEffect,
    /// The value passed to [`crate::start`] is not a valid plugin.
    InvalidPlugin(String),
    /// A service was provided twice in the same realm.
    DuplicateService { name: String, provider: String },
    /// A plugin's config failed validation.
    Validation(String),
    /// A plugin body returned an error or panicked.
    PluginFailed { name: String, source: String },
    /// A required service is missing or inactive.
    MissingService(String),
    /// A service or config had an unexpected type.
    TypeMismatch { name: String },
    /// The root fiber owns no plugin runtime, so its config cannot be
    /// updated.
    RootUpdate,
    /// A builder method ran after the plugin value was already cloned or
    /// started, so the shared plugin base is no longer exclusively owned.
    PluginShared { name: String },
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::InactiveEffect => write!(f, "cannot create effect on inactive context"),
            Self::InvalidPlugin(what) => write!(
                f,
                "invalid plugin, expect function or object with an apply method, received {what}"
            ),
            Self::DuplicateService { name, provider } => {
                write!(f, "service {name:?} has been registered at <{provider}>")
            }
            Self::Validation(message) => write!(f, "invalid config: {message}"),
            Self::PluginFailed { name, source } => write!(f, "plugin <{name}> failed: {source}"),
            Self::MissingService(name) => {
                write!(f, "cannot get required service {name:?} in inactive context")
            }
            Self::TypeMismatch { name } => write!(f, "service {name:?} has an unexpected type"),
            Self::RootUpdate => write!(f, "cannot update the root fiber"),
            Self::PluginShared { name } => write!(
                f,
                "plugin {name:?} was already started or shared; call inject before cloning or starting it"
            ),
        }
    }
}

impl StdError for Error {}

/// The crate wide result type.
pub type Result<T> = std::result::Result<T, Error>;
