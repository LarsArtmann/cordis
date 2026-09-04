package timer

import (
	"sync/atomic"
	"testing"
	"time"

	cordis "github.com/LarsArtmann/cordis/go"
)

func TestAfterFunc(t *testing.T) {
	ctx := cordis.New()
	if _, err := Start(ctx); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	d, err := AfterFunc(ctx, 20*time.Millisecond, func() { calls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	defer d()
	time.Sleep(60 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("AfterFunc must fire exactly once, got %d", calls.Load())
	}
}

func TestAfterFuncDispose(t *testing.T) {
	ctx := cordis.New()
	var calls atomic.Int32
	d, err := AfterFunc(ctx, 20*time.Millisecond, func() { calls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	d()
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatal("disposed timer must not fire")
	}
}

func TestAwait(t *testing.T) {
	ctx := cordis.New()
	ch, d, err := Await(ctx, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer d()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("Await must resolve after the delay")
	}
	// The channel is closed after the single value.
	if _, open := <-ch; open {
		t.Fatal("Await channel must close after firing")
	}
}

func TestIntervalRollsBackWithFiber(t *testing.T) {
	ctx := cordis.New()
	fiber, err := cordis.Start(ctx, cordis.NewPlugin("timer-user", func(c *cordis.Context, _ int) error {
		_, _, err := Interval(c, 10*time.Millisecond)
		return err
	}), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := fiber.Await(); err != nil {
		t.Fatal(err)
	}
	fiber.Dispose()
	time.Sleep(30 * time.Millisecond)
}

func TestIntervalFuncDispose(t *testing.T) {
	ctx := cordis.New()
	var calls atomic.Int32
	d, err := IntervalFunc(ctx, 10*time.Millisecond, func() { calls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	d()
	// Ticks run on their own goroutine: let an in-flight callback finish
	// before sampling the baseline, otherwise its increment would land
	// inside the quiet window and look like a tick after disposal.
	time.Sleep(60 * time.Millisecond)
	baseline := calls.Load()
	time.Sleep(40 * time.Millisecond)
	if calls.Load() != baseline {
		t.Fatal("disposed interval must stop ticking")
	}
}

func TestDebounce(t *testing.T) {
	ctx := cordis.New()
	var calls atomic.Int32
	fn, d, err := Debounce(ctx, func(args ...any) { calls.Add(1) }, 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer d()
	fn(1)
	time.Sleep(10 * time.Millisecond)
	fn(2)
	time.Sleep(10 * time.Millisecond)
	fn(3)
	if calls.Load() != 0 {
		t.Fatal("debounced fn must not fire inside the window")
	}
	time.Sleep(60 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("debounce must fire exactly once after the window, got %d", calls.Load())
	}
}

func TestThrottle(t *testing.T) {
	ctx := cordis.New()
	var calls atomic.Int32
	fn, d, err := Throttle(ctx, func(args ...any) { calls.Add(1) }, 30*time.Millisecond, false)
	if err != nil {
		t.Fatal(err)
	}
	defer d()
	fn()
	fn()
	if calls.Load() != 1 {
		t.Fatal("throttle must execute the leading call immediately")
	}
	time.Sleep(60 * time.Millisecond)
	if calls.Load() != 2 {
		t.Fatalf("trailing call must fire once the window elapses, got %d", calls.Load())
	}

	var leading atomic.Int32
	fn2, d2, err := Throttle(ctx, func(args ...any) { leading.Add(1) }, 30*time.Millisecond, true)
	if err != nil {
		t.Fatal(err)
	}
	defer d2()
	fn2()
	fn2()
	time.Sleep(60 * time.Millisecond)
	if leading.Load() != 1 {
		t.Fatalf("noTrailing must drop the trailing call, got %d", leading.Load())
	}
}
