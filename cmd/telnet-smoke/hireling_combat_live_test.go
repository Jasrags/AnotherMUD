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

// TestLive_HirelingCombat proves hireable-mobs.md §6 end to end: a hired
// sellsword auto-assists its owner's fight (§6.1), and the owner earns the kill
// experience because they actively fought the foe (§6.4 / PD-4 — XP rewards
// participation, and the assist only ever targets the owner's foe).
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_HirelingCombat -v
func TestLive_HirelingCombat(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_CORPSE_LIFETIME": "5m",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Capara"); err != nil {
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

	// Hire a sellsword, then buff + arm the owner so the fight resolves.
	send("set gold amount self 500")
	if out := send("hire sellsword"); !strings.Contains(out, "hire a grizzled sellsword") {
		t.Fatalf("hire failed:\n%s", out)
	}
	send("set stat str Capara 20")
	send("restore")
	send("get greatsword")
	send("equip greatsword")
	send("teleport meadow") // the sellsword is bound → follows into the meadow

	// Engage the bandit and CAPTURE the engagement output — the assist fires once,
	// on the first engagement (§6.1), so its room broadcast lands here.
	var acc string
	engaged := false
	edl := time.Now().Add(45 * time.Second)
	for time.Now().Before(edl) {
		acc += send("kill bandit") + c.Drain(1500*time.Millisecond)
		if strings.Contains(acc, "You attack a road bandit") || strings.Contains(acc, "already fighting") {
			engaged = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !engaged {
		t.Fatalf("could not engage the bandit:\n%s", acc)
	}
	slainRe := regexp.MustCompile(`(?i)slain a road bandit|road bandit is dead`)
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		acc += send("kill bandit") + c.Drain(2000*time.Millisecond)
		send("restore")
		if slainRe.MatchString(acc) {
			break
		}
	}
	if !slainRe.MatchString(acc) {
		t.Fatalf("never slew the bandit within 120s:\n%s", acc)
	}
	// §6.1 — the sellsword joined the fight.
	if !strings.Contains(acc, "moves to defend Capara") {
		t.Fatalf("the sellsword did not auto-assist the owner's fight:\n%s", acc)
	}
	// §6.4 — the owner earns the kill XP (they fought it), whether they or the
	// sellsword landed the killing blow.
	if !strings.Contains(acc, "You gain 30 experience.") {
		t.Fatalf("owner did not earn the participation kill-XP:\n%s", acc)
	}
	t.Log("hireling combat verified live: sellsword assisted, owner earned the participation XP")
}
