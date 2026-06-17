//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_RangedMob proves a bow-wielding mob fights at range end to end
// (ranged-combat MR1+MR3). A hostile brigand archer is placed on the Quarry
// Road wielding the Two Rivers longbow (a projectile). When a player steps in,
// it aggros and INITIATES the engagement — which opens at the FAR band because
// the opener's weapon is ranged (its ranged class flowed into combat.Stats via
// MR1). A melee/unarmed character then auto-CLOSES one band per round; the "now
// at near range" line is deterministic (no combat RNG) and appears only if the
// mob opened the fight at range. Boots in daylight so the outdoor road is lit
// (a mob can't see — or aggro — a target in the dark).
func TestLive_RangedMob(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "wot",
		"ANOTHERMUD_START_ROOM": "wot:the-forge",
		"ANOTHERMUD_START_HOUR": "12", // daylight — the archer can see (and aggro) the player outdoors
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	// Any WoT character works — unarmed is melee for bands, so it auto-closes.
	if err := createChanneler(c, "Closer", "male"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	// East from the Smithy onto the Quarry Road, where the hostile archer waits
	// (statically placed, present at boot). It aggros (mob-initiated engage →
	// opens at far) and looses while we close — we should see ourselves close to
	// near range.
	if err := c.SendLine("east"); err != nil {
		t.Fatalf("send east: %v", err)
	}
	if _, err := c.ExpectStringTimeout("near range", 12*time.Second); err != nil {
		t.Fatalf("expected to auto-close to near range against the ranged archer: %v", err)
	}
	t.Log("ranged mob verified: a bow-wielding archer opened at far and we closed the distance")
}

// TestLive_RangedMobKites proves the MR2 kiting AI end to end: with the kite
// chance pinned to 100%, the ranged archer ALWAYS opens the distance instead of
// shooting once a foe has closed inside far. After the player auto-closes to
// near, the archer withdraws — narrated to the room as "opens the distance …
// far range". Deterministic (kite chance 100, daylight so the archer can see).
func TestLive_RangedMobKites(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":       "wot",
		"ANOTHERMUD_START_ROOM":  "wot:the-forge",
		"ANOTHERMUD_START_HOUR":  "12",
		"ANOTHERMUD_KITE_CHANCE": "100", // the archer always kites when it can
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createChanneler(c, "Chaser", "male"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	if err := c.SendLine("east"); err != nil {
		t.Fatalf("send east: %v", err)
	}
	// We close (near range); the archer then opens the distance back to far.
	if _, err := c.ExpectStringTimeout("opens the distance", 15*time.Second); err != nil {
		t.Fatalf("expected the ranged archer to kite (open the distance): %v", err)
	}
	t.Log("kiting AI verified: the archer opened the distance instead of letting us close")
}
