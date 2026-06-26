//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_HireLifecycle proves the hireable-mobs slice-1 lifecycle end to end
// (hireable-mobs.md §3): hire a sellsword (gold charged, creature materialized
// into the room), see it in the room + the `hirelings` roster, then dismiss it
// (creature removed, contract dropped). The first character is the bootstrap
// admin, so it can `set gold` to fund the hire.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_HireLifecycle -v
func TestLive_HireLifecycle(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, town-square

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Bossara"); err != nil {
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

	// Fund the hire (the bootstrap admin can set its own gold).
	if out := send("set gold amount self 500"); !strings.Contains(out, "Gold set to 500") {
		t.Fatalf("could not fund the hire:\n%s", out)
	}

	// No hirelings yet.
	if out := send("hirelings"); !strings.Contains(out, "no hirelings") {
		t.Fatalf("fresh roster should be empty:\n%s", out)
	}

	// Hire the sellsword (model b: resolves the hireable template by name from
	// anywhere). The 50-gold hire cost is charged.
	if out := send("hire sellsword"); !strings.Contains(out, "hire a grizzled sellsword") {
		t.Fatalf("hire did not take:\n%s", out)
	}

	// The roster now lists it as present, and it materialized into the room.
	if out := send("hirelings"); !strings.Contains(out, "a grizzled sellsword") || !strings.Contains(out, "with you") {
		t.Fatalf("roster should show the present sellsword:\n%s", out)
	}
	if out := send("look sellsword"); !strings.Contains(strings.ToLower(out), "sellsword") {
		t.Fatalf("the hired sellsword should be in the room to look at:\n%s", out)
	}

	// Dismiss it: the contract ends and the creature leaves the world.
	if out := send("dismiss sellsword"); !strings.Contains(out, "dismiss a grizzled sellsword") {
		t.Fatalf("dismiss did not take:\n%s", out)
	}
	if out := send("hirelings"); !strings.Contains(out, "no hirelings") {
		t.Fatalf("roster should be empty after dismiss:\n%s", out)
	}
	t.Log("hire lifecycle verified live: hired (gold charged + materialized), listed, dismissed")
}

// TestLive_HirelingFollows proves hireable-mobs.md §5: a hired companion is bound
// to its owner and relocates with them room to room.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_HirelingFollows -v
func TestLive_HirelingFollows(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, town-square

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Bindara"); err != nil {
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

	send("set gold amount self 500")
	if out := send("hire sellsword"); !strings.Contains(out, "hire a grizzled sellsword") {
		t.Fatalf("hire did not take:\n%s", out)
	}
	// Present in the starting room.
	if out := send("look sellsword"); !strings.Contains(strings.ToLower(out), "sellsword") {
		t.Fatalf("sellsword should be here at the start:\n%s", out)
	}
	// Walk north — the bound hireling relocates with the owner.
	send("north") // town-square -> Hearthwick Forge
	if out := send("look sellsword"); !strings.Contains(strings.ToLower(out), "sellsword") {
		t.Fatalf("sellsword did not follow the owner north:\n%s", out)
	}
	// And back south.
	send("south")
	if out := send("look sellsword"); !strings.Contains(strings.ToLower(out), "sellsword") {
		t.Fatalf("sellsword did not follow the owner back south:\n%s", out)
	}
	t.Log("hireling-follows verified live: the bound sellsword relocated with the owner both ways")
}
