package clock_test

import (
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
)

func TestManualClock_NowDoesNotAdvanceOnItsOwn(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := clock.NewManual(start)
	got1 := m.Now()
	got2 := m.Now()
	if !got1.Equal(start) || !got2.Equal(start) {
		t.Fatalf("Now drifted without Advance: %v / %v", got1, got2)
	}
}

func TestManualClock_AdvanceFiresTicker(t *testing.T) {
	t.Parallel()
	m := clock.NewManual(time.Unix(0, 0))
	ch, stop := m.Ticker(100 * time.Millisecond)
	defer stop()

	m.Advance(100 * time.Millisecond)

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("ticker did not fire after Advance")
	}
}

func TestManualClock_AdvanceCollapsesMultipleIntervals(t *testing.T) {
	t.Parallel()
	m := clock.NewManual(time.Unix(0, 0))
	ch, stop := m.Ticker(100 * time.Millisecond)
	defer stop()

	// 350ms covers 3 intervals; like time.Ticker on a slow receiver,
	// these collapse to a single deliverable tick.
	m.Advance(350 * time.Millisecond)

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("ticker did not fire after long Advance")
	}
	select {
	case <-ch:
		t.Fatal("ticker fired twice — collapse expected")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestManualClock_StopClosesChannel(t *testing.T) {
	t.Parallel()
	m := clock.NewManual(time.Unix(0, 0))
	ch, stop := m.Ticker(time.Millisecond)
	stop()
	stop() // idempotent
	if _, ok := <-ch; ok {
		t.Fatal("channel did not close after stop")
	}
}
