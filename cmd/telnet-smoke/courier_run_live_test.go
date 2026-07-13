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

// TestLive_CourierRun proves the deliver-shaped run "The Hand-Off" end to end,
// and with it the give-to-NPC path that makes the `deliver` objective usable:
// pick up a spawned package at the meet, carry it across the sprawl, and
// `give package to contact` in person — the delivery completes the run and pays
// out. The run is prereq-gated behind Redmond Retrieval, so the test clears that
// first (the same proven flow as TestLive_JohnsonsRun).
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_CourierRun -v
func TestLive_CourierRun(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner", // the corner has a katana
		"ANOTHERMUD_ROLE_SEED":  "Courier:admin",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Courier"); err != nil {
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
	nuyenRe := regexp.MustCompile(`Nuyen\s+([\d,]+)`)
	nuyen := func() int {
		m := nuyenRe.FindStringSubmatch(send("score"))
		if m == nil {
			t.Fatal("no Nuyen purse on the score sheet")
		}
		n, _ := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
		return n
	}

	// --- clear the prereq: Redmond Retrieval ---
	send("xp 5000")
	send("set stat strength Courier 6")
	send("restore")
	send("get katana")
	send("equip katana wield")
	send("teleport shadowrun:dantes-inferno")
	send("accept Redmond Retrieval")
	send("teleport shadowrun:avondale")
	send("get chip")
	slainRe := regexp.MustCompile(`(?i)slain a street ganger|street ganger is dead|killed a street ganger`)
	kills := 0
	deadline := time.Now().Add(150 * time.Second)
	for kills < 2 && time.Now().Before(deadline) {
		acc := send("kill ganger") + c.Drain(2000*time.Millisecond)
		send("restore")
		kills += len(slainRe.FindAllString(acc, -1))
	}
	if kills < 2 {
		t.Fatalf("could not clear the prereq run (killed %d/2 gangers)", kills)
	}
	send("teleport shadowrun:dantes-inferno")
	send("talk johnson") // turn in Redmond Retrieval

	// --- the courier run: The Hand-Off ---
	if out := send("talk johnson"); !strings.Contains(out, "The Hand-Off") {
		t.Fatalf("The Hand-Off should be offered once Redmond Retrieval is done:\n%s", out)
	}
	before := nuyen()
	send("accept The Hand-Off")

	// Stage 1 (pickup): the package spawned here at the meet.
	if out := send("get package"); !strings.Contains(strings.ToLower(out), "package") &&
		!strings.Contains(strings.ToLower(out), "pick up") {
		t.Fatalf("could not pick up the courier package at the meet:\n%s", out)
	}
	// Stage 2 (deliver): the contact spawned in Loveland — hand it over in person.
	send("teleport shadowrun:loveland")
	if out := send("give package to contact"); !strings.Contains(strings.ToLower(out), "give") {
		t.Fatalf("give package to contact did not resolve the NPC recipient:\n%s", out)
	}
	c.Drain(2 * time.Second) // absorb the completion + reward banners

	// Delivery auto-grants the run: the payout lands on the score sheet.
	after := nuyen()
	if after-before < 2000 {
		t.Fatalf("The Hand-Off payout did not land after delivery: before=%d after=%d (want ~+2200)", before, after)
	}
}
