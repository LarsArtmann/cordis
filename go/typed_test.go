package cordis

import (
	"strings"
	"testing"
)

type typedTestConfig struct {
	Retries int
}

type typedTestDatabase struct {
	DSN string
}

type typedTestEvent struct {
	Payload string
}

type typedTestOtherEvent struct {
	Payload string
}

func TestTypedServiceRoundTrip(t *testing.T) {
	ctx := New()
	db := &typedTestDatabase{DSN: "postgres://localhost"}

	if _, ok := TryGet[*typedTestDatabase](ctx); ok {
		t.Fatal("expected missing typed service")
	}
	if _, err := Get[*typedTestDatabase](ctx); err == nil {
		t.Fatal("expected typed get to fail for missing service")
	}

	if _, err := Provide(ctx, db); err != nil {
		t.Fatal(err)
	}
	got := MustGet[*typedTestDatabase](ctx)
	if got != db {
		t.Fatal("expected the provided instance")
	}
	if _, ok := TryGet[typedTestDatabase](ctx); ok {
		t.Fatal("expected value type mismatch to hide the service")
	}
}

func TestTypedServiceBoundToFiber(t *testing.T) {
	ctx := New()
	provider := NewPlugin("provider", func(ctx *Context, _ struct{}) error {
		_, err := Provide(ctx, &typedTestDatabase{})
		return err
	})
	if _, err := Start(ctx, provider, struct{}{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Get[*typedTestDatabase](ctx); err != nil {
		t.Fatal("expected typed service after provider activation")
	}

	if err := ctx.Fiber().Restart(); err != nil {
		t.Fatal(err)
	}
	// Restarting the root rolled the provider back with it.
	if _, err := Get[*typedTestDatabase](ctx); err == nil {
		t.Fatal("expected typed service to withdraw on rollback")
	}
}

func TestTypedServiceDuplicate(t *testing.T) {
	ctx := New()
	if _, err := Provide(ctx, &typedTestDatabase{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Provide(ctx, &typedTestDatabase{}); err == nil {
		t.Fatal("expected duplicate typed provide to fail")
	}
}

func TestTypedServiceInjectReactivity(t *testing.T) {
	ctx := New()
	activations := 0
	consumer := NewPlugin("consumer", func(ctx *Context, _ struct{}) error {
		activations++
		MustGet[*typedTestDatabase](ctx)
		return nil
	}).Inject(ServiceName[*typedTestDatabase]())

	fiber, err := Start(ctx, consumer, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if state := fiber.State(); state != StatePending {
		t.Fatalf("expected PENDING, got %s", state)
	}

	dispose, err := Provide(ctx, &typedTestDatabase{})
	if err != nil {
		t.Fatal(err)
	}
	if state := fiber.State(); state != StateActive {
		t.Fatalf("expected ACTIVE, got %s", state)
	}
	dispose()
	if state := fiber.State(); state != StatePending {
		t.Fatalf("expected PENDING after withdrawal, got %s", state)
	}
}

func TestTypedServiceIsolation(t *testing.T) {
	ctx := New()
	isolated := ctx.Isolate(ServiceName[*typedTestDatabase]())

	if _, err := Provide(ctx, &typedTestDatabase{DSN: "root"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Provide(isolated, &typedTestDatabase{DSN: "isolated"}); err != nil {
		t.Fatal(err)
	}

	if got := MustGet[*typedTestDatabase](ctx).DSN; got != "root" {
		t.Fatalf("expected root realm, got %q", got)
	}
	if got := MustGet[*typedTestDatabase](isolated).DSN; got != "isolated" {
		t.Fatalf("expected isolated realm, got %q", got)
	}
}

func TestTypedEvents(t *testing.T) {
	ctx := New()
	var received []string

	if _, err := On(ctx, func(e typedTestEvent) { received = append(received, e.Payload) }); err != nil {
		t.Fatal(err)
	}
	if _, err := On(ctx, func(e typedTestOtherEvent) { received = append(received, "other:"+e.Payload) }); err != nil {
		t.Fatal(err)
	}

	Emit(ctx, typedTestEvent{Payload: "one"})
	Emit(ctx, typedTestOtherEvent{Payload: "two"})

	if len(received) != 2 || received[0] != "one" || received[1] != "other:two" {
		t.Fatalf("expected typed dispatch to reach only matching listeners, got %v", received)
	}
}

func TestTypedEventOnce(t *testing.T) {
	ctx := New()
	calls := 0
	if _, err := Once(ctx, func(e typedTestEvent) { calls++ }); err != nil {
		t.Fatal(err)
	}
	Emit(ctx, typedTestEvent{})
	Emit(ctx, typedTestEvent{})
	if calls != 1 {
		t.Fatalf("expected exactly one delivery, got %d", calls)
	}
}

func TestTypedEventBoundToFiber(t *testing.T) {
	ctx := New()
	received := 0
	plugin := NewPlugin("listener", func(ctx *Context, _ struct{}) error {
		_, err := On(ctx, func(e typedTestEvent) { received++ })
		return err
	})
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	Emit(ctx, typedTestEvent{})
	if received != 1 {
		t.Fatal("expected delivery while active")
	}
	if err := fiber.Restart(); err != nil {
		t.Fatal(err)
	}
	// Restart reloads the plugin body, which registers a fresh listener.
	Emit(ctx, typedTestEvent{})
	if received != 2 {
		t.Fatal("restart must reload the plugin with a fresh listener")
	}
}

func TestTypedEventRollsBackWithEffect(t *testing.T) {
	ctx := New()
	received := 0
	dispose, err := ctx.Effect(func(ctx *Context) error {
		_, err := On(ctx, func(e typedTestEvent) { received++ })
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	Emit(ctx, typedTestEvent{})
	dispose()
	Emit(ctx, typedTestEvent{})
	if received != 1 {
		t.Fatal("typed listener must roll back with its effect")
	}
}

func TestTypedEventStringNameMatchesEventName(t *testing.T) {
	name := EventName[typedTestEvent]()
	if !strings.Contains(name, "typedTestEvent") {
		t.Fatalf("expected derived type name, got %q", name)
	}
	if ServiceName[typedTestConfig]() == ServiceName[typedTestDatabase]() {
		t.Fatal("distinct types must derive distinct service names")
	}
}
