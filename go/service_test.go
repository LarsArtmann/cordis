package cordis

import (
	"reflect"
	"testing"
)

func TestProvideGet(t *testing.T) {
	ctx := New()
	type Counter struct{ Value int }
	counter := &Counter{Value: 1}

	if _, ok := ctx.Get("counter"); ok {
		t.Fatal("expected missing service")
	}
	if _, err := Get[*Counter](ctx, "counter"); err == nil {
		t.Fatal("expected typed get to fail for missing service")
	}

	if _, err := ctx.Provide("counter", counter); err != nil {
		t.Fatal(err)
	}
	got := MustGet[*Counter](ctx, "counter")
	if got != counter {
		t.Fatal("expected the provided instance")
	}

	if _, err := ctx.Provide("counter", counter); err == nil {
		t.Fatal("expected duplicate provide to fail")
	}
}

func TestProvideUnprovideCycle(t *testing.T) {
	ctx := New()
	dispose, err := ctx.Provide("foo", 42)
	if err != nil {
		t.Fatal(err)
	}
	if !ctx.Has("foo") {
		t.Fatal("expected foo declared")
	}
	dispose()
	if _, ok := ctx.Get("foo"); ok {
		t.Fatal("expected foo withdrawn")
	}
	// Providing again after withdrawal must succeed.
	if _, err := ctx.Provide("foo", 43); err != nil {
		t.Fatal(err)
	}
	if value, _ := ctx.Get("foo"); value != 43 {
		t.Fatalf("expected 43, got %v", value)
	}
}

func TestInjectWaitsForService(t *testing.T) {
	ctx := New()
	calls := 0
	fiber := ctx.Inject([]string{"foo"}, func(ctx *Context) error {
		calls++
		return nil
	})
	if fiber == nil {
		t.Fatal("expected fiber")
	}
	if state := fiber.State(); state != StatePending {
		t.Fatalf("expected PENDING, got %s", state)
	}
	if calls != 0 {
		t.Fatal("callback must not fire before the dependency exists")
	}
	if _, err := ctx.Provide("foo", 1); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected callback after provide, got %d calls", calls)
	}
	if state := fiber.State(); state != StateActive {
		t.Fatalf("expected ACTIVE, got %s", state)
	}
}

func TestInjectUnloadsWithDependency(t *testing.T) {
	ctx := New()
	var seq []string
	fiber := ctx.Inject([]string{"foo"}, func(ctx *Context) error {
		seq = append(seq, "apply")
		_, err := ctx.Effect(func(ctx *Context) error {
			ctx.registerTestCleanup(func() { seq = append(seq, "cleanup") })
			return nil
		})
		return err
	})
	dispose, err := ctx.Provide("foo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seq, []string{"apply"}) {
		t.Fatalf("expected [apply], got %v", seq)
	}

	dispose()
	if state := fiber.State(); state != StatePending {
		t.Fatalf("expected PENDING after dependency loss, got %s", state)
	}
	if !reflect.DeepEqual(seq, []string{"apply", "cleanup"}) {
		t.Fatalf("expected effects rolled back, got %v", seq)
	}

	if _, err := ctx.Provide("foo", 2); err != nil {
		t.Fatal(err)
	}
	if state := fiber.State(); state != StateActive {
		t.Fatalf("expected ACTIVE after dependency return, got %s", state)
	}
	if !reflect.DeepEqual(seq, []string{"apply", "cleanup", "apply"}) {
		t.Fatalf("expected reload in place, got %v", seq)
	}
}

func TestMultipleInjectsResolveOrderIndependently(t *testing.T) {
	ctx := New()
	inits := map[string]int{}
	foo := NewPlugin("foo", func(ctx *Context, _ struct{}) error {
		inits["foo"]++
		return nil
	}).Inject("qux")
	bar := NewPlugin("bar", func(ctx *Context, _ struct{}) error {
		inits["bar"]++
		return nil
	}).Inject("foo", "qux")
	qux := NewPlugin("qux", func(ctx *Context, _ struct{}) error {
		inits["qux"]++
		_, err := ctx.Provide("qux", true)
		return err
	})

	// foo provides its service only when active, like a Service class.
	fooSvc := NewPlugin("foo-service", func(ctx *Context, _ struct{}) error {
		_, err := ctx.Provide("foo", true)
		return err
	}).Inject("qux")

	for _, p := range []struct {
		handle func() error
	}{
		{func() error { _, err := Start(ctx, foo, struct{}{}); return err }},
		{func() error { _, err := Start(ctx, bar, struct{}{}); return err }},
		{func() error { _, err := Start(ctx, fooSvc, struct{}{}); return err }},
		{func() error { _, err := Start(ctx, qux, struct{}{}); return err }},
	} {
		if err := p.handle(); err != nil {
			t.Fatal(err)
		}
	}

	if inits["foo"] != 1 || inits["bar"] != 1 || inits["qux"] != 1 {
		t.Fatalf("expected each plugin initialized once, got %v", inits)
	}
}

func TestUpdateCoordination(t *testing.T) {
	ctx := New()
	type pair struct {
		value int
		mode  string
	}
	var calls []pair

	provider := NewPlugin("provider", func(ctx *Context, value int) error {
		_, err := ctx.Provide("value", value)
		return err
	})
	consumer := NewPlugin("consumer", func(ctx *Context, mode string) error {
		calls = append(calls, pair{MustGet[int](ctx, "value"), mode})
		return nil
	}).Inject("value")

	pf, err := Start(ctx, provider, 1)
	if err != nil {
		t.Fatal(err)
	}
	cf, err := Start(ctx, consumer, "old")
	if err != nil {
		t.Fatal(err)
	}

	// Both updates inside one batch must produce a single consistent
	// consumer restart, never a torn intermediate state.
	ctx.Batch(func(ctx *Context) {
		if err := pf.Update(2); err != nil {
			t.Fatal(err)
		}
		if err := cf.Update("new"); err != nil {
			t.Fatal(err)
		}
	})

	want := []pair{{1, "old"}, {2, "new"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("expected %v, got %v", want, calls)
	}
	if state := cf.State(); state != StateActive {
		t.Fatalf("expected ACTIVE, got %s", state)
	}
}

func TestServiceCheck(t *testing.T) {
	ctx := New()
	ready := false
	fiber := ctx.Inject([]string{"foo"}, func(ctx *Context) error { return nil })
	if _, err := ctx.Provide("foo", 1, func() bool { return ready }); err != nil {
		t.Fatal(err)
	}
	if state := fiber.State(); state != StatePending {
		t.Fatalf("check must hide unready services, got %s", state)
	}
	ready = true
	if _, err := ctx.Provide("bar", 1); err != nil {
		t.Fatal(err) // any store change re-evaluates dependents
	}
	// check changes alone do not notify; emulate readiness signaling.
	if state := fiber.State(); state != StatePending {
		t.Fatalf("expected PENDING until re-provided, got %s", state)
	}
}
