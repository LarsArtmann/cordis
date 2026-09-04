package cordis

import (
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOnEmitDispose(t *testing.T) {
	ctx := New()
	calls := 0
	dispose, err := ctx.On("test", func(args ...any) any {
		calls++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx.Emit("test")
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	dispose()
	ctx.Emit("test")
	if calls != 1 {
		t.Fatalf("expected listener removed, got %d calls", calls)
	}
	dispose() // idempotent
}

func TestOnce(t *testing.T) {
	ctx := New()
	calls := 0
	dispose, err := ctx.Once("test", func(args ...any) any {
		calls++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx.Emit("test")
	ctx.Emit("test")
	if calls != 1 {
		t.Fatalf("expected exactly 1 call, got %d", calls)
	}
	dispose() // no-op after firing
}

func TestEmitOrder(t *testing.T) {
	ctx := New()
	var seq []int
	must := func(_ Disposer, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(ctx.On("test", func(...any) any { seq = append(seq, 1); return nil }))
	must(ctx.On("test", func(...any) any { seq = append(seq, 2); return nil }))
	must(ctx.On("test", func(...any) any { seq = append(seq, 0); return nil }, Prepend()))
	ctx.Emit("test")
	if !reflect.DeepEqual(seq, []int{0, 1, 2}) {
		t.Fatalf("expected [0 1 2], got %v", seq)
	}
}

func TestParallel(t *testing.T) {
	ctx := New()
	var settled1, settled2 atomic.Bool
	mustOn(t, ctx, "test", func(...any) any {
		settled1.Store(true)
		return errors.New("test")
	})
	mustOn(t, ctx, "test", func(...any) any {
		settled2.Store(true)
		return errors.New("async")
	})
	err := ctx.Parallel("test")
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	if !settled1.Load() || !settled2.Load() {
		t.Fatal("a failing listener must not short-circuit the others")
	}
	for _, want := range []string{"test", "async"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected joined error to contain %q, got %q", want, err.Error())
		}
	}
}

func TestSerialAndBail(t *testing.T) {
	ctx := New()
	calls := 0
	mustOn(t, ctx, "test", func(...any) any { calls++; return nil })
	mustOn(t, ctx, "test", func(...any) any { calls++; return "bailed" })
	mustOn(t, ctx, "test", func(...any) any { calls++; return "unreachable" })
	if result := ctx.Serial("test"); result != "bailed" {
		t.Fatalf("expected serial to bail with %q, got %v", "bailed", result)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls before bail, got %d", calls)
	}
	calls = 0
	if result := ctx.Bail("test"); result != "bailed" {
		t.Fatalf("expected bail result, got %v", result)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls before bail, got %d", calls)
	}
}

func TestWaterfall(t *testing.T) {
	ctx := New()
	mustOn(t, ctx, "test", func(args ...any) any {
		v := args[0].(int)
		next := args[1].(func(...any) any)
		return v + next(v).(int)
	})
	mustOn(t, ctx, "test", func(args ...any) any {
		v := args[0].(int)
		next := args[1].(func(...any) any)
		return v + next(v).(int)
	})
	result := ctx.Waterfall("test", 1, func(args ...any) any { return 2 })
	if result != 4 {
		t.Fatalf("expected 4, got %v", result)
	}
}

func TestWaterfallShortCircuit(t *testing.T) {
	ctx := New()
	calls := 0
	mustOn(t, ctx, "test", func(args ...any) any {
		calls++
		v := args[0].(int)
		next := args[1].(func(...any) any)
		return v + next(v).(int)
	})
	mustOn(t, ctx, "test", func(args ...any) any {
		calls++
		return args[0].(int) + 1 // does not call next
	})
	mustOn(t, ctx, "test", func(args ...any) any {
		calls++
		t.Error("listener after short-circuit must not run")
		return nil
	})
	result := ctx.Waterfall("test", 1, func(args ...any) any { return 2 })
	if result != 3 {
		t.Fatalf("expected 3, got %v", result)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestEventFilter(t *testing.T) {
	ctx := New()
	calls := 0
	listenerCtx := ctx.Intercept("flag", true)
	mustOn(t, listenerCtx, "test", func(...any) any { calls++; return nil })

	// No filter on the emitter: every listener receives the event.
	ctx.Emit("test")
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}

	// Emitter filter rejects the listener.
	emitter := ctx.WithFilter(func(listener *Context) bool {
		v, ok := listener.Intercepted("flag")
		return ok && v.(bool)
	})
	emitter.Emit("test")
	if calls != 2 {
		t.Fatalf("expected accepted listener to receive event, got %d calls", calls)
	}

	rejecting := ctx.WithFilter(func(listener *Context) bool { return false })
	rejecting.Emit("test")
	if calls != 2 {
		t.Fatalf("expected filter to suppress listener, got %d calls", calls)
	}

	// Global listeners bypass filters.
	mustOn(t, ctx, "global-test", func(...any) any { calls++; return nil }, Global())
	rejecting.Emit("global-test")
	if calls != 3 {
		t.Fatalf("expected global listener to bypass filter, got %d calls", calls)
	}
}

func TestEmitPanicPropagates(t *testing.T) {
	ctx := New()
	mustOn(t, ctx, "test", func(...any) any { panic("boom") })
	defer func() {
		if r := recover(); r != "boom" {
			t.Fatalf("expected panic to propagate, got %v", r)
		}
	}()
	ctx.Emit("test")
}

func mustOn(t *testing.T, ctx *Context, name string, listener Listener, opts ...EventOption) {
	t.Helper()
	if _, err := ctx.On(name, listener, opts...); err != nil {
		t.Fatal(err)
	}
}
