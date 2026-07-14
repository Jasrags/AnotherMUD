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

// TestLive_ShadowrunNuyenShop closes the last SR-M3 box, acceptance [99]:
// nuyen is spent at a shop. On a real shadowrun boot a Street Samurai walks up
// to the street fixer on the safe corner, `list`s her wares, and `buy`s a clip
// of ammunition — the price comes off the runner's nuyen balance (the loot-
// converted "gold" purse) and the clip lands in inventory. The earn half of
// [99] already works (nuyen auto-credits from looted corpses); this proves the
// spend half through the standard shop service (economy-survival §3.5) — no
// bespoke SR economy code.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunNuyenShop -v
func TestLive_ShadowrunNuyenShop(t *testing.T) {
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

	// Starting nuyen (the street-kid background's 500), read off the score purse.
	// The SR currency reskin renders the purse as "Nuyen  <amount>¥" (currency
	// label Title + Symbol), not the generic "Gold" noun.
	goldRe := regexp.MustCompile(`(?i)Nuyen\s+([\d,]+)¥`)
	m := goldRe.FindStringSubmatch(send("score"))
	if m == nil {
		t.Fatal("no Nuyen purse on the score sheet")
	}
	before, _ := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	if before <= 0 {
		t.Fatalf("a fresh Street Samurai has %d nuyen; want > 0 (street-kid grants 500)", before)
	}

	// The fixer shares the safe corner — `list` finds her shop in the room.
	wares := send("list")
	if !strings.Contains(strings.ToLower(wares), "caseless round") {
		t.Fatalf("the fixer's `list` did not show her wares (no caseless round):\n%s", wares)
	}

	// Buy a clip: the price comes off the nuyen balance. The buy line reskins to
	// nuyen too ("You buy X for <n>¥. You have <n>¥ left.").
	buyRe := regexp.MustCompile(`(?i)You buy .* for ([\d,]+)¥\. You have ([\d,]+)¥ left\.`)
	out := send("buy round")
	bm := buyRe.FindStringSubmatch(out)
	if bm == nil {
		t.Fatalf("buy round did not confirm a purchase (\"You buy … ¥ left.\"):\n%s", out)
	}
	price, _ := strconv.Atoi(strings.ReplaceAll(bm[1], ",", ""))
	remaining, _ := strconv.Atoi(strings.ReplaceAll(bm[2], ",", ""))
	if price <= 0 {
		t.Fatalf("clip cost %d nuyen; want > 0 (a real spend)", price)
	}
	if remaining != before-price {
		t.Fatalf("nuyen balance wrong after buy: %d left, want %d (%d - %d)", remaining, before-price, before, price)
	}

	// The clip is now in inventory.
	inv := send("inventory")
	if !strings.Contains(strings.ToLower(inv), "caseless round") {
		t.Fatalf("bought clip not in inventory:\n%s", inv)
	}
	t.Logf("shadowrun verified live: bought a clip from the fixer for %d nuyen (%d→%d) — nuyen spent at a shop", price, before, remaining)
}
