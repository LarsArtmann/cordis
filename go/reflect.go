package cordis

import "fmt"

// Provide publishes value under name in this context's service realm. The
// service is bound to the context's fiber: it is withdrawn automatically
// when the fiber unloads, and every fiber injecting name is re-evaluated on
// both publication and withdrawal.
//
// An optional check function can hide the service from dependent fibers
// while it is not ready, mirroring Service.check upstream.
//
// Provide fails when name is already provided in this realm or when the
// fiber is inactive.
func (c *Context) Provide(name string, value any, check ...func() bool) (Disposer, error) {
	co := c.core
	co.enter()
	defer co.leave()

	var chk func() bool
	if len(check) > 0 {
		chk = check[0]
	}

	co.mu.Lock()
	if t, ok := co.props[name]; ok && t != propService {
		co.mu.Unlock()
		return nil, fmt.Errorf("property %q is already declared as accessor", name)
	}
	co.props[name] = propService
	key := c.isolateKeyLocked(co, name)
	if old := co.store[key]; old != nil {
		co.mu.Unlock()
		return nil, fmt.Errorf("service %q has been registered at <%s>", name, old.fiber.Name())
	}
	im := &impl{name: name, key: key, fiber: c.fiber, value: value, check: chk}
	co.store[key] = im
	co.bumpGenerationLocked()
	co.mu.Unlock()

	dispose, err := c.registerCleanup(fmt.Sprintf("ctx.provide(%q)", name), func() {
		co.mu.Lock()
		if co.store[key] == im {
			delete(co.store, key)
			co.bumpGenerationLocked()
		}
		co.mu.Unlock()
		co.notifyDependents(c, name)
		c.emitService(name, nil)
	})
	if err != nil {
		co.mu.Lock()
		delete(co.store, key)
		co.bumpGenerationLocked()
		co.mu.Unlock()
		return nil, err
	}

	co.notifyDependents(c, name)
	c.emitService(name, value)
	return dispose, nil
}

// emitService fires EventService scoped to the realm of name, mirroring the
// filtered emission in ReflectService.notify upstream.
func (c *Context) emitService(name string, value any) {
	c.WithFilter(c.RealmFilter(name)).Emit(EventService, name, value)
}

// Get returns the service published under name in this context's realm. The
// second return value is false when the service is missing, its provider is
// not active or its check function rejects it.
func (c *Context) Get(name string) (any, bool) {
	co := c.core
	co.mu.Lock()
	key := c.isolateKeyLocked(co, name)
	im := co.store[key]
	if im == nil {
		co.mu.Unlock()
		return nil, false
	}
	state := im.fiber.state
	value := im.value
	check := im.check
	co.mu.Unlock()

	if state != StateActive {
		return nil, false
	}
	if check != nil && !check() {
		return nil, false
	}
	return value, true
}

// Has reports whether name is declared as a service in this context tree,
// regardless of its current availability.
func (c *Context) Has(name string) bool {
	co := c.core
	co.mu.Lock()
	defer co.mu.Unlock()
	_, ok := co.props[name]
	return ok
}

// Get is the statically typed variant of Context.Get. It returns an error
// when the service is unavailable or has an unexpected type, with messages
// mirroring the TypeScript runtime.
func Get[T any](c *Context, name string) (T, error) {
	var zero T
	value, ok := c.Get(name)
	if !ok {
		if !c.Has(name) {
			return zero, fmt.Errorf("cannot get property %q without inject", name)
		}
		return zero, fmt.Errorf("cannot get required service %q in inactive context", name)
	}
	typed, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("service %q has type %T, expected %T", name, value, zero)
	}
	return typed, nil
}

// MustGet is Get that panics on failure, for wiring code where a missing
// service is a programming error.
func MustGet[T any](c *Context, name string) T {
	value, err := Get[T](c, name)
	if err != nil {
		panic(err)
	}
	return value
}
