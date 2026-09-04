package cordis

import (
	"errors"
	"reflect"
	"testing"
)

func TestEffectLabels(t *testing.T) {
	ctx := New()
	cleanupRan := 0
	dispose, err := ctx.Effect(func(ctx *Context) error {
		if _, err := ctx.On("custom-event", func(...any) any { return nil }); err != nil {
			return err
		}
		_, err := ctx.Effect(func(ctx *Context) error {
			ctx.registerTestCleanup(func() { cleanupRan++ })
			return nil
		})
		return err
	}, "test")
	if err != nil {
		t.Fatal(err)
	}

	effects := ctx.Fiber().GetEffects()
	want := []EffectMeta{{
		Label: "test",
		Children: []EffectMeta{
			{Label: `ctx.on("custom-event")`, Children: []EffectMeta{}},
			{Label: "anonymous", Children: []EffectMeta{{Label: "test-cleanup", Children: []EffectMeta{}}}},
		},
	}}
	if !reflect.DeepEqual(effects, want) {
		t.Fatalf("unexpected effect tree:\ngot  %+v\nwant %+v", effects, want)
	}

	dispose()
	if cleanupRan != 1 {
		t.Fatalf("expected cleanup once, got %d", cleanupRan)
	}
	dispose()
	if cleanupRan != 1 {
		t.Fatal("dispose must be idempotent")
	}
}

func TestEffectLIFOOrder(t *testing.T) {
	ctx := New()
	var seq []int
	_, err := ctx.Effect(func(ctx *Context) error {
		ctx.registerTestCleanup(func() { seq = append(seq, 1) })
		if _, err := ctx.Effect(func(ctx *Context) error {
			ctx.registerTestCleanup(func() { seq = append(seq, 2) })
			return nil
		}); err != nil {
			return err
		}
		ctx.registerTestCleanup(func() { seq = append(seq, 3) })
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx.Fiber().restartRoot()
	if !reflect.DeepEqual(seq, []int{3, 2, 1}) {
		t.Fatalf("expected LIFO [3 2 1], got %v", seq)
	}
}

func TestEffectSyncError(t *testing.T) {
	ctx := New()
	boom := errors.New("test")
	seq := 0
	_, err := ctx.Effect(func(ctx *Context) error {
		ctx.registerTestCleanup(func() { seq++ })
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	if seq != 1 {
		t.Fatal("cleanup registered before the failure must still run")
	}
}

func TestEffectPanic(t *testing.T) {
	ctx := New()
	seq := 0
	_, err := ctx.Effect(func(ctx *Context) error {
		ctx.registerTestCleanup(func() { seq++ })
		panic("boom")
	})
	if err == nil || !reflect.DeepEqual(seq, 1) {
		t.Fatalf("expected recovered panic and cleanup, got err=%v seq=%d", err, seq)
	}
}

func TestEffectOnInactiveContext(t *testing.T) {
	ctx := New()
	var inner *Context
	plugin := NewPlugin("p", func(ctx *Context, _ struct{}) error {
		inner = ctx
		return nil
	})
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	fiber.Dispose()

	if _, err := inner.Effect(func(ctx *Context) error { return nil }); !IsInactiveEffect(err) {
		t.Fatalf("expected inactive effect error, got %v", err)
	}
	if _, err := inner.On("x", func(...any) any { return nil }); !IsInactiveEffect(err) {
		t.Fatalf("expected inactive effect error, got %v", err)
	}
	if _, err := Start(inner, plugin, struct{}{}); !IsInactiveEffect(err) {
		t.Fatalf("expected inactive effect error, got %v", err)
	}
}

// registerTestCleanup attaches a raw cleanup to the current collection
// target. Tests use it to assert on disposal order.
func (c *Context) registerTestCleanup(run Cleanup) {
	if _, err := c.registerCleanup("test-cleanup", run); err != nil {
		panic(err)
	}
}
