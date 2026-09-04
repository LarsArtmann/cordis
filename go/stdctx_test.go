package cordis

import (
	"context"
	"testing"
	"time"
)

func TestStdContextCancelledOnDependencyWithdrawal(t *testing.T) {
	ctx := New()
	workerDone := make(chan struct{})
	plugin := NewPlugin("worker", func(ctx *Context, _ struct{}) error {
		stdCtx := ctx.Fiber().StdContext()
		if err := stdCtx.Err(); err != nil {
			t.Fatalf("fresh activation must have a live context, got %v", err)
		}
		go func() {
			<-stdCtx.Done()
			close(workerDone)
		}()
		return nil
	}).Inject("dep")

	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if state := fiber.State(); state != StatePending {
		t.Fatalf("expected PENDING, got %s", state)
	}

	dispose, err := ctx.Provide("dep", 1)
	if err != nil {
		t.Fatal(err)
	}
	if state := fiber.State(); state != StateActive {
		t.Fatalf("expected ACTIVE, got %s", state)
	}

	firstDone := fiber.Done()
	dispose()
	select {
	case <-firstDone:
	default:
		t.Fatal("Done must close when the fiber unloads")
	}
	if err := fiber.StdContext().Err(); err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	select {
	case <-workerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("worker goroutine did not observe cancellation")
	}
}

func TestStdContextRenewedOnReload(t *testing.T) {
	ctx := New()
	var generations []context.Context
	plugin := NewPlugin("worker", func(ctx *Context, _ struct{}) error {
		generations = append(generations, ctx.Fiber().StdContext())
		return nil
	}).Inject("dep")

	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	dispose, err := ctx.Provide("dep", 1)
	if err != nil {
		t.Fatal(err)
	}
	dispose()
	if _, err := ctx.Provide("dep", 2); err != nil {
		t.Fatal(err)
	}

	if len(generations) != 2 {
		t.Fatalf("expected two activations, got %d", len(generations))
	}
	if err := generations[0].Err(); err == nil {
		t.Fatal("first generation must be cancelled")
	}
	if err := generations[1].Err(); err != nil {
		t.Fatalf("second generation must be live, got %v", err)
	}
	if fiber.Done() == generations[0].Done() {
		t.Fatal("Done must rotate with every reload")
	}
}

func TestStdContextCancelledOnDisposeAndRestart(t *testing.T) {
	ctx := New()
	plugin := NewPlugin("worker", func(ctx *Context, _ struct{}) error { return nil })
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}

	done := fiber.Done()
	if err := fiber.Restart(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	default:
		t.Fatal("Done must close on restart")
	}

	done = fiber.Done()
	fiber.Dispose()
	select {
	case <-done:
	default:
		t.Fatal("Done must close on dispose")
	}
	if err := fiber.StdContext().Err(); err != context.Canceled {
		t.Fatalf("expected context.Canceled after dispose, got %v", err)
	}
}

func TestFiberDoneSelectCoordination(t *testing.T) {
	ctx := New()
	plugin := NewPlugin("worker", func(ctx *Context, _ struct{}) error { return nil })
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}

	exited := make(chan struct{})
	go func() {
		select {
		case <-fiber.Done():
			close(exited)
		case <-time.After(5 * time.Second):
			t.Error("worker goroutine timed out waiting for Done")
		}
	}()

	fiber.Dispose()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("goroutine did not observe Done")
	}
}

func TestRootStdContextRotatesOnRestart(t *testing.T) {
	ctx := New()
	first := ctx.Fiber().StdContext()
	if err := ctx.Fiber().Restart(); err != nil {
		t.Fatal(err)
	}
	if err := first.Err(); err == nil {
		t.Fatal("root context must cancel on restart")
	}
	if err := ctx.Fiber().StdContext().Err(); err != nil {
		t.Fatalf("root must receive a fresh context, got %v", err)
	}
}
