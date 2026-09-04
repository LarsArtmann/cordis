//! Service provision and lookup.
//!
//! The typed API ([`Context::provide`], [`Context::get`]) is the primary
//! form: services are keyed by their type identity, resolved through the
//! same realm machinery as named services. The named forms
//! ([`Context::provide_named`], [`Context::get_named`]) remain for dynamic
//! names (loader and hmr ports) and cross realm contracts.

use crate::sync::Rc;
#[cfg(feature = "thread-safe")]
use crate::sync::BorrowExt as _;

use crate::context::{Context, Disposer};
use crate::core::{self, Bag, Impl};
use crate::events::Value;
use crate::fiber::FiberState;

impl Context {
    /// Publish `v` under `name` in this context's service realm. The service
    /// is bound to the context's fiber: it is withdrawn automatically when
    /// the fiber unloads, and every fiber injecting `name` is re-evaluated
    /// on both publication and withdrawal.
    pub fn provide_named(&self, name: &str, v: Value) -> crate::Result<Disposer> {
        core::enter(&self.core);
        let result = self.provide_inner(name, v);
        core::leave(&self.core);
        result
    }

    /// Publish `value` as the service identified by its type `T` in this
    /// context's realm. The service is bound to the context's fiber exactly
    /// like a named service and rolls back with it. Providing the same type
    /// twice in one realm fails with [`crate::Error::DuplicateService`].
    pub fn provide<T: crate::sync::Shared>(&self, value: T) -> crate::Result<Disposer> {
        self.provide_named(crate::events::service_name::<T>(), Rc::new(value))
    }

    fn provide_inner(&self, name: &str, v: Value) -> crate::Result<Disposer> {
        self.fiber().assert_active()?;
        let key = self.isolate_key(name);
        let fiber_id = self.data.fiber;
        {
            let mut core = self.core.borrow_mut();
            if let Some(old) = core.store.get(&key) {
                let provider_fiber = core.fiber(old.fiber);
                let provider_name = {
                    let f = provider_fiber.borrow();
                    f.runtime
                        .and_then(|id| core_runtime_name(&core, id))
                        .unwrap_or_else(|| "root".to_string())
                };
                return Err(crate::Error::DuplicateService {
                    name: name.to_string(),
                    provider: provider_name,
                });
            }
            core.props.insert(name.to_string(), ());
            core.store.insert(
                key,
                Impl {
                    fiber: fiber_id,
                    value: Rc::clone(&v),
                },
            );
        }

        let bag = match self.bag() {
            Some(bag) => bag,
            None => {
                self.core.borrow_mut().store.remove(&key);
                return Err(crate::Error::InactiveEffect);
            }
        };
        let entry = Bag::push(
            &bag,
            format!("ctx.provide({name:?})"),
            Box::new({
                let core = Rc::clone(&self.core);
                let ctx = self.clone();
                let name = name.to_string();
                move || {
                    let mut c = core.borrow_mut();
                    c.store.remove(&key);
                    let names = [name.clone()];
                    c.notify_dependents(&ctx, &names);
                }
            }),
        );

        self.core.borrow_mut().notify_dependents(self, &[name.to_string()]);
        Ok(Disposer::new({
            let core = Rc::clone(&self.core);
            move || Bag::dispose_entry(&core, &bag, &entry)
        }))
    }

    /// The service published under `name` in this context's realm, when its
    /// provider is active.
    pub fn get_named(&self, name: &str) -> Option<Value> {
        let key = {
            let mut core = self.core.borrow_mut();
            match self.find_isolate_override(name) {
                Some(key) => key,
                None => core.root_key(name),
            }
        };
        let core = self.core.borrow();
        let imp = core.store.get(&key)?;
        let provider = core.fiber(imp.fiber);
        if provider.borrow().state != FiberState::Active {
            return None;
        }
        Some(Rc::clone(&imp.value))
    }

    /// The service of type `T` published in this context's realm. Fails when
    /// the service is missing, its provider is inactive or the value has an
    /// unexpected type.
    pub fn get<T: crate::sync::Shared>(&self) -> crate::Result<Rc<T>> {
        match self.get_named(crate::events::service_name::<T>()) {
            Some(v) => v.downcast::<T>().map_err(|_| crate::Error::TypeMismatch {
                name: crate::events::service_name::<T>().to_string(),
            }),
            None => Err(crate::Error::MissingService(
                crate::events::service_name::<T>().to_string(),
            )),
        }
    }

    /// The service of type `T` when it is currently available, mirroring the
    /// two value lookup of [`Context::get_named`].
    pub fn try_get<T: crate::sync::Shared>(&self) -> Option<Rc<T>> {
        self.get::<T>().ok()
    }

    /// Whether `name` is declared as a service in this context tree.
    pub fn has(&self, name: &str) -> bool {
        self.core.borrow().props.contains_key(name)
    }
}

fn core_runtime_name(core: &core::Core, id: u64) -> Option<String> {
    core.runtimes.get(&id).map(|r| r.name.clone())
}
