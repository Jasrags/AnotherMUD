//go:build unix

package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_WeaveInterruptedByHit is the WoT S2 Phase 4 slice-2 regression test:
// a hit on a mid-cast channeler ABORTS the weave (the interrupt game). A
// channeler engages a wild boar and repeatedly weaves Bonds of Air (cast_time
// 2 rounds, the longest warmup) at it; while the boar trades blows, one of its
// hits during the two-round warmup disrupts the weave — surfaced as
// "Your weave of Bonds of Air is disrupted!".
//
// Combat hits are probabilistic, so the test re-attempts the weave across a
// generous window rather than asserting a single cast is disrupted: over many
// rounds of an actively-attacking boar a hit landing inside a 2-round warmup is
// near-certain. It fails only if NO weave is disrupted in the whole window,
// which would mean the interrupt path is dead.
func TestLive_WeaveInterruptedByHit(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "wot",
		"ANOTHERMUD_START_ROOM": "wot:deep-westwood",
	})

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createChanneler(c, "Disrupt", "male"); err != nil {
		t.Fatalf("create channeler: %v", err)
	}
	if err := engageBoar(c); err != nil {
		t.Fatalf("engage boar: %v", err)
	}

	// Outcome of one weave attempt: disrupted (win), resolved, or the boar
	// died / wandered (re-engage and retry).
	outcome := regexp.MustCompile(`is disrupted|cast Bonds of Air|isn't here|don't see|no longer`)
	// ~75s ≈ 25 combat rounds at the default 3s cadence — many dozens of
	// 2-round warmup windows for a boar swing to land in. Scale this if
	// ANOTHERMUD_COMBAT_CADENCE is raised in the test environment.
	deadline := time.Now().Add(75 * time.Second)
	for time.Now().Before(deadline) {
		if err := c.SendLine("channel bonds-of-air boar"); err != nil {
			t.Fatalf("send weave: %v", err)
		}
		out, err := c.ExpectTimeout(outcome, 15*time.Second)
		if err != nil {
			continue // no clear outcome in time; loop and retry
		}
		if strings.Contains(out, "is disrupted") {
			t.Log("interrupt verified: a boar's hit disrupted the in-flight weave")
			return
		}
		// Resolved uninterrupted, or the boar is gone — make sure we are still
		// in combat (a fresh boar respawns on the area reset) before retrying.
		if strings.Contains(out, "isn't here") || strings.Contains(out, "don't see") || strings.Contains(out, "no longer") {
			_ = engageBoar(c)
		}
	}
	t.Fatal("no weave was interrupted in 75s of sustained combat — the interrupt path appears dead")
}
