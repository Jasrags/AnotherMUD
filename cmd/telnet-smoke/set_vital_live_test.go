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

// TestLive_SetVitalPools proves the admin `set vital` verb is data-driven: it
// sets not just hp but any named resource pool the target actually carries
// (admin-verbs §4). Movement is seeded on every player in every pack, so it is
// the cross-ruleset demonstrator here — set it to a mid value below max and
// read it back off the score sheet.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_SetVitalPools -v
func TestLive_SetVitalPools(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_ROLE_SEED": "Setter:admin", // set is admin-gated
	})

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	if err := createAndLogin(c, "Setter"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	send := func(cmd string) string {
		t.Helper()
		if err := c.SendLine(cmd); err != nil {
			t.Fatalf("send %q: %v", cmd, err)
		}
		return c.Drain(700 * time.Millisecond)
	}

	// A fresh character starts with a full movement pool; pick a target below
	// its max so the set is observable and not clamped.
	_, max, err := scoreMovement(c)
	if err != nil {
		t.Fatalf("read starting movement: %v", err)
	}
	if max < 4 {
		t.Fatalf("movement max %d too small to test a mid value", max)
	}
	target := max - 3

	out := send("set vital movement self " + strconv.Itoa(target))
	if !strings.Contains(strings.ToUpper(out), "MOVEMENT SET TO") {
		t.Fatalf("set vital movement did not confirm (want 'MOVEMENT set to …'):\n%s", out)
	}

	cur, _, err := scoreMovement(c)
	if err != nil {
		t.Fatalf("read movement after set: %v", err)
	}
	if cur != target {
		t.Fatalf("movement after set = %d, want %d", cur, target)
	}

	// An unknown vital is refused and lists the target's real settable vitals
	// (hp + its pools), not a hardcoded "hp" only.
	bad := send("set vital bogus self 1")
	if !strings.Contains(strings.ToLower(bad), "unknown vital") ||
		!strings.Contains(strings.ToLower(bad), "movement") {
		t.Fatalf("unknown vital should list the target's real vitals (incl. movement):\n%s", bad)
	}

	t.Logf("set vital movement verified: set to %d/%d, unknown-vital lists the real surface", cur, max)
}
