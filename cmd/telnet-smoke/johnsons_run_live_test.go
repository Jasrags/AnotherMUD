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

// TestLive_JohnsonsRun drives "Redmond Retrieval" (quests/johnsons-run) end to
// end on a real Shadowrun boot — the iconic meet -> run -> payout:
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_JohnsonsRun -v
//
// Flow: an (admin, over-equipped) runner meets the Johnson at Dante's, hears his
// patter (`ask johnson`) and the offer (`talk johnson`), accepts, teleports to
// Avondale (visit objective), grabs the paydata chip (collect objective), kills
// the two Rusted Stilettos (kill objective x2), returns, and turns the run in —
// at which point the nuyen payout lands on the score sheet. Combat is made
// reliable the same way the other SR combat tests do it: admin xp/str bootstrap,
// a katana, and `restore` between swings, with a generous deadline.
func TestLive_JohnsonsRun(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner", // the corner has a katana on the floor
		"ANOTHERMUD_ROLE_SEED":  "Runner:admin",            // xp/set/teleport/restore for a reliable fight
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

	// Bootstrap a fighter (mirrors shadowrun_lethal): level + max Strength so the
	// katana out-paces the gangers' soak, grab the corner's katana, wield it.
	send("xp 5000")
	send("set stat strength Runner 6")
	send("restore")
	if out := send("get katana"); strings.Contains(strings.ToLower(out), "don't see") {
		t.Fatalf("could not get the katana from the street corner:\n%s", out)
	}
	send("equip katana wield")

	// The SR score sheet renders the purse as "Nuyen  <n>¥" (currency reskin).
	nuyenRe := regexp.MustCompile(`Nuyen\s+([\d,]+)`)
	nuyen := func() int {
		m := nuyenRe.FindStringSubmatch(send("score"))
		if m == nil {
			t.Fatal("no Nuyen purse on the score sheet")
		}
		n, _ := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
		return n
	}

	// The meet at Dante's Inferno.
	send("teleport shadowrun:dantes-inferno")

	// Johnson's patter (the dialogue verb) works on him too.
	if out := send("ask johnson about run"); !strings.Contains(strings.ToLower(out), "courier") &&
		!strings.Contains(strings.ToLower(out), "retrieval") {
		t.Fatalf("ask johnson about run did not speak his run patter:\n%s", out)
	}
	// The offer surfaces from the giver.
	if out := send("talk johnson"); !strings.Contains(out, "Redmond Retrieval") {
		t.Fatalf("talk johnson did not surface the Redmond Retrieval offer:\n%s", out)
	}

	before := nuyen()

	if out := send("accept Redmond Retrieval"); strings.Contains(strings.ToLower(out), "requirements") {
		t.Fatalf("Redmond Retrieval was prereq-refused:\n%s", out)
	}

	// Stage 1 — the site: teleport emits PlayerMoved, advancing the visit objective.
	send("teleport shadowrun:avondale")

	// Stage 2 — the job: recover the chip (collect) ...
	if out := send("get chip"); !strings.Contains(strings.ToLower(out), "paydata") &&
		!strings.Contains(strings.ToLower(out), "pick up") {
		t.Fatalf("could not pick up the paydata chip in Avondale:\n%s", out)
	}

	// ... and drop the two Rusted Stilettos (kill x2), restoring between swings.
	slainRe := regexp.MustCompile(`(?i)slain a street ganger|street ganger is dead|killed a street ganger`)
	kills := 0
	deadline := time.Now().Add(150 * time.Second)
	for kills < 2 && time.Now().Before(deadline) {
		acc := send("kill ganger") + c.Drain(2000*time.Millisecond)
		send("restore") // survive the gangers' katanas
		for _, ln := range slainRe.FindAllString(acc, -1) {
			_ = ln
			kills++
		}
	}
	if kills < 2 {
		t.Fatalf("did not kill both Rusted Stilettos within the deadline (killed %d)", kills)
	}

	// The payout: return to Johnson and turn the run in. The reward (nuyen)
	// lands on the score sheet.
	send("teleport shadowrun:dantes-inferno")
	send("talk johnson") // claims the ready turn-in; completion banner flows via the notifier
	c.Drain(2 * time.Second)

	after := nuyen()
	if after <= before {
		t.Fatalf("nuyen did not increase after turning in Redmond Retrieval: before=%d after=%d (want +2500 reward)", before, after)
	}
	if got := after - before; got < 2000 {
		t.Fatalf("Redmond Retrieval payout too small: +%d nuyen (want ~2500)", got)
	}

	// The collected chip is a quest spawn — turn-in cleanup removes it from the
	// runner's bag (quest-spawns.md §5 / Phase 1b), so it is not a souvenir.
	if out := send("inventory"); strings.Contains(strings.ToLower(out), "paydata") {
		t.Fatalf("the paydata chip should be cleaned from inventory after turn-in:\n%s", out)
	}
}
