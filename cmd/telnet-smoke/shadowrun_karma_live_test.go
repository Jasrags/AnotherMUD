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

// TestLive_ShadowrunKarmaAdvance closes the last combat-adjacent SR-M3 gate
// (acceptance [100]): advancement runs on the existing engine as karma-as-XP
// (pinned decision D3, Option A) — a kill grants karma on the Shadowrun world
// track, and accumulating it advances the track a level. It proves the SR pack
// composes with the generic progression engine end to end:
//
//   - a Street Samurai's class binds the shadowrun `street` track ("The Long
//     Run"), shown on `score`;
//
//   - killing a street ganger (xp_value 30) awards the full 30 to the solo
//     killer via the grouping kill-XP seam ("You gain 30 experience.");
//
//   - crossing the track's level-2 threshold (100 XP on street.yaml's curve)
//     advances the character to Level 2 on that track.
//
//     ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunKarmaAdvance -v
//
// Deterministic: the runner is Strength-buffed (SR damage) + `restore`d each
// round so it always wins without dying; the kill supplies the earn signal, and
// an admin `xp` top-up fast-forwards the accumulation to the level threshold
// (the *earn from a kill* is already proven above — this half proves the SR
// track LEVELS, a generic mechanic exercised here on the SR track specifically).
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
	// Explicitly select the Shadowrun class + background: the default wizard flow
	// picks the FIRST offered option, which today is tapestry-core's `fighter`
	// (on the `adventurer` track) — the shadowrun world has no CreationFlowFor
	// customization yet, so core classes leak into its creation menu. Selecting
	// street-samurai gets us a real Street Samurai on the SR `street` track.
	isNew, err := doLogin(c, "Runner")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	// Answers are matched by case-insensitive LABEL prefix (wizard resolveChoice),
	// not by id — so "Street Samurai"/"Street Kid" (the display labels), not the
	// hyphenated ids.
	if err := finishLogin(c, "Runner", isNew, map[string]string{
		"class":      "Street Samurai",
		"background": "Street Kid",
	}); err != nil {
		t.Fatalf("create Street Samurai: %v", err)
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
	levelRe := regexp.MustCompile(`(?i)Level (\d+)`)
	scoreLevel := func(sheet string) int {
		t.Helper()
		m := levelRe.FindStringSubmatch(sheet)
		if m == nil {
			t.Fatalf("no \"Level N\" on the score sheet:\n%s", sheet)
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("parse level from %q: %v", m[1], err)
		}
		return n
	}

	xpRe := regexp.MustCompile(`(?i)XP\s+([\d,]+)`)
	scoreXP := func(sheet string) int {
		t.Helper()
		m := xpRe.FindStringSubmatch(sheet)
		if m == nil {
			t.Fatalf("no \"XP N\" on the score sheet:\n%s", sheet)
		}
		n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
		if err != nil {
			t.Fatalf("parse XP from %q: %v", m[1], err)
		}
		return n
	}

	// Starting state: a fresh Street Samurai's HEADLINE track is the SR world
	// track (its class bound_track), Level 1 / 0 XP — not core's "adventurer".
	sheet := send("score")
	if !strings.Contains(sheet, "The Long Run") {
		t.Fatalf("score headline track is not the Shadowrun \"The Long Run\" — the primary-track (class bound_track) resolution regressed:\n%s", sheet)
	}
	if start := scoreLevel(sheet); start != 1 {
		t.Fatalf("fresh Street Samurai started at Level %d, want 1", start)
	}
	startXP := scoreXP(sheet)

	// Gear + Strength buff for a survivable, XP-clean fight (no admin xp yet, so
	// the kill's award is the only XP on the board when we assert the earn line).
	send("set stat strength Runner 6")
	send("restore")
	if out := send("get katana"); strings.Contains(strings.ToLower(out), "don't see") {
		t.Fatalf("could not get the katana:\n%s", out)
	}
	send("equip katana wield")
	send("teleport shadowrun:market-street")

	// Kill the ganger → the earn signal proves an SR-mob kill grants karma on the
	// SR track (the grouping kill-XP seam awarding xp_value 30 to a party of one).
	gainRe := regexp.MustCompile(`You gain 30 experience\.`)
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
		t.Fatalf("kill did not grant 30 karma/XP on the SR track (grouping kill-XP → street.yaml):\n%s", acc.String())
	}

	// The kill's karma landed on the SR track specifically: the headline (street)
	// XP rose by the ganger's 30 — proof the kill-XP routes to the killer's class
	// bound_track, not the engine-default "adventurer".
	if got := scoreXP(send("score")); got != startXP+30 {
		t.Fatalf("kill karma did not land on The Long Run: street XP %d, want %d (start %d + 30 from the ganger)", got, startXP+30, startXP)
	}

	// A track advances: top the street track past street.yaml's level-2 threshold
	// (100 XP; the kill already banked 30) and confirm The Long Run leveled to 2.
	// The track must be named explicitly — the admin `xp` verb defaults to the
	// engine track, which is the whole bug this test guards against.
	send("xp 100 street")
	after := send("score")
	if lvl := scoreLevel(after); lvl != 2 {
		t.Fatalf("SR track did not advance: Level %d after crossing the 100-XP threshold, want 2:\n%s", lvl, after)
	}
	t.Log("shadowrun verified live: ganger kill banked 30 karma on The Long Run (its class bound_track); crossing 100 XP advanced the track to Level 2")
}
