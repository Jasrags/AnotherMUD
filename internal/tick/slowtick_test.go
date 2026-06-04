package tick_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/tick"
)

// A tick whose wall-clock duration (read through the loop's Clock)
// exceeds the configured threshold invokes the slow-tick observer with
// the total + handlers breakdown. Simulated deterministically: a handler
// advances the ManualClock once, so the loop measures elapsed time
// without any real waiting.
func TestLoop_SlowTickObserver_FiresWhenOverThreshold(t *testing.T) {
	t.Parallel()
	m := clock.NewManual(time.Unix(0, 0))
	loop := tick.New(m, time.Millisecond)

	const threshold = 50 * time.Millisecond
	reports := make(chan time.Duration, 4)
	loop.SetSlowTickObserver(threshold, func(n uint64, total, handlers time.Duration) {
		reports <- total
	})

	var once sync.Once
	mustRegister(t, loop, "slow", 1, func(ctx context.Context, n uint64) {
		once.Do(func() { m.Advance(2 * threshold) }) // one slow tick, then fast
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = loop.Run(ctx) }()
	<-loop.Ready()

	m.Advance(time.Millisecond) // fire tick 1

	select {
	case total := <-reports:
		if total <= threshold {
			t.Fatalf("slow-tick total = %v, want > threshold %v", total, threshold)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slow-tick observer did not fire for an over-threshold tick")
	}
}

// A tick that stays under the threshold must not invoke the observer.
func TestLoop_SlowTickObserver_SilentWhenFast(t *testing.T) {
	t.Parallel()
	m := clock.NewManual(time.Unix(0, 0))
	loop := tick.New(m, time.Millisecond)

	fired := make(chan struct{}, 1)
	loop.SetSlowTickObserver(50*time.Millisecond, func(n uint64, total, handlers time.Duration) {
		fired <- struct{}{}
	})
	// Handler does no clock advance, so the tick measures as instantaneous.
	mustRegister(t, loop, "fast", 1, func(ctx context.Context, n uint64) {})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = loop.Run(ctx) }()
	<-loop.Ready()

	m.Advance(time.Millisecond) // fire a tick

	select {
	case <-fired:
		t.Fatal("slow-tick observer fired for a fast tick")
	case <-time.After(150 * time.Millisecond):
		// expected: no report
	}
}
