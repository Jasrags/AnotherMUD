package session

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

func newIdleRig(t *testing.T) (*Manager, *clock.ManualClock, *connActor, *fakeConn) {
	t.Helper()
	mc := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	mgr := NewManager()
	r := &world.Room{ID: "x:1"}
	a, fc := newFakeActor("c1", "p1", "acc1", "Alice", r)
	a.lastInputAt = mc.Now()
	mgr.Add(a)
	return mgr, mc, a, fc
}

// IdleSweep is a no-op when both WarnAfter and TimeoutAfter are zero.
func TestIdleSweep_DisabledByZeroConfig(t *testing.T) {
	mgr, mc, _, fc := newIdleRig(t)
	mc.Advance(1 * time.Hour)
	mgr.IdleSweep(context.Background(), IdleConfig{}, mc)
	if got := fc.writes(); len(got) != 0 {
		t.Errorf("disabled sweep wrote: %v", got)
	}
}

// Below WarnAfter, IdleSweep does nothing.
func TestIdleSweep_NoOpBelowWarn(t *testing.T) {
	mgr, mc, _, fc := newIdleRig(t)
	cfg := IdleConfig{
		WarnAfter:      5 * time.Second,
		TimeoutAfter:   10 * time.Second,
		WarnMessage:    "warn",
		TimeoutMessage: "bye",
	}
	mc.Advance(2 * time.Second)
	mgr.IdleSweep(context.Background(), cfg, mc)
	if got := fc.writes(); len(got) != 0 {
		t.Errorf("pre-warn sweep wrote: %v", got)
	}
}

// At WarnAfter, IdleSweep delivers the warn message once. Subsequent
// sweeps within the same idle window do NOT re-warn.
func TestIdleSweep_WarnExactlyOnce(t *testing.T) {
	mgr, mc, _, fc := newIdleRig(t)
	cfg := IdleConfig{
		WarnAfter:      5 * time.Second,
		TimeoutAfter:   1 * time.Hour,
		WarnMessage:    "WARN_TEXT",
		TimeoutMessage: "bye",
	}
	mc.Advance(6 * time.Second)
	mgr.IdleSweep(context.Background(), cfg, mc)
	mgr.IdleSweep(context.Background(), cfg, mc) // second sweep, same window

	got := fc.writes()
	warnCount := 0
	for _, s := range got {
		if strings.Contains(s, "WARN_TEXT") {
			warnCount++
		}
	}
	if warnCount != 1 {
		t.Errorf("warn count = %d, want 1; writes=%v", warnCount, got)
	}
}

// After noteInput, the warn latch clears and the next quiet window
// produces a fresh warn.
func TestIdleSweep_WarnReArmsAfterInput(t *testing.T) {
	mgr, mc, a, fc := newIdleRig(t)
	cfg := IdleConfig{
		WarnAfter:      5 * time.Second,
		TimeoutAfter:   1 * time.Hour,
		WarnMessage:    "WARN_TEXT",
		TimeoutMessage: "bye",
	}
	mc.Advance(6 * time.Second)
	mgr.IdleSweep(context.Background(), cfg, mc) // first warn

	// Input resets idle bookkeeping.
	mc.Advance(1 * time.Second)
	a.noteInput(mc.Now())

	// Idle out again.
	mc.Advance(6 * time.Second)
	mgr.IdleSweep(context.Background(), cfg, mc)

	warnCount := 0
	for _, s := range fc.writes() {
		if strings.Contains(s, "WARN_TEXT") {
			warnCount++
		}
	}
	if warnCount != 2 {
		t.Errorf("warn count after re-arm = %d, want 2", warnCount)
	}
}

// At TimeoutAfter, IdleSweep sends the timeout message AND closes the
// underlying connection.
func TestIdleSweep_TimeoutClosesConn(t *testing.T) {
	mgr, mc, _, fc := newIdleRig(t)
	cfg := IdleConfig{
		WarnAfter:      5 * time.Second,
		TimeoutAfter:   10 * time.Second,
		WarnMessage:    "warn",
		TimeoutMessage: "GOODBYE_IDLE",
	}
	mc.Advance(11 * time.Second)
	mgr.IdleSweep(context.Background(), cfg, mc)

	found := false
	for _, s := range fc.writes() {
		if strings.Contains(s, "GOODBYE_IDLE") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("timeout message missing; writes=%v", fc.writes())
	}
	if !fc.closed() {
		t.Errorf("conn not closed after idle timeout")
	}
}

// A second sweep that arrives before the read loop has unwound the
// disconnect MUST NOT re-deliver the timeout message or re-close the
// connection.
func TestIdleSweep_TimeoutIsIdempotent(t *testing.T) {
	mgr, mc, _, fc := newIdleRig(t)
	cfg := IdleConfig{
		WarnAfter:      5 * time.Second,
		TimeoutAfter:   10 * time.Second,
		WarnMessage:    "warn",
		TimeoutMessage: "GOODBYE",
	}
	mc.Advance(11 * time.Second)
	mgr.IdleSweep(context.Background(), cfg, mc)
	mgr.IdleSweep(context.Background(), cfg, mc) // second sweep, same state

	goodbyes := 0
	for _, s := range fc.writes() {
		if strings.Contains(s, "GOODBYE") {
			goodbyes++
		}
	}
	if goodbyes != 1 {
		t.Errorf("timeout message delivered %d times, want 1", goodbyes)
	}
}

// A misconfigured policy (WarnAfter >= TimeoutAfter) must not produce
// a warn message — the timeout fires first. We don't assert the slog
// output but we do assert no warn was emitted.
func TestIdleSweep_InvertedThresholdsSkipsWarn(t *testing.T) {
	mgr, mc, _, fc := newIdleRig(t)
	cfg := IdleConfig{
		WarnAfter:      10 * time.Second,
		TimeoutAfter:   5 * time.Second, // inverted
		WarnMessage:    "WARN",
		TimeoutMessage: "BYE",
	}
	mc.Advance(11 * time.Second)
	mgr.IdleSweep(context.Background(), cfg, mc)

	for _, s := range fc.writes() {
		if strings.Contains(s, "WARN") {
			t.Errorf("inverted-threshold sweep delivered warn: %q", s)
		}
	}
}

// A first-tick session (lastInputAt unset) is never timed out. Guards
// against the case where the sweep fires before run() initializes
// lastInputAt.
func TestIdleSweep_ZeroLastInputIsImmune(t *testing.T) {
	mc := clock.NewManual(time.Now())
	mgr := NewManager()
	r := &world.Room{ID: "x:1"}
	a, fc := newFakeActor("c1", "p1", "acc1", "Alice", r)
	// Deliberately leave lastInputAt as zero.
	mgr.Add(a)

	mc.Advance(1 * time.Hour)
	mgr.IdleSweep(context.Background(), DefaultIdleConfig(), mc)
	if len(fc.writes()) != 0 || fc.closed() {
		t.Errorf("zero-lastInputAt actor was disturbed: writes=%v closed=%v",
			fc.writes(), fc.closed())
	}
}
