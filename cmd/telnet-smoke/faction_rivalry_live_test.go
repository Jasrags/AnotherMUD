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

// TestLive_FactionRivalry proves the Johnson-vs-Kestrel rivalry is mechanical:
// the two content factions (The Megacorps / The Streets) load, and running a
// street job for Kestrel shifts standing BOTH ways — up with The Streets, down
// with The Megacorps — so choosing one side literally lowers the other. It also
// confirms the positive side of the faction gate (Kestrel's follow-up opens once
// street standing is non-negative).
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_FactionRivalry -v
func TestLive_FactionRivalry(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":  "Sideswitch:admin",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Sideswitch"); err != nil {
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

	// Both content factions exist and start Neutral for a fresh runner.
	fresh := send("standing")
	if !strings.Contains(fresh, "The Megacorps") || !strings.Contains(fresh, "The Streets") {
		t.Fatalf("both rival factions should appear on a fresh runner's standing sheet:\n%s", fresh)
	}

	// Bootstrap a fighter and run Kestrel's street starter (Protection).
	send("xp 5000")
	send("set stat strength Sideswitch 6")
	send("restore")
	send("get katana")
	send("equip katana wield")
	send("teleport shadowrun:the-big-rhino")
	send("accept Protection")
	send("teleport shadowrun:sophocles") // stage 2 spawns 3 Stiletto raiders here

	slainRe := regexp.MustCompile(`(?i)slain a street ganger|street ganger is dead|killed a street ganger`)
	kills := 0
	deadline := time.Now().Add(180 * time.Second)
	for kills < 3 && time.Now().Before(deadline) {
		acc := send("kill ganger") + c.Drain(2000*time.Millisecond)
		send("restore")
		kills += len(slainRe.FindAllString(acc, -1))
	}
	if kills < 3 {
		t.Fatalf("could not clear the raiders (killed %d/3)", kills)
	}
	// Turn Protection in to Kestrel — the faction reward lands here.
	send("teleport shadowrun:the-big-rhino")
	send("talk kestrel")
	c.Drain(2 * time.Second)

	// Standing moved BOTH ways: +150 with The Streets, -100 with The Megacorps.
	out := send("standing")
	streetsUp := regexp.MustCompile(`The Streets\s+\S+\s+\(150\)`)
	corpsDown := regexp.MustCompile(`The Megacorps\s+\S+\s+\(-100\)`)
	if !streetsUp.MatchString(out) {
		t.Fatalf("running for Kestrel did not raise The Streets to 150:\n%s", out)
	}
	if !corpsDown.MatchString(out) {
		t.Fatalf("running for Kestrel did not lower The Megacorps to -100 (the rivalry isn't mechanical):\n%s", out)
	}

	// The positive gate: with street standing >= 0, Kestrel's gated follow-up opens.
	if board := send("talk kestrel"); !strings.Contains(board, "Care Package") {
		t.Fatalf("Care Package should open once street standing is earned:\n%s", board)
	}
}
