//! Typed plugins, the registry, and Context::inject.
//!
//! A plugin is defined either by implementing the [`Plugin`] trait (the
//! native form: a named type with an associated `Config`) or by the
//! [`plugin`] constructor (the closure form, returning an [`FnPlugin`]).
//! Both start through their `start` functions and share one runtime and
//! registry per identity.

use std::any::{Any, TypeId};
use std::collections::HashMap;
use std::marker::PhantomData;
use crate::sync::Rc;
use crate::sync::BorrowExt as _;
use std::sync::{LazyLock, Mutex, OnceLock};

use crate::context::Context;
use crate::core::{self, Bag, PluginBase, RuntimeData};
use crate::fiber::{self, Fiber};

/// Plugin ids are process-global so plugins constructed anywhere can serve
/// as registry identities.
static NEXT_PLUGIN_ID: OnceLock<Mutex<u64>> = OnceLock::new();

/// Registry ids assigned to `Plugin` trait implementations, keyed by type.
static TRAIT_PLUGIN_IDS: LazyLock<Mutex<HashMap<TypeId, u64>>> =
    LazyLock::new(|| Mutex::new(HashMap::new()));

fn next_plugin_id() -> u64 {
    let counter = NEXT_PLUGIN_ID.get_or_init(|| Mutex::new(1));
    let mut next = counter.lock().expect("plugin id lock");
    let id = *next;
    *next += 1;
    id
}

/// The registry id of a `Plugin` trait implementation: one id per type, so
/// starting the same plugin type twice creates two fibers of one runtime.
pub fn plugin_type_id<P: Plugin + ?Sized + 'static>() -> u64 {
    let mut ids = TRAIT_PLUGIN_IDS.lock().expect("trait plugin id lock");
    if let Some(id) = ids.get(&TypeId::of::<P>()) {
        return *id;
    }
    let id = next_plugin_id();
    ids.insert(TypeId::of::<P>(), id);
    id
}

/// A unit of composable behavior defined natively: a type implementing this
/// trait carries a name, an associated config type, injected service
/// dependencies and an apply function. Implement it for a unit struct and
/// start it with [`start`]:
///
/// ```
/// # use cordis::{start, Context, Plugin};
/// struct Greeter;
/// struct GreeterConfig { name: String }
///
/// impl Plugin for Greeter {
///     type Config = GreeterConfig;
///     fn name(&self) -> &str { "greeter" }
///     fn apply(&self, ctx: &Context, config: &GreeterConfig) -> cordis::Result<()> {
///         let name = config.name.clone();
///         ctx.attach(move || println!("bye, {name}"))?;
///         Ok(())
///     }
/// }
///
/// # fn main() {
/// let ctx = Context::new();
/// start(&ctx, Greeter, GreeterConfig { name: "ada".into() }).unwrap();
/// # }
/// ```
///
/// The plugin type is the registry identity: starting `Greeter` twice
/// creates two fibers of one runtime, addressed through
/// [`plugin_type_id::<Greeter>()`](plugin_type_id).
pub trait Plugin: crate::sync::Shared {
    /// The statically typed configuration the plugin applies with.
    type Config: crate::sync::Shared;

    /// The plugin name; empty falls back to the parent chain's name.
    fn name(&self) -> &str;

    /// Service dependencies. The fiber stays pending until all of them are
    /// available and active, unloads when one disappears and reloads when
    /// it returns.
    fn inject(&self) -> Vec<String> {
        Vec::new()
    }

    /// The plugin body. Returning an error fails the fiber and rolls back
    /// every effect it registered.
    fn apply(&self, ctx: &Context, config: &Self::Config) -> crate::Result<()>;
}

/// Start a trait plugin on `ctx` with the given config and return the new
/// fiber. The fiber activates before start returns unless its injected
/// services are missing, in which case it stays pending and activates when
/// they appear.
pub fn start<P: Plugin + 'static>(ctx: &Context, plugin: P, config: P::Config) -> crate::Result<Fiber> {
    let name = plugin.name().to_string();
    let inject = plugin.inject();
    let plugin = Rc::new(plugin);
    let base = Rc::new(PluginBase {
        id: plugin_type_id::<P>(),
        name,
        inject,
        apply: Rc::new(move |ctx, raw| {
            let typed = raw.downcast::<P::Config>().map_err(|_| crate::Error::TypeMismatch {
                name: plugin.name().to_string(),
            })?;
            plugin.apply(ctx, &typed)
        }),
    });
    start_base(ctx, base, Rc::new(config))
}

/// The closure form of a plugin, built by [`plugin`].
pub struct FnPlugin<C: 'static> {
    pub(crate) base: Rc<PluginBase>,
    _marker: PhantomData<fn() -> C>,
}

impl<C: 'static> FnPlugin<C> {
    /// The plugin name.
    pub fn name(&self) -> &str {
        &self.base.name
    }

    /// Declare service dependencies. The fiber stays pending until all of
    /// them are available and active, unloads when one disappears and
    /// reloads when it returns.
    ///
    /// # Panics
    /// When called after the plugin was started or cloned.
    pub fn inject(mut self, deps: &[&str]) -> FnPlugin<C> {
        let base = Rc::get_mut(&mut self.base)
            .expect("plugin.inject must be called before the plugin is started or shared");
        base.inject.extend(deps.iter().map(|s| s.to_string()));
        self
    }

    /// The plugin's registry id, unique per constructed plugin value.
    pub fn id(&self) -> u64 {
        self.base.id
    }
}

impl<C: 'static> Clone for FnPlugin<C> {
    fn clone(&self) -> Self {
        FnPlugin {
            base: Rc::clone(&self.base),
            _marker: PhantomData,
        }
    }
}

/// Create a plugin from a closure. The apply function receives the fiber's
/// context and a reference to the statically typed config; its return value
/// decides the fiber state. Two separately created values are two distinct
/// plugins; share one value (it is cheaply cloneable) for one registry
/// identity.
pub fn plugin<C, F>(name: &str, apply: F) -> FnPlugin<C>
where
    C: 'static,
    F: Fn(&Context, &C) -> crate::Result<()> + 'static,
{
    FnPlugin {
        base: Rc::new(PluginBase {
            id: next_plugin_id(),
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

/// Start a closure plugin on `ctx` with the given config and return the new
/// fiber.
pub fn start_fn<C: crate::sync::Shared>(ctx: &Context, plugin: &FnPlugin<C>, config: C) -> crate::Result<Fiber> {
    start_base(ctx, Rc::clone(&plugin.base), Rc::new(config))
}

impl Context {
    /// Start an anonymous plugin that runs `f` once every service in `deps`
    /// is available, mirroring ctx.inject upstream.
    pub fn inject(&self, deps: &[&str], f: impl Fn(&Context) -> crate::Result<()> + 'static) -> crate::Result<Fiber> {
        let p = plugin("anonymous", move |ctx: &Context, _: &()| f(ctx)).inject(deps);
        start_fn(self, &p, ())
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
    pub(crate) core: Rc<crate::sync::RefCell<crate::core::Core>>,
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

    /// Whether the plugin identified by `id` has at least one live fiber.
    /// Trait plugins use [`plugin_type_id`], closure plugins their
    /// [`FnPlugin::id`].
    pub fn has_id(&self, id: u64) -> bool {
        self.core.borrow().runtimes.contains_key(&id)
    }

    /// Whether the closure plugin has at least one live fiber.
    pub fn has<C: 'static>(&self, plugin: &FnPlugin<C>) -> bool {
        self.has_id(plugin.base.id)
    }

    /// Dispose every fiber of the plugin identified by `id` and remove its
    /// runtime, restoring the state from before its first start.
    pub fn delete_id(&self, id: u64) {
        core::enter(&self.core);
        let fibers = {
            let mut core = self.core.borrow_mut();
            match core.runtimes.remove(&id) {
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

    /// Dispose every fiber of the closure plugin and remove its runtime.
    pub fn delete<C: 'static>(&self, plugin: &FnPlugin<C>) {
        self.delete_id(plugin.base.id);
    }
}

/// Shared start path for both plugin forms.
fn start_base(ctx: &Context, base: Rc<PluginBase>, config: crate::events::Value) -> crate::Result<Fiber> {
    core::enter(&ctx.core);
    let result = start_inner(ctx, base, config);
    core::leave(&ctx.core);
    result
}

fn start_inner(ctx: &Context, base: Rc<PluginBase>, config: crate::events::Value) -> crate::Result<Fiber> {
    ctx.fiber().assert_active()?;
    let parent_bag = match ctx.bag() {
        Some(bag) => bag,
        None => return Err(crate::Error::InactiveEffect),
    };

    let runtime_id = base.id;
    {
        let mut core = ctx.core.borrow_mut();
        core.runtimes.entry(runtime_id).or_insert_with(|| RuntimeData {
            name: base.name.clone(),
            apply: Rc::clone(&base.apply),
            fibers: Vec::new(),
        });
    }

    let data = fiber::new_fiber(ctx, config, &base.inject, runtime_id);
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
