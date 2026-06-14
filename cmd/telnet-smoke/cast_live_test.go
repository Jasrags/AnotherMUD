//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_WeaveCastWarmup is the WoT S2 Phase 4 slice-1 regression test: a
// weave with a cast_time no longer resolves instantly — it BEGINS a warmup
// ("you begin to weave …") and resolves a round or two later ("you cast …").
// Warding is a self-buff (no target needed) with cast_time 2, so it exercises
// the out-of-combat warmup drain (the idle tick advancing a cast whose queue
// entry was already consumed at begin). Seeing the begin line strictly before
// the resolution line is the proof the warmup state machine is live.
func TestLive_WeaveCastWarmup(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "wot",
		"ANOTHERMUD_START_ROOM": "wot:deep-westwood",
	})

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createChanneler(c, "Warmup", "female"); err != nil {
		t.Fatalf("create channeler: %v", err)
	}

	if err := c.SendLine("channel warding"); err != nil {
		t.Fatalf("send channel: %v", err)
	}
	// The warmup announces itself immediately...
	if _, err := c.ExpectString("begin to weave Warding"); err != nil {
		t.Fatalf("no cast-begin line (warmup did not start): %v", err)
	}
	// ...and the weave resolves only after the cast_time elapses. ExpectString
	// scans forward, so this matches the later resolution, not the begin line.
	if _, err := c.ExpectString("cast Warding"); err != nil {
		t.Fatalf("weave never resolved after its warmup: %v", err)
	}
	t.Log("cast warmup verified: 'begin to weave Warding' preceded 'cast Warding'")
}
