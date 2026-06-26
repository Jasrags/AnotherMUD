//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_HirelingOrders proves the order-stance flow (hireable-mobs.md §8): a
// `stay` hireling holds its room when the owner walks off, and a later `follow`
// order makes it trail again. Deterministic — the stances are owner-driven, no
// RNG/timing. The first character is the bootstrap admin, so it can fund the hire.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_HirelingOrders -v
func TestLive_HirelingOrders(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, town-square

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Captara"); err != nil {
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
	// The sellsword is present in the owner's room iff a targeted look resolves
	// it (absent → "You don't see that here.").
	present := func() bool {
		return !strings.Contains(send("look sellsword"), "don't see that")
	}

	send("set gold amount self 500")
	if out := send("hire sellsword"); !strings.Contains(out, "hire a grizzled sellsword") {
		t.Fatalf("hire did not take:\n%s", out)
	}

	// Order it to hold the square, then walk north: it must NOT trail.
	if out := send("order sellsword stay"); !strings.Contains(out, "hold this position") {
		t.Fatalf("stay order not confirmed:\n%s", out)
	}
	send("north") // town-square -> Hearthwick Forge
	if present() {
		t.Fatal("a stay-stance hireling followed the owner north; it should have held the square")
	}

	// Return to the square — the held sellsword is still there.
	send("south")
	if !present() {
		t.Fatal("the stay-stance hireling did not hold the town square")
	}

	// Order it to follow again, then walk north: now it trails.
	if out := send("order sellsword follow"); !strings.Contains(out, "follow you") {
		t.Fatalf("follow order not confirmed:\n%s", out)
	}
	send("north")
	if !present() {
		t.Fatal("a re-ordered follow hireling did not trail the owner north")
	}
	t.Log("order stances verified live: stay held the room, follow resumed trailing")
}
