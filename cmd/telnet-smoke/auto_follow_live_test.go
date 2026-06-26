//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_AutoFollowOnJoin proves grouping.md §9 auto-follow end to end: joining
// a party starts the new member trailing the leader (no manual `follow`), so a
// leader's step pulls them along; leaving the party stops the trail, so a later
// step does not.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_AutoFollowOnJoin -v
func TestLive_AutoFollowOnJoin(t *testing.T) {
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
	member, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial member: %v", err)
	}
	defer member.Close()
	if err := createAndLogin(member, "Joinara"); err != nil {
		t.Fatalf("create member: %v", err)
	}

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

	leader.Drain(800 * time.Millisecond)
	member.Drain(800 * time.Millisecond)

	// Form the party — the join itself starts the follow (no `follow` verb).
	if out := send(leader, "group Joinara"); !strings.Contains(out, "invite Joinara") {
		t.Fatalf("invite did not send:\n%s", out)
	}
	if _, err := member.ExpectStringTimeout("invites you to their party", 6*time.Second); err != nil {
		t.Fatalf("member never received the invitation: %v", err)
	}
	if out := send(member, "join Leadara"); !strings.Contains(out, "begin following them") {
		t.Fatalf("join did not auto-follow the leader:\n%s", out)
	}

	// The leader walks north; the auto-followed member is pulled into the forge.
	send(leader, "north")
	if _, err := member.ExpectStringTimeout("Hearthwick Forge", 6*time.Second); err != nil {
		t.Fatalf("member was not pulled along by the auto-follow: %v", err)
	}

	// Leaving the party ends the party-induced follow.
	if out := send(member, "leave"); !strings.Contains(out, "leave the party") {
		t.Fatalf("leave failed:\n%s", out)
	}

	// The leader walks back south; the member, no longer following, stays put.
	send(leader, "south")
	if out := send(member, "look"); !strings.Contains(out, "Hearthwick Forge") {
		t.Fatalf("member followed the leader after leaving (should have stayed in the forge):\n%s", out)
	}
	t.Log("auto-follow verified live: join pulled the member north, leave stopped the trail")
}
