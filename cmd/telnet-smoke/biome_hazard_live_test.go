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
//   - A runner WEARING the sealed enviro-suit (tag `rad-shielded`, the hazard's
//     protection key, equipped to the body slot) is IMMUNE: standing in the same
//     room across multiple hazard ticks produces no searing line and no HP loss.
//
//   - The same runner with the suit only CARRIED (unequipped back to the pack)
//     is harmed again — proving protection is wear-only, not carry-or-wear.
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

	// --- Phase 2: WEARING the suit → immunity. -------------------------------
	// Protection is wear-only: spawn the suit, then equip it to the body slot.
	// Carrying it in the pack must NOT protect (that's Phase 3).
	send("restore")
	send("spawn item rad-suit me")
	if out := send("equip suit"); !strings.Contains(strings.ToLower(out), "suit") {
		t.Fatalf("could not equip the enviro-suit to the body slot:\n%s", out)
	}
	shielded, _ := hpFrom(send("look"))

	send("teleport shadowrun:glow-city")
	quiet := c.Drain(3000 * time.Millisecond) // spans ~3 hazard ticks
	back := send("teleport shadowrun:westlake-plaza")

	if strings.Contains(strings.ToLower(quiet), "sears through you") ||
		strings.Contains(strings.ToLower(quiet), "the glow sears") {
		t.Fatalf("worn suit did not protect against the Glow (protection key ignored):\n%s", quiet)
	}
	if hp, ok := hpFrom(back); ok && shielded > 0 && hp < shielded {
		t.Fatalf("worn-suit HP dropped in the Glow: before %d, after %d (immunity failed)", shielded, hp)
	}

	// --- Phase 3: carrying the suit (not worn) → NO immunity. ----------------
	// Wear-only: unequip the suit back to the pack, and the Glow must bite again.
	send("restore")
	if out := send("unequip suit"); !strings.Contains(strings.ToLower(out), "suit") {
		t.Fatalf("could not unequip the enviro-suit:\n%s", out)
	}
	send("teleport shadowrun:glow-city")
	carried := c.Drain(2500 * time.Millisecond)
	send("teleport shadowrun:westlake-plaza")

	if !strings.Contains(strings.ToLower(carried), "sears through you") &&
		!strings.Contains(strings.ToLower(carried), "the glow sears") {
		t.Fatalf("carrying the suit (not worn) wrongly protected against the Glow:\n%s", carried)
	}
}
