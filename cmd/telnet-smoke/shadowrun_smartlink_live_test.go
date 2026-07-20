//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_SmartlinkPairing proves the smartlink↔smartgun cross-domain pairing
// (item-modification §6): the to-hit bonus (surfaced as the `score` "Smartlink:
// active" cue) requires BOTH a smartlink installed in a worn cybereye AND a
// smartgun accessory on the WIELDED weapon — either alone does nothing. It ties
// the cyberware-cluster and weapon-accessory host domains together.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_SmartlinkPairing -v
func TestLive_SmartlinkPairing(t *testing.T) {
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
	paired := func(sheet string) bool { return strings.Contains(sheet, "Smartlink") }

	// Baseline: no smart gear → no pairing.
	if paired(send("score")) {
		t.Fatal("score shows a smartlink pairing before any smart gear is fitted")
	}

	// Smartlink into a cybereye, worn — but no smartgun yet: still NO pairing.
	send("spawn item cybereyes me")
	send("spawn item smartlink me")
	if out := send("modify cybereyes smartlink"); !strings.Contains(strings.ToLower(out), "install") {
		t.Fatalf("could not install the smartlink into the cybereyes:\n%s", out)
	}
	if out := send("equip cybereyes"); !strings.Contains(strings.ToLower(out), "cybereyes") {
		t.Fatalf("could not equip the cybereyes:\n%s", out)
	}
	if paired(send("score")) {
		t.Fatal("a smartlink WITHOUT a smartgun wrongly reports a pairing")
	}

	// Now the other half: wield an Ares Predator V and bolt a smartgun system onto
	// it. Both halves present → pairing ACTIVE. Self-provision the pistol (the
	// street-corner start no longer drops one — creation gear replaced the go-bag).
	send("spawn item smartgun-system me")
	send("spawn item ares-predator-v me")
	if out := send("equip predator wield"); !strings.Contains(strings.ToLower(out), "predator") {
		t.Fatalf("could not wield the spawned pistol:\n%s", out)
	}
	if out := send("modify predator smartgun"); !strings.Contains(strings.ToLower(out), "attach") {
		t.Fatalf("could not attach the smartgun to the wielded pistol:\n%s", out)
	}
	if !paired(send("score")) {
		t.Fatal("smartlink + smartgun both present, but score shows no pairing")
	}

	// Un-wield the smart gun → the pairing drops (the smartlink alone is inert).
	send("unequip predator")
	if paired(send("score")) {
		t.Fatal("pairing still reported after un-wielding the smartgun weapon")
	}
}
