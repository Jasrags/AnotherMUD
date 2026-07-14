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

	// Mod a modifiable vest with a chemical-seal kit, then wear it.
	// Use the LIGHT armored jacket (also a modifiable host, capacity 12): light
	// armor dons instantly, so it is actually worn before we step into the Glow —
	// a medium vest triggers the slow-armor don timer and would still be buckling.
	send("restore")
	send("spawn item armored-jacket me")
	send("spawn item chemical-seal me")
	if out := send("modify jacket seal"); !strings.Contains(strings.ToLower(out), "install") {
		t.Fatalf("modify jacket seal did not install the mod:\n%s", out)
	}
	if out := send("equip jacket"); !strings.Contains(strings.ToLower(out), "equip") {
		t.Fatalf("could not equip the modded jacket:\n%s", out)
	}

	// Into the Glow: the mod-granted rad-shielded key must confer immunity across
	// several hazard ticks — no searing line.
	base, ok := hpFrom(send("look"))
	if !ok {
		t.Fatal("could not read baseline HP from the prompt")
	}
	send("teleport shadowrun:glow-city")
	quiet := c.Drain(3000 * time.Millisecond) // ~3 hazard ticks
	back := send("teleport shadowrun:westlake-plaza")

	if seared(quiet) {
		t.Fatalf("mod-sealed vest did not protect against the Glow (mod protection ignored):\n%s", quiet)
	}
	if hp, ok := hpFrom(back); ok && base > 0 && hp < base {
		t.Fatalf("mod-sealed HP dropped in the Glow: before %d, after %d (mod immunity failed)", base, hp)
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
