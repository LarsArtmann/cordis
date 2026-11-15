package cordis

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
)

// Context is a scope in the context tree. It carries the services, event
// listeners and effects registered within it, and it is the single handle
// through which plugins interact with the framework.
//
// Contexts form a parent chain: New creates the root, Extend creates a plain
// child, Isolate creates a child with its own service realm and Intercept
// creates a child with overridden service configuration. A Context is safe
// for concurrent use; all framework operations are serialized internally.
type Context struct {
	core   *core
	parent *Context
	fiber  *Fiber

	// isolate holds this scope's realm overrides, keyed by service name.
	// A nil map inherits the parent chain unchanged.
	isolate map[string]isolateKey

	// injectLayer holds the per-service configuration defaults a plugin
	// declared through InjectConfig. They sit under every scope intercept
	// (upstream #206); only the fiber's own context carries a layer.
	injectLayer map[string]any

	// intercept holds per-service configuration overrides for this scope.
	intercept map[string]any

	// filter, when set, restricts which listeners receive events emitted
	// through this context. It is the Go counterpart of Context.filter.
	filter func(listener *Context) bool

	// collect, when set, is the effect bag currently collecting registrations.
	// It is only non-nil on the context passed to an effect body, which makes
	// effect attribution goroutine safe.
	collect *disposeBag
}

// New creates a root context with its own registry, event bus, service store
// and logger service. The root fiber is always active.
func New() *Context {
	c := newCore()
	ctx := &Context{core: c}
	root := newRootFiber(ctx)
	ctx.fiber = root

	return ctx
}

// Root returns the root context of this tree.
func (c *Context) Root() *Context {
	for c.parent != nil {
		c = c.parent
	}

	return c
}

// Fiber returns the fiber that owns this context. Effects, listeners and
// services registered through this context live and die with this fiber.
func (c *Context) Fiber() *Fiber { return c.fiber }

// Extend creates a plain child scope sharing the parent's fiber, realms and
// intercepts. It is the Go counterpart of ctx.extend().
func (c *Context) Extend() *Context {
	return &Context{core: c.core, parent: c, fiber: c.fiber}
}

// Isolate creates a child scope in which name resolves to a fresh service
// realm. Services provided inside the child are invisible to the parent and
// services provided outside are invisible inside, until the same name is
// provided again within the realm.
//
// Passing the same label to multiple Isolate calls lets them share one
// realm, mirroring ctx.isolate(name, label) upstream: the label itself is
// the realm identity, exactly like the realm symbols upstream. Labels must
// be comparable; the loader passes config-defined labels, so an
// uncomparable label is reported as an error rather than assumed to be a
// programming mistake. When omitted, a fresh realm is created.
func (c *Context) Isolate(name string, label ...any) (*Context, error) {
	child := c.Extend()

	key, err := c.realmKey(name, label)
	if err != nil {
		return nil, err
	}

	child.isolate = map[string]isolateKey{name: key}

	return child, nil
}

// realmKey derives the child's key for name. Without a label a brand new key
// is allocated; with a label the label itself names the realm, so isolated
// contexts created with equal labels share it, mirroring the label symbols
// upstream. Labels are stored in their own table keyed by value, so no
// string formatting can ever make two distinct labels collide.
func (c *Context) realmKey(name string, label []any) (isolateKey, error) {
	if len(label) > 0 && label[0] != nil {
		if key, ok := label[0].(isolateKey); ok {
			return key, nil
		}

		lbl := label[0]
		if !reflect.TypeOf(lbl).Comparable() {
			return 0, fmt.Errorf("cordis: isolate label of type %T is not comparable", lbl)
		}

		c.core.mu.Lock()
		defer c.core.mu.Unlock()

		if key, ok := c.core.labels[lbl]; ok {
			return key, nil
		}

		c.core.lastKey++
		c.core.labels[lbl] = c.core.lastKey

		return c.core.lastKey, nil
	}

	c.core.mu.Lock()
	defer c.core.mu.Unlock()

	c.core.lastKey++

	return c.core.lastKey, nil
}

// Cleanup attaches a labeled cleanup function to the current effect scope:
// the enclosing effect while one runs, otherwise the fiber itself. It is
// the exported entry point for plugins (timer, loader, ...) that roll their
// resources back with the scope that acquired them.
func (c *Context) Cleanup(label string, run Cleanup) (Disposer, error) {
	return c.registerCleanup(label, run)
}

// Intercept creates a child scope in which the configuration of the named
// service is overridden, mirroring ctx.intercept(). Intercepts merge from
// the outermost scope to the nearest one, with the inject configs of the
// governing fibers underneath (#206): the logger service honors
// LoggerIntercept values; other services may read their overrides through
// Intercepted.
func (c *Context) Intercept(name string, config any) *Context {
	child := c.Extend()
	child.intercept = map[string]any{name: config}

	return child
}

// Intercepted returns the configuration override for name, merged from
// the outermost scope to the nearest one (upstream #206): nearer scopes
// overlay fields while outer scopes keep providing the rest. The inject
// configs the governing fibers declared through InjectConfig form the
// bottom layer, under every scope intercept. Map and struct values merge
// field-wise; any other value type uses the nearest scope's value.
func (c *Context) Intercepted(name string) (any, bool) {
	var scopeChain []*Context
	for ctx := c; ctx != nil; ctx = ctx.parent {
		scopeChain = append(scopeChain, ctx)
	}

	var layers []any

	for _, s := range slices.Backward(scopeChain) {
		if layer := s.injectLayer; layer != nil {
			if v, ok := layer[name]; ok {
				layers = append(layers, v)
			}
		}
	}

	for _, s := range slices.Backward(scopeChain) {
		if intercept := s.intercept; intercept != nil {
			if v, ok := intercept[name]; ok {
				layers = append(layers, v)
			}
		}
	}

	if len(layers) == 0 {
		return nil, false
	}

	merged := layers[0]
	for _, overlay := range layers[1:] {
		merged = mergeIntercept(merged, overlay)
	}

	return merged, true
}

// mergeIntercept overlays newer onto older. Two maps merge shallowly; two
// values of the same struct type merge field-wise with the overlay's
// non-zero fields winning; anything else replaces.
func mergeIntercept(older, newer any) any {
	if oldMap, ok := older.(map[string]any); ok {
		if newMap, ok := newer.(map[string]any); ok {
			merged := make(map[string]any, len(oldMap)+len(newMap))
			maps.Copy(merged, oldMap)
			maps.Copy(merged, newMap)

			return merged
		}
	}

	oldVal, newVal := reflect.ValueOf(older), reflect.ValueOf(newer)
	if oldVal.IsValid() && newVal.IsValid() && oldVal.Type() == newVal.Type() && oldVal.Kind() == reflect.Struct {
		merged := reflect.New(oldVal.Type()).Elem()
		merged.Set(oldVal)

		for i := range oldVal.NumField() {
			if field := newVal.Field(i); field.IsValid() && !field.IsZero() {
				merged.Field(i).Set(field)
			}
		}

		return merged.Interface()
	}

	return newer
}

// WithFilter creates a child scope whose filter restricts event dispatch:
// events emitted through the child are only delivered to listeners for which
// filter returns true. Listeners registered with Global always receive them.
func (c *Context) WithFilter(filter func(listener *Context) bool) *Context {
	child := c.Extend()
	child.filter = filter

	return child
}

// RealmFilter returns a filter matching listeners whose realm key for name
// equals this context's realm key. It is the building block services use to
// keep their events inside their isolation realm.
func (c *Context) RealmFilter(name string) func(*Context) bool {
	key := c.isolateKey(name)

	return func(listener *Context) bool {
		return listener.isolateKey(name) == key
	}
}

// isolateKey resolves the realm key of name by walking the scope chain,
// falling back to the root realm.
func (c *Context) isolateKey(name string) isolateKey {
	c.core.mu.Lock()
	defer c.core.mu.Unlock()

	return c.isolateKeyLocked(c.core, name)
}

func (c *Context) isolateKeyLocked(co *core, name string) isolateKey {
	for ctx := c; ctx != nil; ctx = ctx.parent {
		if ctx.isolate == nil {
			continue
		}

		if key, ok := ctx.isolate[name]; ok {
			return key
		}
	}

	return co.rootKeyLocked(name)
}

// Batch runs fn as one framework transaction: fiber state transitions
// triggered inside fn are coalesced and settle only after fn returns. Use it
// when updating several plugins at once so dependents observe a single
// consistent snapshot instead of intermediate states.
func (c *Context) Batch(fn func(ctx *Context)) {
	c.core.enter()
	defer c.core.leave()

	fn(c)
}

// String mirrors the inspect output of the TypeScript runtime.
func (c *Context) String() string {
	return fmt.Sprintf("Context <%s>", c.fiber.Name())
}

// withCollect returns the variant of c that attributes new registrations to
// bag. Used internally while an effect body runs.
func (c *Context) withCollect(bag *disposeBag) *Context {
	child := *c
	child.collect = bag

	return &child
}

// registerCleanup attaches a labeled cleanup to the current collection
// target: the enclosing effect when called inside an effect body, otherwise
// the fiber itself. The returned Disposer removes it ahead of time.
func (c *Context) registerCleanup(label string, run Cleanup) (Disposer, error) {
	f := c.fiber
	if err := f.assertActive(); err != nil {
		return nil, err
	}

	bag := c.collect
	if bag == nil {
		bag = f.bag()
	}

	if bag == nil {
		return nil, ErrInactiveEffect
	}

	item := bag.push(label, run)
	// The fiber may have unloaded between bag lookup and push; a cleanup
	// registered on a drained bag would silently leak, so dispose it
	// immediately and report the fiber as inactive instead.
	f.core.mu.Lock()
	stale := f.disposed || (f.state != StateActive && f.state != StateLoading)
	f.core.mu.Unlock()

	if stale {
		item.dispose(bag)

		return nil, ErrInactiveEffect
	}

	return func() { item.dispose(bag) }, nil
}
