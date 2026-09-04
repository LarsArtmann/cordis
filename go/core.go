package cordis

import "sync"

// isolateKey is the Go equivalent of the per-realm symbols the TypeScript
// implementation stores in ctx[Context.isolate]. Services are stored by key,
// not by name, so isolated contexts can shadow a service name without
// affecting other realms.
type isolateKey uint64

// propType describes how a name on the context was declared.
type propType int

const (
	propService propType = iota
	propAccessor
)

// impl is a provided service instance living in the store of one realm.
type impl struct {
	name  string
	key   isolateKey
	fiber *Fiber
	value any
	check func() bool
}

// core is the mutable state shared by every context of one context tree.
// All fields are guarded by mu. User callbacks (listeners, plugin bodies,
// cleanups, check functions excluded) are never invoked while mu is held.
type core struct {
	mu sync.Mutex

	// depth counts nested public API calls. State transitions are deferred
	// until the outermost call returns, which coalesces cascading updates
	// the same way the microtask queue does in the TypeScript runtime.
	depth    int
	draining bool
	dirty    []*Fiber

	lastKey  isolateKey
	keys     map[string]isolateKey
	hooks    map[string][]*hook
	runtimes map[*pluginBase]*Runtime
	store    map[isolateKey]*impl
	props    map[string]propType

	// storeGen bumps on every service store mutation. Dependency resolution
	// snapshots it to detect concurrent changes.
	storeGen uint64

	// pendingStatus accumulates state changes during a transition for
	// deferred event emission.
	pendingStatus []statusChange

	logger *loggerService

	counter int
}

func newCore() *core {
	c := &core{
		keys:     make(map[string]isolateKey),
		hooks:    make(map[string][]*hook),
		runtimes: make(map[*pluginBase]*Runtime),
		store:    make(map[isolateKey]*impl),
		props:    make(map[string]propType),
	}
	c.logger = newLoggerService()
	return c
}

// generation returns the service store generation counter.
// Callers must hold c.mu.
func (c *core) generation() uint64 {
	return c.storeGen
}

// bumpGenerationLocked invalidates dependency resolution snapshots.
// Callers must hold c.mu.
func (c *core) bumpGenerationLocked() {
	c.storeGen++
}

func (c *core) nextUID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter++
	return c.counter
}

// rootKey returns the isolate key of name in the root realm, assigning one on
// first use. Callers must hold no locks; the key space is allocated lazily
// exactly like `root[isolate][name] ??= Symbol(name)` upstream.
func (c *core) rootKey(name string) isolateKey {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rootKeyLocked(name)
}

func (c *core) rootKeyLocked(name string) isolateKey {
	key, ok := c.keys[name]
	if !ok {
		c.lastKey++
		key = c.lastKey
		c.keys[name] = key
	}
	return key
}

// enter marks the beginning of a public API call.
func (c *core) enter() {
	c.mu.Lock()
	c.depth++
	c.mu.Unlock()
}

// leave marks the end of a public API call. When the outermost call on this
// goroutine returns, all fibers with pending state transitions are settled
// before control is handed back to user code.
func (c *core) leave() {
	c.mu.Lock()
	c.depth--
	if c.depth != 0 || c.draining {
		c.mu.Unlock()
		return
	}
	c.draining = true
	for len(c.dirty) > 0 {
		f := c.dirty[0]
		c.dirty = c.dirty[1:]
		f.queued = false
		c.mu.Unlock()
		f.transition()
		c.mu.Lock()
	}
	c.draining = false
	c.mu.Unlock()
}

// queue schedules a state transition evaluation for f. It is safe to call
// from any goroutine, with or without the core lock held.
func (c *core) queue(f *Fiber) {
	c.mu.Lock()
	if !f.queued {
		f.queued = true
		c.dirty = append(c.dirty, f)
	}
	c.mu.Unlock()
}

// notifyDependents queues every fiber that injects one of names and lives in
// the realm where the change happened, mirroring ReflectService.notify.
func (c *core) notifyDependents(from *Context, names ...string) {
	c.mu.Lock()
	var fibers []*Fiber
	for _, rt := range c.runtimes {
		for _, f := range rt.fibers {
			if f.matchesAnyLocked(from, names) {
				fibers = append(fibers, f)
			}
		}
	}
	c.mu.Unlock()
	for _, f := range fibers {
		c.queue(f)
	}
}

// matchesAnyLocked reports whether f injects any of names in the same realm
// as from. Callers must hold c.mu.
func (f *Fiber) matchesAnyLocked(from *Context, names []string) bool {
	for _, name := range names {
		if _, ok := f.inject[name]; !ok {
			continue
		}
		if f.ctx.isolateKeyLocked(from.core, name) == from.isolateKeyLocked(from.core, name) {
			return true
		}
	}
	return false
}
