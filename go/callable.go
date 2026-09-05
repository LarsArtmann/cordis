package cordis

import (
	"fmt"
	"runtime/debug"
)

// ServiceMeta is embedded in service values that need to know their own
// identity. ProvideService fills in the context the service was provided on
// and the name it is published under; embed it by pointer:
//
//	type Database struct {
//	    *ServiceMeta
//	    Pool *sql.DB
//	}
//
//	db := &Database{ServiceMeta: &ServiceMeta{}}
//	ProvideService(ctx, db)
//	db.Name() // "cordis.Database"
type ServiceMeta struct {
	ctx  *Context
	name string
}

func (m *ServiceMeta) serviceMeta() *ServiceMeta { return m }

// Ctx returns the context the service was provided on.
func (m *ServiceMeta) Ctx() *Context { return m.ctx }

// Name returns the service name the service is published under.
func (m *ServiceMeta) Name() string { return m.name }

// metaCarrier is implemented by ServiceMeta; ProvideService detects it on
// service values.
type metaCarrier interface {
	serviceMeta() *ServiceMeta
}

// ProvideService publishes svc as the service identified by its type and
// fills in an embedded *ServiceMeta, if any, with the providing context and
// the resolved service name. It is the tracker counterpart of upstream's
// service constructor.
func ProvideService[T any](ctx *Context, svc T) (Disposer, error) {
	if carrier, ok := any(svc).(metaCarrier); ok {
		meta := carrier.serviceMeta()
		meta.ctx = ctx
		meta.name = ServiceName[T]()
	}
	return Provide(ctx, svc)
}

// Callable turns fn into a service function bound to ctx, the Go
// counterpart of upstream's callable services: every invocation runs as if
// on the bound context, so the callable can resolve services through it,
// and a panicking call is recovered into an error instead of tearing down
// the caller. Provide the returned function to publish it as a service.
//
//	add := Callable[Sum, int](ctx, "add", func(ctx *Context, req Sum) (int, error) { ... })
//	Provide(ctx, add)
func Callable[Req any, Res any](ctx *Context, name string, fn func(ctx *Context, req Req) (Res, error)) func(Req) (Res, error) {
	return func(req Req) (res Res, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("cordis: callable %q panicked: %v\n%s", name, r, debug.Stack())
			}
		}()
		return fn(ctx, req)
	}
}
