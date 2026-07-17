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

// TestLive_ShadowrunKarmaAdvance proves the Shadowrun world advances on the
// karma-ledger strategy (SR-M5, Decision D3 Option B), NOT the level-track
// engine: the pack's manifest `advancement: karma-ledger` routes every earned
// reward into a spendable karma balance instead of onto a progression track, so
// a Sixth-World runner is level-less. It exercises the SR pack end to end:
//
//   - a fresh Street Samurai's `score` shows a "Karma" line (0 spendable /
//     0 earned) and NO "Level N" line — the level/track block is suppressed for
//     a karma-ledger character;
//
//   - killing a street ganger (xp_value 30) banks karma via the grouping
//     kill-reward seam ("You gain 30 karma." — not "experience");
//
//   - the score's karma balance rises by the ganger's 30 (both spendable and
//     lifetime-earned), and the character never leveled.
//
//     ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunKarmaAdvance -v
//
// Deterministic: the runner is Strength-buffed (SR damage) + `restore`d each
// round so it always wins without dying; the kill supplies the earn signal.
func TestLive_ShadowrunKarmaAdvance(t *testing.T) {
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
	// The DEFAULT wizard flow yields a Street Samurai — world-scoped class/
	// background menus mean the shadowrun world offers only its own
	// `street-samurai`/`street-kid` (tapestry-core's fighter/commoner no longer
	// leak in), so createAndLogin's first-option picks build a real Street Samurai.
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

	// "Karma   <current> (<total> earned)" — grab the spendable balance.
	karmaRe := regexp.MustCompile(`(?i)Karma\s+([\d,]+)`)
	scoreKarma := func(sheet string) int {
		t.Helper()
		m := karmaRe.FindStringSubmatch(sheet)
		if m == nil {
			t.Fatalf("no \"Karma N\" on the score sheet (karma-ledger line missing):\n%s", sheet)
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("parse karma from %q: %v", m[1], err)
		}
		return n
	}
	levelRe := regexp.MustCompile(`(?i)Level \d+`)

	// Starting state: a fresh karma-ledger runner shows a Karma line, no Level.
	sheet := send("score")
	if levelRe.MatchString(sheet) {
		t.Fatalf("a karma-ledger Street Samurai should be level-less, but score shows a Level line:\n%s", sheet)
	}
	if start := scoreKarma(sheet); start != 0 {
		t.Fatalf("fresh runner started with %d karma, want 0", start)
	}

	// Gear + Strength buff for a survivable, karma-clean fight.
	send("set stat strength Runner 6")
	send("restore")
	if out := send("get katana"); regexp.MustCompile(`(?i)don't see`).MatchString(out) {
		t.Fatalf("could not get the katana:\n%s", out)
	}
	send("equip katana wield")
	send("teleport shadowrun:market-street")

	// Kill the ganger → the earn signal proves an SR-mob kill banks KARMA (not
	// XP) via the grouping kill-reward seam awarding xp_value 30 to a party of one.
	gainRe := regexp.MustCompile(`You gain 30 karma\.`)
	deadline := time.Now().Add(120 * time.Second)
	var acc strings.Builder
	for time.Now().Before(deadline) {
		acc.WriteString(send("kill ganger") + c.Drain(2000*time.Millisecond))
		send("restore")
		if gainRe.MatchString(acc.String()) {
			break
		}
	}
	if !gainRe.MatchString(acc.String()) {
		t.Fatalf("kill did not bank 30 karma (grouping kill-reward → karma-ledger routing):\n%s", acc.String())
	}

	// The kill's karma landed in the ledger: the score's spendable balance rose
	// by the ganger's 30, and the runner still never leveled.
	after := send("score")
	if got := scoreKarma(after); got != 30 {
		t.Fatalf("kill karma did not land in the ledger: karma %d, want 30:\n%s", got, after)
	}
	if levelRe.MatchString(after) {
		t.Fatalf("runner leveled on a karma-ledger world (should be impossible):\n%s", after)
	}
	t.Log("shadowrun verified live: a level-less Street Samurai banked 30 karma from a ganger kill into the spendable ledger")
}
