package cordis

import (
	"fmt"
	"reflect"
)

// typeName returns the type identity of T as reported by the reflect
// package. It is the canonical name of typed services and typed events.
func typeName[T any]() string {
	return reflect.TypeOf((*T)(nil)).Elem().String()
}

// ServiceName returns the canonical service name of T. The type-keyed
// service API (Provide, Get, MustGet, TryGet) stores services under this
// name, so lookups are resolved by type identity instead of by hand written
// strings. Pass it to Plugin.Inject and Context.Isolate to declare a
// dependency on, or isolate, a typed service:
//
//	plugin.Inject(cordis.ServiceName[Database]())
//	child := ctx.Isolate(cordis.ServiceName[Database]())
//
// Types with identical reflect strings (for example structurally identical
// anonymous struct types from different packages) share one name; prefer
// named types for services.
func ServiceName[T any]() string {
	return typeName[T]()
}

// EventName returns the canonical event name of E. The typed event API
// (On, Once, Emit) dispatches events under this name. String event names
// remain available for the framework's internal/ namespace.
func EventName[E any]() string {
	return typeName[E]()
}

// Provide publishes value as the service identified by its type T in this
// context's realm. It is the primary service API: the name derives from the
// type, so provider and consumers cannot drift apart on a hand written
// string. The service is bound to the context's fiber exactly like a named
// service and rolls back with it.
//
// An optional check function can hide the service from dependent fibers
// while it is not ready.
func Provide[T any](ctx *Context, value T, check ...func() bool) (Disposer, error) {
	return ctx.Provide(ServiceName[T](), value, check...)
}

// Get returns the service of type T published in ctx's realm. It returns an
// error when the service is missing, its provider is inactive, its check
// function rejects it or the value has an unexpected type.
func Get[T any](ctx *Context) (T, error) {
	return GetNamed[T](ctx, ServiceName[T]())
}

// MustGet is Get that panics on failure, for wiring code where a missing
// service is a programming error.
func MustGet[T any](ctx *Context) T {
	value, err := Get[T](ctx)
	if err != nil {
		panic(err)
	}
	return value
}

// TryGet returns the service of type T when it is currently available. It
// reports false when the service is missing, inactive or has an unexpected
// type, mirroring the two value lookup of named services.
func TryGet[T any](ctx *Context) (T, bool) {
	var zero T
	value, ok := ctx.Get(ServiceName[T]())
	if !ok {
		return zero, false
	}
	typed, ok := value.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

// GetNamed is the statically typed lookup of a named service. Prefer the
// type-keyed Get unless the service name is dynamic (loader and hmr ports)
// or shared across realms by contract.
func GetNamed[T any](c *Context, name string) (T, error) {
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

// MustGetNamed is GetNamed that panics on failure.
func MustGetNamed[T any](c *Context, name string) T {
	value, err := GetNamed[T](c, name)
	if err != nil {
		panic(err)
	}
	return value
}

// On subscribes listener to the event type E. Typed events are the primary
// event API: the event name derives from the type, so emitters and
// listeners cannot drift apart on a hand written string, and the payload
// arrives fully typed. The subscription is bound to the context's fiber and
// rolls back with it; the returned Disposer removes it ahead of time.
func On[E any](ctx *Context, listener func(E), opts ...EventOption) (Disposer, error) {
	want := reflect.TypeOf((*E)(nil)).Elem()
	name := EventName[E]()
	return ctx.On(name, func(args ...any) any {
		if len(args) != 1 {
			panic(fmt.Sprintf("cordis: typed event %q expects 1 argument, got %d", name, len(args)))
		}
		event, ok := args[0].(E)
		if !ok {
			panic(fmt.Sprintf("cordis: typed event %q expects %s, got %T", name, want, args[0]))
		}
		listener(event)
		return nil
	}, opts...)
}

// Once subscribes listener to the event type E and removes it after the
// first delivery.
func Once[E any](ctx *Context, listener func(E), opts ...EventOption) (Disposer, error) {
	want := reflect.TypeOf((*E)(nil)).Elem()
	name := EventName[E]()
	return ctx.Once(name, func(args ...any) any {
		if len(args) != 1 {
			panic(fmt.Sprintf("cordis: typed event %q expects 1 argument, got %d", name, len(args)))
		}
		event, ok := args[0].(E)
		if !ok {
			panic(fmt.Sprintf("cordis: typed event %q expects %s, got %T", name, want, args[0]))
		}
		listener(event)
		return nil
	}, opts...)
}

// Emit delivers event synchronously to every listener registered for its
// type E, in registration order, applying this context's emission filter.
// A panicking listener propagates the panic to the caller.
func Emit[E any](ctx *Context, event E) {
	ctx.Emit(EventName[E](), event)
}
