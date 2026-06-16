//go:build unix

package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_MovementSpend proves the movement-cost gate is live end to end: a
// fresh character starts with a full movement pool (the DefaultPlayerBase
// movement_max), and walking between rooms spends it one point per step. The
// before/after MV cells on the score sheet are the proof. Each step waits for
// the prompt so the commands are processed serially before the follow-up
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

	// Round-trip between town-square and the forge to the north — both default
	// (cost-1) terrain, so each of 12 steps spends exactly one point.
	const steps = 12
	for i := 0; i < steps/2; i++ {
		walkStep(t, c, "north")
		walkStep(t, c, "south")
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

// TestLive_BiomeWeightedMovementCost proves a rough biome costs more than a
// step over easy ground. The route town-square -> village-gate -> meadow are
// default (cost-1) terrain; the final east step enters forest-edge, a forest
// (move_cost 2). So three steps spend 1 + 1 + 2 = 4 points, not 3.
func TestLive_BiomeWeightedMovementCost(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil)

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Strider"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	_, max, err := scoreMovement(c)
	if err != nil {
		t.Fatalf("read starting movement: %v", err)
	}

	walkStep(t, c, "south")              // town-square -> village-gate (cost 1)
	walkStep(t, c, "south")              // village-gate -> meadow      (cost 1)
	forestStep := walkStep(t, c, "east") // meadow -> forest-edge   (forest, cost 2)

	after, _, err := scoreMovement(c)
	if err != nil {
		t.Fatalf("read movement after walking: %v", err)
	}
	const wantSpent = 4 // 1 + 1 + 2
	if spent := max - after; spent != wantSpent {
		t.Fatalf("3 steps ending in a forest should spend %d (1+1+2), spent %d (%d/%d -> %d/%d)",
			wantSpent, spent, max, max, after, max)
	}
	// The forest step crossed onto rougher ground, so it carries the hint.
	if !strings.Contains(forestStep, "going is hard") {
		t.Fatalf("step into forest should show the hard-going hint, got:\n%s", forestStep)
	}
	t.Logf("biome-weighted cost + hard-going hint verified: 3 steps (last into forest) spent %d points, %d/%d -> %d/%d",
		wantSpent, max, max, after, max)
}

// walkStep sends one movement command and waits for the next prompt so the
// engine processes it before the caller's follow-up command. It returns the
// output captured up to that prompt (room view + any movement lines).
func walkStep(t *testing.T, c *telnettest.Client, dir string) string {
	t.Helper()
	if err := c.SendLine(dir); err != nil {
		t.Fatalf("send %s: %v", dir, err)
	}
	out, err := c.ExpectTimeout(promptOrBlocked, 4*time.Second)
	if err != nil {
		t.Fatalf("no prompt after %s: %v", dir, err)
	}
	return out
}

// promptOrBlocked matches the in-game status prompt (every command echoes one)
// — enough to know the step was processed.
var promptOrBlocked = regexp.MustCompile(`\[HP \d+/\d+\]`)
