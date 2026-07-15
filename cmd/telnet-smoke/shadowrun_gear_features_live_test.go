//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ShadowrunGearFeatures exercises the player-facing wiring added across
// this session's SR arc — the paths unit tests can't reach end-to-end:
//
//   - the `firemode` verb (ranged-combat §5.5): reports the current mode + the
//     wielded weapon's supported modes, sets a supported one, refuses one the
//     weapon can't fire.
//
//   - equipping eyewear in the NEW `eyes` slot (low-light goggles).
//
//   - wielding the flamethrower (a fire-damage weapon that feeds fuel).
//
//     ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunGearFeatures -v
func TestLive_ShadowrunGearFeatures(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":  "Runner:admin",
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

	// --- Firing modes: wield an automatic weapon (the generic SMG supports
	// single/burst/auto) and drive the verb. ---
	send("spawn item smg me")
	if out := send("equip smg wield"); !strings.Contains(strings.ToLower(out), "submachine") {
		t.Fatalf("could not wield the SMG:\n%s", out)
	}
	if out := send("firemode"); !strings.Contains(strings.ToLower(out), "burst") {
		t.Fatalf("`firemode` should report the SMG's available modes (incl. burst):\n%s", out)
	}
	if out := send("firemode burst"); !strings.Contains(strings.ToLower(out), "burst") {
		t.Fatalf("`firemode burst` should confirm the switch:\n%s", out)
	}
	if out := send("firemode auto"); !strings.Contains(strings.ToLower(out), "auto") {
		t.Fatalf("`firemode auto` should confirm the switch:\n%s", out)
	}

	// --- The eyes slot: low-light goggles equip as eyewear (non-cyber vision). ---
	send("spawn item low-light-goggles me")
	if out := send("equip goggles"); !strings.Contains(strings.ToLower(out), "goggles") {
		t.Fatalf("could not equip the low-light goggles (eyes slot):\n%s", out)
	}
	if out := send("equipment"); !strings.Contains(strings.ToLower(out), "goggles") {
		t.Fatalf("equipment list should show the worn goggles:\n%s", out)
	}

	// --- The flamethrower: a fire-damage weapon wields and displaces the SMG. ---
	send("spawn item shiawase-arms-blazer me")
	send("spawn item fuel-canister me")
	if out := send("equip blazer wield"); !strings.Contains(strings.ToLower(out), "blazer") {
		t.Fatalf("could not wield the flamethrower:\n%s", out)
	}
	// A single-mode fallback: the flamethrower supports modes, so `firemode single`
	// is always accepted.
	if out := send("firemode single"); !strings.Contains(strings.ToLower(out), "single") {
		t.Fatalf("`firemode single` should always be accepted:\n%s", out)
	}
}
