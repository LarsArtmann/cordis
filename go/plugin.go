package cordis

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

// Plugin is a typed unit of composable behavior: a name, a set of injected
// service dependencies, an optional config validator and an apply function.
//
// A Plugin value is also its own registry identity: starting the same Plugin
// twice creates two fibers of one runtime, and Registry operations address
// every fiber of the plugin at once, mirroring the callback keyed runtime
// map upstream.
type Plugin[C any] struct {
	base *pluginBase
}

// pluginBase is the type erased runtime identity of a Plugin.
type pluginBase struct {
	name         string
	inject       []string
	injectConfig map[string]any
	apply        func(ctx *Context, config any) error
	validate     func(config any) (any, error)
}

// PluginHandle addresses a plugin at the registry without its type
// parameter. Every Plugin implements it.
type PluginHandle interface {
	handle() *pluginBase
}

func (p *Plugin[C]) handle() *pluginBase { return p.base }

// NewPlugin creates a plugin with the given name and apply function. The
// apply function receives the fiber's context and the statically typed
// config. Its return value decides the fiber state: nil activates the fiber,
// a non-nil error moves it to StateFailed after rolling back partial
// effects.
//
// An empty name is replaced by the name of the apply function, mirroring the
// function name fallback upstream.
func NewPlugin[C any](name string, apply func(ctx *Context, config C) error) *Plugin[C] {
	base := &pluginBase{
		name: name,
		apply: func(ctx *Context, config any) error {
			typed, ok := config.(C)
			if !ok {
				var zero C
				return fmt.Errorf("cordis: plugin %q received config of type %T, expected %T", name, config, zero)
			}
			return apply(ctx, typed)
		},
	}
	if base.name == "" {
		base.name = funcName(apply)
	}
	return &Plugin[C]{base: base}
}

func funcName(fn any) string {
	pc := reflect.ValueOf(fn).Pointer()
	if f := runtime.FuncForPC(pc); f != nil {
		name := f.Name()
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		return strings.TrimSuffix(name, "-fm")
	}
	return "anonymous"
}

// Name returns the plugin name.
func (p *Plugin[C]) Name() string { return p.base.name }

// Inject declares services the plugin depends on. The fiber stays
// StatePending until every dependency is available and active, unloads when
// one disappears and reloads when it returns. Inject returns the plugin for
// chaining and must be called before the first Start.
func (p *Plugin[C]) Inject(deps ...string) *Plugin[C] {
	p.base.inject = append(p.base.inject, deps...)
	return p
}

// InjectConfig declares a dependency together with a configuration override
// for the injected service, visible to the plugin through Context.Intercepted.
// It mirrors object style inject entries upstream.
func (p *Plugin[C]) InjectConfig(name string, config any) *Plugin[C] {
	p.base.inject = append(p.base.inject, name)
	if p.base.injectConfig == nil {
		p.base.injectConfig = make(map[string]any)
	}
	p.base.injectConfig[name] = config
	return p
}

// Validate installs a config validator. It runs on Start and on every
// Update; returning an error rejects the call with a ValidationError
// compatible value.
func (p *Plugin[C]) Validate(fn func(config C) error) *Plugin[C] {
	p.base.validate = func(config any) (any, error) {
		typed, ok := config.(C)
		if !ok {
			var zero C
			return nil, fmt.Errorf("cordis: plugin %q received config of type %T, expected %T", p.base.name, config, zero)
		}
		if err := fn(typed); err != nil {
			return nil, err
		}
		return config, nil
	}
	return p
}

// Start applies the plugin on ctx with the given config and returns the new
// fiber. The fiber activates before Start returns unless its injected
// services are missing, in which case it stays StatePending and activates
// when they appear.
func Start[C any](ctx *Context, plugin *Plugin[C], config C) (*Fiber, error) {
	if plugin == nil {
		return nil, fmt.Errorf("invalid plugin, expect function or object with an apply method, received %T", plugin)
	}
	return startPlugin(ctx, plugin.base, config)
}

// Inject starts an anonymous plugin that runs fn once every service in deps
// is available, mirroring ctx.inject upstream. The fiber reloads whenever
// the dependency set becomes complete again after losing a dependency.
func (c *Context) Inject(deps []string, fn func(ctx *Context) error) *Fiber {
	base := &pluginBase{
		name:   funcName(fn),
		inject: deps,
		apply: func(ctx *Context, _ any) error {
			return fn(ctx)
		},
	}
	fiber, err := startPlugin(c, base, nil)
	if err != nil {
		c.core.logError("root", err)
		return nil
	}
	return fiber
}

func startPlugin(ctx *Context, base *pluginBase, config any) (*Fiber, error) {
	c := ctx.core
	c.enter()
	defer c.leave()

	if base == nil || base.apply == nil {
		return nil, fmt.Errorf("invalid plugin, expect function or object with an apply method, received %T", base)
	}
	if err := ctx.fiber.assertActive(); err != nil {
		return nil, err
	}
	parent := ctx.collect
	if parent == nil {
		parent = ctx.fiber.bag()
	}
	if parent == nil {
		return nil, ErrInactiveEffect
	}

	c.mu.Lock()
	rt := c.runtimes[base]
	if rt == nil {
		rt = &Runtime{Name: base.name, base: base, core: c}
		c.runtimes[base] = rt
	}
	c.mu.Unlock()

	f := newFiber(ctx, config, base, rt)
	f.entryBag = parent
	f.entry = parent.push("ctx.plugin()", f.disposeBody)

	c.mu.Lock()
	rt.fibers = append(rt.fibers, f)
	c.mu.Unlock()

	ctx.Emit(EventPlugin, f)

	if base.validate != nil {
		validated, err := base.validate(config)
		if err != nil {
			c.mu.Lock()
			f.err = err
			f.setStateLocked(StateFailed)
			c.mu.Unlock()
			c.logError(f.Name(), err)
			f.settle()
			return f, nil
		}
		c.mu.Lock()
		f.config = validated
		c.mu.Unlock()
	}

	c.queue(f)
	return f, nil
}

// Runtime tracks every live fiber of one plugin, mirroring Plugin.Runtime
// upstream.
type Runtime struct {
	Name string

	core   *core
	base   *pluginBase
	fibers []*Fiber
}

// Fibers returns a snapshot of the runtime's live fibers.
func (rt *Runtime) Fibers() []*Fiber {
	rt.core.mu.Lock()
	defer rt.core.mu.Unlock()
	return append([]*Fiber(nil), rt.fibers...)
}

// removeFiberLocked detaches f from the runtime. Callers must hold core.mu.
func (rt *Runtime) removeFiberLocked(f *Fiber) {
	for i, candidate := range rt.fibers {
		if candidate == f {
			rt.fibers = append(rt.fibers[:i], rt.fibers[i+1:]...)
			return
		}
	}
}

// Registry is the view of every plugin runtime in a context tree, mirroring
// RegistryService upstream.
type Registry struct {
	core *core
}

// Registry returns the registry of this context tree.
func (c *Context) Registry() *Registry {
	return &Registry{core: c.core}
}

// Size returns the number of plugin runtimes with at least one live fiber.
func (r *Registry) Size() int {
	r.core.mu.Lock()
	defer r.core.mu.Unlock()
	return len(r.core.runtimes)
}

// Has reports whether the plugin has at least one live fiber.
func (r *Registry) Has(p PluginHandle) bool {
	r.core.mu.Lock()
	defer r.core.mu.Unlock()
	_, ok := r.core.runtimes[p.handle()]
	return ok
}

// Get returns the runtime of the plugin, or nil when it has no live fiber.
func (r *Registry) Get(p PluginHandle) *Runtime {
	r.core.mu.Lock()
	defer r.core.mu.Unlock()
	return r.core.runtimes[p.handle()]
}

// Delete disposes every fiber of the plugin and removes its runtime,
// restoring the exact state from before its first Start.
func (r *Registry) Delete(p PluginHandle) {
	c := r.core
	c.enter()
	defer c.leave()

	c.mu.Lock()
	rt := c.runtimes[p.handle()]
	if rt == nil {
		c.mu.Unlock()
		return
	}
	delete(c.runtimes, p.handle())
	fibers := append([]*Fiber(nil), rt.fibers...)
	c.mu.Unlock()

	for _, f := range fibers {
		f.Dispose()
	}
}
