package session

import (
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
)

func newTestGate() (*floodGate, *clock.ManualClock, *[]string) {
	mc := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := FloodConfig{
		CommandsPerSecond:  2, // small numbers keep arithmetic obvious
		BurstSize:          3,
		StrikeThreshold:    3,
		StrikeDecaySeconds: 5,
	}
	var sent []string
	g := newFloodGate(cfg, mc)
	return g, mc, &sent
}

func collect(out *[]string) func(string) {
	return func(s string) { *out = append(*out, s) }
}

func TestFloodGate_DisabledAlwaysAllows(t *testing.T) {
	g := newFloodGate(FloodConfig{}, clock.NewManual(time.Unix(0, 0)))
	for i := 0; i < 50; i++ {
		if got := g.Check(nil); got != floodAllow {
			t.Fatalf("disabled gate returned %v at i=%d", got, i)
		}
	}
}

// First Check initializes tokens to BurstSize; that many consecutive
// calls succeed, the next one is dropped.
func TestFloodGate_BurstThenDrop(t *testing.T) {
	g, _, out := newTestGate()
	for i := 0; i < 3; i++ {
		if got := g.Check(collect(out)); got != floodAllow {
			t.Fatalf("burst call %d returned %v, want allow", i, got)
		}
	}
	if got := g.Check(collect(out)); got != floodDrop {
		t.Errorf("post-burst returned %v, want drop", got)
	}
	if len(*out) != 1 || (*out)[0] != "Slow down." {
		t.Errorf("warn output = %v, want one 'Slow down.'", *out)
	}
}

// "Slow down." is sent at most once per strike-decay cycle, regardless
// of how many drops accumulate.
func TestFloodGate_WarnOnceUntilDecay(t *testing.T) {
	g, mc, out := newTestGate()
	for i := 0; i < 3; i++ {
		g.Check(collect(out)) // drain burst
	}
	// Two drops, two strikes. Only one warn.
	g.Check(collect(out))
	g.Check(collect(out))
	if len(*out) != 1 {
		t.Errorf("got %d warn calls, want 1", len(*out))
	}
	// Advance past the decay window with no traffic; the next drop
	// should re-warn.
	mc.Advance(6 * time.Second)
	// After 6s with cps=2, burst should refill to cap (3). Drain it.
	for i := 0; i < 3; i++ {
		if got := g.Check(collect(out)); got != floodAllow {
			t.Fatalf("post-decay refill call %d returned %v", i, got)
		}
	}
	// Now drop again — fresh warn.
	g.Check(collect(out))
	if len(*out) != 2 {
		t.Errorf("got %d warn calls after decay, want 2", len(*out))
	}
}

// Tokens refill over time and the gate allows again.
func TestFloodGate_RefillOverTime(t *testing.T) {
	g, mc, out := newTestGate()
	for i := 0; i < 3; i++ {
		g.Check(collect(out)) // drain burst
	}
	if got := g.Check(collect(out)); got != floodDrop {
		t.Fatalf("immediate after burst = %v, want drop", got)
	}
	// cps=2 → 500ms per token; advance 1s → 2 tokens.
	mc.Advance(1 * time.Second)
	if got := g.Check(collect(out)); got != floodAllow {
		t.Errorf("after 1s refill = %v, want allow", got)
	}
	if got := g.Check(collect(out)); got != floodAllow {
		t.Errorf("second post-refill call = %v, want allow", got)
	}
}

// Hitting StrikeThreshold returns floodDisconnect AND every subsequent
// Check stays in floodDisconnect (so the caller tears down once and
// only once).
func TestFloodGate_StrikeThresholdDisconnects(t *testing.T) {
	g, _, out := newTestGate()
	// Drain burst.
	for i := 0; i < 3; i++ {
		g.Check(collect(out))
	}
	// 3 strikes to disconnect.
	var got floodDecision
	for i := 0; i < 3; i++ {
		got = g.Check(collect(out))
	}
	if got != floodDisconnect {
		t.Errorf("after 3 strikes got %v, want disconnect", got)
	}
	// Sticky: subsequent calls keep returning disconnect.
	if again := g.Check(collect(out)); again != floodDisconnect {
		t.Errorf("post-disconnect call = %v, want disconnect (sticky)", again)
	}
}

// Tokens cap at BurstSize even if a long quiet period elapses.
func TestFloodGate_TokenCap(t *testing.T) {
	g, mc, out := newTestGate()
	g.Check(collect(out)) // initialize and use one token (2 left)
	mc.Advance(1 * time.Hour)
	// Burst is 3; 2 + many >> 3 must cap at 3, so exactly 3 allows.
	for i := 0; i < 3; i++ {
		if got := g.Check(collect(out)); got != floodAllow {
			t.Fatalf("cap-test allow %d = %v", i, got)
		}
	}
	if got := g.Check(collect(out)); got != floodDrop {
		t.Errorf("post-cap drain = %v, want drop", got)
	}
}
