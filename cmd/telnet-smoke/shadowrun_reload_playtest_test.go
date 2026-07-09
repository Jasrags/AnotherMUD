//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ShadowrunReloadPlaytest is a HUMAN-READABLE playtest of the firearm
// reload loop (docs/playtest/shadowrun.md §40) driven over a real telnet session
// against a live-booted shadowrun engine. It doesn't assert much — it t.Logs the
// actual server responses at each step so a person can eyeball the reload UX:
// a partial reload, buying rounds from the fixer, topping to a full magazine,
// the already-full message, firing from the magazine, and the loaded count
// surviving a quit + relogin. (The empty-magazine dry-fire path is covered by
// TestLive_ShadowrunFirearm.)
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunReloadPlaytest -v
func TestLive_ShadowrunReloadPlaytest(t *testing.T) {
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
		return strings.TrimSpace(out)
	}
	step := func(label, line string) string {
		out := send(line)
		t.Logf("\n>>> %s  (%s)\n%s", line, label, out)
		return out
	}

	t.Log("=== §40 Firearms & the magazine — a real telnet reload playtest ===")

	// Gear up. The Predator V spawns EMPTY.
	step("pick up the pistol", "get pistol")
	step("wield it", "equip pistol wield")

	// First reload uses the rounds the Street Kit background hands out at
	// creation ([ares-predator-v, ammo-clip, armored-jacket]) — a partial load.
	// (A truly empty pack would report "You have no rounds left to reload with.")
	step("reload from the starting-kit rounds → partial", "reload")

	// Buy 5 caseless rounds from the fixer on the corner, then reload again.
	for i := 0; i < 5; i++ {
		send("buy clip")
	}
	t.Log("\n(bought 5 caseless rounds from the fixer)")
	step("reload with 5 rounds → partial magazine", "reload")

	// Buy 10 more and top off to a full magazine.
	for i := 0; i < 10; i++ {
		send("buy clip")
	}
	t.Log("\n(bought 10 more rounds)")
	step("reload to full", "reload")
	step("reload again on a full magazine", "reload")

	// Into the ganger's turf; fire the loaded magazine.
	send("xp 5000")
	send("set stat agility Chrome 6")
	send("restore")
	send("teleport shadowrun:market-street")
	t.Log("\n(teleported to Market Street, buffed for a reliable demo hit)")
	hit := ""
	for i := 0; i < 8 && !strings.Contains(strings.ToLower(hit), "hit a street ganger"); i++ {
		hit = send("kill ganger")
		send("restore")
	}
	t.Logf("\n>>> kill ganger  (fire the loaded pistol)\n%s", hit)

	// Persistence: reload to full, quit, log back in, confirm the magazine held.
	step("top the magazine back up", "reload")
	t.Log("\n(quit and reconnect on the same account...)")
	_ = c.SendLine("quit")
	time.Sleep(500 * time.Millisecond)
	c.Close()

	c2, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("re-dial: %v", err)
	}
	defer c2.Close()
	if _, err := doLogin(c2, "Chrome"); err != nil {
		t.Fatalf("relogin: %v", err)
	}
	if err := finishLogin(c2, "Chrome", false, nil); err != nil {
		t.Fatalf("roster select: %v", err)
	}
	if err := c2.SendLine("reload"); err != nil {
		t.Fatalf("send reload: %v", err)
	}
	out, err := c2.ExpectTimeout(gamePrompt, 8*time.Second)
	if err != nil {
		t.Fatalf("no prompt after relogin reload: %v", err)
	}
	t.Logf("\n>>> reload  (after relogin — did the magazine persist?)\n%s", strings.TrimSpace(out))
}
