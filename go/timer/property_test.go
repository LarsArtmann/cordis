package timer

import (
	"math/rand/v2"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	cordis "github.com/LarsArtmann/cordis/go"
)

// TestDebounceThrottleProperty drives randomized call sequences through
// Debounce and both Throttle modes inside a virtual-time bubble and checks
// the observed fire counts against an independent event-simulation model
// of each schedule. The fixed seed keeps any failure reproducible.
//
// A call landing on the same virtual instant as a pending timer fire has
// no portable ordering, so the generator rejection-samples gaps that never
// coincide with a modelled fire deadline.
func TestDebounceThrottleProperty(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := cordis.New()
		rng := rand.New(rand.NewPCG(20260908, 42))

		for round := range 40 {
			delay := time.Duration(2+rng.IntN(6)) * time.Millisecond

			var (
				debFires atomic.Int64
				debGot   atomic.Int64
				thrFires atomic.Int64
				thrNT    atomic.Int64
			)
			deb, stopDeb, err := Debounce(ctx, func(args ...any) {
				debFires.Add(1)
				if n, ok := args[0].(int); ok {
					debGot.Store(int64(n))
				}
			}, delay)
			if err != nil {
				t.Fatal(err)
			}
			thr, stopThr, err := Throttle(ctx, func(args ...any) { thrFires.Add(1) }, delay, false)
			if err != nil {
				t.Fatal(err)
			}
			thrNoTrailing, stopThrNT, err := Throttle(ctx, func(args ...any) { thrNT.Add(1) }, delay, true)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				stopDeb()
				stopThr()
				stopThrNT()
			}()

			m := newDispatchModel(delay)
			for call := 1; call <= 1+rng.IntN(10); call++ {
				synctest.Sleep(m.nextGap(rng))
				m.call(call)
				deb(call)
				thr(call)
				thrNoTrailing(call)
			}
			m.finish()
			synctest.Sleep(10 * delay)
			synctest.Wait()

			if got, want := debFires.Load(), int64(m.debFires); got != want {
				t.Fatalf("round %d: debounce fired %d times, want %d", round, got, want)
			}
			if got, want := debGot.Load(), int64(m.debLastFiredCall); got != want {
				t.Fatalf("round %d: debounce delivered call %d, want %d", round, got, want)
			}
			if got, want := thrFires.Load(), int64(m.thrFires); got != want {
				t.Fatalf("round %d: throttle fired %d times, want %d", round, got, want)
			}
			if got, want := thrNT.Load(), int64(m.thrNTFires); got != want {
				t.Fatalf("round %d: no-trailing throttle fired %d times, want %d", round, got, want)
			}
		}
	})
}

// dispatchModel is an event-simulation oracle for the three schedules over
// one generated timeline. Times are virtual milliseconds since the round
// started.
type dispatchModel struct {
	delay time.Duration
	now   time.Duration

	debLast          time.Duration // last debounce call
	debPending       bool
	debFires         int
	debLastFiredCall int
	lastDebCall      int

	thrStarted   bool
	thrLastRun   time.Duration
	thrTrailing  time.Duration // 0 = none
	thrFires     int
	thrNTStarted bool
	thrNTRuns    time.Duration
	thrNTFires   int
}

func newDispatchModel(delay time.Duration) *dispatchModel {
	return &dispatchModel{delay: delay}
}

// nextGap rejection-samples the duration of the next sleep: short gaps land
// inside the running windows, long gaps strictly after them, and a gap
// ending exactly on a pending fire deadline is redrawn.
func (m *dispatchModel) nextGap(rng *rand.Rand) time.Duration {
	for {
		g := time.Duration(1+rng.IntN(20)) * time.Millisecond
		end := m.now + g
		if m.debPending && end == m.debLast+m.delay {
			continue
		}
		if m.thrTrailing != 0 && end == m.thrTrailing {
			continue
		}
		m.now = end
		return g
	}
}

// call advances the model by one call of every wrapper at the current time.
func (m *dispatchModel) call(call int) {
	// Debounce: a pending fire whose deadline passed strictly before the
	// call fired during the sleep; a still-pending one is superseded.
	if m.debPending && m.debLast+m.delay < m.now {
		m.debFires++
		m.debLastFiredCall = m.lastDebCall
	}
	m.debLast = m.now
	m.debPending = true
	m.lastDebCall = call

	// Throttle (with trailing): the trailing fire may have happened
	// during the sleep, then the call is a leading run or lands inside
	// the window. The first call always runs: no window is open yet.
	if m.thrTrailing != 0 && m.thrTrailing < m.now {
		m.thrFires++
		m.thrLastRun = m.thrTrailing
		m.thrTrailing = 0
	}
	if !m.thrStarted || m.now-m.thrLastRun >= m.delay {
		m.thrFires++
		m.thrLastRun = m.now
		m.thrStarted = true
	} else {
		m.thrTrailing = m.thrLastRun + m.delay
	}

	// No-trailing throttle: leading runs only.
	if !m.thrNTStarted || m.now-m.thrNTRuns >= m.delay {
		m.thrNTFires++
		m.thrNTRuns = m.now
		m.thrNTStarted = true
	}
}

// finish models the final flush sleep that drains every pending timer.
func (m *dispatchModel) finish() {
	if m.debPending {
		m.debFires++
		m.debLastFiredCall = m.lastDebCall
	}
	m.debPending = false
	if m.thrTrailing != 0 {
		m.thrFires++
	}
}
