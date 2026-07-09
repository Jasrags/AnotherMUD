//go:build unix

package main

import (
	"os"
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
	if out := send1("get pistol"); strings.Contains(strings.ToLower(out), "don't see") {
		t.Fatalf("could not get the Ares Predator V:\n%s", out)
	}
	send1("equip pistol wield")
	send1("spawn item ammo-clip 20 me") // more than a magazine's worth
	if out := send1("reload"); !strings.Contains(out, "(15/15)") {
		t.Fatalf("reload did not fill the magazine to 15/15:\n%s", out)
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
	// The Predator V is still wielded (equipment persists). Reloading it now must
	// report ALREADY FULL — proof the loaded count survived the save. If
	// persistence had failed it would respawn empty and this reload would load
	// rounds from the (also-persisted) inventory instead.
	out := send2("reload")
	if !strings.Contains(strings.ToLower(out), "already fully loaded") || !strings.Contains(out, "(15/15)") {
		t.Fatalf("after relogin the magazine did not persist as full (15/15):\n%s", out)
	}
	t.Log("shadowrun verified live: a full Ares Predator V magazine (15/15) survived quit + relogin — loaded count round-tripped through the player save")
}
