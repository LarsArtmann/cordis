package cordis

import (
	"errors"
	"reflect"
	"testing"
)

func TestFunctionalPlugin(t *testing.T) {
	ctx := New()
	type Config struct{ Value int }
	calls := 0
	var gotConfig Config
	plugin := NewPlugin("test", func(ctx *Context, cfg Config) error {
		calls++
		gotConfig = cfg
		return nil
	})
	if _, err := Start(ctx, plugin, Config{Value: 42}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || gotConfig.Value != 42 {
		t.Fatalf("expected one call with config 42, got calls=%d config=%+v", calls, gotConfig)
	}
}

func TestInvalidPlugin(t *testing.T) {
	ctx := New()
	var plugin *Plugin[struct{}]
	if _, err := Start(ctx, plugin, struct{}{}); err == nil {
		t.Fatal("expected error for nil plugin")
	}
}

func TestPluginNameDerivation(t *testing.T) {
	ctx := New()
	plugin := NewPlugin("", func(ctx *Context, _ struct{}) error { return nil })
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if name := fiber.Context().String(); name == "Context <>" {
		t.Fatal("expected derived plugin name")
	}
}

func TestNestedPlugins(t *testing.T) {
	ctx := New()
	calls := 0
	listener := func(...any) any { calls++; return nil }
	mustOn(t, ctx, "custom-event", listener)

	inner := NewPlugin("inner", func(ctx *Context, _ struct{}) error {
		_, err := ctx.On("custom-event", listener)
		return err
	})
	mid := NewPlugin("mid", func(ctx *Context, _ struct{}) error {
		if _, err := ctx.On("custom-event", listener); err != nil {
			return err
		}
		_, err := Start(ctx, inner, struct{}{})
		return err
	})
	outer := NewPlugin("outer", func(ctx *Context, _ struct{}) error {
		if _, err := ctx.On("custom-event", listener); err != nil {
			return err
		}
		_, err := Start(ctx, mid, struct{}{})
		return err
	})

	fiber, err := Start(ctx, outer, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if size := ctx.Registry().Size(); size != 3 {
		t.Fatalf("expected registry size 3, got %d", size)
	}
	ctx.Emit("custom-event")
	if calls != 4 {
		t.Fatalf("expected 4 listeners, got %d", calls)
	}

	fiber.Dispose()
	if size := ctx.Registry().Size(); size != 0 {
		t.Fatalf("expected empty registry, got %d", size)
	}
	ctx.Emit("custom-event")
	if calls != 5 {
		t.Fatalf("expected only root listener, got %d calls", calls)
	}
	fiber.Dispose() // idempotent
}

func TestRegistryDeleteRestoresSnapshot(t *testing.T) {
	ctx := New()
	calls := 0
	plugin := NewPlugin("p", func(ctx *Context, _ struct{}) error {
		_, err := ctx.On("custom-event", func(...any) any { calls++; return nil })
		return err
	})
	if _, err := Start(ctx, plugin, struct{}{}); err != nil {
		t.Fatal(err)
	}
	ctx.Emit("custom-event")
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}

	ctx.Registry().Delete(plugin)
	ctx.Emit("custom-event")
	if calls != 1 {
		t.Fatalf("expected snapshot restore, got %d calls", calls)
	}
	if ctx.Registry().Has(plugin) {
		t.Fatal("expected plugin removed from registry")
	}

	if _, err := Start(ctx, plugin, struct{}{}); err != nil {
		t.Fatal(err)
	}
	ctx.Emit("custom-event")
	if calls != 2 {
		t.Fatalf("expected re-added plugin listener, got %d calls", calls)
	}
}

func TestRootDispose(t *testing.T) {
	ctx := New()
	disposed := 0
	plugin := NewPlugin("p", func(ctx *Context, _ struct{}) error {
		_, err := ctx.Effect(func(ctx *Context) error {
			ctx.registerTestCleanup(func() { disposed++ })
			return nil
		})
		return err
	})
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Fiber().UID() != 0 {
		t.Fatalf("expected root uid 0, got %d", ctx.Fiber().UID())
	}
	if fiber.UID() != 1 {
		t.Fatalf("expected child uid 1, got %d", fiber.UID())
	}

	ctx.Fiber().Dispose()
	if ctx.Fiber().UID() != 0 {
		t.Fatal("root uid must stay 0 after dispose")
	}
	if fiber.UID() != -1 {
		t.Fatalf("expected child uid -1 after dispose, got %d", fiber.UID())
	}
	if disposed != 1 {
		t.Fatalf("expected dispose once, got %d", disposed)
	}
	if effects := ctx.Fiber().GetEffects(); len(effects) != 0 {
		t.Fatalf("expected no effects left, got %v", effects)
	}
	ctx.Fiber().Dispose() // idempotent
	if disposed != 1 {
		t.Fatalf("expected idempotent root dispose, got %d", disposed)
	}
}

func TestPluginError(t *testing.T) {
	ctx := New()
	errs := captureErrors(ctx)

	calls := 0
	faulty := NewPlugin("faulty", func(ctx *Context, _ struct{}) error {
		if _, err := ctx.On("custom-event", func(...any) any { calls++; return nil }); err != nil {
			return err
		}
		return errors.New("plugin failed")
	})
	healthy := NewPlugin("healthy", func(ctx *Context, _ struct{}) error {
		_, err := ctx.On("custom-event", func(...any) any { calls++; return nil })
		return err
	})

	faultyFiber, err := Start(ctx, faulty, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Start(ctx, healthy, struct{}{}); err != nil {
		t.Fatal(err)
	}

	if state := faultyFiber.State(); state != StateFailed {
		t.Fatalf("expected FAILED, got %s", state)
	}
	if len(*errs) != 1 {
		t.Fatalf("expected 1 logged error, got %d", len(*errs))
	}
	ctx.Emit("custom-event")
	if calls != 1 {
		t.Fatalf("expected only the healthy listener, got %d calls", calls)
	}
	if err := faultyFiber.Await(); err == nil {
		t.Fatal("expected Await to return the plugin error")
	}
}

func TestDisposeErrorIsLogged(t *testing.T) {
	ctx := New()
	errs := captureErrors(ctx)
	plugin := NewPlugin("p", func(ctx *Context, _ struct{}) error {
		_, err := ctx.Effect(func(ctx *Context) error {
			ctx.registerTestCleanup(func() { panic("cleanup boom") })
			return nil
		})
		return err
	})
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	fiber.Dispose()
	if len(*errs) != 1 {
		t.Fatalf("expected cleanup panic to be logged, got %d errors", len(*errs))
	}
	if state := fiber.State(); state != StateDisposed {
		t.Fatalf("expected DISPOSED, got %s", state)
	}
}

func TestUpdateConfig(t *testing.T) {
	ctx := New()
	type Config struct{ Msg string }
	var msgs []string
	plugin := NewPlugin("p", func(ctx *Context, cfg Config) error {
		msgs = append(msgs, cfg.Msg)
		return nil
	})
	fiber, err := Start(ctx, plugin, Config{Msg: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fiber.Update(Config{Msg: "world"}); err != nil {
		t.Fatal(err)
	}
	if err := fiber.Await(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(msgs, []string{"hello", "world"}) {
		t.Fatalf("expected [hello world], got %v", msgs)
	}
	if state := fiber.State(); state != StateActive {
		t.Fatalf("expected ACTIVE, got %s", state)
	}
}

func TestRestart(t *testing.T) {
	ctx := New()
	calls := 0
	plugin := NewPlugin("p", func(ctx *Context, _ struct{}) error {
		calls++
		return nil
	})
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if err := fiber.Restart(); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls after restart, got %d", calls)
	}
	if state := fiber.State(); state != StateActive {
		t.Fatalf("expected ACTIVE, got %s", state)
	}
}

func TestConfigValidation(t *testing.T) {
	ctx := New()
	type Config struct{ Value int }
	plugin := NewPlugin("validated", func(ctx *Context, cfg Config) error { return nil })
	plugin.Validate(func(cfg Config) error {
		if cfg.Value < 0 {
			return &ValidationError{Issues: []Issue{{Path: []string{"value"}, Message: "must be non-negative"}}}
		}
		return nil
	})
	fiber, err := Start(ctx, plugin, Config{Value: -1})
	if err != nil {
		t.Fatal(err)
	}
	if state := fiber.State(); state != StateFailed {
		t.Fatalf("expected FAILED on invalid config, got %s", state)
	}
	if err := fiber.Update(Config{Value: -2}); err == nil {
		t.Fatal("expected validation error on update")
	}
	if err := fiber.Update(Config{Value: 3}); err != nil {
		t.Fatal(err)
	}
	if state := fiber.State(); state != StateActive {
		t.Fatalf("expected ACTIVE after valid update, got %s", state)
	}
}

// captureErrors installs an exporter collecting error messages.
func captureErrors(ctx *Context) *[]error {
	errs := &[]error{}
	ctx.core.logger.AddExporter(ExporterFunc(func(m Message) {
		if m.Level == LevelError {
			for _, arg := range m.Args {
				if err, ok := arg.(error); ok {
					*errs = append(*errs, err)
				}
			}
		}
	}))
	return errs
}

func TestStartAnyAndInjectSpec(t *testing.T) {
	type Config struct{ N int }
	seen := ""
	p := NewPlugin("erased", func(ctx *Context, c Config) error {
		cfg, ok := ctx.Intercepted("dep")
		if ok {
			seen = cfg.(string)
		}
		if c.N != 7 {
			t.Fatalf("config = %+v, want N=7", c)
		}
		return nil
	})
	InjectSpec(p, map[string]any{"dep": "hello", "nildep": nil})
	if len(p.base.inject) != 2 {
		t.Fatalf("inject deps = %v, want 2 entries", p.base.inject)
	}
	if p.base.injectConfig["dep"] != "hello" {
		t.Fatalf("inject config = %v", p.base.injectConfig)
	}

	ctx := New()
	if _, err := ctx.Provide("nildep", true); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.Provide("dep", "service"); err != nil {
		t.Fatal(err)
	}
	f, err := StartAny(ctx, p, Config{N: 7})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Await()
	if state := f.State(); state != StateActive {
		t.Fatalf("state = %s, want active", state)
	}
	if seen != "hello" {
		t.Fatalf("Intercepted dep = %q, want hello", seen)
	}

	if _, err := StartAny(ctx, nil, nil); err == nil {
		t.Fatal("StartAny(nil) succeeded")
	}
}
