//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_HirelingRecruiter proves the recruiter access point (hireable-mobs.md
// §3.1): a bare `hire` at the town-square recruiter browses its catalog, hiring
// works there, and `hire` is refused in a room with no recruiter. Deterministic.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_HirelingRecruiter -v
func TestLive_HirelingRecruiter(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, town-square (the recruiter is here)

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Sword"); err != nil {
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

	// The recruiter is in the square: a bare `hire` browses its catalog.
	if out := send("hire"); !strings.Contains(out, "Available for hire here") || !strings.Contains(out, "a grizzled sellsword — 50 gold") {
		t.Fatalf("catalog not shown at the recruiter:\n%s", out)
	}

	// No recruiter in the forge to the north — hiring is refused there.
	send("north") // town-square -> Hearthwick Forge
	if out := send("hire sellsword"); !strings.Contains(out, "no one here to hire from") {
		t.Fatalf("hire should be refused away from a recruiter:\n%s", out)
	}

	// Back at the square, hiring works.
	send("south")
	send("set gold amount self 200")
	if out := send("hire sellsword"); !strings.Contains(out, "hire a grizzled sellsword") {
		t.Fatalf("hire at the recruiter failed:\n%s", out)
	}
	t.Log("recruiter access point verified live: catalog browse, hire at post, refused away from it")
}
