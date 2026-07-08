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

// TestLive_KillXP proves combat kill-experience (grouping.md §4) end to end on
// the default boot: a solo character slays the road bandit (xp_value 30) and
// gains the full 30 experience — a party of one. The split among party members
// is covered by the killXPRecipients unit test.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_KillXP -v
//
// Deterministic like the combat-loot test: the character is admin-seeded,
// STR-buffed, greatsword-armed, and `restore`d each round, so it always wins.
func TestLive_KillXP(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_ROLE_SEED": "Slayara:admin",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Slayara"); err != nil {
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

	send("set stat str Slayara 20")
	send("restore")
	send("get greatsword")
	send("equip greatsword")
	send("teleport meadow")
	if err := engageBandit(c); err != nil {
		t.Fatalf("engage: %v", err)
	}

	slainRe := regexp.MustCompile(`(?i)slain a road bandit|road bandit is dead`)
	deadline := time.Now().Add(120 * time.Second)
	var acc strings.Builder
	for time.Now().Before(deadline) {
		out := send("kill bandit")
		acc.WriteString(out + c.Drain(2000*time.Millisecond))
		send("restore")
		if slainRe.MatchString(acc.String()) {
			break
		}
	}
	if !slainRe.MatchString(acc.String()) {
		t.Fatalf("never slew the bandit within 120s")
	}
	if !strings.Contains(acc.String(), "You gain 30 experience.") {
		t.Fatalf("kill did not grant the full 30 XP to the solo killer:\n%s", acc.String())
	}
}
