//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_CrossRoomShot proves ranged-combat Model C (§10) end to end: a player
// snipes a mob in the ADJACENT room, and the shot mob retaliates by charging
// into the player's room (slice 2). Boots in the WoT Smithy (the-forge), which
// stocks a simple hunting bow + arrows and sits one step WEST of the Quarry
// Road, where a hostile, STATIONARY brigand archer waits.
//
//	the-forge (player)  --east-->  quarry-road (brigand archer)
//
// The player wields the bow, gets arrows, and `shoot archer east` looses into
// the Quarry Road (slice 1 — the cross-room shot). The brigand survives a single
// hunting-bow shot (20 HP), so it takes the grudge and, on the next AI tick,
// charges back WEST into the Smithy — narrated there as arriving "from the east"
// (slice 2 — retaliation preempting its stationary behavior). Daylight so the
// outdoor Quarry Road is lit (you can't aim into a room you can't see).
func TestLive_CrossRoomShot(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "wot",
		"ANOTHERMUD_START_ROOM": "wot:the-forge",
		"ANOTHERMUD_START_HOUR": "12", // daylight — the outdoor Quarry Road is lit
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	// Any WoT character works — the hunting bow is a SIMPLE weapon, so every
	// class is proficient with it (no non-proficient miss penalty).
	if err := createChanneler(c, "Sniper", "male"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	// Arm: grab the bow + arrows from the Smithy and wield the bow.
	for _, cmd := range []string{"get hunting-bow", "get arrow", "get arrow", "get arrow", "wield hunting-bow"} {
		if err := c.SendLine(cmd); err != nil {
			t.Fatalf("send %q: %v", cmd, err)
		}
	}
	// Confirm the bow is wielded before shooting (so a failed pickup/equip fails
	// here with a clear message rather than as a confusing "nothing to shoot").
	if _, err := c.ExpectStringTimeout("hunting bow", 5*time.Second); err != nil {
		t.Fatalf("expected to wield the hunting bow: %v", err)
	}

	// Slice 1 — the cross-room shot into the adjacent Quarry Road.
	if err := c.SendLine("shoot archer east"); err != nil {
		t.Fatalf("send shoot: %v", err)
	}
	if _, err := c.ExpectStringTimeout("loose a shot to the east", 8*time.Second); err != nil {
		t.Fatalf("expected the cross-room shot to fire east: %v", err)
	}

	// Slice 2 — the shot brigand charges back into the Smithy to retaliate,
	// arriving from the east (it stepped west out of the Quarry Road).
	if _, err := c.ExpectStringTimeout("charges in from the east", 12*time.Second); err != nil {
		t.Fatalf("expected the shot mob to charge in and retaliate: %v", err)
	}
	t.Log("cross-room shot verified: sniped a mob next door and it charged in to retaliate")
}
