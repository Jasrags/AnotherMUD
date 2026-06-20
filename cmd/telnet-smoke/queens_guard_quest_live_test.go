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

// TestLive_QueensGuardQuestline drives the M30 demo questline end-to-end against
// a real WoT boot, proving the faction + reputation earn paths and the prereq
// gate fire in play:
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_QueensGuardQuestline -v
//
// Flow: a fresh character reads Unknown renown; the follow-up `the-queens-trust`
// is refused (faction prereq unmet); accepting + completing `oath-to-the-queen`
// (auto-grant on the visit objective, reachable by admin teleport) grants +700
// Queen's Guard standing and +120 renown; afterwards `score`/`standing` show the
// new values and `the-queens-trust` is accepted (the prereq now met).
func TestLive_QueensGuardQuestline(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "wot",
		"ANOTHERMUD_START_ROOM": "wot:the-green",
	})

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	// First character of a fresh store is auto-granted admin → `teleport` works.
	if err := createAndLogin(c, "Oathkeeper"); err != nil {
		t.Fatalf("create player: %v", err)
	}

	// 1. A fresh character is renown Unknown (0).
	if err := c.SendLine("score"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExpectStringTimeout("Unknown (0)", 6*time.Second); err != nil {
		t.Fatalf("fresh score did not show Renown Unknown (0): %v", err)
	}

	// 2. The follow-up quest is refused before the standing is earned (F2 prereq).
	if err := c.SendLine("accept the-queens-trust"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExpectStringTimeout("don't meet the requirements", 6*time.Second); err != nil {
		t.Fatalf("the-queens-trust should be prereq-refused before standing is earned: %v", err)
	}

	// 3. Accept the oath quest (no giver-presence required; no prereq).
	if err := c.SendLine("accept oath-to-the-queen"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExpectTimeout(regexp.MustCompile(`(?i)oath|accepted|square`), 6*time.Second); err != nil {
		t.Fatalf("oath-to-the-queen was not accepted: %v", err)
	}

	// 4. Complete the visit objective by teleporting to the square (teleport emits
	//    PlayerMoved, which the quest watcher advances visit objectives on). The
	//    auto-grant reward then fires (+700 standing, +120 renown, teach the skill).
	if err := c.SendLine("teleport wot:the-caemlyn-square"); err != nil {
		t.Fatal(err)
	}
	c.Drain(2 * time.Second) // absorb arrival + reward/completion banners

	// 5. score now shows the earned renown: 120 → Known Locally.
	if err := c.SendLine("score"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExpectStringTimeout("Known Locally (120)", 6*time.Second); err != nil {
		t.Fatalf("score did not show the earned renown Known Locally (120): %v", err)
	}

	// 6. standing shows the Queen's Guard at the granted +700.
	if err := c.SendLine("standing"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExpectStringTimeout("(700)", 6*time.Second); err != nil {
		t.Fatalf("standing did not show the Queen's Guard at 700: %v", err)
	}

	// 7. With +700 (≥ the 500 floor) the follow-up is now accepted (F2 prereq met).
	if err := c.SendLine("accept the-queens-trust"); err != nil {
		t.Fatal(err)
	}
	out, err := c.ExpectTimeout(regexp.MustCompile(`(?i)trust|accepted|plaza`), 6*time.Second)
	if err != nil {
		t.Fatalf("the-queens-trust was not accepted after earning standing: %v", err)
	}
	if strings.Contains(strings.ToLower(out), "requirements") {
		t.Fatalf("the-queens-trust still prereq-refused after earning standing: %q", out)
	}
}
