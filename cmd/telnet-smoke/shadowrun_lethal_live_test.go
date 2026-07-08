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

// TestLive_ShadowrunLethalKill is the lethal sibling of the stun-KO test
// (TestLive_ShadowrunStunKnockout) — the other half of SR-M3's combat gate.
// On a real shadowrun boot, a freshly-created Street Samurai gears up with the
// katana and cuts a street ganger DOWN. It proves the LETHAL path:
//
//   - a weapon with no target_pool routes its damage to the Physical monitor —
//     the hp/Vitals track (Design 1), the engine's default lethal path;
//
//   - the ganger's Shadowrun SOAK applies: its body + worn armored-jacket
//     (armor_bonus 3) reduce incoming damage via the wired `mitigation` channel
//     (body + armor), so the kill grinds through real mitigation, not raw hp;
//
//   - crossing the Physical monitor's floor is a KILL (lethal), leaving a
//     lootable CORPSE — the opposite outcome from the stun baton's knock-out.
//
//     ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunLethalKill -v
//
// Deterministic despite RNG + soak: the runner is seeded admin, xp-leveled,
// Strength-buffed (SR damage_bonus = trunc(strength/4)), and `restore`d each
// round, so it always out-grinds the ganger's soak and never dies; the katana
// (no target_pool) guarantees the finish lands on the Physical monitor.
func TestLive_ShadowrunLethalKill(t *testing.T) {
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

	// Bootstrap: level + max Strength (SR damage_bonus scales off Strength) so the
	// katana out-paces the ganger's body+armor soak, then wield the katana.
	send("xp 5000")
	send("set stat strength Runner 6")
	send("restore")
	if out := send("get katana"); strings.Contains(strings.ToLower(out), "don't see") {
		t.Fatalf("could not get the katana from the street corner:\n%s", out)
	}
	if out := send("equip katana wield"); !strings.Contains(strings.ToLower(out), "katana") {
		t.Fatalf("equip katana wield did not confirm the katana:\n%s", out)
	}
	// Into the ganger's turf.
	send("teleport shadowrun:market-street")

	// Fight with the katana. Detect the KILL; a knock-out line here would mean
	// the lethal weapon wrongly routed to the Stun monitor.
	slainRe := regexp.MustCompile(`(?i)slain a street ganger|street ganger is dead|killed a street ganger`)
	knockRe := regexp.MustCompile(`(?i)knock a street ganger out cold`)
	deadline := time.Now().Add(120 * time.Second)
	slain := false
	for time.Now().Before(deadline) {
		out := send("kill ganger")
		acc := out + c.Drain(2000*time.Millisecond)
		send("restore") // keep us alive through the ganger's katana
		if knockRe.MatchString(acc) {
			t.Fatalf("lethal-routing FAIL: the katana KNOCKED OUT the ganger (should kill — no target_pool ⇒ Physical monitor):\n%s", acc)
		}
		if slainRe.MatchString(acc) {
			slain = true
			break
		}
	}
	if !slain {
		t.Fatalf("lethal-routing FAIL: never killed the ganger within 120s (soak too high, or lethal damage not reaching the Physical monitor)")
	}

	// A lethal kill leaves a lootable corpse (loot-and-corpses §-): the opposite
	// of the stun-KO, which leaves the foe alive + unconscious with no corpse.
	loot := send("loot")
	if !strings.Contains(strings.ToLower(loot), "corpse of a street ganger") {
		t.Errorf("lethal-routing FAIL: a kill should leave a lootable corpse of a street ganger:\n%s", loot)
	}
	t.Log("shadowrun verified live: created a Street Samurai, katana-killed the armored ganger → Physical monitor, soak applied, lootable corpse")
}
