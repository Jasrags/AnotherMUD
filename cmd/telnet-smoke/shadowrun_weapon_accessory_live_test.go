//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_WeaponAccessoryMounts proves the weapon-mod first slice end to end:
// a firearm now exposes its SR5 mount geometry, and the Tier-1 accessories
// attach to distinct mounts, reject a taken mount (occupancy), and detach
// cleanly. It drives the whole loop live: spawn → `modify` (list) → attach
// across three mounts → occupancy rejection → `unmodify` → re-attach.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_WeaponAccessoryMounts -v
func TestLive_WeaponAccessoryMounts(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:westlake-plaza",
		"ANOTHERMUD_ROLE_SEED":  "Runner:admin",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
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
	has := func(out, want string) bool { return strings.Contains(strings.ToLower(out), strings.ToLower(want)) }

	// An assault rifle carries the full geometry (barrel/top/under-barrel/stock/
	// internal); spawn it plus the Tier-1 accessories.
	send("spawn item ares-alpha me")
	send("spawn item reflex-sight me")
	send("spawn item foregrip me")
	send("spawn item smartgun-system-internal me")
	send("spawn item telescopic-scope me")

	// The host lists its mounts, all free.
	if out := send("modify alpha"); !has(out, "mount") || !has(out, "internal") {
		t.Fatalf("modify (list) did not show the rifle's mounts (incl. internal):\n%s", out)
	}

	// Attach three accessories to three DISTINCT mounts.
	if out := send("modify alpha reflex"); !has(out, "attach") || !has(out, "top") {
		t.Fatalf("reflex sight did not seat on the top mount:\n%s", out)
	}
	if out := send("modify alpha foregrip"); !has(out, "attach") || !has(out, "under-barrel") {
		t.Fatalf("foregrip did not seat on the under-barrel mount:\n%s", out)
	}
	if out := send("modify alpha internal"); !has(out, "attach") || !has(out, "internal") {
		t.Fatalf("internal smartgun did not seat on the internal mount:\n%s", out)
	}

	// Occupancy: the scope only fits the top rail, which the reflex already holds
	// — attachment is refused, not silently double-seated.
	if out := send("modify alpha scope"); !has(out, "no free mount") {
		t.Fatalf("scope should be refused (top mount taken by the reflex):\n%s", out)
	}

	// The listing reflects all three occupants.
	if out := send("modify alpha"); !has(out, "reflex") || !has(out, "foregrip") || !has(out, "smartgun") {
		t.Fatalf("modify (list) did not show the three installed accessories:\n%s", out)
	}

	// Detach the reflex → the top mount frees → the scope now attaches there.
	if out := send("unmodify alpha reflex"); !has(out, "reflex") {
		t.Fatalf("unmodify did not detach the reflex sight:\n%s", out)
	}
	if out := send("modify alpha scope"); !has(out, "attach") || !has(out, "top") {
		t.Fatalf("scope should seat on the now-free top mount after the reflex was removed:\n%s", out)
	}
}
