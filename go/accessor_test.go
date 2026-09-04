package cordis_test

import (
	"errors"
	"testing"
	"time"

	cordis "github.com/LarsArtmann/cordis/go"
)

type accConfig struct {
	Port int
	Host string
}

type accLogger struct {
	Level string
}

func TestAccessorDerivesFromSource(t *testing.T) {
	ctx := cordis.New()
	if _, err := cordis.Provide(ctx, &accConfig{Port: 8080, Host: "localhost"}); err != nil {
		t.Fatal(err)
	}

	fiber, _, err := cordis.Accessor(ctx, "port",
		func(ctx *cordis.Context, cfg *accConfig) (int, error) { return cfg.Port, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	awaitFiber(t, fiber, cordis.StateActive)

	port, err := cordis.GetNamed[int](ctx, "port")
	if err != nil {
		t.Fatal(err)
	}
	if port != 8080 {
		t.Fatalf("port = %d, want 8080", port)
	}
}

func TestAccessorWithoutSourceStaysPending(t *testing.T) {
	ctx := cordis.New()

	fiber, _, err := cordis.Accessor(ctx, "port",
		func(ctx *cordis.Context, cfg *accConfig) (int, error) { return cfg.Port, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if fiber.State() != cordis.StatePending {
		t.Fatalf("state = %s, want pending before the source exists", fiber.State())
	}

	if _, err := cordis.Provide(ctx, &accConfig{Port: 80}); err != nil {
		t.Fatal(err)
	}
	awaitFiber(t, fiber, cordis.StateActive)
	port, err := cordis.GetNamed[int](ctx, "port")
	if err != nil || port != 80 {
		t.Fatalf("port = %d, %v; want 80, nil", port, err)
	}

	// The derived service follows the source out again.
	cordis.MustGet[*accConfig](ctx)
}
func awaitFiber(t *testing.T, fiber *cordis.Fiber, want cordis.FiberState) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for fiber.State() != want {
		if time.Now().After(deadline) {
			t.Fatalf("state = %s, want %s", fiber.State(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAccessorFollowsSourceLifecycle(t *testing.T) {
	ctx := cordis.New()
	dispose, err := cordis.Provide(ctx, &accConfig{Port: 1})
	if err != nil {
		t.Fatal(err)
	}

	fiber, _, err := cordis.Accessor(ctx, "port",
		func(ctx *cordis.Context, cfg *accConfig) (int, error) { return cfg.Port, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	awaitFiber(t, fiber, cordis.StateActive)

	// Unloading the source unloads the accessor and withdraws the derived
	// service. Has stays true: the name remains declared; the live value
	// is gone.
	dispose()
	awaitFiber(t, fiber, cordis.StatePending)
	if _, ok := ctx.Get("port"); ok {
		t.Fatal("derived service outlived its source")
	}
}

func TestAccessorWriteBack(t *testing.T) {
	ctx := cordis.New()
	cfg := &accConfig{Port: 8080}
	if _, err := cordis.Provide(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	_, port, err := cordis.Accessor(ctx, "port",
		func(ctx *cordis.Context, cfg *accConfig) (int, error) { return cfg.Port, nil },
		func(ctx *cordis.Context, cfg *accConfig, v int) error {
			cfg.Port = v
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := port.Set(9090); err != nil {
		t.Fatal(err)
	}
	awaitFiber(t, port.Fiber(), cordis.StateActive)
	if cfg.Port != 9090 {
		t.Fatalf("source port = %d, want 9090", cfg.Port)
	}
	got, err := cordis.GetNamed[int](ctx, "port")
	if err != nil || got != 9090 {
		t.Fatalf("derived port = %d, %v; want 9090, nil", got, err)
	}
}

func TestAccessorReadOnlySetErrors(t *testing.T) {
	ctx := cordis.New()
	if _, err := cordis.Provide(ctx, &accConfig{}); err != nil {
		t.Fatal(err)
	}

	_, port, err := cordis.Accessor(ctx, "port",
		func(ctx *cordis.Context, cfg *accConfig) (int, error) { return cfg.Port, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := port.Set(1); !errors.Is(err, cordis.ErrReadOnlyAccessor) {
		t.Fatalf("err = %v, want ErrReadOnlyAccessor", err)
	}
}

func TestMixinProjectsMember(t *testing.T) {
	ctx := cordis.New()
	logger := &accLogger{Level: "info"}
	if _, err := cordis.Provide(ctx, logger); err != nil {
		t.Fatal(err)
	}

	fiber, level, err := cordis.Mixin(ctx, "level",
		func(l *accLogger) string { return l.Level },
		func(l *accLogger, v string) { l.Level = v },
	)
	if err != nil {
		t.Fatal(err)
	}
	awaitFiber(t, fiber, cordis.StateActive)

	got, err := cordis.GetNamed[string](ctx, "level")
	if err != nil || got != "info" {
		t.Fatalf("level = %q, %v; want info, nil", got, err)
	}

	if err := level.Set("debug"); err != nil {
		t.Fatal(err)
	}
	awaitFiber(t, fiber, cordis.StateActive)
	if logger.Level != "debug" {
		t.Fatalf("source level = %q, want debug", logger.Level)
	}
	if got, _ = cordis.GetNamed[string](ctx, "level"); got != "debug" {
		t.Fatalf("derived level = %q, want debug", got)
	}
}

func TestAccessorGetFailsWithoutSource(t *testing.T) {
	ctx := cordis.New()
	if _, err := cordis.GetNamed[int](ctx, "port"); err == nil {
		t.Fatal("GetNamed succeeded without the accessor")
	}
	if ctx.Has("port") {
		t.Fatal("accessor registered without a source")
	}
}
