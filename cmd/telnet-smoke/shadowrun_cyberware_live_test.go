//go:build unix

package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ShadowrunCyberware closes SR-M3 acceptance [98]: cyberware
// equipped/removed shifts the sourced attribute (via srckey), visible on
// `score`. On a real shadowrun boot a Street Samurai installs wired reflexes
// (a cyberware-slot item whose modifier adds +2 Reaction) and the REA on the
// score sheet rises by 2; unequipping it drops REA back. No bespoke cyberware
// code — the item rides the standard equip → EquipmentSourceKey → stat-block
// modifier pipeline, and `score` reads the effective attribute.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunCyberware -v
func TestLive_ShadowrunCyberware(t *testing.T) {
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
	reaRe := regexp.MustCompile(`(?i)REA\s+(\d+)`)
	reaction := func(sheet string) int {
		t.Helper()
		m := reaRe.FindStringSubmatch(sheet)
		if m == nil {
			t.Fatalf("no REA attribute on the score sheet:\n%s", sheet)
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("parse REA from %q: %v", m[1], err)
		}
		return n
	}

	base := reaction(send("score"))

	// Install wired reflexes: grab it from the street-corner sample tray and
	// equip it into the cyberware slot (its single eligible slot auto-resolves).
	if out := send("get reflexes"); strings.Contains(strings.ToLower(out), "don't see") {
		t.Fatalf("could not get wired reflexes from the street corner:\n%s", out)
	}
	if out := send("equip reflexes"); !strings.Contains(strings.ToLower(out), "wired reflexes") {
		t.Fatalf("equip reflexes did not confirm:\n%s", out)
	}
	if got := reaction(send("score")); got != base+2 {
		t.Fatalf("cyberware did not raise Reaction: REA %d after installing wired reflexes, want %d (base %d + 2)", got, base+2, base)
	}

	// Remove it: the modifier comes off its source, REA returns to base.
	if out := send("unequip reflexes"); !strings.Contains(strings.ToLower(out), "wired reflexes") {
		t.Fatalf("unequip reflexes did not confirm:\n%s", out)
	}
	if got := reaction(send("score")); got != base {
		t.Fatalf("removing cyberware did not restore Reaction: REA %d after unequip, want base %d", got, base)
	}
	t.Logf("shadowrun verified live: wired reflexes raised Reaction %d→%d on equip and restored it on unequip (srckey equip pipeline)", base, base+2)
}
