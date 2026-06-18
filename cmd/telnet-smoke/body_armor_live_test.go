//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_BodyArmorRaisesAC proves the equipment.md armor authoring pass made
// body armor LIVE, not inert: the new `body` equipment slot accepts a torso
// armor (a leather jerkin), and its armor_bonus reaches Armor Class through the
// armor-depth defense channel (it sums armor across every equipped slot, so the
// new slot contributes). A light jerkin on an Initiate (trained in light armor)
// equips cleanly and raises AC by at least its armor bonus.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_BodyArmor -v
func TestLive_BodyArmorRaisesAC(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "wot",
		"ANOTHERMUD_START_ROOM": "wot:the-forge",
		"ANOTHERMUD_START_HOUR": "12",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createChanneler(c, "Bodyarmor", "male"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	before, err := scoreArmorClass(c)
	if err != nil {
		t.Fatalf("AC before: %v", err)
	}

	// A leather jerkin (light, armor_bonus 2) worn on the new `body` slot —
	// trained light armor, so it equips cleanly.
	if err := c.SendLine("get leather"); err != nil {
		t.Fatalf("get leather: %v", err)
	}
	if err := c.SendLine("equip leather"); err != nil {
		t.Fatalf("equip leather: %v", err)
	}
	if _, err := c.ExpectStringTimeout("You equip a leather jerkin", 5*time.Second); err != nil {
		t.Fatalf("the leather jerkin did not equip onto the body slot: %v", err)
	}

	after, err := scoreArmorClass(c)
	if err != nil {
		t.Fatalf("AC after: %v", err)
	}
	if after-before < 2 {
		t.Errorf("body armor AC delta = %d (before %d, after %d), want >= 2 (the jerkin's armor_bonus reaching AC via the body slot)", after-before, before, after)
	}
	t.Logf("body-slot armor verified: leather jerkin raised AC %d -> %d", before, after)
}
