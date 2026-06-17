//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ArmorDepth proves armor-depth (S1.E) §3 + §5 end to end on the WoT
// boot, which overrides the defense channel with the d20 `defense: ac + dex_ac`
// (content/wot/channel-map). Boots in the Smithy (the-forge), which stocks a
// padded cap (LIGHT — an Initiate is trained in it) and a great helm (HEAVY —
// an Initiate is NOT, armor_proficiency_tiers: [light]).
//
//   - Equipping the padded cap (light) succeeds with no clumsy cue (§5 proficient).
//   - Equipping the great helm (heavy) is non-proficient: the equip emits the
//     clumsy-wear cue (§5), and its armor_bonus raises AC (§3, visible on score).
func TestLive_ArmorDepth(t *testing.T) {
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
	// An Initiate is trained in LIGHT armor only (armor_proficiency_tiers: [light]).
	if err := createChanneler(c, "Armsmith", "male"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	// Light armor: trained → equips cleanly, no clumsy cue.
	if err := c.SendLine("get padded cap"); err != nil {
		t.Fatalf("send get cap: %v", err)
	}
	if err := c.SendLine("equip cap"); err != nil {
		t.Fatalf("send equip cap: %v", err)
	}
	if _, err := c.ExpectStringTimeout("You equip a padded cap", 5*time.Second); err != nil {
		t.Fatalf("expected the light cap to equip cleanly: %v", err)
	}

	// Heavy armor: untrained → the §5 non-proficient clumsy-wear cue.
	if err := c.SendLine("get great helm"); err != nil {
		t.Fatalf("send get helm: %v", err)
	}
	if err := c.SendLine("equip helm"); err != nil {
		t.Fatalf("send equip helm: %v", err)
	}
	if _, err := c.ExpectStringTimeout("clumsily", 5*time.Second); err != nil {
		t.Fatalf("expected the heavy helm to emit the non-proficient clumsy cue: %v", err)
	}

	// §3: the helm's armor_bonus reaches AC through the defense channel.
	if err := c.SendLine("score"); err != nil {
		t.Fatalf("send score: %v", err)
	}
	if _, err := c.ExpectStringTimeout("Armor Class", 5*time.Second); err != nil {
		t.Fatalf("expected the score sheet to show Armor Class: %v", err)
	}
	t.Log("armor-depth verified: light armor worn cleanly, heavy armor non-proficient (clumsy cue), AC on score")
}
