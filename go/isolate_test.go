package cordis

import "testing"

func TestIsolatedContexts(t *testing.T) {
	ctx := New()
	ctx1 := ctx.Isolate("foo")
	ctx2 := ctx.Isolate("foo")

	calls := 0
	disposed := 0
	watch := func(c *Context) {
		c.Inject([]string{"foo"}, func(ctx *Context) error {
			calls++
			_, err := ctx.Effect(func(ctx *Context) error {
				ctx.registerTestCleanup(func() { disposed++ })
				return nil
			})
			return err
		})
	}
	watch(ctx)
	watch(ctx1)
	watch(ctx2)

	if _, err := ctx.Provide("foo", 100); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("only the root realm plugin may apply, got %d calls", calls)
	}
	if _, ok := ctx1.Get("foo"); ok {
		t.Fatal("isolated context must not see the parent service")
	}

	if _, err := ctx1.Provide("foo", 200); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected ctx1 plugin applied, got %d calls", calls)
	}
	if _, ok := ctx2.Get("foo"); ok {
		t.Fatal("sibling isolation realms must not share services")
	}
	if value, _ := ctx.Get("foo"); value != 100 {
		t.Fatalf("root realm must be unaffected, got %v", value)
	}
	if disposed != 0 {
		t.Fatalf("no disposals expected, got %d", disposed)
	}
}

func TestIsolateSharedLabel(t *testing.T) {
	ctx := New()
	ctx1 := ctx.Isolate("foo", "shared")
	ctx2 := ctx.Isolate("foo", "shared")

	calls := 0
	disposed := 0
	watch := func(c *Context) {
		c.Inject([]string{"foo"}, func(ctx *Context) error {
			calls++
			_, err := ctx.Effect(func(ctx *Context) error {
				ctx.registerTestCleanup(func() { disposed++ })
				return nil
			})
			return err
		})
	}
	watch(ctx)
	watch(ctx1)
	watch(ctx2)

	if _, err := ctx.Provide("foo", 100); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}

	dispose, err := ctx1.Provide("foo", 200)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("shared realm must activate both isolated plugins, got %d calls", calls)
	}
	if value, _ := ctx2.Get("foo"); value != 200 {
		t.Fatalf("expected shared realm visibility, got %v", value)
	}

	dispose()
	if _, ok := ctx1.Get("foo"); ok {
		t.Fatal("expected realm empty after withdrawal")
	}
	if _, ok := ctx2.Get("foo"); ok {
		t.Fatal("expected realm empty after withdrawal")
	}
	if disposed != 2 {
		t.Fatalf("expected both isolated plugins disposed, got %d", disposed)
	}
	if value, _ := ctx.Get("foo"); value != 100 {
		t.Fatal("root realm must be unaffected")
	}
}

func TestIsolatedEvents(t *testing.T) {
	ctx := New()
	isolated := ctx.Isolate("foo")

	rootCalls := 0
	isolatedCalls := 0
	mustOn(t, ctx, "custom-event", func(...any) any { rootCalls++; return nil })
	mustOn(t, isolated, "custom-event", func(...any) any { isolatedCalls++; return nil })

	// A service emitting inside its realm must not reach root listeners.
	emitter := isolated.WithFilter(isolated.RealmFilter("foo"))
	emitter.Emit("custom-event")
	if rootCalls != 0 {
		t.Fatalf("root listener must not fire, got %d", rootCalls)
	}
	if isolatedCalls != 1 {
		t.Fatalf("isolated listener must fire, got %d", isolatedCalls)
	}

	// An unfiltered emission reaches both realms.
	ctx.Emit("custom-event")
	if rootCalls != 1 || isolatedCalls != 2 {
		t.Fatalf("expected both listeners, got root=%d isolated=%d", rootCalls, isolatedCalls)
	}
}
