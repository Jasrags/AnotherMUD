//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_VisionModes proves a vision mode changes what a runner sees in the dark,
// end to end, in the Downtown maintenance sublevel (`terrain: underground`):
//
//   - A HUMAN runner (no racial dark vision — unlike a dwarf/troll, whose
//     thermographic floor would already resolve the dark) sees nothing in the
//     sublevel: the pitch-black render withholds the name, prose, and exits (§5.1).
//   - Installing + wearing a THERMOGRAPHIC cybereye enhancement floors effective
//     light to gloom, and the room resolves — its name and shapes come back.
//
// The runner is a plain (non-admin) human — admins get a dark-room bypass, and a
// dwarf (the wizard's first race) sees in the dark natively — who grabs the gear
// from the plaza tray and walks down, so no admin verbs are used.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_VisionModes -v
func TestLive_VisionModes(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:westlake-plaza",
		// No ROLE_SEED: an ordinary runner, so darkness is not admin-bypassed.
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	// Create a HUMAN (metatype option 3: Dwarf/Elf/Human/Ork/Troll) — no racial
	// dark vision, so the sublevel is genuinely black for them.
	isNew, err := doLogin(c, "Runner")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := finishLogin(c, "Runner", isNew, map[string]string{"race": "3"}); err != nil {
		t.Fatalf("create human: %v", err)
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

	// Grab a thermographic cybereye from the plaza tray and install it — unworn.
	send("get cybereyes")
	send("get thermographic")
	if out := send("modify cybereyes thermographic"); !strings.Contains(strings.ToLower(out), "install") {
		t.Fatalf("could not install the thermographic mod:\n%s", out)
	}

	// Descend without the eyes: pitch black — the suppressed render.
	send("down")
	blind := send("look")
	if strings.Contains(blind, "Maintenance Sublevel") {
		t.Fatalf("a human runner should NOT resolve the dark room in pitch black:\n%s", blind)
	}
	if !strings.Contains(strings.ToLower(blind), "pitch black") {
		t.Fatalf("expected the pitch-black suppression render:\n%s", blind)
	}

	// Back up, wear the eyes, descend again — thermographic resolves the room.
	send("up")
	if out := send("equip cybereyes"); !strings.Contains(strings.ToLower(out), "cybereyes") {
		t.Fatalf("could not equip the cybereyes:\n%s", out)
	}
	send("down")
	seen := send("look")
	if !strings.Contains(seen, "Maintenance Sublevel") {
		t.Fatalf("thermographic vision should resolve the dark room's name:\n%s", seen)
	}
	_ = time.Second
}
