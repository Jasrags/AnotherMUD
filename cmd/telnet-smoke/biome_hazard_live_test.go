//go:build unix

package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// hpRe pulls current HP out of the default prompt ("[HP 30/30] …").
var hpRe = regexp.MustCompile(`\[HP (\d+)/(\d+)\]`)

// TestLive_BiomeHazard proves area-effects.md §4.6 (biome ambient hazards)
// end to end against the Shadowrun Glow City content:
//
//   - An UNSHIELDED runner who teleports into the `toxic` biome (Glow City)
//     takes the radiation payload on the hazard tick: they see the searing
//     room-copy line and their HP drops. Environmental — no attacker.
//
//   - A runner carrying the sealed enviro-suit (tag `rad-shielded`, the
//     hazard's protection key) is IMMUNE: standing in the same room across
//     multiple hazard ticks produces no searing line and no HP loss. This
//     also exercises the carry-OR-wear rule (the suit is only in inventory).
//
//     ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_BiomeHazard -v
func TestLive_BiomeHazard(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:westlake-plaza", // safe start
		// Admin so the runner can teleport into the Glow and spawn the suit.
		"ANOTHERMUD_ROLE_SEED": "Ghoul:admin",
		// Tick the hazard fast so the test window is short and deterministic.
		"ANOTHERMUD_BIOME_HAZARD_INTERVAL": "1s",
		// Full daylight keeps unrelated darkness friction out of the readout.
		"ANOTHERMUD_START_HOUR": "12",
	})

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	if err := createAndLogin(c, "Ghoul"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	send := func(cmd string) string {
		t.Helper()
		if err := c.SendLine(cmd); err != nil {
			t.Fatalf("send %q: %v", cmd, err)
		}
		return c.Drain(700 * time.Millisecond)
	}
	hpFrom := func(s string) (int, bool) {
		m := hpRe.FindStringSubmatch(s)
		if m == nil {
			return 0, false
		}
		hp, _ := strconv.Atoi(m[1])
		return hp, true
	}

	// --- Phase 1: unshielded → the Glow harms. -------------------------------
	// Restore to full and read the baseline HP off the prompt.
	send("restore")
	base, ok := hpFrom(send("look"))
	if !ok {
		t.Fatal("could not read baseline HP from the prompt")
	}

	// Step into the Glow and dwell across a couple hazard ticks.
	send("teleport shadowrun:glow-city")
	seared := c.Drain(2500 * time.Millisecond)
	// Teleport back to safety immediately so the char can't die mid-assert.
	after := send("teleport shadowrun:westlake-plaza")

	if !strings.Contains(strings.ToLower(seared), "sears through you") &&
		!strings.Contains(strings.ToLower(seared), "the glow sears") {
		t.Fatalf("unshielded runner saw no radiation line in the Glow:\n%s", seared)
	}
	if hp, ok := hpFrom(after); ok {
		if hp >= base {
			t.Fatalf("unshielded HP did not drop in the Glow: baseline %d, after %d", base, hp)
		}
	} else {
		// Fall back to a fresh prompt if the teleport line carried none.
		if hp, ok := hpFrom(send("look")); ok && hp >= base {
			t.Fatalf("unshielded HP did not drop in the Glow: baseline %d, after %d", base, hp)
		}
	}

	// --- Phase 2: carrying the suit → immunity. ------------------------------
	send("restore")
	if out := send("spawn item rad-suit me"); !strings.Contains(strings.ToLower(out), "suit") &&
		!strings.Contains(strings.ToLower(out), "spawn") {
		// Non-fatal: some spawn confirmations are terse. Verify via inventory.
		if inv := send("inventory"); !strings.Contains(strings.ToLower(inv), "suit") {
			t.Fatalf("rad-suit was not conjured into inventory:\n%s", inv)
		}
	}
	shielded, _ := hpFrom(send("look"))

	send("teleport shadowrun:glow-city")
	quiet := c.Drain(3000 * time.Millisecond) // spans ~3 hazard ticks
	back := send("teleport shadowrun:westlake-plaza")

	if strings.Contains(strings.ToLower(quiet), "sears through you") ||
		strings.Contains(strings.ToLower(quiet), "the glow sears") {
		t.Fatalf("shielded runner was harmed by the Glow (protection key ignored):\n%s", quiet)
	}
	if hp, ok := hpFrom(back); ok && shielded > 0 && hp < shielded {
		t.Fatalf("shielded HP dropped in the Glow: before %d, after %d (immunity failed)", shielded, hp)
	}
}
