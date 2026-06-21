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

// TestLive_CombatAndLoot exercises PLAYTEST §6 (combat) and §7 (loot & corpses)
// on the default boot: a character goes to the Meadow, engages the road bandit,
// fights it to death, and loots the corpse.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_CombatAndLoot -v
//
// Two properties make this deterministic despite RNG and time-of-day:
//   - The character is seeded admin, leveled (xp), buffed (STR 20 + the
//     two-handed greatsword), and `restore`d every round, so it always wins and
//     never dies — the fight resolves regardless of darkness fumbles.
//   - The kill is detected via the "You have slain a road bandit" line, and §7
//     is proven by `loot` whose message names "the corpse of a road bandit".
//     Both are LIGHT-INDEPENDENT: the meadow is outdoors and the in-game clock
//     is seeded at boot, so a `look`-based corpse check would be night-flaky
//     (darkness suppresses room detail and gates `look corpse`) — but taking is
//     never gated, so loot-by-feel works at any hour.
func TestLive_CombatAndLoot(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_ROLE_SEED":       "Bruiser:admin", // xp/set/teleport/restore for a reliable fight
		"ANOTHERMUD_CORPSE_LIFETIME": "5m",            // keep the corpse alive long enough to loot
	})

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Bruiser"); err != nil {
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

	// Bootstrap: level + STR + the heavy two-hander so the fight resolves fast.
	send("xp 5000")
	send("set stat str Bruiser 20")
	send("restore")
	send("get greatsword")
	send("equip greatsword")
	send("teleport meadow")

	// §6 — engage the bandit (it spawns on the area reset, so poll for it).
	if err := engageBandit(c); err != nil {
		t.Fatalf("PLAYTEST FAIL [§6]: %v", err)
	}

	// §6 — fight to the death. `restore` each round so a dark-fumble slugfest
	// can't kill us; detect the kill on the (light-independent) slain line.
	slainRe := regexp.MustCompile(`(?i)slain a road bandit|road bandit is dead`)
	deadline := time.Now().Add(120 * time.Second)
	killed := false
	for time.Now().Before(deadline) {
		out := send("kill bandit")
		if strings.Contains(strings.ToLower(out), "safe room") {
			t.Fatalf("PLAYTEST FAIL [§6]: combat refused in the Meadow (should be allowed):\n%s", out)
		}
		acc := out + c.Drain(2000*time.Millisecond)
		send("restore") // keep us alive through darkness fumbles
		if slainRe.MatchString(acc) {
			killed = true
			break
		}
	}
	if !killed {
		t.Fatalf("PLAYTEST FAIL [§6]: never slew the road bandit within 120s of combat")
	}

	// §7 — look the corpse (content is light-gated; just no crash) then loot it.
	// The loot line names the corpse — a light-independent proof a lootable
	// road-bandit corpse exists and the loot path works.
	if look := send("look corpse"); strings.Contains(strings.ToLower(look), "huh?") {
		t.Errorf("PLAYTEST FAIL [§7 look corpse]: %s", look)
	}
	loot := send("loot")
	if !strings.Contains(strings.ToLower(loot), "corpse of a road bandit") {
		t.Errorf("PLAYTEST FAIL [§7 loot corpse]: loot did not name the bandit corpse:\n%s", loot)
	}
	t.Log("combat + loot verified live: engaged → slew bandit → looted the corpse")
}
