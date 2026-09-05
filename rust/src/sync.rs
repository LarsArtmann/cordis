//! The sharing strategy of one crate build. By default the tree is
//! single-threaded (`Rc`/`RefCell`, zero synchronization cost). With the
//! `thread-safe` feature the same tree becomes shareable across threads:
//! shared cells are `Arc<Mutex<T>>` and callbacks are `Send + Sync`.
//!
//! The framework's architecture — a drain queue that coalesces fiber
//! transitions and snapshots state before running user code — guarantees
//! that the core mutex is never held while a listener, plugin body or
//! cleanup runs, so no lock is ever held across a re-entrant call.

/// The shared strong reference. `Rc` in the single-threaded build, `Arc`
/// when the `thread-safe` feature is on.
#[cfg(not(feature = "thread-safe"))]
pub type Rc<T> = std::rc::Rc<T>;
#[cfg(feature = "thread-safe")]
pub type Rc<T> = std::sync::Arc<T>;

/// The shared interior-mutable cell. `RefCell` in the single-threaded
/// build, `Mutex` when the `thread-safe` feature is on.
#[cfg(not(feature = "thread-safe"))]
pub type RefCell<T> = std::cell::RefCell<T>;
#[cfg(feature = "thread-safe")]
pub type RefCell<T> = std::sync::Mutex<T>;

/// Uniform access to the shared cell: `borrow` for reading, `borrow_mut`
/// for writing. On `RefCell` these are the native methods; on `Mutex` they
/// lock and panic on poisoning, which only happens if a panic escapes the
/// framework's guarded plugin bodies.
pub trait BorrowExt<T> {
    type Guard<'a>
    where
        Self: 'a,
        T: 'a;
    type GuardMut<'a>
    where
        Self: 'a,
        T: 'a;

    fn borrow(&self) -> Self::Guard<'_>;
    fn borrow_mut(&self) -> Self::GuardMut<'_>;
}

#[cfg(not(feature = "thread-safe"))]
impl<T> BorrowExt<T> for RefCell<T> {
    type Guard<'a> = std::cell::Ref<'a, T> where T: 'a;
    type GuardMut<'a> = std::cell::RefMut<'a, T> where T: 'a;

    fn borrow(&self) -> Self::Guard<'_> {
        Self::borrow(self)
    }

    fn borrow_mut(&self) -> Self::GuardMut<'_> {
        Self::borrow_mut(self)
    }
}

#[cfg(feature = "thread-safe")]
impl<T> BorrowExt<T> for RefCell<T> {
    type Guard<'a> = std::sync::MutexGuard<'a, T> where T: 'a;
    type GuardMut<'a> = std::sync::MutexGuard<'a, T> where T: 'a;

    fn borrow(&self) -> Self::Guard<'_> {
        match self.lock() {
            Ok(guard) => guard,
            Err(poisoned) => poisoned.into_inner(),
        }
    }

    fn borrow_mut(&self) -> Self::GuardMut<'_> {
        match self.lock() {
            Ok(guard) => guard,
            Err(poisoned) => poisoned.into_inner(),
        }
    }
}

/// The auto-trait requirement for values that cross the tree: nothing in
/// the single-threaded build, `Send + Sync` under the `thread-safe`
/// feature. Plugin configs, service values and event payloads must
/// implement it.
#[cfg(not(feature = "thread-safe"))]
pub trait Shared: Any {}
#[cfg(not(feature = "thread-safe"))]
impl<T: Any> Shared for T {}

#[cfg(feature = "thread-safe")]
pub trait Shared: Any + Send + Sync {}
#[cfg(feature = "thread-safe")]
impl<T: Any + Send + Sync> Shared for T {}

use std::any::Any;

/// The erased cleanup closure of a [`Disposer`](crate::context::Disposer).
#[cfg(not(feature = "thread-safe"))]
pub type CleanupFn = dyn FnOnce();
#[cfg(feature = "thread-safe")]
pub type CleanupFn = dyn FnOnce() + Send + Sync;

/// The auto-trait requirement for closures stored in the tree.
#[cfg(not(feature = "thread-safe"))]
pub trait MaybeSendSync {}
#[cfg(not(feature = "thread-safe"))]
impl<T> MaybeSendSync for T {}

#[cfg(feature = "thread-safe")]
pub trait MaybeSendSync: Send + Sync {}
#[cfg(feature = "thread-safe")]
impl<T: Send + Sync> MaybeSendSync for T {}

/// The `Send` requirement for closures stored in the tree.
#[cfg(not(feature = "thread-safe"))]
pub trait MaybeSend {}
#[cfg(not(feature = "thread-safe"))]
impl<T> MaybeSend for T {}

#[cfg(feature = "thread-safe")]
pub trait MaybeSend: Send {}
#[cfg(feature = "thread-safe")]
impl<T: Send> MaybeSend for T {}
