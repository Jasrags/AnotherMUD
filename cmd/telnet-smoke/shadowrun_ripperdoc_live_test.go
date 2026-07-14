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

// TestLive_ShadowrunRipperdocClinic exercises the first real street-doc: on a
// real shadowrun boot (starting in Downtown, the make run-shadowrun start) a
// runner reaches Scalpel's Chrome Den in the Puyallup Barrens, sees her deep
// chrome stock, buys a cyberarm, and installs it — the nuyen comes off the purse
// and the Essence budget (SR-M4) drops by the arm's 1.0 cost. Uses admin
// teleport + a nuyen grant so the test doesn't have to grind a payday first.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunRipperdocClinic -v
func TestLive_ShadowrunRipperdocClinic(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:westlake-plaza", // the real Downtown start
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
	essRe := regexp.MustCompile(`(?i)Essence\s+(\d+\.\d+)`)
	essence := func(sheet string) string {
		t.Helper()
		m := essRe.FindStringSubmatch(sheet)
		if m == nil {
			t.Fatalf("no Essence line on the score sheet:\n%s", sheet)
		}
		return m[1]
	}

	// Grant a payday's worth of nuyen so we can afford chrome, then drop into the
	// clinic (teleport is the admin shortcut for the multi-room Barrens walk).
	send("set gold amount Runner 200000")
	den := send("teleport shadowrun:chrome-den")
	if !strings.Contains(strings.ToLower(den), "chrome den") {
		t.Fatalf("teleport did not land in the Chrome Den:\n%s", den)
	}
	if !strings.Contains(strings.ToLower(den), "scalpel") {
		t.Fatalf("Scalpel the ripperdoc is not in the Chrome Den:\n%s", den)
	}

	// Her stock is the deep chrome the tutorial chop-doc only teases.
	stock := strings.ToLower(send("list"))
	for _, want := range []string{"cyberarm", "dermal plating", "reaction enhancers"} {
		if !strings.Contains(stock, want) {
			t.Fatalf("Scalpel's stock is missing %q:\n%s", want, stock)
		}
	}

	if got := essence(send("score")); got != "6.0" {
		t.Fatalf("Essence before install = %s, want 6.0", got)
	}

	// Buy the cyberarm (nuyen off the purse) and install it (Essence off the soul).
	buy := send("buy cyberarm")
	if !strings.Contains(strings.ToLower(buy), "you buy") {
		t.Fatalf("buy cyberarm did not confirm a purchase:\n%s", buy)
	}
	if out := send("equip cyberarm"); !strings.Contains(strings.ToLower(out), "cyberarm") {
		t.Fatalf("equip cyberarm did not confirm:\n%s", out)
	}
	if got := essence(send("score")); got != "5.0" {
		t.Fatalf("Essence after installing the cyberarm = %s, want 5.0 (6.0 - 1.0)", got)
	}
	t.Logf("first real street-doc verified live: Scalpel's Chrome Den stocks deep chrome; bought + installed a cyberarm, Essence 6.0 -> 5.0")
}
