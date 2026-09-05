package cordis

// The typed Inject helpers are the statically typed sugar over
// Context.Inject: the dependency list derives from the function signature,
// so deps and fn cannot drift apart, and the resolved services arrive
// typed. The fibers reload whenever a dependency set becomes complete
// again after losing a dependency, exactly like Context.Inject.

// Inject1 runs fn once the service A is available and active.
func Inject1[A any](ctx *Context, fn func(ctx *Context, a A) error) (*Fiber, error) {
	return ctx.Inject([]string{ServiceName[A]()}, func(ctx *Context) error {
		a, err := Get[A](ctx)
		if err != nil {
			return err
		}
		return fn(ctx, a)
	})
}

// Inject2 runs fn once the services A and B are available and active.
func Inject2[A any, B any](ctx *Context, fn func(ctx *Context, a A, b B) error) (*Fiber, error) {
	return ctx.Inject([]string{ServiceName[A](), ServiceName[B]()}, func(ctx *Context) error {
		a, err := Get[A](ctx)
		if err != nil {
			return err
		}
		b, err := Get[B](ctx)
		if err != nil {
			return err
		}
		return fn(ctx, a, b)
	})
}

// Inject3 runs fn once the services A, B and C are available and active.
func Inject3[A any, B any, C any](ctx *Context, fn func(ctx *Context, a A, b B, c C) error) (*Fiber, error) {
	return ctx.Inject([]string{ServiceName[A](), ServiceName[B](), ServiceName[C]()}, func(ctx *Context) error {
		a, err := Get[A](ctx)
		if err != nil {
			return err
		}
		b, err := Get[B](ctx)
		if err != nil {
			return err
		}
		c, err := Get[C](ctx)
		if err != nil {
			return err
		}
		return fn(ctx, a, b, c)
	})
}
