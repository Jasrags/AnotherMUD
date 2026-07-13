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
		"ANOTHERMUD_PACKS":           "shadowrun",
		"ANOTHERMUD_START_ROOM":      "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":       "Chrome:admin",
		"ANOTHERMUD_RELOAD_DURATION": "0", // instant reload for deterministic asserts
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

	t.Log("=== §40 Firearms & the holder model — a real telnet reload playtest ===")

	// Gear up. The Predator V is HOLDER-FED — it fires from a clip, not loose
	// rounds. Grab the gun + an (empty) clip from the corner.
	step("pick up the pistol", "get pistol")
	step("wield it", "equip pistol wield")
	step("pick up a clip (empty)", "get clip")

	// `reload` with only an empty clip — nothing loaded to insert.
	step("reload with only an empty clip", "reload")

	// Buy 5 loose rounds from the fixer, load them INTO the clip (a partial clip).
	for i := 0; i < 5; i++ {
		send("buy round")
	}
	t.Log("\n(bought 5 caseless rounds from the fixer)")
	step("reload clip → load the 5 rounds into the clip (partial)", "reload clip")

	// Buy 10 more, top the clip off, then insert the loaded clip into the gun.
	for i := 0; i < 10; i++ {
		send("buy round")
	}
	t.Log("\n(bought 10 more rounds)")
	step("reload clip → top the clip to full", "reload clip")
	step("reload clip on a full clip", "reload clip")
	step("reload → insert the loaded clip into the pistol", "reload")

	// Into the ganger's turf; fire the loaded clip.
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

	// Ejection: load a SECOND clip, insert it — the first (partly-spent) clip
	// ejects to the ground.
	send("spawn item predator-clip me")
	send("spawn item caseless-round 20 me")
	step("fill a fresh clip", "reload clip")
	step("reload → swap clips; the spent one ejects to the ground", "reload")
	step("look — the ejected clip is on the floor", "look")

	// Persistence: quit, log back in, and fire — the inserted clip survived.
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
	send2 := func(line string) string {
		t.Helper()
		_ = c2.SendLine(line)
		out, _ := c2.ExpectTimeout(gamePrompt, 8*time.Second)
		return strings.TrimSpace(out)
	}
	send2("restore")
	send2("teleport shadowrun:market-street")
	hit2 := ""
	for i := 0; i < 8 && !strings.Contains(strings.ToLower(hit2), "hit a street ganger"); i++ {
		hit2 = send2("kill ganger")
		send2("restore")
	}
	t.Logf("\n>>> kill ganger  (after relogin — the inserted clip persisted?)\n%s", hit2)
}
