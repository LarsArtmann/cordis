package cordis

import (
	"errors"
	"fmt"
	"sync"
)

// Listener handles an event. The returned value is significant for the Bail,
// Serial and Waterfall dispatch modes and ignored by Emit and Parallel.
// Returning a non-nil error from a listener invoked through Parallel
// aggregates it into the returned error.
type Listener func(args ...any) any

// EventOption customizes listener registration.
type EventOption func(*eventOptions)

type eventOptions struct {
	prepend bool
	global  bool
}

// Prepend registers the listener before existing ones for the same event.
func Prepend() EventOption {
	return func(o *eventOptions) { o.prepend = true }
}

// Global marks the listener as exempt from emission filters. Global
// listeners receive every event regardless of the emitter's context filter.
func Global() EventOption {
	return func(o *eventOptions) { o.global = true }
}

// hook is one registered listener.
type hook struct {
	owner  *Context
	fn     Listener
	global bool
	seq    int64
}

// Well known internal events, mirroring the Events interface upstream.
const (
	// EventPlugin fires when a fiber is created or disposed. Args: (*Fiber).
	EventPlugin = "internal/plugin"
	// EventStatus fires on every fiber state change. Args: (*Fiber, FiberState).
	EventStatus = "internal/status"
	// EventService fires when a service is provided or unprovided.
	// Args: (name string, value any).
	EventService = "internal/service"
	// EventUpdate intercepts config updates as a waterfall.
	// Args: (config any, noSave bool, next func(...any) any).
	EventUpdate = "internal/update"
)

// On subscribes listener to name. The subscription is bound to the context's
// fiber: it is removed automatically when the fiber unloads or is disposed.
// The returned Disposer removes it ahead of time and is idempotent.
func (c *Context) On(name string, listener Listener, opts ...EventOption) (Disposer, error) {
	c.core.enter()
	defer c.core.leave()

	var options eventOptions
	for _, opt := range opts {
		opt(&options)
	}

	h := &hook{owner: c, fn: listener, global: options.global}

	c.core.mu.Lock()
	if options.prepend {
		c.core.hooks[name] = append([]*hook{h}, c.core.hooks[name]...)
	} else {
		c.core.hooks[name] = append(c.core.hooks[name], h)
	}
	c.core.mu.Unlock()

	dispose, err := c.registerCleanup(fmt.Sprintf("ctx.on(%q)", name), func() {
		c.core.mu.Lock()
		defer c.core.mu.Unlock()
		hooks := c.core.hooks[name]
		for i, candidate := range hooks {
			if candidate == h {
				c.core.hooks[name] = append(hooks[:i], hooks[i+1:]...)
				return
			}
		}
	})
	if err != nil {
		c.core.mu.Lock()
		c.core.removeHookLocked(name, h)
		c.core.mu.Unlock()
		return nil, err
	}
	return dispose, nil
}

func (c *core) removeHookLocked(name string, h *hook) {
	hooks := c.hooks[name]
	for i, candidate := range hooks {
		if candidate == h {
			c.hooks[name] = append(hooks[:i], hooks[i+1:]...)
			return
		}
	}
}

// Once subscribes listener to name and removes it after the first delivery.
func (c *Context) Once(name string, listener Listener, opts ...EventOption) (Disposer, error) {
	var mu sync.Mutex
	var dispose Disposer
	var fired bool
	d, err := c.On(name, func(args ...any) any {
		mu.Lock()
		if fired {
			mu.Unlock()
			return nil
		}
		fired = true
		d := dispose
		mu.Unlock()
		if d != nil {
			d()
		}
		return listener(args...)
	}, opts...)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	dispose = d
	alreadyFired := fired
	mu.Unlock()
	if alreadyFired {
		// The event fired between registration and assignment; remove the
		// listener now so Once semantics hold exactly.
		d()
	}
	return d, nil
}

// resolveHooks snapshots the listeners for name visible from the emitting
// context. Filters are user code and may re-enter the framework, so they run
// after the lock is released.
func (c *Context) resolveHooks(name string) []*hook {
	c.core.mu.Lock()
	hooks := append([]*hook(nil), c.core.hooks[name]...)
	filter := c.filter
	c.core.mu.Unlock()
	if len(hooks) == 0 {
		return nil
	}
	result := make([]*hook, 0, len(hooks))
	for _, h := range hooks {
		if h.global || filter == nil || filter(h.owner) {
			result = append(result, h)
		}
	}
	return result
}

// Emit delivers name synchronously to every matching listener in
// registration order. A panicking listener propagates the panic to the
// caller, mirroring ctx.emit upstream.
func (c *Context) Emit(name string, args ...any) {
	for _, h := range c.resolveHooks(name) {
		h.fn(args...)
	}
}

// Parallel delivers name to every matching listener concurrently and waits
// for all of them. Listeners that return an error or panic contribute to the
// joined error, mirroring the AggregateError of ctx.parallel upstream.
func (c *Context) Parallel(name string, args ...any) error {
	hooks := c.resolveHooks(name)
	if len(hooks) == 0 {
		return nil
	}
	var wg sync.WaitGroup
	errs := make([]error, len(hooks))
	for i, h := range hooks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errs[i] = fmt.Errorf("%v", r)
				}
			}()
			if result := h.fn(args...); result != nil {
				if err, ok := result.(error); ok {
					errs[i] = err
				}
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// Serial delivers name to matching listeners one at a time and returns the
// first non-nil result, mirroring ctx.serial upstream.
func (c *Context) Serial(name string, args ...any) any {
	for _, h := range c.resolveHooks(name) {
		if result := h.fn(args...); isBailed(result) {
			return result
		}
	}
	return nil
}

// Bail is the synchronous counterpart of Serial: it returns the first
// non-nil listener result, mirroring ctx.bail upstream.
func (c *Context) Bail(name string, args ...any) any {
	return c.Serial(name, args...)
}

// Waterfall composes listeners around a terminal function, mirroring
// ctx.waterfall upstream. The last argument must be the terminal function of
// type func(...any) any; each listener receives the remaining arguments
// followed by a next function invoking the rest of the chain. A listener
// that does not call next short-circuits the composition.
func (c *Context) Waterfall(name string, args ...any) any {
	if len(args) == 0 {
		panic("cordis: Waterfall requires a terminal function as last argument")
	}
	inner, ok := args[len(args)-1].(func(...any) any)
	if !ok {
		panic("cordis: Waterfall last argument must be func(...any) any")
	}
	hooks := c.resolveHooks(name)
	callArgs := args[:len(args)-1]
	var next func(...any) any
	next = func(nextArgs ...any) any {
		if len(hooks) == 0 {
			return inner(nextArgs...)
		}
		h := hooks[0]
		hooks = hooks[1:]
		// Copy: appending next must never write into the caller's slice
		// backing array.
		full := make([]any, 0, len(nextArgs)+1)
		full = append(full, nextArgs...)
		full = append(full, next)
		return h.fn(full...)
	}
	return next(callArgs...)
}

// isBailed mirrors the upstream helper: nil, false and missing results do
// not bail; everything else does.
func isBailed(value any) bool {
	if value == nil {
		return false
	}
	if b, ok := value.(bool); ok && !b {
		return false
	}
	return true
}
