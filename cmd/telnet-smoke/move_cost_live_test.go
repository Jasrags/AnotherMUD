//go:build unix

package main

import (
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_MovementSpend proves the movement-cost gate is live end to end: a
// fresh character starts with a full movement pool (the DefaultPlayerBase
// movement_max), and walking between rooms spends it one point per step. The
// before/after MV cells on the score sheet are the proof. Each step waits for
// the room view so the commands are processed serially before the follow-up
// score (a fire-and-forget burst races the score read).
func TestLive_MovementSpend(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // default starter-world boot, start = town-square

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Walker"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	cur, max, err := scoreMovement(c)
	if err != nil {
		t.Fatalf("read starting movement: %v", err)
	}
	if max <= 0 {
		t.Fatalf("fresh character has no movement pool (max %d); DefaultPlayerBase movement_max not applied", max)
	}
	if cur != max {
		t.Fatalf("fresh character should start with a full pool, got %d/%d", cur, max)
	}

	// Round-trip between town-square and the forge to the north. town-square
	// has north->forge and forge has south->town-square, so every step is a
	// real traversal that spends a point.
	const steps = 12
	roomMarker := regexp.MustCompile(`Forge|Town Square|cannot go`)
	for i := 0; i < steps/2; i++ {
		for _, dir := range []string{"north", "south"} {
			if err := c.SendLine(dir); err != nil {
				t.Fatalf("send %s: %v", dir, err)
			}
			if _, err := c.ExpectTimeout(roomMarker, 4*time.Second); err != nil {
				t.Fatalf("no room view after %s (step %d): %v", dir, i, err)
			}
		}
	}

	after, _, err := scoreMovement(c)
	if err != nil {
		t.Fatalf("read movement after walking: %v", err)
	}
	if want := max - steps; after != want {
		t.Fatalf("movement after %d steps = %d/%d; want %d/%d (one point per step)", steps, after, max, want, max)
	}
	t.Logf("movement spend verified: %d/%d at start, %d/%d after %d steps", cur, max, after, max, steps)
}
