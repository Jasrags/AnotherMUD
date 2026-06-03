package session

import (
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
)

func newTestGate() (*floodGate, *clock.ManualClock) {
	mc := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := FloodConfig{
		CommandsPerSecond:  2, // small numbers keep arithmetic obvious
		BurstSize:          3,
		StrikeThreshold:    3,
		StrikeDecaySeconds: 5,
	}
	return newFloodGate(cfg, mc), mc
}

// The inbound-GMCP gate allows a burst, then DROPS over-rate frames, and
// NEVER disconnects (StrikeThreshold 0) — a Tab-spamming client keeps its
// command channel; only its excess GMCP is shed.
func TestGmcpFloodGate_BurstThenDropNeverDisconnects(t *testing.T) {
	mc := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	g := newFloodGate(gmcpFloodConfig(DefaultFloodConfig()), mc)
	burst := int(DefaultFloodConfig().BurstSize * 2) // gmcp burst = 2× command

	for i := 0; i < burst; i++ {
		if d, _ := g.Check(); d != floodAllow {
			t.Fatalf("frame %d/%d dropped within burst", i, burst)
		}
	}
	// Past the burst with no time advance: dropped, not disconnected.
	if d, _ := g.Check(); d != floodDrop {
		t.Errorf("over-burst decision = %v, want floodDrop", d)
	}
	// Keep spamming: never escalates to disconnect.
	for i := 0; i < 100; i++ {
		if d, _ := g.Check(); d == floodDisconnect {
			t.Fatalf("GMCP gate disconnected at spam %d (StrikeThreshold must be 0)", i)
		}
	}
}

// A zero command flood config (the test default) yields a disabled GMCP
// gate, so tests aren't throttled.
func TestGmcpFloodConfig_DisabledWhenCommandFloodZero(t *testing.T) {
	g := newFloodGate(gmcpFloodConfig(FloodConfig{}), clock.NewManual(time.Unix(0, 0)))
	if d, _ := g.Check(); d != floodAllow {
		t.Errorf("zero command config should disable the GMCP gate, got %v", d)
	}
}

func TestFloodGate_DisabledAlwaysAllows(t *testing.T) {
	g := newFloodGate(FloodConfig{}, clock.NewManual(time.Unix(0, 0)))
	for i := 0; i < 50; i++ {
		d, warn := g.Check()
		if d != floodAllow || warn {
			t.Fatalf("disabled gate i=%d: decision=%v warn=%v", i, d, warn)
		}
	}
}

// First Check initializes tokens to BurstSize; that many consecutive
// calls succeed, the next one is dropped and warns once.
func TestFloodGate_BurstThenDrop(t *testing.T) {
	g, _ := newTestGate()
	for i := 0; i < 3; i++ {
		if d, _ := g.Check(); d != floodAllow {
			t.Fatalf("burst call %d returned %v, want allow", i, d)
		}
	}
	d, warn := g.Check()
	if d != floodDrop {
		t.Errorf("post-burst returned %v, want drop", d)
	}
	if !warn {
		t.Errorf("first drop did not warn")
	}
}

// "Slow down." is signaled at most once per strike-decay cycle.
func TestFloodGate_WarnOnceUntilDecay(t *testing.T) {
	g, mc := newTestGate()
	for i := 0; i < 3; i++ {
		g.Check() // drain burst
	}
	_, warnA := g.Check()
	_, warnB := g.Check()
	if !warnA || warnB {
		t.Errorf("first drop warn=%v, second=%v; want true, false", warnA, warnB)
	}
	// Advance past the decay window with no traffic; the next drop
	// after the next allow cycle should re-warn.
	mc.Advance(6 * time.Second)
	// cps=2 → 6s refills past cap. Drain.
	for i := 0; i < 3; i++ {
		if d, _ := g.Check(); d != floodAllow {
			t.Fatalf("post-decay refill call %d = %v", i, d)
		}
	}
	_, warnAfterDecay := g.Check()
	if !warnAfterDecay {
		t.Errorf("drop after decay did not re-warn")
	}
}

// A drop that arrives at the exact decay boundary should reset and
// re-warn — no quiet allow in between.
func TestFloodGate_DropImmediatelyAfterDecayReWarns(t *testing.T) {
	g, mc := newTestGate()
	// Drain burst and rack up 1 strike (with warn).
	for i := 0; i < 3; i++ {
		g.Check()
	}
	if _, warn := g.Check(); !warn {
		t.Fatal("setup: expected initial warn")
	}
	// Advance just past decay AND refill so the next drop triggers
	// the decay path mid-Check.
	mc.Advance(6 * time.Second)
	// Drain refilled tokens (cap=3).
	for i := 0; i < 3; i++ {
		g.Check()
	}
	// Now drop: decay should reset warned=false, and this drop must
	// warn fresh.
	_, warn := g.Check()
	if !warn {
		t.Errorf("first drop after decay did not warn")
	}
}

// Tokens refill over time and the gate allows again.
func TestFloodGate_RefillOverTime(t *testing.T) {
	g, mc := newTestGate()
	for i := 0; i < 3; i++ {
		g.Check() // drain burst
	}
	if d, _ := g.Check(); d != floodDrop {
		t.Fatalf("immediate after burst = %v, want drop", d)
	}
	// cps=2 → 500ms per token; advance 1s → 2 tokens.
	mc.Advance(1 * time.Second)
	if d, _ := g.Check(); d != floodAllow {
		t.Errorf("after 1s refill = %v, want allow", d)
	}
	if d, _ := g.Check(); d != floodAllow {
		t.Errorf("second post-refill call = %v, want allow", d)
	}
}

// Hitting StrikeThreshold returns floodDisconnect AND every subsequent
// Check stays in floodDisconnect (so the caller tears down once and
// only once).
func TestFloodGate_StrikeThresholdDisconnects(t *testing.T) {
	g, _ := newTestGate()
	for i := 0; i < 3; i++ {
		g.Check() // drain burst
	}
	var got floodDecision
	for i := 0; i < 3; i++ {
		got, _ = g.Check()
	}
	if got != floodDisconnect {
		t.Errorf("after 3 strikes got %v, want disconnect", got)
	}
	// Sticky: subsequent calls keep returning disconnect.
	if again, _ := g.Check(); again != floodDisconnect {
		t.Errorf("post-disconnect call = %v, want disconnect (sticky)", again)
	}
}

// Tokens cap at BurstSize even if a long quiet period elapses.
func TestFloodGate_TokenCap(t *testing.T) {
	g, mc := newTestGate()
	g.Check() // initialize and use one token (2 left)
	mc.Advance(1 * time.Hour)
	// Burst is 3; 2 + many >> 3 must cap at 3, so exactly 3 allows.
	for i := 0; i < 3; i++ {
		if d, _ := g.Check(); d != floodAllow {
			t.Fatalf("cap-test allow %d = %v", i, d)
		}
	}
	if d, _ := g.Check(); d != floodDrop {
		t.Errorf("post-cap drain = %v, want drop", d)
	}
}
