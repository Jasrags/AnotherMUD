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

// TestLive_ShadowrunWalkthrough is a guide-verification harness, not a mechanics
// test: it drives a raw Shadowrun session and t.Logs the ACTUAL text a player
// sees at each surface claim in docs/playtest/shadowrun.md — the creation menus
// (§38), the score sheet (§38), the fixer's wares (§42), and the cyberware
// deltas (§41) — so the guide's wording can be checked against reality. The six
// shadowrun_*_live_test.go tests already prove the underlying mechanics.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunWalkthrough -v
func TestLive_ShadowrunWalkthrough(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":  "Walker:admin",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	// --- Account + character-name (manual, so we can capture the wizard menus) ---
	if _, err := c.ExpectString("Account username:"); err != nil {
		t.Fatalf("username prompt: %v", err)
	}
	c.SendLine("Walker")
	if _, err := c.Expect(regexp.MustCompile(`(?i)choose a password`)); err != nil {
		t.Fatalf("choose-password prompt: %v", err)
	}
	c.SendLine("smoke-pass-123")
	if _, err := c.ExpectString("Confirm"); err != nil {
		t.Fatalf("confirm prompt: %v", err)
	}
	c.SendLine("smoke-pass-123")
	if _, err := c.ExpectString("new character's name"); err != nil {
		t.Fatalf("char-name prompt: %v", err)
	}
	c.SendLine("Walker")

	// --- Walk the wizard, logging each menu verbatim (§38 creation menus) ---
	step := regexp.MustCompile(`Choose your [\w ]+:|\(yes/no\)|` + gamePrompt.String())
	for i := 0; i < 20; i++ {
		out, err := c.ExpectTimeout(step, 8*time.Second)
		if err != nil {
			t.Fatalf("wizard step %d: %v", i, err)
		}
		if gamePrompt.MatchString(out) {
			t.Logf("§38 WIZARD DONE — reached game:\n%s", strings.TrimSpace(out))
			break
		}
		if strings.Contains(out, "(yes/no)") {
			t.Logf("§38 CONFIRM prompt:\n%s", strings.TrimSpace(out))
			c.SendLine("yes")
			continue
		}
		t.Logf("§38 MENU:\n%s", strings.TrimSpace(out))
		c.SendLine("1") // first option each time
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

	// §38 — the score sheet: identity line, eight primaries, track, MA, nuyen.
	t.Logf("§38 SCORE:\n%s", strings.TrimSpace(send("score")))
	// §37 — the room + exits (map claim).
	t.Logf("§37 LOOK:\n%s", strings.TrimSpace(send("look")))
	// §42 — the fixer's wares.
	t.Logf("§42 LIST:\n%s", strings.TrimSpace(send("list")))
	// §40 — pick up the caseless round on the ground (`get round`).
	t.Logf("§40 GET ROUND:\n%s", strings.TrimSpace(send("get round")))

	// §41 — cyberware deltas on score.
	t.Logf("§41 get reflexes:\n%s", strings.TrimSpace(send("get reflexes")))
	t.Logf("§41 equip reflexes:\n%s", strings.TrimSpace(send("equip reflexes")))
	t.Logf("§41 SCORE after wired reflexes (expect REA +2):\n%s", strings.TrimSpace(send("score")))
	send("unequip reflexes")
	t.Logf("§41 get muscle:\n%s", strings.TrimSpace(send("get muscle")))
	t.Logf("§41 equip muscle:\n%s", strings.TrimSpace(send("equip muscle")))
	t.Logf("§41 SCORE after muscle replacement (expect STR +1, BOD +1):\n%s", strings.TrimSpace(send("score")))
	send("unequip muscle")
	t.Logf("§41 get cybereyes:\n%s", strings.TrimSpace(send("get cybereyes")))
	t.Logf("§41 equip cybereyes:\n%s", strings.TrimSpace(send("equip cybereyes")))
	t.Logf("§41 SCORE after cybereyes (expect INT +1):\n%s", strings.TrimSpace(send("score")))
}
