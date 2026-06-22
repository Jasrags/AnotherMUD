//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_Promote proves leader-named succession end to end (grouping.md §3):
// a leader hands leadership to a party member with `promote`, the member becomes
// leader (and may now run leader-only verbs), and the old leader — still in the
// party — may not.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_Promote -v
func TestLive_Promote(t *testing.T) {
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

	// Form the party.
	if out := send(leader, "group Recruita"); !strings.Contains(out, "invite Recruita") {
		t.Fatalf("invite did not send:\n%s", out)
	}
	if _, err := member.ExpectStringTimeout("invites you to their party", 6*time.Second); err != nil {
		t.Fatalf("member never received the invitation: %v", err)
	}
	if out := send(member, "join Captara"); !strings.Contains(out, "join Captara's party") {
		t.Fatalf("join failed:\n%s", out)
	}
	if _, err := leader.ExpectStringTimeout("Recruita joins the party", 6*time.Second); err != nil {
		t.Fatalf("leader never saw the join: %v", err)
	}

	// Hand leadership to the member.
	if out := send(leader, "promote Recruita"); !strings.Contains(out, "hand leadership") || !strings.Contains(out, "Recruita") {
		t.Fatalf("promote did not announce the handoff:\n%s", out)
	}
	if _, err := member.ExpectStringTimeout("now the party leader", 6*time.Second); err != nil {
		t.Fatalf("member never learned they lead: %v", err)
	}

	// The old leader is now a regular member: a leader-only verb is refused.
	if out := send(leader, "disband"); !strings.Contains(out, "aren't leading a party") {
		t.Fatalf("old leader could still act as leader:\n%s", out)
	}
	// The new leader may run the leader-only verb.
	if out := send(member, "disband"); !strings.Contains(out, "disband your party") {
		t.Fatalf("new leader could not disband:\n%s", out)
	}
	if _, err := leader.ExpectStringTimeout("disbands the party", 6*time.Second); err != nil {
		t.Fatalf("old leader never saw the disband: %v", err)
	}
	t.Log("promote verified live: leadership handed off, old leader demoted, new leader in control")
}
