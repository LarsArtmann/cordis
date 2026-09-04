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
//! This crate is currently single-threaded (`Rc`/`RefCell` based), matching
//! the execution model of the TypeScript original. A thread-safe variant is
//! on the roadmap.

mod context;
mod core;
mod events;
mod fiber;
mod plugin;
mod service;

pub use context::{Context, Disposer, Filter};
pub use events::{value, EventOptions, Listener, Next, Value};
pub use fiber::{EffectMeta, Fiber, FiberState};
pub use plugin::{plugin, start, Plugin, Registry, Runtime};

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
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Error::InactiveEffect => write!(f, "cannot create effect on inactive context"),
            Error::InvalidPlugin(what) => write!(
                f,
                "invalid plugin, expect function or object with an apply method, received {what}"
            ),
            Error::DuplicateService { name, provider } => {
                write!(f, "service {name:?} has been registered at <{provider}>")
            }
            Error::Validation(message) => write!(f, "invalid config: {message}"),
            Error::PluginFailed { name, source } => write!(f, "plugin <{name}> failed: {source}"),
            Error::MissingService(name) => {
                write!(f, "cannot get required service {name:?} in inactive context")
            }
            Error::TypeMismatch { name } => write!(f, "service {name:?} has an unexpected type"),
        }
    }
}

impl StdError for Error {}

pub(crate) type Result<T> = std::result::Result<T, Error>;
