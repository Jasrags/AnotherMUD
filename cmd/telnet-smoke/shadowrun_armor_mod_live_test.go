//go:build unix

package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ArmorModHazardProtection proves the item-modification ↔ biome-hazard
// tie-in end to end: an armor mod slotted into a worn vest confers the hazard's
// protection key, so a runner who mods a vest with a chemical-seal kit walks the
// Glow unharmed — no dedicated enviro-suit required. It exercises the whole loop
// live: spawn → `modify <armor> <mod>` → equip → survive the radiation ticks.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ArmorModHazardProtection -v
func TestLive_ArmorModHazardProtection(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":                 "shadowrun",
		"ANOTHERMUD_START_ROOM":            "shadowrun:westlake-plaza",
		"ANOTHERMUD_ROLE_SEED":             "Runner:admin",
		"ANOTHERMUD_BIOME_HAZARD_INTERVAL": "1s",
		"ANOTHERMUD_START_HOUR":            "12",
	})

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	if err := createAndLogin(c, "Runner"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	send := func(cmd string) string {
		t.Helper()
		if err := c.SendLine(cmd); err != nil {
			t.Fatalf("send %q: %v", cmd, err)
		}
		return c.Drain(700 * time.Millisecond)
	}
	seared := func(s string) bool {
		s = strings.ToLower(s)
		return strings.Contains(s, "sears through you") || strings.Contains(s, "the glow sears")
	}

	// Self-provision an armored jacket (light — capacity 12; dons instantly): the
	// default-created runner's origin grants a vest, not a jacket, and Westlake no
	// longer drops a go-bag (creation gear replaced it), so spawn exactly one so
	// "jacket" stays unambiguous. Wear it, then mod it WHILE WORN (item-mod §5).
	send("restore")
	send("spawn item armored-jacket me")
	send("spawn item chemical-seal me")
	if out := send("equip jacket"); !strings.Contains(strings.ToLower(out), "equip") {
		t.Fatalf("could not equip the spawned jacket:\n%s", out)
	}
	if out := send("modify jacket seal"); !strings.Contains(strings.ToLower(out), "install") {
		t.Fatalf("modify-while-worn did not install the mod:\n%s", out)
	}

	// Phase 1: a mod installed while worn confers immunity immediately — no sear.
	base, ok := hpFrom(send("look"))
	if !ok {
		t.Fatal("could not read baseline HP from the prompt")
	}
	send("teleport shadowrun:glow-city")
	quiet := c.Drain(3000 * time.Millisecond) // ~3 hazard ticks
	back := send("teleport shadowrun:westlake-plaza")
	if seared(quiet) {
		t.Fatalf("a mod installed while worn did not protect against the Glow:\n%s", quiet)
	}
	if hp, ok := hpFrom(back); ok && base > 0 && hp < base {
		t.Fatalf("mod-sealed HP dropped in the Glow: before %d, after %d (immunity failed)", base, hp)
	}

	// Phase 2: REMOVE the mod while still worn — the protection must reverse live,
	// so the Glow bites again on the next visit (item-modification §5).
	send("restore")
	if out := send("unmodify jacket seal"); !strings.Contains(strings.ToLower(out), "pocket") {
		t.Fatalf("unmodify-while-worn did not remove the mod:\n%s", out)
	}
	send("teleport shadowrun:glow-city")
	bitten := c.Drain(2500 * time.Millisecond)
	send("teleport shadowrun:westlake-plaza")
	if !seared(bitten) {
		t.Fatalf("removing the seal while worn did not reverse the protection (still immune):\n%s", bitten)
	}
}

// hpFrom pulls current HP off the default prompt; shared with the biome-hazard
// live test's hpRe.
func hpFrom(s string) (int, bool) {
	m := hpRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	hp, _ := strconv.Atoi(m[1])
	return hp, true
}
