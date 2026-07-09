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

// TestLive_ShadowrunFirearm is the SR-M3d payoff: the in-room firefight. The
// SR-M3 combat proofs used the katana (melee); this proves the FIREARM + AMMO
// path — a Street Samurai wields the heavy pistol (ranged_class: projectile,
// ammo_kind: bullet) and fires it point-blank at the ganger (single district =
// the melee band, where a gun fires at a penalty per SR5). Two behaviours,
// deterministically decoupled so neither depends on the ganger's hp:
//
//   - DRY FIRST (no ammo): engaging with an empty gun clicks empty every swing
//     (the AmmoFor hook returns can't-fire → RangedDry → the ranged-flavor "dry"
//     line). The runner does no damage, so the ganger stays up while we observe
//     the empty click.
//
//   - THEN HIT (spawn a stack): the same pistol now fires, spending one `bullet`
//     per shot, and a landed shot is lethal — no target_pool, so it lands on the
//     Physical monitor through the ganger's soak (like the katana proof).
//
//     ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunFirearm -v
//
// Deterministic: admin-seeded, Agility-buffed (SR firearm to-hit = skill +
// Agility) so shots connect despite the point-blank penalty, Strength-buffed,
// and `restore`d each round so the ganger's katana never drops the runner.
func TestLive_ShadowrunFirearm(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":           "shadowrun",
		"ANOTHERMUD_START_ROOM":      "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":       "Runner:admin",
		"ANOTHERMUD_RELOAD_DURATION": "0", // instant reload for deterministic asserts
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

	send("xp 5000")
	send("set stat agility Runner 6")  // SR firearm to-hit = skill + Agility
	send("set stat strength Runner 6") // firearm damage bonus (trunc(str/4))
	send("restore")
	// Wield the pistol with NO clip inserted — the dry-fire phase depends on it.
	if out := send("get pistol"); strings.Contains(strings.ToLower(out), "don't see") {
		t.Fatalf("could not get the Ares Predator V from the street corner:\n%s", out)
	}
	if out := send("equip pistol wield"); !strings.Contains(strings.ToLower(out), "pistol") {
		t.Fatalf("equip pistol wield did not confirm:\n%s", out)
	}
	send("teleport shadowrun:market-street")

	// Phase 1 — the empty click. Holder-fed with no clip inserted, the pistol
	// can't fire; every swing runs dry. The runner deals no damage.
	dryRe := regexp.MustCompile(`(?i)no bullet left to shoot`)
	if !fightUntil(t, send, c, dryRe, 30*time.Second) {
		t.Fatal("a clipless pistol never clicked dry in melee — the holder-fed AmmoFor gate isn't skipping the ammoless swing")
	}

	// Phase 2 — the holder model (ammo-and-reloading §3-§5): a clip must be
	// FILLED from loose rounds, then LOADED into the weapon. Spawn a clip + a
	// stack of rounds, `reload clip` to fill it, `reload` to insert it, and the
	// next swings spend its rounds; a landed shot lands on the Physical monitor
	// (lethal, no target_pool) through the ganger's soak.
	send("spawn item predator-clip me")
	send("spawn item ammo-clip 20 me")
	if out := send("reload clip"); !strings.Contains(out, "(15/15)") {
		t.Fatalf("`reload clip` did not fill the clip to 15/15 from loose rounds:\n%s", out)
	}
	if out := send("reload"); !strings.Contains(strings.ToLower(out), "fresh clip") || !strings.Contains(out, "(15/15)") {
		t.Fatalf("`reload` did not insert the loaded clip into the Predator V:\n%s", out)
	}
	hitRe := regexp.MustCompile(`(?i)hit a street ganger for \d+ damage`)
	if !fightUntil(t, send, c, hitRe, 30*time.Second) {
		t.Fatal("the clip-loaded pistol never landed a shot on the ganger — the firearm isn't firing from its inserted holder")
	}
	t.Log("shadowrun verified live: a clipless Ares Predator V clicked dry, then a clip filled from loose rounds and loaded into the gun fired point-blank and hit the ganger on the Physical monitor")
}

// fightUntil keeps the runner engaged with the ganger, `restore`-ing each round,
// until the combat output matches want or the deadline passes. Returns whether
// want was seen.
func fightUntil(t *testing.T, send func(string) string, c *telnettest.Client, want *regexp.Regexp, d time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	var acc strings.Builder
	for time.Now().Before(deadline) {
		acc.WriteString(send("kill ganger") + c.Drain(2000*time.Millisecond))
		send("restore")
		if want.MatchString(acc.String()) {
			return true
		}
	}
	return false
}
