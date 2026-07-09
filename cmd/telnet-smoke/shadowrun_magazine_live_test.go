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
		"ANOTHERMUD_PACKS":           "shadowrun",
		"ANOTHERMUD_START_ROOM":      "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":       "Gunner:admin",
		"ANOTHERMUD_RELOAD_DURATION": "0", // instant reload for deterministic asserts
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
		"ANOTHERMUD_PACKS":           "shadowrun",
		"ANOTHERMUD_START_ROOM":      "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":       "Filler:admin",
		"ANOTHERMUD_RELOAD_DURATION": "0", // instant reload for deterministic asserts
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

// TestLive_ShadowrunLoadedClipShop proves the pre-loaded-clip SKU (ammo-and-
// reloading §6): a runner buys a loaded clip from the fixer and it drops in
// ready — loading it into the gun reads a full 15/15 with no fill step, because
// the holder's `preload` seeds it at spawn.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunLoadedClipShop -v
func TestLive_ShadowrunLoadedClipShop(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":           "shadowrun",
		"ANOTHERMUD_START_ROOM":      "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":       "Buyer:admin",
		"ANOTHERMUD_RELOAD_DURATION": "0", // instant reload for deterministic asserts
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Buyer"); err != nil {
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
	send("restore") // ensure nuyen; street-kid starts with 500
	if out := send("buy loaded"); !strings.Contains(strings.ToLower(out), "loaded ares predator v clip") {
		t.Fatalf("buy loaded did not purchase a loaded clip from the fixer:\n%s", out)
	}
	send("get pistol")
	send("equip pistol wield")
	// The bought clip is already full → inserting it reads 15/15 with no fill.
	if out := send("reload"); !strings.Contains(out, "(15/15)") {
		t.Fatalf("a bought loaded clip did not insert as full (15/15) — preload not seeded:\n%s", out)
	}
	t.Log("shadowrun verified live: a loaded clip bought from the fixer inserted as a full 15/15 with no fill step — preload seeds a pre-loaded holder")
}

// TestLive_ShadowrunClipDecay proves ejected-clip decay (ammo-and-reloading §7):
// a spent clip ejected to the ground is recoverable for a lifetime window, then
// swept. Boots with a 1s lifetime + 1s sweep cadence so the test doesn't wait.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunClipDecay -v
func TestLive_ShadowrunClipDecay(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":                   "shadowrun",
		"ANOTHERMUD_START_ROOM":              "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":               "Litter:admin",
		"ANOTHERMUD_RELOAD_DURATION":         "0", // instant reload for deterministic asserts
		"ANOTHERMUD_EJECTED_HOLDER_LIFETIME": "1s",
		"ANOTHERMUD_CORPSE_DECAY_INTERVAL":   "1s", // the scrap sweep shares this cadence
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Litter"); err != nil {
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
	// Two loaded clips; insert one, then insert the other to EJECT the first.
	// Do it in the empty, safe back alley — the street corner keeps a clip on the
	// ground as starter gear, which would mask the decay.
	send("get pistol")
	send("spawn item predator-clip-loaded me")
	send("spawn item predator-clip-loaded me")
	send("teleport shadowrun:back-alley")
	send("equip pistol wield")
	send("reload") // insert the first clip
	if out := send("reload"); !strings.Contains(strings.ToLower(out), "ejects") {
		t.Fatalf("second reload did not eject the first clip:\n%s", out)
	}
	if out := send("look"); !strings.Contains(strings.ToLower(out), "clip") {
		t.Fatalf("the ejected clip is not on the ground:\n%s", out)
	}
	// Wait out the lifetime (1s) + a sweep cadence (1s) + margin.
	time.Sleep(4 * time.Second)
	if out := send("look"); strings.Contains(strings.ToLower(out), "clip") {
		t.Fatalf("the ejected clip did not decay off the ground:\n%s", out)
	}
	t.Log("shadowrun verified live: an ejected clip lingered recoverable, then decayed off the ground after its lifetime")
}

// TestLive_ShadowrunReloadTimed proves reload is a TIMED busy action (ammo-and-
// reloading §9): `reload` begins the action and returns immediately, a second
// action mid-reload is refused as busy, and the reload completes a beat later
// (the action-complete sweep replays it). Boots with a 1s reload duration.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunReloadTimed -v
func TestLive_ShadowrunReloadTimed(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":           "shadowrun",
		"ANOTHERMUD_START_ROOM":      "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":       "Timer:admin",
		"ANOTHERMUD_RELOAD_DURATION": "1s", // a real, short reload time
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Timer"); err != nil {
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
	send("get pistol")
	send("equip pistol wield")
	send("buy loaded") // a full clip in inventory, ready to insert

	// Phase 1: reload begins and returns immediately (no (15/15) yet).
	out := send("reload")
	if !strings.Contains(strings.ToLower(out), "begin reloading") {
		t.Fatalf("reload did not begin as a timed action:\n%s", out)
	}
	if strings.Contains(out, "(15/15)") {
		t.Fatalf("reload completed instantly — it should be a timed action:\n%s", out)
	}
	// A second action mid-reload is refused as busy.
	if busy := send("reload"); !strings.Contains(strings.ToLower(busy), "busy") {
		t.Fatalf("a reload mid-reload was not refused as busy:\n%s", busy)
	}
	// The reload completes a beat later (pushed async by the action sweep).
	time.Sleep(1500 * time.Millisecond)
	if comp := c.Drain(1000 * time.Millisecond); !strings.Contains(comp, "(15/15)") {
		t.Fatalf("the timed reload never completed (no fresh-clip message):\n%s", comp)
	}
	t.Log("shadowrun verified live: reload is a timed busy action — it begins immediately, blocks a second action as busy, and completes (clip inserted) a beat later")
}
