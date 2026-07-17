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

// TestLive_ShadowrunImprove exercises the karma-ledger `improve` verb end to end
// (SR-M5b): a runner banks karma from a kill, then spends it à la carte on a
// QUALITY (a feat), an ATTRIBUTE (+1, metatype-capped), and a SKILL (its
// trainer-cap tier), with the balance falling by each spend's price.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunImprove -v
func TestLive_ShadowrunImprove(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":  "Chrome:admin",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Chrome"); err != nil {
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
	karmaRe := regexp.MustCompile(`(?i)Karma:?\s+([\d,]+)`)
	scoreKarma := func(sheet string) int {
		t.Helper()
		m := karmaRe.FindStringSubmatch(sheet)
		if m == nil {
			t.Fatalf("no karma line:\n%s", sheet)
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("parse karma %q: %v", m[1], err)
		}
		return n
	}

	// Bank karma from a ganger kill (30 per the mob's xp_value).
	send("set stat strength Chrome 6")
	send("restore")
	send("get katana")
	send("equip katana wield")
	send("teleport shadowrun:market-street")
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
		t.Fatalf("could not bank karma from a ganger kill:\n%s", acc.String())
	}
	if got := scoreKarma(send("score")); got != 30 {
		t.Fatalf("banked karma = %d, want 30", got)
	}

	// The listing shows what is improvable + the balance.
	listing := send("improve")
	if !strings.Contains(strings.ToLower(listing), "ambidextrous") {
		t.Fatalf("improve listing missing the Ambidextrous quality:\n%s", listing)
	}

	// Quality: buy Ambidextrous (4 karma) → 30 - 4 = 26.
	if out := send("improve ambidextrous"); !strings.Contains(out, "gain Ambidextrous") {
		t.Fatalf("improve ambidextrous failed:\n%s", out)
	}

	// Attribute: raise Agility 3 -> 4 (cost 4 x 5 = 20) → 26 - 20 = 6.
	if out := send("improve agility"); !regexp.MustCompile(`(?i)Agility rises to 4`).MatchString(out) {
		t.Fatalf("improve agility failed:\n%s", out)
	}

	// Skill: raise Sneaking a tier. The street-kid background already grants it
	// (cap 30), so the next tier is Apprentice (50), rank 2, cost 2 x 2 = 4 → 6 - 4 = 2.
	if out := send("improve sneaking"); !regexp.MustCompile(`(?i)Sneaking ceiling rises to Apprentice`).MatchString(out) {
		t.Fatalf("improve sneaking failed:\n%s", out)
	}

	// The balance reflects every spend: 30 - 4 - 20 - 4 = 2.
	if got := scoreKarma(send("score")); got != 2 {
		t.Fatalf("karma after three improves = %d, want 2 (30 - 4 - 20 - 4)", got)
	}

	// An unaffordable buy reports its price rather than silently failing (2 left,
	// Agility 4 -> 5 now costs 25).
	if out := send("improve agility"); !regexp.MustCompile(`(?i)costs 25 karma; you have 2`).MatchString(out) {
		t.Fatalf("unaffordable improve should quote the price:\n%s", out)
	}
	t.Log("shadowrun verified live: improve spent karma on a quality, an attribute, and a skill; balance tracked every purchase")
}
