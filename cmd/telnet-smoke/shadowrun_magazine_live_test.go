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

// TestLive_ShadowrunMagazinePersist proves the magazine model persists across a
// relogin (the SR firearm reload arc, persist option): a Street Samurai reloads
// the Ares Predator V to a full 15-round magazine, quits, logs back in on the
// SAME account, and the magazine is STILL full — the loaded count round-tripped
// through the player save (EquippedItem.Loaded) rather than resetting to empty.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunMagazinePersist -v
func TestLive_ShadowrunMagazinePersist(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":  "Gunner:admin",
	})

	// --- Session 1: gear up, reload to full, quit. ---
	c1, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial 1: %v", err)
	}
	if err := createAndLogin(c1, "Gunner"); err != nil {
		t.Fatalf("create+login: %v", err)
	}
	send1 := func(line string) string {
		t.Helper()
		if err := c1.SendLine(line); err != nil {
			t.Fatalf("send %q: %v", line, err)
		}
		out, err := c1.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
		return out
	}
	// Buff for a reliable demo hit later (base stats persist across relog).
	send1("xp 5000")
	send1("set stat agility Gunner 6")
	send1("set stat strength Gunner 6")
	send1("restore")
	if out := send1("get pistol"); strings.Contains(strings.ToLower(out), "don't see") {
		t.Fatalf("could not get the Ares Predator V:\n%s", out)
	}
	send1("equip pistol wield")
	// Fill a clip and load it into the gun (a full 15/15 inserted holder).
	send1("spawn item predator-clip me")
	send1("spawn item ammo-clip 20 me")
	if out := send1("reload clip"); !strings.Contains(out, "(15/15)") {
		t.Fatalf("reload clip did not fill the clip to 15/15:\n%s", out)
	}
	if out := send1("reload"); !strings.Contains(strings.ToLower(out), "fresh clip") || !strings.Contains(out, "(15/15)") {
		t.Fatalf("reload did not insert the loaded clip:\n%s", out)
	}
	// A clean quit flushes the dirty actor's save to disk.
	_ = c1.SendLine("quit")
	time.Sleep(500 * time.Millisecond)
	c1.Close()

	// --- Session 2: log back in on the same account; the magazine survives. ---
	c2, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial 2: %v", err)
	}
	defer c2.Close()
	isNew, err := doLogin(c2, "Gunner")
	if err != nil {
		t.Fatalf("relogin: %v", err)
	}
	if isNew {
		t.Fatal("account Gunner should already exist on relogin")
	}
	if err := finishLogin(c2, "Gunner", false, nil); err != nil {
		t.Fatalf("relogin roster select: %v", err)
	}
	send2 := func(line string) string {
		t.Helper()
		if err := c2.SendLine(line); err != nil {
			t.Fatalf("send %q: %v", line, err)
		}
		out, err := c2.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
		return out
	}
	// The Predator V is still wielded with its inserted clip (equipment +
	// inserted-holder persist). Prove it by FIRING: a persisted loaded clip lets
	// the gun shoot; if the inserted holder hadn't round-tripped the gun would
	// respawn empty and every swing would click dry (never landing a shot).
	send2("restore")
	send2("teleport shadowrun:market-street")
	hitRe := regexp.MustCompile(`(?i)hit a street ganger for \d+ damage`)
	if !fightUntil(t, send2, c2, hitRe, 30*time.Second) {
		t.Fatal("after relogin the pistol never fired — the inserted clip did not persist (the gun respawned empty)")
	}
	t.Log("shadowrun verified live: a loaded clip inserted in the Ares Predator V survived quit + relogin — the gun fired on return, so the inserted holder round-tripped through the player save")
}

// TestLive_ShadowrunLooseClipPersist is the regression guard for the FillHolder
// persistence bug: filling a carried (not-yet-inserted) clip must round-trip its
// new load through the save. It fills a loose clip to 15/15, quits WITHOUT
// inserting it (the bug's trigger — the clip stays in inventory), logs back in,
// and confirms the clip still holds 15 by loading it into the gun (a reverted
// clip would respawn empty and `reload` would report no loaded clip).
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunLooseClipPersist -v
func TestLive_ShadowrunLooseClipPersist(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":  "Filler:admin",
	})

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial 1: %v", err)
	}
	if err := createAndLogin(c, "Filler"); err != nil {
		t.Fatalf("create+login: %v", err)
	}
	send := func(line string) string {
		t.Helper()
		_ = c.SendLine(line)
		out, err := c.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
		return out
	}
	send("spawn item predator-clip me")
	// Exactly enough to fill the clip once; the few rounds left over are < a full
	// magazine, so a REVERTED clip couldn't refill to 15 in session 2.
	send("spawn item ammo-clip 15 me")
	if out := send("reload clip"); !strings.Contains(out, "(15/15)") {
		t.Fatalf("reload clip did not fill the loose clip to 15/15:\n%s", out)
	}
	// Quit with the clip still LOOSE (never inserted) — the bug's trigger.
	_ = c.SendLine("quit")
	time.Sleep(500 * time.Millisecond)
	c.Close()

	c2, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial 2: %v", err)
	}
	defer c2.Close()
	if _, err := doLogin(c2, "Filler"); err != nil {
		t.Fatalf("relogin: %v", err)
	}
	if err := finishLogin(c2, "Filler", false, nil); err != nil {
		t.Fatalf("relogin roster select: %v", err)
	}
	send2 := func(line string) string {
		t.Helper()
		_ = c2.SendLine(line)
		out, err := c2.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
		return out
	}
	// `reload clip` on the persisted clip reports "already full (15/15)" if the
	// fill round-tripped. If it had reverted to empty, this would instead refill
	// from the handful of carried rounds and read a lower count (never 15/15).
	out := send2("reload clip")
	if !strings.Contains(out, "(15/15)") {
		t.Fatalf("a loose clip's fill did not persist across relogin (expected the clip still full at 15/15):\n%s", out)
	}
	t.Log("shadowrun verified live: a filled-but-not-inserted clip kept its 15 rounds across quit + relogin — FillHolder's inventory sync round-trips the loose-holder load")
}
