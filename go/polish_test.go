package cordis_test

import (
	"context"
	"errors"
	"testing"
	"time"

	cordis "github.com/LarsArtmann/cordis/go"
)

func TestFiberErrReturnsActivationError(t *testing.T) {
	ctx := cordis.New()
	sentinel := errors.New("apply failed")
	handle := cordis.NewPlugin("broken", func(ctx *cordis.Context, conf string) error {
		return sentinel
	})
	fiber, err := cordis.StartAny(ctx, handle, "x")
	if err != nil {
		t.Fatal(err)
	}
	awaitState(t, fiber, cordis.StateFailed)

	if got := fiber.Err(); !errors.Is(got, sentinel) {
		t.Fatalf("Err() = %v, want the apply error", got)
	}
	if fiber.State() != cordis.StateFailed {
		t.Fatalf("state = %s, want failed", fiber.State())
	}
}

func awaitState(t *testing.T, fiber *cordis.Fiber, want cordis.FiberState) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for fiber.State() != want {
		if time.Now().After(deadline) {
			t.Fatalf("state = %s, want %s", fiber.State(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAwaitContextReturnsOnCancel(t *testing.T) {
	ctx := cordis.New()
	release := make(chan struct{})
	fiberCh := make(chan *cordis.Fiber, 1)
	if _, err := ctx.On(cordis.EventPlugin, func(args ...any) any {
		if f, ok := args[0].(*cordis.Fiber); ok {
			fiberCh <- f
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	go func() {
		// The drain blocks this goroutine until release closes; the
		// fiber is observable as executing from outside meanwhile.
		_, _ = ctx.Inject([]string{}, func(ctx *cordis.Context) error {
			close(fiberCh) // in case the plugin event never fired
			<-release
			return nil
		})
	}()

	fiber := <-fiberCh
	parent, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := fiber.AwaitContext(parent); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("AwaitContext = %v, want DeadlineExceeded", err)
	}

	close(release)
	if err := fiber.Await(); err != nil {
		t.Fatalf("Await after cancel = %v", err)
	}
}

func TestAwaitContextReturnsNilWhenSettled(t *testing.T) {
	ctx := cordis.New()
	fiber, err := ctx.Inject([]string{}, func(ctx *cordis.Context) error {
		time.Sleep(30 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fiber.AwaitContext(context.Background()); err != nil {
		t.Fatalf("AwaitContext = %v, want nil", err)
	}
}

func TestInjectTypedHelpers(t *testing.T) {
	ctx := cordis.New()
	if _, err := cordis.Provide(ctx, &accConfig{Port: 8080}); err != nil {
		t.Fatal(err)
	}
	if _, err := cordis.Provide(ctx, &accLogger{Level: "info"}); err != nil {
		t.Fatal(err)
	}

	f1, err := cordis.Inject1(ctx, func(ctx *cordis.Context, cfg *accConfig) error {
		if cfg.Port != 8080 {
			t.Fatalf("cfg.Port = %d", cfg.Port)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f1.Await(); err != nil {
		t.Fatal(err)
	}

	f2, err := cordis.Inject2(ctx, func(ctx *cordis.Context, cfg *accConfig, logger *accLogger) error {
		if cfg == nil || logger == nil {
			t.Fatal("nil dependency")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f2.Await(); err != nil {
		t.Fatal(err)
	}

	f3, err := cordis.Inject3(ctx, func(ctx *cordis.Context, cfg *accConfig, logger *accLogger, port int) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// port is not provided: the fiber must stay pending until it arrives.
	if f3.State() != cordis.StatePending {
		t.Fatalf("state = %s, want pending", f3.State())
	}
	if _, err := cordis.Provide(ctx, 42); err != nil {
		t.Fatal(err)
	}
	awaitState(t, f3, cordis.StateActive)
}
