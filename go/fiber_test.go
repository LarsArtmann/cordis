package cordis

import (
	"testing"
	"time"
)

func TestFiberStateLifecycle(t *testing.T) {
	ctx := New()
	var states []FiberState
	mustOn(t, ctx, EventStatus, func(args ...any) any {
		f := args[0].(*Fiber)
		if f.Name() == "p" {
			states = append(states, f.State())
		}
		return nil
	})

	plugin := NewPlugin("p", func(ctx *Context, _ struct{}) error { return nil }).Inject("foo")
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if fiber.State() != StatePending {
		t.Fatalf("expected PENDING, got %s", fiber.State())
	}

	dispose, err := ctx.Provide("foo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if fiber.State() != StateActive {
		t.Fatalf("expected ACTIVE, got %s", fiber.State())
	}

	dispose()
	if fiber.State() != StatePending {
		t.Fatalf("expected PENDING, got %s", fiber.State())
	}

	fiber.Dispose()
	if fiber.State() != StateDisposed {
		t.Fatalf("expected DISPOSED, got %s", fiber.State())
	}
}

func TestFiberReloadsInPlace(t *testing.T) {
	ctx := New()
	applies := 0
	plugin := NewPlugin("p", func(ctx *Context, _ struct{}) error {
		applies++
		return nil
	}).Inject("foo")
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}

	dispose1, err := ctx.Provide("foo", 1)
	if err != nil {
		t.Fatal(err)
	}
	dispose1()
	dispose2, err := ctx.Provide("foo", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer dispose2()

	if applies != 2 {
		t.Fatalf("expected reload in place, got %d applies", applies)
	}
	if fiber.State() != StateActive {
		t.Fatalf("expected ACTIVE, got %s", fiber.State())
	}
	if uid := fiber.UID(); uid != 1 {
		t.Fatalf("the same fiber instance must reload, got uid %d", uid)
	}
}

func TestFiberAwait(t *testing.T) {
	ctx := New()
	plugin := NewPlugin("p", func(ctx *Context, _ struct{}) error { return nil })
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- fiber.Await() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Await must resolve promptly for settled fibers")
	}
}

func TestFiberNames(t *testing.T) {
	ctx := New()
	if ctx.String() != "Context <root>" {
		t.Fatalf("expected root context, got %s", ctx.String())
	}
	named := NewPlugin("foo", func(ctx *Context, _ struct{}) error { return nil })
	fiber, err := Start(ctx, named, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if fiber.Context().String() != "Context <foo>" {
		t.Fatalf("expected named context, got %s", fiber.Context().String())
	}
}

func TestConcurrentAccess(t *testing.T) {
	ctx := New()
	plugin := NewPlugin("p", func(ctx *Context, _ struct{}) error {
		_, err := ctx.On("tick", func(...any) any { return nil })
		return err
	})
	if _, err := Start(ctx, plugin, struct{}{}); err != nil {
		t.Fatal(err)
	}

	const workers = 8
	const iterations = 100
	done := make(chan struct{}, workers)
	for w := 0; w < workers; w++ {
		go func(w int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < iterations; i++ {
				switch w % 4 {
				case 0:
					ctx.Emit("tick")
				case 1:
					d, err := ctx.On("tick", func(...any) any { return nil })
					if err == nil {
						d()
					}
				case 2:
					_, _ = ctx.Get("missing")
				case 3:
					_ = ctx.Registry().Size()
				}
			}
		}(w)
	}
	for w := 0; w < workers; w++ {
		<-done
	}
}
