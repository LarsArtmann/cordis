package cordis

import (
	"reflect"
	"testing"
)

// Parity battery for the upstream rc.13 core fixes. Each test mirrors an
// upstream spec scenario (commit referenced per test) against the Go port;
// a red result means the port drifted and the port gets fixed, not the test.

// #56 (7d0a90d): internal/update hooks must be disposed with the fiber, so
// reloads never accumulate them. Upstream leaked them because the hooks
// lived in a per-fiber list outside the effect scope; the Go port routes
// EventUpdate through the ordinary On machinery, which should make the
// leak impossible. This pins it.
func TestEventUpdateHooksDoNotAccumulateAcrossReloads(t *testing.T) {
	root := New()

	type config struct{ N int }

	var calls []int

	plugin := NewPlugin("p", func(ctx *Context, cfg config) error {
		_, err := ctx.On(EventUpdate, func(args ...any) any {
			if len(args) < 4 {
				t.Fatalf("internal/update listener received %d args, want 4", len(args))
			}

			next, ok := args[3].(func(...any) any)
			if !ok {
				t.Fatalf("arg 3 is %T, want next func", args[3])
			}

			updated, ok := args[1].(config)
			if !ok {
				t.Fatalf("arg 1 is %T, want config", args[1])
			}

			calls = append(calls, updated.N)

			return next()
		})
		return err
	})

	fiber, err := Start(root, plugin, config{N: 0})
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range []int{1, 2, 3} {
		if err := fiber.Update(config{N: n}); err != nil {
			t.Fatal(err)
		}

		if err := fiber.Await(); err != nil {
			t.Fatal(err)
		}
	}

	if !reflect.DeepEqual(calls, []int{1, 2, 3}) {
		t.Fatalf("internal/update hooks accumulated across reloads: %v", calls)
	}
}

// #133 (5dac695): a pending fiber that is disposed must settle to DISPOSED,
// not linger in PENDING.
func TestDisposedPendingFiberSettlesDisposed(t *testing.T) {
	root := New()

	fiber, err := root.Inject([]string{"never-provided"}, func(*Context) error {
		t.Fatal("body must never run")

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if state := fiber.State(); state != StatePending {
		t.Fatalf("expected PENDING before dispose, got %s", state)
	}

	fiber.Dispose()

	if state := fiber.State(); state != StateDisposed {
		t.Fatalf("expected DISPOSED after dispose, got %s", state)
	}
}

// #209 (bc9a344), scenario 1: an isolated child scope must not inherit the
// injected service view across the isolation boundary.
func TestIsolationBoundaryStopsInjectedView(t *testing.T) {
	root := New()

	if _, err := root.Provide("foo", "outer"); err != nil {
		t.Fatal(err)
	}

	var results []any

	_, err := root.Inject([]string{"foo"}, func(ctx *Context) error {
		// A plain child plugin still resolves the outer realm.
		child := NewPlugin("child", func(c *Context, _ struct{}) error {
			value, ok := c.Get("foo")
			if !ok {
				results = append(results, "cannot get property \"foo\" without inject")
			} else {
				results = append(results, value)
			}

			return nil
		})

		if _, err := Start(ctx, child, struct{}{}); err != nil {
			return err
		}

		// An isolated child must not: the fresh realm for "foo" stops the
		// lookup walk at the boundary.
		iso, err := ctx.Isolate("foo")
		if err != nil {
			return err
		}

		isolated := NewPlugin("isolated", func(c *Context, _ struct{}) error {
			value, ok := c.Get("foo")
			if !ok {
				results = append(results, "cannot get property \"foo\" without inject")
			} else {
				results = append(results, value)
			}

			return nil
		})

		if _, err := Start(iso, isolated, struct{}{}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []any{"outer", "cannot get property \"foo\" without inject"}
	if !reflect.DeepEqual(results, want) {
		t.Fatalf("isolation boundary leaked the injected view: %v", results)
	}
}

// #191 (147e4f2): after an isolated provider unloads, providing the same
// service in the same isolated scope again must succeed — no stale impl
// left behind by the isolate move.
func TestIsolateProviderUnloadAllowsReprovide(t *testing.T) {
	root := New()

	iso, err := root.Isolate("svc")
	if err != nil {
		t.Fatal(err)
	}

	provider := NewPlugin("provider", func(ctx *Context, _ struct{}) error {
		_, err := ctx.Provide("svc", "value")
		return err
	})

	first, err := Start(iso, provider, struct{}{})
	if err != nil {
		t.Fatal(err)
	}

	first.Dispose()

	if _, ok := iso.Get("svc"); ok {
		t.Fatal("service still visible after provider disposal")
	}

	second, err := Start(iso, provider, struct{}{})
	if err != nil {
		t.Fatalf("re-provide after isolate provider unload failed: %v", err)
	}

	if value, ok := second.Context().Get("svc"); !ok || value != "value" {
		t.Fatalf("re-provided service missing, got (%v, %v)", value, ok)
	}
}

// #196 (9cad5e8): a provider's earlier effects (the resources backing a
// service) are released only after its dependents have unloaded — the
// dependent's teardown observes a working service value. The Go port
// implements this inside the provide disposer: withdrawal first notifies
// and settles dependents, then the provider's LIFO cleanups continue.
func TestProviderResourcesOutliveDependentTeardown(t *testing.T) {
	root := New()

	var order []string

	resource := new(string)
	*resource = "value"

	provider := NewPlugin("provider", func(ctx *Context, _ struct{}) error {
		if _, err := ctx.registerCleanup("resource", func() {
			order = append(order, "resource-released")
			*resource = ""
		}); err != nil {
			return err
		}

		_, err := ctx.Provide("svc", resource)
		return err
	})

	providerFiber, err := Start(root, provider, struct{}{})
	if err != nil {
		t.Fatal(err)
	}

	dependent, err := root.Inject([]string{"svc"}, func(ctx *Context) error {
		value, ok := ctx.Get("svc")
		if !ok {
			t.Fatal("dependent could not resolve svc")
		}

		captured, ok := value.(*string)
		if !ok {
			t.Fatalf("svc value is %T", value)
		}

		_, err := ctx.registerCleanup("dependent", func() {
			order = append(order, "dependent-teardown")

			if *captured != "value" {
				t.Errorf("service backing resource released before dependent teardown")
			}
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := dependent.Await(); err != nil {
		t.Fatal(err)
	}

	providerFiber.Dispose()

	if !reflect.DeepEqual(order, []string{"dependent-teardown", "resource-released"}) {
		t.Fatalf("provider resource released before dependent teardown: %v", order)
	}

	if _, ok := root.Get("svc"); ok {
		t.Fatal("service still visible after provider and dependents drained")
	}
}

// #134 (1431598): config is resolved through the internal/config waterfall
// on EVERY (re)load — start, update-restart and dependency return — so
// config sources participate each time, not only at start.
func TestConfigResolvedOnEveryReload(t *testing.T) {
	root := New()

	type config struct{ N int }

	var seen []int

	_, err := root.On(EventConfig, func(args ...any) any {
		next, ok := args[1].(func(...any) any)
		if !ok {
			t.Fatalf("arg 1 is %T, want next func", args[1])
		}

		resolved := next()
		if cfg, ok := resolved.(config); ok {
			cfg.N++
			resolved = cfg
		}

		return resolved
	}, Global())
	if err != nil {
		t.Fatal(err)
	}

	plugin := NewPlugin("p", func(_ *Context, cfg config) error {
		seen = append(seen, cfg.N)
		return nil
	})

	fiber, err := Start(root, plugin, config{N: 0})
	if err != nil {
		t.Fatal(err)
	}

	if err := fiber.Update(config{N: 5}); err != nil {
		t.Fatal(err)
	}

	if err := fiber.Await(); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(seen, []int{1, 6}) {
		t.Fatalf("internal/config not applied on every reload: %v", seen)
	}
}
