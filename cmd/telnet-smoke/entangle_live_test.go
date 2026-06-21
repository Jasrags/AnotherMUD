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

// TestLive_Entangle proves the net/entangle slice (special-weapons §9) end to
// end on the default boot: the town-square holds a weighted net, a fresh fighter
// knows the NET-ONLY `entangle` maneuver, and entangling the meadow bandit with
// the net wielded fires the maneuver (it is save-gated, so it lands or is
// resisted — either is a success for "the content loaded and the verb works").
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_Entangle -v
func TestLive_Entangle(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil)

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Netter"); err != nil {
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

	// Pick up + wield the net in town-square (out of combat).
	if out := send("get net"); !strings.Contains(strings.ToLower(out), "net") {
		t.Fatalf("the town-square should hold a weighted net:\n%s", out)
	}
	if out := send("equip net"); strings.Contains(strings.ToLower(out), "huh?") {
		t.Fatalf("equip net misbehaved:\n%s", out)
	}

	// To the meadow and engage the bandit (entangle is offensive — needs combat).
	send("south")
	send("south")
	if err := engageBandit(c); err != nil {
		t.Fatalf("engage bandit: %v", err)
	}

	// Entangle the bandit. The maneuver is save-gated, so the outcome is "lands"
	// or "resisted" — both prove the verb + content work. What must NOT appear:
	// the not-learned / wrong-equipment / unknown-verb errors.
	bad := regexp.MustCompile(`(?i)haven't learned|right equipment|huh\?`)
	fired := regexp.MustCompile(`(?i)entangl|resist|tangl|net`)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		out := send("entangle bandit")
		out += c.Drain(1500 * time.Millisecond)
		if bad.MatchString(out) {
			t.Fatalf("entangle reported an error (gate/learn/verb) with a net wielded:\n%s", out)
		}
		if fired.MatchString(out) {
			t.Logf("entangle fired:\n%s", strings.TrimSpace(out))
			return
		}
		// Bandit may have died / fallen out of combat — re-engage and retry.
		if strings.Contains(strings.ToLower(out), "don't see") || strings.Contains(strings.ToLower(out), "fighting") {
			_ = engageBandit(c)
		}
	}
	t.Fatal("entangle never produced a clear outcome within 30s")
}
