package cordis

import (
	"reflect"
	"testing"
)

// The unload guard (the calculus's L-Unload guard, Theorem 70; upstream's
// `await Promise.allSettled` inside the provide disposer): when a provider's
// effect withdraws a service, the dependents notified by that withdrawal
// settle before the provider's later-ordered cleanups run, so a consumer's
// teardown hands its handles back while the resource they belong to still
// exists.

type fakePool struct {
	destroyed bool
	name      string
	order     *[]string
}

func (p *fakePool) destroy() {
	p.destroyed = true
	*p.order = append(*p.order, p.name+" pool destroyed")
}

// poolProvider builds the canonical provider idiom: one effect owning a pool
// and its service, with the pool cleanup registered first so LIFO disposal
// withdraws the service before destroying the pool.
func poolProvider(name, service string, order *[]string) *Plugin[int] {
	return NewPlugin(name, func(ctx *Context, _ int) error {
		_, err := ctx.Effect(func(ectx *Context) error {
			pool := &fakePool{name: name, order: order}
			if _, err := ectx.Cleanup("pool "+name, pool.destroy); err != nil {
				return err
			}
			if _, err := ectx.Provide(service, pool); err != nil {
				return err
			}
			return nil
		}, service+"-pool")
		return err
	})
}

func simpleConsumer(name string, services []string, order *[]string) *Plugin[int] {
	return NewPlugin(name, func(ctx *Context, _ int) error {
		for _, service := range services {
			if _, ok := ctx.Get(service); !ok {
				return nil
			}
		}
		_, err := ctx.Cleanup("teardown "+name, func() {
			*order = append(*order, name+" teardown")
		})
		return err
	}).Inject(services...)
}

func TestUnloadGuardDependentsSettleBeforeProviderCleanup(t *testing.T) {
	ctx := New()
	var order []string

	providerFiber, err := Start(ctx, poolProvider("provider", "db", &order), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Start(ctx, simpleConsumer("consumer", []string{"db"}, &order), 0); err != nil {
		t.Fatal(err)
	}

	providerFiber.Dispose()

	want := []string{"consumer teardown", "provider pool destroyed"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("teardown order = %v, want %v", order, want)
	}
}

// A chain A provides s1, B injects s1 and provides s2, C injects s2:
// disposing A must drain the cascade leaf first — C settles inside B's
// withdrawal, and B settles inside A's — before each pool is destroyed.
func TestUnloadGuardSettlesDependencyChainsLeafFirst(t *testing.T) {
	ctx := New()
	var order []string

	a := NewPlugin("a", func(ctx *Context, _ int) error {
		_, err := ctx.Effect(func(ectx *Context) error {
			pool := &fakePool{name: "a", order: &order}
			if _, err := ectx.Cleanup("pool a", pool.destroy); err != nil {
				return err
			}
			if _, err := ectx.Provide("s1", pool); err != nil {
				return err
			}
			return nil
		}, "s1-pool")
		return err
	})
	b := NewPlugin("b", func(ctx *Context, _ int) error {
		if _, ok := ctx.Get("s1"); !ok {
			return nil
		}
		_, err := ctx.Effect(func(ectx *Context) error {
			pool := &fakePool{name: "b", order: &order}
			if _, err := ectx.Cleanup("pool b", pool.destroy); err != nil {
				return err
			}
			if _, err := ectx.Provide("s2", pool); err != nil {
				return err
			}
			return nil
		}, "s2-pool")
		return err
	}).Inject("s1")
	c := simpleConsumer("c", []string{"s2"}, &order)

	aFiber, err := Start(ctx, a, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Start(ctx, b, 0); err != nil {
		t.Fatal(err)
	}
	cFiber, err := Start(ctx, c, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cFiber.Dispose()

	aFiber.Dispose()

	want := []string{"c teardown", "b pool destroyed", "a pool destroyed"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("teardown order = %v, want %v", order, want)
	}
}

// Withdrawing by disposing the effect directly (not the whole fiber) must
// settle dependents before the disposer call returns, including when the
// disposal happens inside another framework call.
func TestUnloadGuardDirectEffectDisposal(t *testing.T) {
	ctx := New()
	var order []string

	var effectDisposer Disposer
	provider := NewPlugin("provider", func(ctx *Context, _ int) error {
		disposer, err := ctx.Effect(func(ectx *Context) error {
			pool := &fakePool{name: "provider", order: &order}
			if _, err := ectx.Cleanup("pool provider", pool.destroy); err != nil {
				return err
			}
			if _, err := ectx.Provide("db", pool); err != nil {
				return err
			}
			return nil
		}, "db-pool")
		if err != nil {
			return err
		}
		effectDisposer = disposer
		return nil
	})
	consumer := simpleConsumer("consumer", []string{"db"}, &order)

	if _, err := Start(ctx, provider, 0); err != nil {
		t.Fatal(err)
	}
	consumerFiber, err := Start(ctx, consumer, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer consumerFiber.Dispose()
	if effectDisposer == nil {
		t.Fatal("effect disposer was not captured")
	}

	// Dispose from inside a batch (a nested public call): the guard must
	// still settle the dependents before the pool cleanup runs.
	ctx.Batch(func(*Context) {
		effectDisposer()
	})

	want := []string{"consumer teardown", "provider pool destroyed"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("teardown order = %v, want %v", order, want)
	}
}

// Restarting the provider unloads it first: the guard must drain the
// dependents inside that unload, then the reload reactivates them.
func TestUnloadGuardRestartUnloadsDependentsFirst(t *testing.T) {
	ctx := New()
	var order []string

	providerFiber, err := Start(ctx, poolProvider("provider", "db", &order), 0)
	if err != nil {
		t.Fatal(err)
	}
	consumerFiber, err := Start(ctx, simpleConsumer("consumer", []string{"db"}, &order), 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := providerFiber.Restart(); err != nil {
		t.Fatal(err)
	}

	if got := consumerFiber.State(); got != StateActive {
		t.Fatalf("consumer state after provider restart = %v, want ACTIVE", got)
	}
	want := []string{"consumer teardown", "provider pool destroyed"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("teardown order = %v, want %v", order, want)
	}
}
