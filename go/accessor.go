package cordis

import "fmt"

// Member is the write-back handle of an Accessor or Mixin. Set forwards a
// new value into the source service through the accessor's set function and
// restarts the accessor so the derived service re-derives. A read-only
// member's Set returns ErrReadOnlyAccessor.
type Member[V any] struct {
	set      func(V) error
	accessor *Fiber
}

// Fiber returns the accessor fiber that derives and publishes the service.
func (m *Member[V]) Fiber() *Fiber {
	if m == nil {
		return nil
	}
	return m.accessor
}

// Set writes v back into the source service and refreshes the derived
// service.
func (m *Member[V]) Set(v V) error {
	if m == nil || m.set == nil {
		return ErrReadOnlyAccessor
	}
	if err := m.set(v); err != nil {
		return err
	}
	return m.accessor.Restart()
}

// ErrReadOnlyAccessor is returned by Member.Set on an accessor declared
// without a set function.
var ErrReadOnlyAccessor = fmt.Errorf("cordis: accessor is read-only")

// Accessor publishes a derived service under name: the value of type V is
// projected from the service S through get, and the derived service follows
// the source's lifecycle — it appears when S becomes active, disappears when
// S unloads and re-derives when S restarts. This is the Go counterpart of
// upstream's property accessors: consumers read the projection as a plain
// service instead of injecting the source.
//
// The optional set function writes an updated V back into the source
// service (typically by mutating it in place through a pointer). When set
// is provided, the returned Member can write values back; Set also restarts
// the accessor so the published projection refreshes.
//
//	fiber, port := Accessor[*Config](ctx, "port",
//	    func(ctx *Context, cfg *Config) (int, error) { return cfg.Port, nil },
//	    func(ctx *Context, cfg *Config, v int) error { cfg.Port = v; return nil },
//	)
//	port.Set(9090)
func Accessor[S any, V any](ctx *Context, name string, get func(*Context, S) (V, error), set ...func(*Context, S, V) error) (*Fiber, *Member[V], error) {
	sourceName := ServiceName[S]()
	fiber, err := ctx.Inject([]string{sourceName}, func(actx *Context) error {
		source, err := GetNamed[S](actx, sourceName)
		if err != nil {
			return err
		}
		value, err := get(actx, source)
		if err != nil {
			return err
		}
		_, err = actx.Provide(name, any(value))
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	member := &Member[V]{accessor: fiber}
	if len(set) > 0 {
		write := set[0]
		member.set = func(v V) error {
			source, err := GetNamed[S](ctx, sourceName)
			if err != nil {
				return err
			}
			return write(ctx, source, v)
		}
	}
	return fiber, member, nil
}

// Mixin is the member-shaped sugar over Accessor for the common case of
// exposing one member of a service as its own service, mirroring the
// upstream mixin helper: reads project the member through get, and the
// optional set writes it back into the source in place.
//
//	_, level := Mixin[*Logger](ctx, "level",
//	    func(l *Logger) string { return l.Level },
//	    func(l *Logger, v string) { l.Level = v },
//	)
//	level.Set("debug")
func Mixin[S any, V any](ctx *Context, name string, get func(S) V, set ...func(S, V)) (*Fiber, *Member[V], error) {
	project := func(_ *Context, source S) (V, error) { return get(source), nil }
	var write []func(*Context, S, V) error
	if len(set) > 0 {
		write = append(write, func(_ *Context, source S, v V) error {
			set[0](source, v)
			return nil
		})
	}
	return Accessor[S](ctx, name, project, write...)
}
