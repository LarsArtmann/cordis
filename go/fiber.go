package cordis

import (
	"context"
	"fmt"
	"maps"
)

// FiberState describes the lifecycle of a fiber, mirroring FiberState in the
// TypeScript implementation.
type FiberState int

const (
	// StatePending: the fiber waits for its injected services.
	StatePending FiberState = iota
	// StateLoading: the plugin body is executing.
	StateLoading
	// StateActive: the plugin body has completed and its effects are live.
	StateActive
	// StateFailed: the plugin body returned an error or panicked. Partial
	// effects have been rolled back.
	StateFailed
	// StateDisposed: the fiber has been disposed and can never activate again.
	StateDisposed
	// StateUnloading: the fiber's effects are being rolled back.
	StateUnloading
)

func (s FiberState) String() string {
	switch s {
	case StatePending:
		return "PENDING"
	case StateLoading:
		return "LOADING"
	case StateActive:
		return "ACTIVE"
	case StateFailed:
		return "FAILED"
	case StateDisposed:
		return "DISPOSED"
	case StateUnloading:
		return "UNLOADING"
	}
	return "UNKNOWN"
}

// Fiber is an effect scope: one instance of a running plugin. Every fiber
// carries the cleanups its plugin registered and is activated, unloaded and
// reloaded by the framework as its injected services appear and disappear.
//
// The zero-lifecycle rule: when a fiber leaves StateActive, every effect it
// registered is rolled back in reverse order; when its dependencies return,
// the same fiber instance reloads in place.
type Fiber struct {
	core   *core
	ctx    *Context
	parent *Context

	runtime *Runtime

	// All fields below are guarded by core.mu unless documented otherwise.

	uid    int
	config any

	inject       map[string]struct{}
	injectConfig map[string]any

	state            FiberState
	err              error
	disposed         bool
	restartRequested bool
	queued           bool
	executing        bool

	activeBag *disposeBag
	entry     *disposeItem
	entryBag  *disposeBag

	idleCh chan struct{}

	// stdCtx is the stdlib context of the fiber's current activation. It is
	// cancelled on unload and disposal and renewed on every load, giving
	// plugin owned goroutines a uniform shutdown signal.
	stdCtx    context.Context
	stdCancel context.CancelCauseFunc
}

// newRootFiber creates the fiber backing a root context. The root fiber is
// always active, has uid 0 and no runtime.
func newRootFiber(ctx *Context) *Fiber {
	f := &Fiber{
		core:      ctx.core,
		ctx:       ctx,
		uid:       0,
		state:     StateActive,
		inject:    map[string]struct{}{},
		activeBag: newDisposeBag(ctx.core),
		idleCh:    make(chan struct{}),
	}
	f.stdCtx, f.stdCancel = context.WithCancelCause(context.Background())
	return f
}

// newFiber creates a plugin fiber in StatePending. Activation is deferred to
// the drain queue.
func newFiber(parent *Context, config any, base *pluginBase, rt *Runtime) *Fiber {
	f := &Fiber{
		core:         parent.core,
		parent:       parent,
		config:       config,
		runtime:      rt,
		inject:       make(map[string]struct{}, len(base.inject)),
		injectConfig: base.injectConfig,
		state:        StatePending,
		idleCh:       make(chan struct{}),
	}
	for _, name := range base.inject {
		f.inject[name] = struct{}{}
	}
	f.uid = f.core.nextUID()

	ctx := parent.Extend()
	ctx.fiber = f
	if len(f.injectConfig) > 0 {
		ctx.intercept = make(map[string]any, len(f.injectConfig))
		maps.Copy(ctx.intercept, f.injectConfig)
	}
	f.ctx = ctx
	f.stdCtx, f.stdCancel = context.WithCancelCause(context.Background())
	return f
}

// Context returns the context owned by this fiber. Plugins receive it as
// their apply argument.
func (f *Fiber) Context() *Context { return f.ctx }

// Config returns the config the fiber was last started or updated with.
func (f *Fiber) Config() any {
	f.core.mu.Lock()
	defer f.core.mu.Unlock()
	return f.config
}

// UID returns the framework-wide unique id of the fiber. It is 0 for the
// root fiber and -1 after disposal, mirroring uid semantics upstream.
func (f *Fiber) UID() int {
	f.core.mu.Lock()
	defer f.core.mu.Unlock()
	return f.uid
}

// State returns the current lifecycle state.
func (f *Fiber) State() FiberState {
	f.core.mu.Lock()
	defer f.core.mu.Unlock()
	return f.state
}

// Name returns the plugin name, or "root" for the root fiber, mirroring the
// name resolution upstream: unnamed fibers inherit from their parent chain.
func (f *Fiber) Name() string {
	f.core.mu.Lock()
	rt := f.runtime
	parent := f.parent
	f.core.mu.Unlock()
	if rt != nil && rt.Name != "" {
		return rt.Name
	}
	if parent != nil {
		return parent.fiber.Name()
	}
	return "root"
}

// assertActive mirrors Fiber.assertActive upstream: only disposed fibers are
// rejected.
func (f *Fiber) assertActive() error {
	f.core.mu.Lock()
	defer f.core.mu.Unlock()
	if f.disposed {
		return ErrInactiveEffect
	}
	return nil
}

// bag returns the cleanup collection of the current activation, or nil when
// the fiber cannot accept effects right now.
func (f *Fiber) bag() *disposeBag {
	f.core.mu.Lock()
	defer f.core.mu.Unlock()
	if f.disposed {
		return nil
	}
	if f.state == StateActive || f.state == StateLoading {
		return f.activeBag
	}
	return nil
}

// StdContext returns the stdlib context of the fiber's current activation.
// It is cancelled when the fiber unloads, restarts or is disposed, and a
// fresh one is installed on every load. Use it to propagate fiber lifetime
// into plugin owned goroutines and I/O calls:
//
//	go doWork(ctx.Fiber().StdContext())
//
// The context of the root fiber is cancelled when the root scope restarts.
func (f *Fiber) StdContext() context.Context {
	f.core.mu.Lock()
	defer f.core.mu.Unlock()
	return f.stdCtx
}

// Done returns a channel closed when the fiber's current activation ends:
// it unloads (a dependency disappeared), restarts or is disposed. It is the
// select friendly view of StdContext and receives a fresh channel on every
// reload, mirroring context.Done semantics.
func (f *Fiber) Done() <-chan struct{} {
	return f.StdContext().Done()
}

// renewStdLocked installs a fresh stdlib context, cancelling the previous
// one. Callers must hold core.mu.
func (f *Fiber) renewStdLocked() {
	if f.stdCancel != nil {
		f.stdCancel(nil)
	}
	f.stdCtx, f.stdCancel = context.WithCancelCause(context.Background())
}

// cancelStdLocked cancels the stdlib context of the current activation.
// Callers must hold core.mu. Cancelling is idempotent, so fibers that never
// activated can call it unconditionally.
func (f *Fiber) cancelStdLocked() {
	if f.stdCancel != nil {
		f.stdCancel(nil)
	}
}

// GetEffects returns the introspection tree of live effects, mirroring
// Fiber.getEffects() upstream.
func (f *Fiber) GetEffects() []EffectMeta {
	bag := f.bag()
	if bag == nil {
		return nil
	}
	return bag.meta()
}

// Effect executes fn within this context's effect scope. Cleanups registered
// through the context passed to fn (via On, Provide, nested Effect or the
// returned Cleanup of a nested effect) become children of this effect and
// roll back together, last in, first out.
//
// If fn returns an error or panics, everything it registered is rolled back
// immediately and the error is returned. The returned Disposer disposes the
// effect ahead of the fiber's own lifecycle and is idempotent.
func (c *Context) Effect(fn func(ctx *Context) error, label ...string) (Disposer, error) {
	c.core.enter()
	defer c.core.leave()

	lbl := "anonymous"
	if len(label) > 0 {
		lbl = label[0]
	}

	if err := c.fiber.assertActive(); err != nil {
		return nil, err
	}
	parent := c.collect
	if parent == nil {
		parent = c.fiber.bag()
	}
	if parent == nil {
		return nil, ErrInactiveEffect
	}

	child := newDisposeBag(c.core)
	item := parent.pushEffect(lbl, child)
	err := runGuardedEffect(fn, c.withCollect(child))
	if err != nil {
		item.dispose(parent)
		return nil, err
	}
	return func() { item.dispose(parent) }, nil
}

func runGuardedEffect(fn func(*Context) error, ctx *Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("effect panicked: %v", r)
		}
	}()
	return fn(ctx)
}

// resolveDeps reports whether every injected service is currently available
// in this fiber's realm. Check functions run without locks held.
func (f *Fiber) resolveDeps() bool {
	f.core.mu.Lock()
	type candidate struct {
		im *impl
		st FiberState
	}
	candidates := make([]candidate, 0, len(f.inject))
	for name := range f.inject {
		key := f.ctx.isolateKeyLocked(f.core, name)
		im := f.core.store[key]
		if im == nil {
			f.core.mu.Unlock()
			return false
		}
		candidates = append(candidates, candidate{im: im, st: im.fiber.state})
	}
	f.core.mu.Unlock()

	for _, cand := range candidates {
		if cand.st != StateActive {
			return false
		}
		if cand.im.check != nil && !cand.im.check() {
			return false
		}
	}
	return true
}

// transition performs one step of the fiber state machine. It runs on the
// drain queue and never holds locks while executing user code.
func (f *Fiber) transition() {
	c := f.core
	for {
		c.mu.Lock()
		if f.executing {
			c.mu.Unlock()
			return
		}
		restart := f.restartRequested
		f.restartRequested = false
		disposed := f.disposed
		state := f.state
		gen := c.generation()
		c.mu.Unlock()

		depsOK := !disposed && f.resolveDeps()

		c.mu.Lock()
		if f.executing || c.generation() != gen || disposed != f.disposed {
			// The world changed while resolving dependencies; re-evaluate.
			c.mu.Unlock()
			continue
		}

		wantActive := depsOK && !disposed
		isActive := state == StateActive

		switch {
		case disposed && !isActive:
			f.setStateLocked(StateDisposed)
			f.uid = -1
			f.cancelStdLocked()
			c.mu.Unlock()
			f.settle()
			return
		case isActive && (restart || !wantActive):
			f.executing = true
			f.setStateLocked(StateUnloading)
			f.cancelStdLocked()
			c.mu.Unlock()
			f.unload()
			if wantActive {
				f.load()
			}
			c.mu.Lock()
			f.executing = false
			if f.disposed {
				f.setStateLocked(StateDisposed)
				f.uid = -1
				f.cancelStdLocked()
			} else if f.state != StateFailed {
				if wantActive {
					f.setStateLocked(StateActive)
				} else {
					f.setStateLocked(StatePending)
				}
			}
			c.mu.Unlock()
			f.settle()
			return
		case !isActive && wantActive && state != StateDisposed:
			f.executing = true
			c.mu.Unlock()
			f.load()
			c.mu.Lock()
			f.executing = false
			if f.disposed {
				f.setStateLocked(StateDisposed)
				f.uid = -1
				f.cancelStdLocked()
			} else if f.state != StateFailed {
				f.setStateLocked(StateActive)
			}
			c.mu.Unlock()
			f.settle()
			return
		default:
			c.mu.Unlock()
			f.settle()
			return
		}
	}
}

// unload drains the effect bag, running every cleanup in reverse order, and
// cancels the stdlib context of the activation.
func (f *Fiber) unload() {
	c := f.core
	c.mu.Lock()
	bag := f.activeBag
	f.activeBag = nil
	f.cancelStdLocked()
	c.mu.Unlock()
	if bag == nil {
		return
	}
	for _, item := range bag.take() {
		item.execute(c)
	}
}

// load executes the plugin body, collecting its effects. On failure the
// partial effects are rolled back and the fiber enters StateFailed.
func (f *Fiber) load() {
	c := f.core
	c.mu.Lock()
	bag := newDisposeBag(c)
	f.activeBag = bag
	f.renewStdLocked()
	f.setStateLocked(StateLoading)
	apply := f.runtime.base.apply
	config := f.config
	ctx := f.ctx
	c.mu.Unlock()

	err := runGuardedApply(apply, ctx, config)
	name := f.Name()

	c.mu.Lock()
	if err != nil {
		f.err = err
		f.activeBag = nil
		f.cancelStdLocked()
		c.mu.Unlock()
		for _, item := range bag.take() {
			item.execute(c)
		}
		c.logError(name, err)
		c.mu.Lock()
		f.setStateLocked(StateFailed)
		c.mu.Unlock()
		return
	}
	f.err = nil
	c.mu.Unlock()
}

func runGuardedApply(fn func(*Context, any) error, ctx *Context, config any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin panicked: %v", r)
		}
	}()
	return fn(ctx, config)
}

// setStateLocked assigns the new state and emits EventStatus. Callers must
// hold core.mu; emission is deferred until the lock is released by the
// caller, so this only records the change.
func (f *Fiber) setStateLocked(state FiberState) {
	old := f.state
	if old == state {
		return
	}
	f.state = state
	f.core.pendingStatus = append(f.core.pendingStatus, statusChange{fiber: f, old: old})
}

// settle closes the current idle channel, waking every waiter of Await, and
// flushes deferred status events.
func (f *Fiber) settle() {
	c := f.core
	c.mu.Lock()
	close(f.idleCh)
	f.idleCh = make(chan struct{})
	changes := c.pendingStatus
	c.pendingStatus = nil
	c.mu.Unlock()
	for _, change := range changes {
		change.fiber.ctx.Emit(EventStatus, change.fiber, change.old)
	}
}

// Await blocks until the fiber has no pending or running transitions and
// then returns its error, if any. It mirrors awaiting the thenable fiber
// upstream.
func (f *Fiber) Await() error {
	c := f.core
	for {
		c.mu.Lock()
		if !f.queued && !f.executing {
			err := f.err
			c.mu.Unlock()
			return err
		}
		ch := f.idleCh
		c.mu.Unlock()
		<-ch
	}
}

// Dispose permanently deactivates the fiber: its effects roll back, it
// leaves its plugin runtime and it can never activate again. Dispose is
// idempotent. Disposing the root fiber restarts it instead, mirroring
// upstream.
func (f *Fiber) Dispose() {
	c := f.core
	c.enter()
	defer c.leave()

	c.mu.Lock()
	if f.runtime == nil {
		c.mu.Unlock()
		f.restartRoot()
		return
	}
	if f.disposed {
		c.mu.Unlock()
		return
	}
	f.disposed = true
	rt := f.runtime
	rt.removeFiberLocked(f)
	if len(rt.fibers) == 0 {
		delete(c.runtimes, rt.base)
	}
	c.mu.Unlock()

	if f.entry != nil {
		f.entry.detach(f.entryBag)
	}
	f.ctx.Emit(EventPlugin, f)
	c.queue(f)
}

// disposeBody is the cleanup registered on the parent fiber's bag. It routes
// through Dispose so manual disposal and parent disposal share one path.
func (f *Fiber) disposeBody() {
	f.Dispose()
}

// Restart unloads and reloads the fiber with its current config. If the
// fiber is waiting for dependencies it simply re-evaluates them. Restarting
// the root fiber rolls back every root scope effect, mirroring upstream.
func (f *Fiber) Restart() error {
	c := f.core
	c.enter()
	defer c.leave()

	if err := f.assertActive(); err != nil {
		return err
	}
	c.mu.Lock()
	if f.runtime == nil {
		c.mu.Unlock()
		f.restartRoot()
		return nil
	}
	f.restartRequested = true
	f.err = nil
	c.mu.Unlock()
	c.queue(f)
	return nil
}

// restartRoot implements root fiber disposal: every effect of the root scope
// rolls back, the stdlib context rotates and the root starts over with an
// empty bag.
func (f *Fiber) restartRoot() {
	c := f.core
	c.mu.Lock()
	f.restartRequested = false
	c.mu.Unlock()
	f.unload()
	c.mu.Lock()
	f.activeBag = newDisposeBag(c)
	f.renewStdLocked()
	c.mu.Unlock()
}

// Update replaces the fiber's config and restarts it. The update is
// interceptable through the EventUpdate waterfall. Validation errors are
// returned synchronously; the restart itself settles through the drain
// queue, so cascading dependency updates never observe torn states.
func (f *Fiber) Update(config any) error {
	c := f.core
	c.enter()
	defer c.leave()

	if err := f.assertActive(); err != nil {
		return err
	}
	if f.runtime == nil {
		return fmt.Errorf("cordis: cannot update the root fiber")
	}
	if validate := f.runtime.base.validate; validate != nil {
		validated, err := validate(config)
		if err != nil {
			return err
		}
		config = validated
	}

	next := func(args ...any) any {
		c.mu.Lock()
		if len(args) > 1 {
			f.config = args[1]
		}
		f.err = nil
		f.restartRequested = true
		c.mu.Unlock()
		c.queue(f)
		return nil
	}
	f.ctx.Waterfall(EventUpdate, f, config, false, next)
	return nil
}

// statusChange records one state transition for deferred emission.
type statusChange struct {
	fiber *Fiber
	old   FiberState
}
