//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_HirelingBand proves the cap>1 band behaviors (hireable-mobs.md §3.3):
// an owner fields two same-template hirelings, sees them NUMBERED, commands the
// whole band with `order all`, and dismisses one by its roster number. The
// default cap is 3, so two sellswords fit. Deterministic — owner-driven.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_HirelingBand -v
func TestLive_HirelingBand(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, town-square

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Captain"); err != nil {
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
	present := func() bool { return !strings.Contains(send("look sellsword"), "don't see that") }

	send("set gold amount self 500")
	send("hire sellsword")
	send("hire sellsword")

	// Two hirelings, numbered.
	if out := send("hirelings"); !strings.Contains(out, "1) a grizzled sellsword") || !strings.Contains(out, "2) a grizzled sellsword") {
		t.Fatalf("roster not numbered for two hirelings:\n%s", out)
	}

	// Band order: both hold the square when the captain walks off.
	if out := send("order all stay"); !strings.Contains(out, "Your 2 hirelings hold this position") {
		t.Fatalf("order all not confirmed:\n%s", out)
	}
	send("north")
	if present() {
		t.Fatal("a stayed band followed the captain north")
	}
	send("south")
	if !present() {
		t.Fatal("the stayed band did not hold the square")
	}

	// Dismiss one by its roster number; one remains.
	if out := send("dismiss 1"); !strings.Contains(out, "dismiss a grizzled sellsword") {
		t.Fatalf("dismiss by number failed:\n%s", out)
	}
	if out := send("hirelings"); !strings.Contains(out, "1) a grizzled sellsword") || strings.Contains(out, "2)") {
		t.Fatalf("roster should hold exactly one after dismiss:\n%s", out)
	}
	t.Log("band verified live: two numbered hirelings, order all, dismiss by number")
}
