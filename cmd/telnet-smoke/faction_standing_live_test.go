//go:build unix

package main

import (
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_FactionStanding boots the WoT engine (which loads the three faction
// definitions) and confirms the `standing` verb renders them at the starting
// Neutral standing for a fresh character:
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_FactionStanding -v
//
// The on-kill earn path (a mob's faction membership → Shift) is unit-verified
// (internal/faction Shift tests) and rides combat's proven killer attribution;
// this smoke proves the registry loads, the three WoT factions are present, and
// the command read path (Env → Context → faction.Entity → Manager) computes the
// starting rank on a real boot.
func TestLive_FactionStanding(t *testing.T) {
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
	if err := createAndLogin(c, "Standwatch"); err != nil {
		t.Fatalf("create player: %v", err)
	}

	if err := c.SendLine("standing"); err != nil {
		t.Fatal(err)
	}
	// A fresh character reads every faction at the starting standing (Neutral/0).
	if _, err := c.ExpectTimeout(regexp.MustCompile(`Children of the Light\s+Neutral \(0\)`), 6*time.Second); err != nil {
		t.Fatalf("standing did not render the Children of the Light at Neutral (0): %v", err)
	}
	if _, err := c.ExpectStringTimeout("Friends of the Dark", 4*time.Second); err != nil {
		t.Fatalf("standing did not list the Friends of the Dark: %v", err)
	}
}
