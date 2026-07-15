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

// TestLive_ShadowrunSkillUse proves the recent skills breadth END TO END on a
// real shadowrun boot, all surfaced through the gain-feedback line (skills
// §3.5) with the milestone step forced to 1 so a single gain shows the message:
//
//   - Slice C stealth wiring (skills §2): `hide` trains SNEAKING — the SR pack's
//     single merged stealth skill (manifest stealth_skill: sneaking), NOT the two
//     inert core hide/move-silently skills. Proven by the "You feel your Sneaking
//     improve." line and a skills sheet that carries Sneaking but no Move Silently.
//   - train-on-attack (skills §7): each weapon swing trains the wielded weapon's
//     bound skill. The runner batons the ganger (stun-baton → Clubs) and the
//     "You feel your Clubs improve." line lands mid-fight.
//
// gain-feedback itself (the message + the ANOTHERMUD_SKILL_GAIN_NOTIFY_STEP knob)
// is exercised throughout. Seeded admin + xp/strength/restore so the fight is
// reliable, like the stun-knockout test.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunSkillUse -v
func TestLive_ShadowrunSkillUse(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":                  "shadowrun",
		"ANOTHERMUD_START_ROOM":             "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":              "Runner:admin",
		"ANOTHERMUD_SKILL_GAIN_NOTIFY_STEP": "1", // a line on the FIRST gain, so one gain proves the feature
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

	// --- Slice C: hiding trains SNEAKING (the merged SR stealth skill) ---
	sneakImprove := regexp.MustCompile(`(?i)you feel your sneaking improve`)
	sneaked := false
	for i := 0; i < 60 && !sneaked; i++ {
		out := send("hide")
		if strings.Contains(strings.ToLower(out), "can't hide") {
			t.Fatalf("PLAYTEST FAIL [§26 stealth]: hiding refused on the SR boot:\n%s", out)
		}
		if sneakImprove.MatchString(out) {
			sneaked = true
			break
		}
		// Not hidden anymore so the next `hide` is a fresh attempt (each is one
		// use-gain roll on Sneaking).
		send("unhide")
	}
	if !sneaked {
		t.Fatalf("PLAYTEST FAIL [§26 stealth]: `hide` never trained Sneaking (Slice C wiring) in 60 tries")
	}

	// The skills sheet carries Sneaking (SR's single stealth skill) and NOT the
	// two-axis core Move Silently — proof the redundancy is gone.
	sheet := send("skills")
	if !strings.Contains(sheet, "Sneaking") {
		t.Errorf("PLAYTEST FAIL [§26 stealth]: skills sheet missing Sneaking:\n%s", sheet)
	}
	if strings.Contains(sheet, "Move Silently") {
		t.Errorf("PLAYTEST FAIL [§26 stealth]: SR skills sheet still shows the inert core Move Silently:\n%s", sheet)
	}

	// --- §7: swinging a bound weapon trains its skill (train-on-attack) ---
	// Mirror the stun-knockout setup so the fight is reliable.
	send("unhide") // don't walk into the fight concealed
	send("xp 5000")
	send("set stat strength Runner 6")
	send("restore")
	if out := send("get baton"); strings.Contains(strings.ToLower(out), "don't see") {
		t.Fatalf("could not get the stun baton (Clubs) from the street corner:\n%s", out)
	}
	if out := send("equip baton wield"); !strings.Contains(strings.ToLower(out), "baton") {
		t.Fatalf("equip baton wield did not confirm the baton:\n%s", out)
	}
	send("teleport shadowrun:market-street")

	clubsImprove := regexp.MustCompile(`(?i)you feel your clubs improve`)
	trained := false
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) && !trained {
		out := send("kill ganger")
		acc := out + c.Drain(2000*time.Millisecond)
		send("restore") // survive the ganger's katana
		if clubsImprove.MatchString(acc) {
			trained = true
			break
		}
	}
	if !trained {
		t.Fatalf("PLAYTEST FAIL [§7 train-on-attack]: batoning the ganger never trained Clubs in 90s")
	}

	t.Log("shadowrun skills verified live: hide→Sneaking (Slice C) + baton swings→Clubs (train-on-attack), both via gain-feedback")
}
