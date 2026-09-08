package timer

import (
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	cordis "github.com/LarsArtmann/cordis/go"
)

func TestAfterFunc(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
		synctest.Sleep(60 * time.Millisecond)
		if calls.Load() != 1 {
			t.Fatalf("AfterFunc must fire exactly once, got %d", calls.Load())
		}
	})
}

func TestAfterFuncDispose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := cordis.New()
		var calls atomic.Int32
		d, err := AfterFunc(ctx, 20*time.Millisecond, func() { calls.Add(1) })
		if err != nil {
			t.Fatal(err)
		}
		d()
		synctest.Sleep(50 * time.Millisecond)
		if calls.Load() != 0 {
			t.Fatal("disposed timer must not fire")
		}
	})
}

func TestAwait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
		synctest.Wait()
		if _, open := <-ch; open {
			t.Fatal("Await channel must close after firing")
		}
	})
}

func TestIntervalRollsBackWithFiber(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
		synctest.Wait()
	})
}

func TestIntervalFuncDispose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := cordis.New()
		var calls atomic.Int32
		d, err := IntervalFunc(ctx, 10*time.Millisecond, func() { calls.Add(1) })
		if err != nil {
			t.Fatal(err)
		}
		synctest.Sleep(50 * time.Millisecond)
		d()
		synctest.Wait()
		baseline := calls.Load()
		synctest.Sleep(40 * time.Millisecond)
		if calls.Load() != baseline {
			t.Fatal("disposed interval must stop ticking")
		}
	})
}

// TestIntervalFuncNoCallbackAfterDispose pins the pump-lifetime contract: a
// callback already running at disposal completes, but no callback starts
// afterwards, even though ticks fired while it was still running.
func TestIntervalFuncNoCallbackAfterDispose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := cordis.New()
		release := make(chan struct{})
		var started atomic.Int32
		d, err := IntervalFunc(ctx, 10*time.Millisecond, func() {
			started.Add(1)
			<-release
		})
		if err != nil {
			t.Fatal(err)
		}
		synctest.Sleep(10 * time.Millisecond)
		d()
		close(release)
		synctest.Wait()
		if started.Load() != 1 {
			t.Fatalf("exactly one callback must have started, got %d", started.Load())
		}
		synctest.Sleep(100 * time.Millisecond)
		synctest.Wait()
		if started.Load() != 1 {
			t.Fatalf("no callback may start after disposal, got %d", started.Load())
		}
	})
}

func TestDebounce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := cordis.New()
		var calls atomic.Int32
		fn, d, err := Debounce(ctx, func(args ...any) { calls.Add(1) }, 30*time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		defer d()
		fn(1)
		synctest.Sleep(10 * time.Millisecond)
		fn(2)
		synctest.Sleep(10 * time.Millisecond)
		fn(3)
		if calls.Load() != 0 {
			t.Fatal("debounced fn must not fire inside the window")
		}
		synctest.Sleep(60 * time.Millisecond)
		if calls.Load() != 1 {
			t.Fatalf("debounce must fire exactly once after the window, got %d", calls.Load())
		}
	})
}

func TestThrottle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
		synctest.Sleep(60 * time.Millisecond)
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
		synctest.Sleep(60 * time.Millisecond)
		if leading.Load() != 1 {
			t.Fatalf("noTrailing must drop the trailing call, got %d", leading.Load())
		}
	})
}
