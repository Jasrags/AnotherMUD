//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_CrossbowReload proves the crossbow reload lifecycle end to end on the
// WoT boot (action-economy.md §7.1): a character buys a light crossbow + bolts
// from Haral Luhhan's forge, wields it, and `load`s it — a TIMED busy action
// that begins ("you begin loading") and completes asynchronously on the
// action-complete tick ("ready to fire"), after which a second `load` reports it
// is already loaded.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_CrossbowReload -v
//
// First character of a fresh store is auto-granted admin, so `teleport` + `set
// gold` work to set the fight up deterministically.
func TestLive_CrossbowReload(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "wot",
		"ANOTHERMUD_START_ROOM": "wot:the-green",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Bolt"); err != nil {
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

	// Outfit at the forge: gold, then buy a light crossbow + a handful of bolts.
	send("teleport wot:the-forge")
	send("set gold amount Bolt 2000")
	if out := send("buy light crossbow"); strings.Contains(strings.ToLower(out), "don't") || strings.Contains(strings.ToLower(out), "can't") {
		t.Fatalf("could not buy a light crossbow at the forge:\n%s", out)
	}
	send("buy bolt")
	send("buy bolt")
	if out := send("wield crossbow"); !strings.Contains(strings.ToLower(out), "crossbow") {
		t.Fatalf("could not wield the crossbow:\n%s", out)
	}

	// Load is a TIMED action: phase 1 announces the start.
	if out := send("load"); !strings.Contains(strings.ToLower(out), "begin loading") {
		t.Fatalf("load should begin a timed reload, got:\n%s", out)
	}

	// The action-complete tick chambers it shortly after (reload_ticks=20 ≈ 2s).
	if _, err := c.ExpectStringTimeout("ready to fire", 6*time.Second); err != nil {
		t.Fatalf("the timed reload never completed (no 'ready to fire'): %v", err)
	}

	// Now chambered: a second load reports it's already loaded.
	if out := send("load"); !strings.Contains(strings.ToLower(out), "already loaded") {
		t.Fatalf("a loaded crossbow should report already loaded, got:\n%s", out)
	}
}
