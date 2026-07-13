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

// TestLive_ShadowrunEssence is the SR-M4 end-to-end gate: on a real shadowrun
// boot a Street Samurai starts at 6.0 Essence, installing wired reflexes (a 2.0
// implant) drops the `score` Essence line to 4.0, and removing it restores 6.0.
// Essence is DERIVED from installed chrome — the equip recompute sets it, no
// spend/regen tick involved.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunEssence -v
func TestLive_ShadowrunEssence(t *testing.T) {
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
	// "Essence   6.0 / 6.0" — grab the current (first) value.
	essRe := regexp.MustCompile(`(?i)Essence\s+(\d+\.\d+)`)
	essence := func(sheet string) string {
		t.Helper()
		m := essRe.FindStringSubmatch(sheet)
		if m == nil {
			t.Fatalf("no Essence line on the score sheet:\n%s", sheet)
		}
		return m[1]
	}

	if got := essence(send("score")); got != "6.0" {
		t.Fatalf("fresh runner Essence = %s, want 6.0", got)
	}

	// Install wired reflexes (2.0 Essence) from the street-corner sample tray.
	if out := send("get reflexes"); strings.Contains(strings.ToLower(out), "don't see") {
		t.Fatalf("could not get wired reflexes from the street corner:\n%s", out)
	}
	if out := send("equip reflexes"); !strings.Contains(strings.ToLower(out), "wired reflexes") {
		t.Fatalf("equip reflexes did not confirm:\n%s", out)
	}
	if got := essence(send("score")); got != "4.0" {
		t.Fatalf("Essence after installing wired reflexes = %s, want 4.0 (6.0 − 2.0)", got)
	}

	// Remove it: the Essence is restored.
	if out := send("unequip reflexes"); !strings.Contains(strings.ToLower(out), "wired reflexes") {
		t.Fatalf("unequip reflexes did not confirm:\n%s", out)
	}
	if got := essence(send("score")); got != "6.0" {
		t.Fatalf("Essence after removing wired reflexes = %s, want 6.0 (restored)", got)
	}
	t.Logf("shadowrun verified live: Essence 6.0 → 4.0 on installing wired reflexes and restored to 6.0 on removal")
}
