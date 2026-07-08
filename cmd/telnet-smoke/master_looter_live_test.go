//go:build unix

package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_MasterLooter proves the master-looter loot policy end to end
// (grouping.md §9): a two-person party sets `lootmode master <member>`, the
// OTHER member (not the master) lands the kill, and the resulting corpse is
// owned by the master alone — the killer is refused, the master may loot.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_MasterLooter -v
//
// This is the integration proof for the wiring units can't reach: main's
// OwnerSet hook → Manager.LootOwners (master-only set, killer excluded) →
// corpse owner set → MayLoot. Determinism mirrors the combat-loot test: the
// killer is admin-seeded, STR-buffed, greatsword-armed, and `restore`d each
// round, so it always wins; a long ownership window keeps both loot attempts
// inside the rights gate regardless of how long the fight takes.
func TestLive_MasterLooter(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_ROLE_SEED":               "Captara:admin;Recruita:admin",
		"ANOTHERMUD_CORPSE_LIFETIME":         "5m", // keep the corpse alive long enough to loot
		"ANOTHERMUD_CORPSE_OWNERSHIP_WINDOW": "5m", // both loot attempts stay inside the window
	})

	leader, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial leader: %v", err)
	}
	defer leader.Close()
	if err := createAndLogin(leader, "Captara"); err != nil {
		t.Fatalf("create leader: %v", err)
	}
	master, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial master: %v", err)
	}
	defer master.Close()
	if err := createAndLogin(master, "Recruita"); err != nil {
		t.Fatalf("create master: %v", err)
	}

	send := func(c *telnettest.Client, line string) string {
		t.Helper()
		c.Drain(150 * time.Millisecond) // clear unsolicited pushes + stale prompts
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
	master.Drain(800 * time.Millisecond)

	// Form the party in the town square (invite is room-scoped).
	if out := send(leader, "group Recruita"); !strings.Contains(out, "invite Recruita") {
		t.Fatalf("invite did not send:\n%s", out)
	}
	if _, err := master.ExpectStringTimeout("invites you to their party", 6*time.Second); err != nil {
		t.Fatalf("master never received the invitation: %v", err)
	}
	if out := send(master, "join Captara"); !strings.Contains(out, "join Captara's party") {
		t.Fatalf("join failed:\n%s", out)
	}
	if _, err := leader.ExpectStringTimeout("Recruita joins the party", 6*time.Second); err != nil {
		t.Fatalf("leader never saw the join: %v", err)
	}

	// Designate the OTHER member (Recruita) as master-looter. The leader is the
	// one who will land the kill, so this proves the killer is excluded.
	if out := send(leader, "lootmode master Recruita"); !strings.Contains(out, "Recruita") || !strings.Contains(strings.ToLower(out), "master-looter") {
		t.Fatalf("lootmode master did not take:\n%s", out)
	}

	// Buff the leader so the fight resolves; move both to the meadow.
	send(leader, "xp 5000")
	send(leader, "set stat str Captara 20")
	send(leader, "restore")
	send(leader, "get greatsword")
	send(leader, "equip greatsword")
	send(leader, "teleport meadow")
	send(master, "teleport meadow")

	// Leader engages + fights the bandit to death.
	if err := engageBandit(leader); err != nil {
		t.Fatalf("engage: %v", err)
	}
	slainRe := regexp.MustCompile(`(?i)slain a road bandit|road bandit is dead`)
	deadline := time.Now().Add(120 * time.Second)
	var acc strings.Builder
	for time.Now().Before(deadline) {
		out := send(leader, "kill bandit")
		acc.WriteString(out + leader.Drain(2000*time.Millisecond))
		send(leader, "restore")
		if slainRe.MatchString(acc.String()) {
			break
		}
	}
	if !slainRe.MatchString(acc.String()) {
		t.Fatalf("never slew the bandit within 120s:\n%s", acc.String())
	}

	// The KILLER (leader) is NOT the master → refused during the rights window.
	// Loot BY KEYWORD so the rights gate reports the refusal (a bare `loot`
	// silently skips a corpse you can't loot, reporting "nothing here").
	if out := send(leader, "loot corpse"); !strings.Contains(strings.ToLower(out), "don't have the right to loot") {
		t.Fatalf("master-looter did not lock out the killer:\n%s", out)
	}
	// The MASTER (Recruita) owns the kill → may loot it.
	if out := send(master, "loot"); !strings.Contains(strings.ToLower(out), "corpse of a road bandit") {
		t.Fatalf("master-looter could not loot the corpse:\n%s", out)
	}
	t.Log("master-looter verified live: killer locked out, master looted the corpse")
}
