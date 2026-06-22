//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_AutoAssist proves grouping.md §9 auto-assist end to end: two
// party-mates stand in the meadow, the member has `autoassist on`, and when the
// leader engages a road bandit the member is automatically pulled into the
// fight (the "you leap to ... aid" line). This is the offensive case; it also
// exercises the recursion-safe engage-from-OnEngagement path (a hang here would
// mean the InCombat terminator regressed).
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_AutoAssist -v
func TestLive_AutoAssist(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		// Both admin so each can `teleport` to the meadow combat room.
		"ANOTHERMUD_ROLE_SEED": "Captara:admin;Recruita:admin",
	})

	leader, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial leader: %v", err)
	}
	defer leader.Close()
	if err := createAndLogin(leader, "Captara"); err != nil {
		t.Fatalf("create leader: %v", err)
	}

	member, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial member: %v", err)
	}
	defer member.Close()
	if err := createAndLogin(member, "Recruita"); err != nil {
		t.Fatalf("create member: %v", err)
	}

	send := func(c *telnettest.Client, line string) string {
		t.Helper()
		c.Drain(150 * time.Millisecond) // clear unsolicited pushes + stale prompt
		if err := c.SendLine(line); err != nil {
			t.Fatalf("send %q: %v", line, err)
		}
		out, err := c.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
		return out
	}

	// Drain the arrival broadcasts so the first expects match fresh prompts.
	leader.Drain(800 * time.Millisecond)
	member.Drain(800 * time.Millisecond)

	// Form the party.
	send(leader, "group Recruita")
	if _, err := member.ExpectStringTimeout("invites you to their party", 6*time.Second); err != nil {
		t.Fatalf("member never received the invitation: %v", err)
	}
	if out := send(member, "join Captara"); !strings.Contains(out, "join Captara's party") {
		t.Fatalf("join failed:\n%s", out)
	}
	if _, err := leader.ExpectStringTimeout("Recruita joins the party", 6*time.Second); err != nil {
		t.Fatalf("leader not notified of the join: %v", err)
	}

	// Member opts in to auto-assist.
	if out := send(member, "autoassist on"); !strings.Contains(out, "enabled") {
		t.Fatalf("autoassist on did not confirm:\n%s", out)
	}

	// Both move to the meadow; the member first, so it is present in the room
	// when the leader's engagement fires the auto-assist fan-out.
	send(member, "teleport meadow")
	send(leader, "teleport meadow")

	// Leader engages a bandit (polls until one has spawned).
	if err := engageBandit(leader); err != nil {
		t.Fatalf("leader engage: %v", err)
	}

	// The member is auto-pulled into the fight.
	if _, err := member.ExpectStringTimeout("leap to Captara's aid", 8*time.Second); err != nil {
		t.Fatalf("member was not auto-pulled into the leader's fight: %v", err)
	}
}
