// Package timer ports the upstream @cordis/timer plugin to Go using the
// platform's own timing primitives. Timers are ordinary effects: they roll
// back with the enclosing scope of the context they are registered on, in
// last-in, first-out order.
//
// Start the service once per context tree so registry tooling can see,
// restart and dispose timers as one unit:
//
//	timer.Start(ctx)
//
// then schedule work with any live context:
//
//	timer.AfterFunc(ctx, time.Second, func() { fmt.Println("tick") })
package timer

import (
	"sync"
	"time"

	cordis "github.com/LarsArtmann/cordis/go"
)

// ServiceName is the canonical service name of the timer service.
const ServiceName = "timer"

// Service is the timer service value published under ServiceName. Timers
// are registered per context, so the service itself is only an
// introspection anchor.
type Service struct{}

// Start installs the timer service on the context tree.
func Start(ctx *cordis.Context) (cordis.Disposer, error) {
	return ctx.Provide(ServiceName, &Service{})
}

// AfterFunc runs fn once after delay. The timer rolls back with ctx's
// scope; the returned disposer cancels it early. Cancelling an already
// fired timer is a no-op.
func AfterFunc(ctx *cordis.Context, delay time.Duration, fn func()) (cordis.Disposer, error) {
	timer := time.AfterFunc(delay, fn)
	return ctx.Cleanup("timer.timeout", func() { timer.Stop() })
}

// Await resolves after delay, or aborts early when ctx's scope rolls back.
// The channel receives one value exactly once and is closed afterwards.
func Await(ctx *cordis.Context, delay time.Duration) (<-chan struct{}, cordis.Disposer, error) {
	ch := make(chan struct{})
	timer := time.AfterFunc(delay, func() {
		defer close(ch)
		select {
		case ch <- struct{}{}:
		default:
		}
	})
	d, err := ctx.Cleanup("timer.timeout", func() { timer.Stop() })
	if err != nil {
		timer.Stop()
		return nil, nil, err
	}
	return ch, d, nil
}

// Interval delivers a Tick after every delay until the owning scope rolls
// back or the returned disposer is called. The channel is closed on
// disposal and never while active.
func Interval(ctx *cordis.Context, delay time.Duration) (<-chan time.Time, cordis.Disposer, error) {
	ch := make(chan time.Time)
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(delay)
		defer ticker.Stop()
		defer close(ch)
		for {
			select {
			case at := <-ticker.C:
				select {
				case ch <- at:
				case <-stop:
					return
				}
			case <-stop:
				return
			}
		}
	}()
	disposer, err := ctx.Cleanup("timer.interval", func() { close(stop) })
	if err != nil {
		close(stop)
		return nil, nil, err
	}
	return ch, disposer, nil
}

// IntervalFunc runs fn after every delay until the owning scope rolls back
// or the returned disposer is called. fn runs on its own goroutine.
func IntervalFunc(ctx *cordis.Context, delay time.Duration, fn func()) (cordis.Disposer, error) {
	ch, d, err := Interval(ctx, delay)
	if err != nil {
		return nil, err
	}
	go func() {
		for range ch {
			fn()
		}
	}()
	return d, nil
}

// Throttle returns a wrapper around fn that invokes it at most once per
// delay; the last call inside the window fires when the window elapses
// unless noTrailing is set. The returned disposer cancels any pending
// trailing invocation and rolls back with ctx's scope.
func Throttle(ctx *cordis.Context, fn func(args ...any), delay time.Duration, noTrailing bool) (func(args ...any), cordis.Disposer, error) {
	return scheduler(ctx, fn, delay, &options{throttle: true, noTrailing: noTrailing})
}

// Debounce delays invoking fn until delay has passed without a new call.
// The returned disposer cancels the pending invocation.
func Debounce(ctx *cordis.Context, fn func(args ...any), delay time.Duration) (func(args ...any), cordis.Disposer, error) {
	return scheduler(ctx, fn, delay, &options{})
}

type options struct {
	throttle   bool
	noTrailing bool
}

func scheduler(ctx *cordis.Context, fn func(args ...any), delay time.Duration, opts *options) (func(args ...any), cordis.Disposer, error) {
	var (
		mu        sync.Mutex
		timer     *time.Timer
		lastCall  time.Time
		lastArgs  []any
		isStopped bool
	)
	run := func(args []any) {
		lastCall = time.Now()
		fn(args...)
	}
	wrapper := func(args ...any) {
		mu.Lock()
		defer mu.Unlock()
		if isStopped {
			return
		}
		if opts.throttle {
			now := time.Now()
			remaining := delay - now.Sub(lastCall)
			if remaining <= 0 {
				run(args)
				return
			}
			if opts.noTrailing {
				return
			}
			lastArgs = args
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(remaining, func() {
				mu.Lock()
				defer mu.Unlock()
				run(lastArgs)
			})
			return
		}
		lastArgs = args
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(delay, func() {
			mu.Lock()
			defer mu.Unlock()
			run(lastArgs)
		})
	}
	disposer, err := ctx.Cleanup("timer.schedule", func() {
		mu.Lock()
		defer mu.Unlock()
		isStopped = true
		if timer != nil {
			timer.Stop()
		}
	})
	if err != nil {
		return nil, nil, err
	}
	return wrapper, disposer, nil
}
