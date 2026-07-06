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

// TestLive_ShadowrunStunKnockout is the SR-M3c-3 end-to-end payoff: on a real
// shadowrun boot, a freshly-created Street Samurai gears up on the street
// corner, walks into the ganger's turf, and STUN-BATONS the ganger unconscious.
// It proves the whole Shadowrun stack live — the shadowrun pack boots, the
// creation flow builds a Street Samurai on the eight SR primaries, the stun
// baton (target_pool: stun) routes its damage to the ganger's Willpower-derived
// Stun monitor, and crossing that monitor's floor is a KNOCK-OUT (Nonlethal),
// not a kill — no "slain" line, no lootable corpse.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunStunKnockout -v
//
// Deterministic despite RNG: the runner is seeded admin, xp-leveled, Strength-
// buffed (SR damage_bonus = Strength), and `restore`d each round, so it always
// grinds the ganger's Stun monitor down and never dies; the stun baton
// guarantees the finish is a knock-out.
func TestLive_ShadowrunStunKnockout(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":  "Runner:admin", // xp/set/teleport/restore for a reliable fight
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	// Default wizard answers build a Street Samurai (first class) / Street Kid
	// (first background); the SR flow is the engine default, no channeling step.
	if err := createAndLogin(c, "Runner"); err != nil {
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

	// Bootstrap: level + Strength (SR damage scales off Strength via the channel
	// map) so the stun baton drives the Stun monitor down fast, then grab and
	// wield the baton from the corner's starter gear.
	send("xp 5000")
	send("set stat strength Runner 6")
	send("restore")
	if out := send("get baton"); strings.Contains(strings.ToLower(out), "don't see") {
		t.Fatalf("could not get the stun baton from the street corner:\n%s", out)
	}
	if out := send("equip baton wield"); !strings.Contains(strings.ToLower(out), "baton") {
		t.Fatalf("equip baton wield did not confirm the baton:\n%s", out)
	}
	// Into the ganger's turf.
	send("teleport shadowrun:market-street")

	// Fight with the baton. Detect the KNOCK-OUT; fail if we ever see a lethal
	// "slain" line (that would mean the stun routing broke and it hit hp).
	knockRe := regexp.MustCompile(`(?i)knock a street ganger out cold`)
	slainRe := regexp.MustCompile(`(?i)slain a street ganger|street ganger is dead`)
	deadline := time.Now().Add(120 * time.Second)
	knocked := false
	for time.Now().Before(deadline) {
		out := send("kill ganger")
		acc := out + c.Drain(2000*time.Millisecond)
		send("restore") // keep us alive through the ganger's katana
		if slainRe.MatchString(acc) {
			t.Fatalf("stun-routing FAIL: the stun baton SLEW the ganger (should knock out — target_pool: stun):\n%s", acc)
		}
		if knockRe.MatchString(acc) {
			knocked = true
			break
		}
	}
	if !knocked {
		t.Fatalf("stun-routing FAIL: never knocked the ganger out within 120s")
	}

	// No corpse: a knock-out leaves the foe alive + unconscious, so nothing to loot.
	loot := send("loot")
	if strings.Contains(strings.ToLower(loot), "corpse of a street ganger") {
		t.Errorf("stun-routing FAIL: a knock-out should leave NO lootable corpse:\n%s", loot)
	}
	t.Log("shadowrun verified live: created a Street Samurai, stun-batoned the ganger → knocked out (Stun monitor), no corpse")
}
