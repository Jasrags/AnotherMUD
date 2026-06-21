//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_Group proves the party roster end to end (grouping.md §2/§6): two
// characters in the town square, one invites the other, the other joins, the
// party channel carries a gtell, and the roster lists both.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_Group -v
func TestLive_Group(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, town-square

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
		// Clear any unsolicited push (invites, arrivals) + its trailing prompt so
		// the expect below matches THIS command's fresh prompt, not a stale one.
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

	// Drain both buffers: the member's arrival pushed a broadcast + stale prompt
	// to the leader's connection, which would otherwise satisfy the next expect.
	leader.Drain(800 * time.Millisecond)
	member.Drain(800 * time.Millisecond)

	// Leader invites; the member sees the invitation.
	if out := send(leader, "group Recruita"); !strings.Contains(out, "invite Recruita") {
		t.Fatalf("invite did not send:\n%s", out)
	}
	if _, err := member.ExpectStringTimeout("invites you to their party", 6*time.Second); err != nil {
		t.Fatalf("member never received the invitation: %v", err)
	}

	// Member joins; the leader is notified.
	if out := send(member, "join Captara"); !strings.Contains(out, "join Captara's party") {
		t.Fatalf("join failed:\n%s", out)
	}
	if _, err := leader.ExpectStringTimeout("Recruita joins the party", 6*time.Second); err != nil {
		t.Fatalf("leader not notified of the join: %v", err)
	}

	// The roster lists both, leader tagged.
	if out := send(leader, "group"); !strings.Contains(out, "Captara (leader)") || !strings.Contains(out, "Recruita") {
		t.Fatalf("roster wrong:\n%s", out)
	}

	// Party chat reaches the member.
	send(leader, "gtell forming up")
	if _, err := member.ExpectStringTimeout("[party] Captara: forming up", 6*time.Second); err != nil {
		t.Fatalf("gtell did not reach the member: %v", err)
	}

	// The leader disbands; the member is told.
	send(leader, "disband")
	if _, err := member.ExpectStringTimeout("disbands the party", 6*time.Second); err != nil {
		t.Fatalf("member not notified of disband: %v", err)
	}
}
