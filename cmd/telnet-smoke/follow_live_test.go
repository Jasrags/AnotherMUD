//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_Follow proves the move-with-leader primitive (follow.md) end to end:
// two characters in the town square, one follows the other, the leader walks
// north, and the follower is pulled along into Hearthwick Forge; then `lose`
// shakes the follower.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_Follow -v
func TestLive_Follow(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, town-square

	leader, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial leader: %v", err)
	}
	defer leader.Close()
	if err := createAndLogin(leader, "Leadara"); err != nil {
		t.Fatalf("create leader: %v", err)
	}

	follower, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial follower: %v", err)
	}
	defer follower.Close()
	if err := createAndLogin(follower, "Follara"); err != nil {
		t.Fatalf("create follower: %v", err)
	}

	send := func(c *telnettest.Client, line string) string {
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

	// Both are in the town square; the follower starts trailing the leader.
	follower.Drain(500 * time.Millisecond)
	if out := send(follower, "follow Leadara"); !strings.Contains(out, "begin following Leadara") {
		t.Fatalf("follow did not begin:\n%s", out)
	}

	// The leader walks north into Hearthwick Forge; the follower is pulled along
	// and receives the forge on their own connection (unsolicited server push).
	send(leader, "north")
	if _, err := follower.ExpectStringTimeout("Hearthwick Forge", 6*time.Second); err != nil {
		t.Fatalf("follower was not pulled into the forge: %v", err)
	}

	// `lose` shakes the follower: they're told they lost the trail.
	send(leader, "lose")
	if _, err := follower.ExpectStringTimeout("lose the trail", 6*time.Second); err != nil {
		t.Fatalf("follower was not shaken by lose: %v", err)
	}
}
