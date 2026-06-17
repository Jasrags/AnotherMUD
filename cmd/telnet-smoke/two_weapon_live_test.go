//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_TwoWeaponEquip proves two-weapon fighting (S1.K) slice 1 end to end
// on the WoT boot: the Smithy stocks a one-handed Two Rivers longsword and a
// SMALL (⇒ light, off-hand eligible) belt knife. A medium character:
//   - wields the longsword in the main hand, then
//   - equips the belt knife to the OFF hand (the `equip knife offhand` route),
//   - sees both blades occupy their two hands in the equipment listing.
//
// The off-hand ATTACK mechanic itself (the extra swing, the penalties, the ½×
// Strength) is covered deterministically by the unit + combat-loop tests; the
// attacker-facing hit line carries no weapon name, so this live check proves
// the content + slot-routing path rather than asserting per-round swing counts.
func TestLive_TwoWeaponEquip(t *testing.T) {
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
	if err := createChanneler(c, "Dualist", "male"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	// Main hand: the one-handed longsword.
	if err := c.SendLine("get longsword"); err != nil {
		t.Fatalf("get longsword: %v", err)
	}
	if err := c.SendLine("equip longsword"); err != nil {
		t.Fatalf("equip longsword: %v", err)
	}
	if _, err := c.ExpectStringTimeout("You equip a Two Rivers longsword", 5*time.Second); err != nil {
		t.Fatalf("expected the longsword wielded: %v", err)
	}

	// Off hand: the light belt knife, routed to the off-hand slot explicitly
	// (the knife is eligible for both hands, so the slot must be named).
	if err := c.SendLine("get knife"); err != nil {
		t.Fatalf("get knife: %v", err)
	}
	if err := c.SendLine("equip knife offhand"); err != nil {
		t.Fatalf("equip knife offhand: %v", err)
	}
	if _, err := c.ExpectStringTimeout("You equip a belt knife", 5*time.Second); err != nil {
		t.Fatalf("expected the belt knife in the off hand: %v", err)
	}

	// Both blades show in the two hands. Anchor on the listing header first so
	// the match can't land on a buffered equip cue (e.g. the non-proficient
	// "You handle a belt knife clumsily" line). Rows render in slot order —
	// wield (the longsword) before offhand (the knife) — so the window from the
	// header through "belt knife" contains both.
	if err := c.SendLine("equipment"); err != nil {
		t.Fatalf("equipment: %v", err)
	}
	if _, err := c.ExpectStringTimeout("You are wearing:", 5*time.Second); err != nil {
		t.Fatalf("equipment listing header not seen: %v", err)
	}
	out, err := c.ExpectStringTimeout("belt knife", 5*time.Second)
	if err != nil {
		t.Fatalf("equipment listing did not show the belt knife: %v", err)
	}
	if !strings.Contains(out, "Two Rivers longsword") {
		t.Errorf("equipment listing missing the longsword (off-hand knife should not displace it): %q", out)
	}
	t.Logf("two-weapon equip verified; both blades wielded across the two hands")
}
