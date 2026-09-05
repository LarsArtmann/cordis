package cordis_test

import (
	"errors"
	"strings"
	"testing"

	cordis "github.com/LarsArtmann/cordis/go"
)

type trackedDB struct {
	*cordis.ServiceMeta
	DSN string
}

type sumRequest struct {
	A, B int
}

func TestProvideServiceFillsMeta(t *testing.T) {
	ctx := cordis.New()
	db := &trackedDB{ServiceMeta: &cordis.ServiceMeta{}, DSN: "postgres://x"}
	if _, err := cordis.ProvideService(ctx, db); err != nil {
		t.Fatal(err)
	}

	if db.Name() != "*cordis_test.trackedDB" {
		t.Fatalf("name = %q", db.Name())
	}
	if db.Ctx() != ctx {
		t.Fatal("meta ctx does not point at the providing context")
	}

	got, err := cordis.Get[*trackedDB](ctx)
	if err != nil || got.DSN != "postgres://x" {
		t.Fatalf("got = %v, %v", got, err)
	}
}

func TestCallableReturnsResult(t *testing.T) {
	ctx := cordis.New()
	add := cordis.Callable[sumRequest](ctx, "add", func(ctx *cordis.Context, req sumRequest) (int, error) {
		return req.A + req.B, nil
	})

	got, err := add(sumRequest{A: 2, B: 3})
	if err != nil || got != 5 {
		t.Fatalf("add = %d, %v; want 5, nil", got, err)
	}
}

func TestCallableRecoversPanic(t *testing.T) {
	ctx := cordis.New()
	boom := cordis.Callable[sumRequest](ctx, "boom", func(ctx *cordis.Context, req sumRequest) (int, error) {
		panic("kaboom")
	})

	_, err := boom(sumRequest{})
	if err == nil {
		t.Fatal("panicking callable returned no error")
	}
	if !strings.Contains(err.Error(), "kaboom") {
		t.Fatalf("err = %v, want the panic message", err)
	}
}

func TestCallableResolvesThroughBoundContext(t *testing.T) {
	ctx := cordis.New()
	if _, err := cordis.Provide(ctx, &accConfig{Port: 8080}); err != nil {
		t.Fatal(err)
	}

	readPort := cordis.Callable[struct{}](ctx, "readPort", func(ctx *cordis.Context, _ struct{}) (int, error) {
		cfg, err := cordis.Get[*accConfig](ctx)
		if err != nil {
			return 0, err
		}
		return cfg.Port, nil
	})

	got, err := readPort(struct{}{})
	if err != nil || got != 8080 {
		t.Fatalf("readPort = %d, %v; want 8080, nil", got, err)
	}
}

func TestCallablePublishableAsService(t *testing.T) {
	ctx := cordis.New()
	add := cordis.Callable[sumRequest](ctx, "add", func(ctx *cordis.Context, req sumRequest) (int, error) {
		return req.A + req.B, nil
	})
	if _, err := cordis.Provide(ctx, add); err != nil {
		t.Fatal(err)
	}

	resolved, err := cordis.Get[func(sumRequest) (int, error)](ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolved(sumRequest{A: 7, B: 8})
	if err != nil || got != 15 {
		t.Fatalf("resolved add = %d, %v; want 15, nil", got, err)
	}

	if _, err := resolved(sumRequest{A: -1, B: -1}); err != nil {
		t.Fatalf("second call failed: %v", errors.Unwrap(err))
	}
}
