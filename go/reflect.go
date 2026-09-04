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

	store := func() (Disposer, error) {
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

	// The EventSet interception can veto or observe the registration; with
	// no interceptor registered this is a direct store.
	co.mu.Lock()
	n := len(co.hooks[EventSet])
	co.mu.Unlock()
	if n == 0 {
		return store()
	}
	result := c.Waterfall(EventSet, name, value, func(...any) any {
		dispose, err := store()
		if err != nil {
			return err
		}
		return dispose
	})
	switch r := result.(type) {
	case error:
		return nil, r
	case Disposer:
		return r, nil
	case nil:
		return nil, fmt.Errorf("cordis: internal/set interception returned no disposer")
	default:
		return nil, fmt.Errorf("cordis: internal/set interception returned %T", result)
	}
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
	value, ok := c.get(name)
	if ok {
		return value, true
	}
	return c.interceptGet(name)
}

func (c *Context) get(name string) (any, bool) {
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
