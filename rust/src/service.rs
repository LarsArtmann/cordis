//! Service provision and lookup.

use std::any::Any;
use std::rc::Rc;

use crate::context::{Context, Disposer};
use crate::core::{self, Bag, Impl};
use crate::events::Value;
use crate::fiber::FiberState;

impl Context {
    /// Publish `v` under `name` in this context's service realm. The service
    /// is bound to the context's fiber: it is withdrawn automatically when
    /// the fiber unloads, and every fiber injecting `name` is re-evaluated
    /// on both publication and withdrawal.
    pub fn provide(&self, name: &str, v: Value) -> crate::Result<Disposer> {
        core::enter(&self.core);
        let result = self.provide_inner(name, v);
        core::leave(&self.core);
        result
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
    pub fn get(&self, name: &str) -> Option<Value> {
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

    /// The statically typed variant of [`Context::get`].
    pub fn get_typed<T: Any>(&self, name: &str) -> crate::Result<Rc<T>> {
        match self.get(name) {
            Some(v) => v.downcast::<T>().map_err(|_| crate::Error::TypeMismatch {
                name: name.to_string(),
            }),
            None => Err(crate::Error::MissingService(name.to_string())),
        }
    }

    /// Whether `name` is declared as a service in this context tree.
    pub fn has(&self, name: &str) -> bool {
        self.core.borrow().props.contains_key(name)
    }
}

fn core_runtime_name(core: &core::Core, id: u64) -> Option<String> {
    core.runtimes.get(&id).map(|r| r.name.clone())
}
