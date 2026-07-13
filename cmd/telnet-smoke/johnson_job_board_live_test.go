//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_JohnsonJobBoard proves the run progression gating: a fresh runner
// who `talk`s to Mr. Johnson is offered only the starter run (Redmond
// Retrieval); the follow-up runs (Bug Hunt, Turf War) are prereq-gated behind
// it and must NOT appear until it's completed (quests §3.2 prerequisites).
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_JohnsonJobBoard -v
func TestLive_JohnsonJobBoard(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:dantes-inferno", // Johnson is here
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Deckard"); err != nil {
		t.Fatalf("create+login: %v", err)
	}
	if err := c.SendLine("talk johnson"); err != nil {
		t.Fatal(err)
	}
	out, err := c.ExpectTimeout(gamePrompt, 8*time.Second)
	if err != nil {
		t.Fatalf("no prompt after talk johnson: %v", err)
	}

	// The starter run is on offer...
	if !strings.Contains(out, "Redmond Retrieval") {
		t.Fatalf("Johnson should offer the starter run to a fresh runner:\n%s", out)
	}
	// ...but the prereq-gated follow-ups are not.
	if strings.Contains(out, "Bug Hunt") || strings.Contains(out, "Turf War") {
		t.Fatalf("prereq-gated runs leaked into a fresh runner's offers (should require Redmond Retrieval first):\n%s", out)
	}
}
