//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_MedkitTreatAndRefill proves the medkit/First-Aid heal arc end to end:
// a flat-heal stimpatch (no skill), a First-Aid `treat` off a rated medkit that
// spends a charge, and a `refill` that restocks a spent kit from a supply box
// (and no-ops on a full one). The stimpatch is the deterministic heal assertion;
// `treat` rides a skill roll (the package RNG is not seeded), so it is retried
// under a full-certainty setup — First Aid granted + a rating-6 trauma kit — so
// only a nat-1 misses and a short retry loop makes the check reliable.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_MedkitTreatAndRefill -v
func TestLive_MedkitTreatAndRefill(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:westlake-plaza",
		"ANOTHERMUD_ROLE_SEED":  "Runner:admin",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	if err := createAndLogin(c, "Runner"); err != nil {
		t.Fatalf("create+login: %v", err)
	}
	send := func(line string) string {
		t.Helper()
		if err := c.SendLine(line); err != nil {
			t.Fatalf("send %q: %v", line, err)
		}
		out, err := c.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
		return out
	}
	has := func(out, want string) bool { return strings.Contains(strings.ToLower(out), strings.ToLower(want)) }

	// First Aid + a rating-6 trauma kit make `treat` near-certain; the stimpatch
	// and supply box round out the kit.
	send("grant ability first-aid to Runner")
	send("spawn item trauma-kit me")
	send("spawn item stimpatch me")
	send("spawn item medkit-supplies me")

	// --- Stimpatch: a flat heal via `use`, no skill roll (deterministic). ---
	send("set vital hp self 5")
	out := send("use stimpatch")
	if !has(out, "relief") {
		t.Fatalf("using a stimpatch did not confirm a heal:\n%s", out)
	}
	if hp, ok := hpFrom(out); ok && hp <= 5 {
		t.Fatalf("stimpatch did not raise HP (still %d, wounded at 5)", hp)
	}

	// --- Treat: a First-Aid check off the medkit. Retry under the near-certain
	// setup so the rare nat-1 doesn't flake the smoke; each attempt re-wounds. ---
	healed := false
	for i := 0; i < 8 && !healed; i++ {
		send("set vital hp self 5")
		out = send("treat")
		if has(out, "patch yourself up") {
			healed = true
			if hp, ok := hpFrom(out); ok && hp <= 5 {
				t.Fatalf("treat reported a heal but HP did not rise (still %d):\n%s", hp, out)
			}
		}
	}
	if !healed {
		t.Fatalf("treat never healed across 8 attempts (First Aid granted + rating-6 kit):\n%s", out)
	}

	// --- Refill: the treat loop spent at least one charge, so the kit is below
	// its max — a supply box restocks it to full. ---
	send("restore")
	if out = send("refill"); !has(out, "restock") || !has(out, "15/15") {
		t.Fatalf("refill did not restock the trauma kit to 15/15:\n%s", out)
	}

	// A full kit is a no-op — the full-stock check precedes the supplies check,
	// so this reports "already fully stocked" without needing another box.
	if out = send("refill"); !has(out, "already fully stocked") {
		t.Fatalf("refilling a full kit should report it is already stocked:\n%s", out)
	}
}
