//! Typed plugins, the registry, and Context::inject.

use std::marker::PhantomData;
use std::rc::Rc;
use std::sync::atomic::{AtomicU64, Ordering};

use crate::context::Context;
use crate::core::{self, Bag, RuntimeData};
use crate::fiber::{self, Fiber};

/// Plugin ids are process-global so plugins constructed anywhere can serve
/// as registry identities.
static NEXT_PLUGIN_ID: AtomicU64 = AtomicU64::new(1);

/// A typed unit of composable behavior: a name, injected dependencies and an
/// apply function. A Plugin value is also its registry identity, mirroring
/// the callback keyed runtime map upstream.
pub struct Plugin<C: 'static> {
    pub(crate) base: Rc<PluginBase>,
    _marker: PhantomData<fn() -> C>,
}

pub(crate) struct PluginBase {
    pub id: u64,
    pub name: String,
    pub inject: Vec<String>,
    pub apply: crate::core::ApplyFn,
}

impl<C: 'static> Plugin<C> {
    /// The plugin name.
    pub fn name(&self) -> &str {
        &self.base.name
    }
}

impl<C: 'static> Clone for Plugin<C> {
    fn clone(&self) -> Self {
        Plugin {
            base: Rc::clone(&self.base),
            _marker: PhantomData,
        }
    }
}

/// Create a plugin. The apply function receives the fiber's context and a
/// reference to the statically typed config; its return value decides the
/// fiber state.
pub fn plugin<C, F>(name: &str, apply: F) -> Plugin<C>
where
    C: 'static,
    F: Fn(&Context, &C) -> crate::Result<()> + 'static,
{
    Plugin {
        base: Rc::new(PluginBase {
            id: NEXT_PLUGIN_ID.fetch_add(1, Ordering::Relaxed),
            name: name.to_string(),
            inject: Vec::new(),
            apply: Rc::new({
                let name = name.to_string();
                move |ctx, config| {
                    let typed = config
                        .downcast::<C>()
                        .map_err(|_| crate::Error::TypeMismatch {
                            name: name.clone(),
                        })?;
                    apply(ctx, &typed)
                }
            }),
        }),
        _marker: PhantomData,
    }
}

impl<C: 'static> Plugin<C> {
    /// Declare service dependencies. The fiber stays pending until all of
    /// them are available and active, unloads when one disappears and
    /// reloads when it returns.
    ///
    /// # Panics
    /// When called after the plugin was started or cloned.
    pub fn inject(mut self, deps: &[&str]) -> Plugin<C> {
        let base = Rc::get_mut(&mut self.base)
            .expect("plugin.inject must be called before the plugin is started or shared");
        base.inject.extend(deps.iter().map(|s| s.to_string()));
        self
    }
}

/// Apply the plugin on `ctx` with the given config and return the new fiber.
/// The fiber activates before start returns unless its injected services are
/// missing, in which case it stays pending and activates when they appear.
pub fn start<C: 'static>(ctx: &Context, plugin: &Plugin<C>, config: C) -> crate::Result<Fiber> {
    core::enter(&ctx.core);
    let result = start_inner(ctx, plugin, config);
    core::leave(&ctx.core);
    result
}

fn start_inner<C: 'static>(ctx: &Context, plugin: &Plugin<C>, config: C) -> crate::Result<Fiber> {
    ctx.fiber().assert_active()?;
    let parent_bag = match ctx.bag() {
        Some(bag) => bag,
        None => return Err(crate::Error::InactiveEffect),
    };

    let base = Rc::clone(&plugin.base);
    let runtime_id = base.id;
    {
        let mut core = ctx.core.borrow_mut();
        core.runtimes.entry(runtime_id).or_insert_with(|| RuntimeData {
            name: base.name.clone(),
            apply: Rc::clone(&base.apply),
            fibers: Vec::new(),
        });
    }

    let data = fiber::new_fiber(ctx, crate::events::value(config), &base.inject, runtime_id);
    let id = ctx.core.borrow_mut().alloc_fiber(data);
    fiber::link_fiber_ctx(&ctx.core, id);
    let fiber = Fiber {
        core: Rc::clone(&ctx.core),
        id,
    };

    // Register the fiber's disposal on the parent fiber's effect bag.
    let entry = Bag::push(
        &parent_bag,
        "ctx.plugin()".to_string(),
        Box::new({
            let fiber = fiber.clone();
            move || fiber.dispose()
        }),
    );
    ctx.core.borrow().fiber(id).borrow_mut().entry = Some((Rc::clone(&parent_bag), entry));

    {
        let mut core = ctx.core.borrow_mut();
        core.runtimes
            .get_mut(&runtime_id)
            .expect("runtime")
            .fibers
            .push(id);
        core.queue(id);
    }
    Ok(fiber)
}

impl Context {
    /// Start an anonymous plugin that runs `f` once every service in `deps`
    /// is available, mirroring ctx.inject upstream.
    pub fn inject(&self, deps: &[&str], f: impl Fn(&Context) -> crate::Result<()> + 'static) -> crate::Result<Fiber> {
        let p = plugin("anonymous", move |ctx: &Context, _: &()| f(ctx)).inject(deps);
        start(self, &p, ())
    }

    /// The registry of this context tree.
    pub fn registry(&self) -> Registry {
        Registry {
            core: Rc::clone(&self.core),
        }
    }
}

/// The view of every plugin runtime in a context tree.
pub struct Registry {
    pub(crate) core: Rc<std::cell::RefCell<crate::core::Core>>,
}

/// A handle to one plugin runtime.
pub struct Runtime {
    pub(crate) name: String,
    pub(crate) fibers: Vec<crate::fiber::FiberId>,
}

impl Runtime {
    /// The runtime name.
    pub fn name(&self) -> &str {
        &self.name
    }

    /// The number of live fibers.
    pub fn len(&self) -> usize {
        self.fibers.len()
    }

    /// Whether the runtime has no live fibers.
    pub fn is_empty(&self) -> bool {
        self.fibers.is_empty()
    }
}

impl Registry {
    /// The number of plugin runtimes with at least one live fiber.
    pub fn size(&self) -> usize {
        self.core.borrow().runtimes.len()
    }

    /// Whether the plugin has at least one live fiber.
    pub fn has<C: 'static>(&self, plugin: &Plugin<C>) -> bool {
        self.core.borrow().runtimes.contains_key(&plugin.base.id)
    }

    /// Dispose every fiber of the plugin and remove its runtime, restoring
    /// the state from before its first start.
    pub fn delete<C: 'static>(&self, plugin: &Plugin<C>) {
        core::enter(&self.core);
        let fibers = {
            let mut core = self.core.borrow_mut();
            match core.runtimes.remove(&plugin.base.id) {
                Some(runtime) => runtime.fibers,
                None => Vec::new(),
            }
        };
        for id in fibers {
            Fiber {
                core: Rc::clone(&self.core),
                id,
            }
            .dispose();
        }
        core::leave(&self.core);
    }
}

impl<C: 'static> Plugin<C> {
    /// The plugin's registry id, unique per constructed plugin value.
    pub fn id(&self) -> u64 {
        self.base.id
    }
}
