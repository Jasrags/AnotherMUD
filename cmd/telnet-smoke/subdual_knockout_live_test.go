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

// TestLive_SubdualKnockout proves the subdual knock-out (subdual-damage §4/§6)
// end to end on the default boot: a character gets the town-square sap, fights
// the meadow road bandit with it, and the finishing blow KNOCKS THE BANDIT OUT
// (unconscious) instead of slaying it — no "slain" line, no lootable corpse.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_SubdualKnockout -v
//
// Deterministic despite RNG: the character is seeded admin, leveled, STR-buffed,
// and `restore`d each round, so it always grinds the bandit down and never dies;
// the sap (a subdual weapon) guarantees the finish is a knock-out, not a kill.
func TestLive_SubdualKnockout(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_ROLE_SEED": "Sapper:admin", // xp/set/teleport/restore for a reliable fight
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Sapper"); err != nil {
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

	// Bootstrap: level + STR so the low-damage sap still grinds the bandit down,
	// then wield the sap (the subdual weapon) and go to the Meadow.
	send("xp 5000")
	send("set stat str Sapper 20")
	send("restore")
	if out := send("get sap"); strings.Contains(strings.ToLower(out), "don't see") {
		t.Fatalf("could not get the sap from the town square:\n%s", out)
	}
	// The sap is multi-eligible (wield|offhand), so the slot MUST be named —
	// bare `equip sap` is refused with "which slot?" (and the character would
	// then fight unarmed, lethally). Name the wield slot and verify it took.
	if out := send("equip sap wield"); !strings.Contains(strings.ToLower(out), "sap") {
		t.Fatalf("equip sap wield did not confirm the sap:\n%s", out)
	}
	send("teleport meadow")

	if err := engageBandit(c); err != nil {
		t.Fatalf("engage: %v", err)
	}

	// Fight with the sap. Detect the KNOCK-OUT (the attacker-facing line); fail
	// immediately if we ever see a lethal "slain" line instead.
	knockRe := regexp.MustCompile(`(?i)knock a road bandit out cold`)
	slainRe := regexp.MustCompile(`(?i)slain a road bandit|road bandit is dead`)
	deadline := time.Now().Add(120 * time.Second)
	knocked := false
	for time.Now().Before(deadline) {
		out := send("kill bandit")
		acc := out + c.Drain(2000*time.Millisecond)
		send("restore") // keep us alive through darkness fumbles
		if slainRe.MatchString(acc) {
			t.Fatalf("subdual FAIL: the sap SLEW the bandit (should knock out):\n%s", acc)
		}
		if knockRe.MatchString(acc) {
			knocked = true
			break
		}
	}
	if !knocked {
		t.Fatalf("subdual FAIL: never knocked the bandit out within 120s")
	}

	// No corpse: a knock-out leaves the foe alive (1 HP) + unconscious, not dead,
	// so there is nothing to loot (the contrast with TestLive_CombatAndLoot).
	loot := send("loot")
	if strings.Contains(strings.ToLower(loot), "corpse of a road bandit") {
		t.Errorf("subdual FAIL: a knock-out should leave NO lootable corpse:\n%s", loot)
	}
	t.Log("subdual verified live: sapped the bandit → knocked out (unconscious), no corpse")
}
