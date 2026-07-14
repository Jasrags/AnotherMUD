//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ShadowrunSafehouse walks the starter-area front door end to end: a new
// runner spawns in the fixer's flop (the make run-shadowrun start), gets oriented
// by Rook, visits the gear table and the tutorial chop-doc, then graduates down
// the stairwell onto the street in Downtown — and can climb back up.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunSafehouse -v
func TestLive_ShadowrunSafehouse(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:the-flop", // the new default onboarding start
		"ANOTHERMUD_ROLE_SEED":  "Runner:admin",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Runner"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	send := func(line string) string {
		t.Helper()
		if err := c.SendLine(line); err != nil {
			t.Fatalf("send %q: %v", line, err)
		}
		out, err := c.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
		return out
	}
	has := func(out, want, ctx string) {
		t.Helper()
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Fatalf("%s: expected %q in:\n%s", ctx, want, out)
		}
	}

	// Spawn: the flop, with Rook the fixer.
	spawn := send("look")
	has(spawn, "the flop", "spawn room")
	has(spawn, "rook", "mentor present at spawn")

	// Rook orients you — the gear topic points at the gun table.
	has(send("ask rook about gear"), "table", "ask rook about gear")

	// East to the gun table: the fixer's shop lists a weapon.
	has(send("east"), "gun table", "gear-table room")
	has(send("list"), "predator", "fixer stocks the Ares Predator")

	// Back to the flop, then west to the chop-doc: starter chrome on offer.
	has(send("west"), "the flop", "return to flop from gear table")
	has(send("west"), "back room", "chop-doc room")
	has(send("list"), "cybereyes", "tutorial chop-doc stocks starter chrome")

	// Back to the flop, then graduate down the stairwell into Downtown.
	has(send("east"), "the flop", "return to flop from back room")
	has(send("down"), "westlake", "graduation into Downtown")

	// And you can climb back up to the fixer.
	has(send("up"), "the flop", "return up to the safehouse")

	t.Logf("starter area verified live: spawn in the flop -> Rook orients -> gear table + chop-doc -> graduate down to Westlake -> back up")
}
