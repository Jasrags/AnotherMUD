//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_LeaderSuccession proves grouping.md §3 leadership succession end to
// end: in a party of three the leader leaves and leadership passes to the
// longest-tenured remaining member (the first to have joined) rather than the
// party disbanding. The new leader and the other survivor get the right
// notices, and the roster reflects the new leader.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_LeaderSuccession -v
func TestLive_LeaderSuccession(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, town-square — all three co-located

	dial := func(name string) *telnettest.Client {
		t.Helper()
		c, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
		if err != nil {
			t.Fatalf("dial %s: %v", name, err)
		}
		t.Cleanup(func() { c.Close() })
		if err := createAndLogin(c, name); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return c
	}

	leader := dial("Leadara")
	first := dial("Firstmate")   // joins first → longest-tenured → successor
	second := dial("Secondmate") // joins second

	send := func(c *telnettest.Client, line string) string {
		t.Helper()
		c.Drain(150 * time.Millisecond)
		if err := c.SendLine(line); err != nil {
			t.Fatalf("send %q: %v", line, err)
		}
		out, err := c.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
		return out
	}

	for _, c := range []*telnettest.Client{leader, first, second} {
		c.Drain(800 * time.Millisecond) // clear arrival broadcasts
	}

	// Build the party of three: Firstmate joins before Secondmate.
	send(leader, "group Firstmate")
	if _, err := first.ExpectStringTimeout("invites you to their party", 6*time.Second); err != nil {
		t.Fatalf("Firstmate never got the invite: %v", err)
	}
	send(first, "join Leadara")

	send(leader, "group Secondmate")
	if _, err := second.ExpectStringTimeout("invites you to their party", 6*time.Second); err != nil {
		t.Fatalf("Secondmate never got the invite: %v", err)
	}
	send(second, "join Leadara")

	// Drain the join broadcasts so the succession notices below match cleanly.
	for _, c := range []*telnettest.Client{leader, first, second} {
		c.Drain(500 * time.Millisecond)
	}

	// Leader leaves → succession to Firstmate (longest-tenured).
	if out := send(leader, "leave"); !strings.Contains(out, "You leave the party") {
		t.Fatalf("leader leave did not confirm:\n%s", out)
	}
	if _, err := first.ExpectStringTimeout("you now lead the party", 6*time.Second); err != nil {
		t.Fatalf("Firstmate was not promoted to leader: %v", err)
	}
	if _, err := second.ExpectStringTimeout("Firstmate now leads the party", 6*time.Second); err != nil {
		t.Fatalf("Secondmate not told of the new leader: %v", err)
	}

	// The roster now tags Firstmate as the leader, with both survivors present.
	out := send(first, "group")
	if !strings.Contains(out, "Firstmate (leader)") || !strings.Contains(out, "Secondmate") {
		t.Fatalf("roster after succession wrong:\n%s", out)
	}
	if strings.Contains(out, "Leadara") {
		t.Fatalf("departed leader still on the roster:\n%s", out)
	}
}
