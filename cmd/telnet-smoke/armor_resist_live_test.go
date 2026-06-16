//go:build unix

package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ArmorPerTypeResistanceSoaksToFloor is the armor-depth §4 regression
// test: per-type damage resistance is applied in a real running engine. The
// road bandit wields a 1d4 slashing dagger (+1 Str damage bonus ⇒ raw hits of
// 2..5). The warded coif (content/starter-world/items/warded-coif.yaml) grants
// slashing resistance 5 and NO armor bonus, so every slashing hit a wearer
// takes is reduced to 0 and floored to the per-swing minimum of 1.
//
// The proof is deterministic, not statistical: while the coif is worn EVERY
// bandit hit lands for exactly 1 (5 raw − 5 resistance, floored), whereas an
// unarmored wearer would always take ≥2. The test collects several bandit hits
// and fails if any exceeds 1 — which would mean the resistance is not applied.
func TestLive_ArmorPerTypeResistanceSoaksToFloor(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // defaults: starter-world pack, town-square start

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Warded"); err != nil {
		t.Fatalf("create/login: %v", err)
	}

	// Pick up and wear the warded coif (starter gear in town-square).
	if err := c.SendLine("get coif"); err != nil {
		t.Fatalf("get coif: %v", err)
	}
	if _, err := c.ExpectTimeout(regexp.MustCompile(`coif`), 4*time.Second); err != nil {
		t.Fatalf("did not see the coif on `get`: %v", err)
	}
	if err := c.SendLine("equip coif"); err != nil {
		t.Fatalf("equip coif: %v", err)
	}
	out, err := c.ExpectTimeout(regexp.MustCompile(`coif|wear|wield|equip`), 4*time.Second)
	if err != nil || strings.Contains(strings.ToLower(out), "can't") || strings.Contains(strings.ToLower(out), "don't") {
		t.Fatalf("equip coif did not confirm (resp %q): %v", strings.TrimSpace(out), err)
	}

	// Walk to the meadow: town-square → south → village-gate → south → meadow.
	for _, dir := range []string{"south", "south"} {
		if err := c.SendLine(dir); err != nil {
			t.Fatalf("move %s: %v", dir, err)
		}
		if _, err := c.ExpectTimeout(gamePrompt, 5*time.Second); err != nil {
			t.Fatalf("no prompt after moving %s: %v", dir, err)
		}
	}

	if err := engageBandit(c); err != nil {
		t.Fatalf("engage bandit: %v", err)
	}

	// Collect bandit hits and assert each is floored to 1 (resistance applied).
	hitRe := regexp.MustCompile(`a road bandit hits you for (\d+) damage`)
	const wantHits = 3
	got := 0
	deadline := time.Now().Add(40 * time.Second)
	for got < wantHits && time.Now().Before(deadline) {
		line, err := c.ExpectTimeout(hitRe, 6*time.Second)
		if err != nil {
			// No bandit hit this window — nudge combat along and retry.
			_ = c.SendLine("kill bandit")
			continue
		}
		m := hitRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		dmg, _ := strconv.Atoi(m[1])
		got++
		if dmg != 1 {
			t.Fatalf("bandit hit for %d damage while wearing the warded coif; want 1 "+
				"(1d4+1 slashing − 5 resistance, floored). Resistance not applied?", dmg)
		}
	}
	if got < wantHits {
		t.Fatalf("only observed %d/%d bandit hits before the deadline; cannot confirm soak", got, wantHits)
	}
}

// engageBandit starts combat with the meadow's road bandit, polling until one
// is present (it spawns on the wilderness area reset interval, like the boar).
func engageBandit(c *telnettest.Client) error {
	deadline := time.Now().Add(45 * time.Second)
	marker := regexp.MustCompile(`You attack a road bandit|don't see|isn't here|not here`)
	var last string
	for time.Now().Before(deadline) {
		if err := c.SendLine("kill bandit"); err != nil {
			return err
		}
		out, err := c.ExpectTimeout(marker, 3*time.Second)
		if err == nil && strings.Contains(out, "You attack") {
			return nil
		}
		last = strings.TrimSpace(out)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("could not engage a road bandit within 45s (last response: %q)", last)
}
