package cordis

import (
	"strings"
	"testing"
)

func TestRootFiberRestartRollsBackEffects(t *testing.T) {
	ctx := New()
	root := ctx.Fiber()

	cleanups := 0
	if _, err := ctx.Effect(func(ctx *Context) error {
		ctx.registerTestCleanup(func() { cleanups++ })
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	strDispose, err := ctx.On("internal/test", func(args ...any) any { return nil })
	if err != nil {
		t.Fatal(err)
	}
	_ = strDispose

	if err := root.Restart(); err != nil {
		t.Fatal(err)
	}

	if cleanups != 1 {
		t.Fatalf("expected root rollback to run cleanups, got %d", cleanups)
	}
	if state := root.State(); state != StateActive {
		t.Fatalf("root must stay active, got %s", state)
	}
	if root.UID() != 0 {
		t.Fatalf("root must keep uid 0, got %d", root.UID())
	}

	// The rolled back listener no longer fires.
	received := 0
	if _, err := On(ctx, func(e typedTestEvent) { received++ }); err != nil {
		t.Fatal(err)
	}
	Emit(ctx, typedTestEvent{})
	if received != 1 {
		t.Fatal("root must accept new registrations after restart")
	}
}

func TestRootFiberRestartDisposesChildPlugins(t *testing.T) {
	ctx := New()
	cleanups := 0
	plugin := NewPlugin("child", func(ctx *Context, _ struct{}) error {
		_, err := ctx.Effect(func(ctx *Context) error {
			ctx.registerTestCleanup(func() { cleanups++ })
			return nil
		})
		return err
	})
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if state := fiber.State(); state != StateActive {
		t.Fatalf("expected ACTIVE, got %s", state)
	}

	if err := ctx.Fiber().Restart(); err != nil {
		t.Fatal(err)
	}
	if cleanups != 1 {
		t.Fatalf("expected child cleanup on root restart, got %d", cleanups)
	}
	if state := fiber.State(); state != StateDisposed {
		t.Fatalf("expected child fiber DISPOSED, got %s", state)
	}
	if size := ctx.Registry().Size(); size != 0 {
		t.Fatalf("expected empty registry after root restart, got %d", size)
	}
}

func TestRootFiberDisposeRestarts(t *testing.T) {
	ctx := New()
	cleanups := 0
	if _, err := ctx.Effect(func(ctx *Context) error {
		ctx.registerTestCleanup(func() { cleanups++ })
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	ctx.Fiber().Dispose()

	if cleanups != 1 {
		t.Fatalf("expected root disposal to roll back effects, got %d", cleanups)
	}
	if state := ctx.Fiber().State(); state != StateActive {
		t.Fatalf("root disposal must restart the root, got %s", state)
	}
}

func TestRootFiberUpdateRejected(t *testing.T) {
	ctx := New()
	err := ctx.Fiber().Update(42)
	if err == nil {
		t.Fatal("expected update on the root fiber to fail")
	}
	if !strings.Contains(err.Error(), "root") {
		t.Fatalf("expected a root specific message, got %v", err)
	}
}

func TestRootFiberProvideAndWithdraw(t *testing.T) {
	ctx := New()
	dispose, err := ctx.Provide("config", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ctx.Get("config"); !ok {
		t.Fatal("expected service provided on root")
	}

	activations := 0
	consumer := NewPlugin("consumer", func(ctx *Context, _ struct{}) error {
		activations++
		return nil
	}).Inject("config")
	fiber, err := Start(ctx, consumer, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if state := fiber.State(); state != StateActive {
		t.Fatalf("expected ACTIVE, got %s", state)
	}

	dispose()
	if state := fiber.State(); state != StatePending {
		t.Fatalf("expected PENDING after withdrawal, got %s", state)
	}
}

func TestRootFiberRestartIsObservableThroughStatus(t *testing.T) {
	ctx := New()
	states := make(chan FiberState, 8)
	if _, err := ctx.On(EventStatus, func(args ...any) any {
		if f, ok := args[0].(*Fiber); ok && f.Name() == "root" {
			states <- f.State()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := ctx.Fiber().Restart(); err != nil {
		t.Fatal(err)
	}
	// Root restart is a rollback, not a state transition: the root stays
	// ACTIVE and emits no status changes for itself.
	if len(states) != 0 {
		t.Fatalf("root restart must not emit status events, got %d", len(states))
	}
}
