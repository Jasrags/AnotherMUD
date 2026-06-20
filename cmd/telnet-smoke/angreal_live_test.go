//go:build unix

package main

import (
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_AngrealBoostsFirebolt is the WoT S2 Phase 4+ regression test for
// angreal / sa'angreal: a same-gender channeling device held while weaving
// amplifies the woven damage payload. It boots its own engine subprocess, drives
// two MALE initiates (both saidin → Fire strong → affinity 1.0, so affinity is
// out of the picture), and compares a baseline firebolt to one cast while the
// saidin figurine is equipped.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_AngrealBoostsFirebolt -v
//
// The proof: Firebolt is 2d4 (range 2–8). At ANOTHERMUD_ANGREAL_PER_POINT=5 a
// power-2 angreal multiplies by 1 + 2×5 = 11. So a bare firebolt is ALWAYS ≤ 8,
// while a figurine-boosted one is ALWAYS ≥ 2×11 = 22. A baseline ≤ 8 and a
// boosted ≥ 22 — same channeler gender, same weave — is dice-proof evidence the
// held device reached the cast, with no statistical sampling. Both start in the
// Smithy (where the figurines sit); the boosted one equips it, then both walk
// west twice to the boar in the Deep Westwood.
func TestLive_AngrealBoostsFirebolt(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":                "wot",
		"ANOTHERMUD_START_ROOM":           "wot:the-forge",
		"ANOTHERMUD_ANGREAL_PER_POINT":    "5",
		"ANOTHERMUD_AFFINITY_WEAK_FACTOR": "0.5", // irrelevant for a Fire-strong male; pinned for clarity
	})

	// Baseline male: no figurine. Walk to the boar and firebolt it (≤ 8).
	cb, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial baseline: %v", err)
	}
	if err := createChanneler(cb, "Plainman", "male"); err != nil {
		cb.Close()
		t.Fatalf("create baseline channeler: %v", err)
	}
	if err := walkToBoar(cb); err != nil {
		cb.Close()
		t.Fatalf("baseline walk: %v", err)
	}
	baseDmg, berr := fireboltBoarDamage(cb)
	cb.Close() // stop his auto-attacks before the next channeler engages
	if berr != nil {
		t.Fatalf("baseline firebolt: %v", berr)
	}

	// Boosted male: grab + hold the saidin figurine, then the same walk + cast.
	ch, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial boosted: %v", err)
	}
	defer ch.Close()
	if err := createChanneler(ch, "Angrealman", "male"); err != nil {
		t.Fatalf("create boosted channeler: %v", err)
	}
	if err := equipSaidinFigurine(ch); err != nil {
		t.Fatalf("equip figurine: %v", err)
	}
	if err := walkToBoar(ch); err != nil {
		t.Fatalf("boosted walk: %v", err)
	}
	boostDmg, err := fireboltBoarDamage(ch)
	if err != nil {
		t.Fatalf("boosted firebolt: %v", err)
	}

	if baseDmg > 8 {
		t.Errorf("baseline Firebolt = %d, want <= 8 (2d4, no angreal)", baseDmg)
	}
	if boostDmg < 22 {
		t.Errorf("angreal Firebolt = %d, want >= 22 (2d4 × 11 at per-point 5, power 2)", boostDmg)
	}
	if boostDmg <= baseDmg {
		t.Errorf("angreal-boosted firebolt (%d) was not stronger than baseline (%d) — the held device did not reach the cast", boostDmg, baseDmg)
	}
	t.Logf("angreal verified: bare saidin Fire=%d <= 8 < 22 <= figurine-boosted Fire=%d", baseDmg, boostDmg)
}

// equipSaidinFigurine picks up the man-figurine (saidin angreal) sitting in the
// Smithy and equips it into a hand. `man` is a keyword unique to the saidin
// device (the saidar one carries `woman`).
func equipSaidinFigurine(c *telnettest.Client) error {
	if err := c.SendLine("get man"); err != nil {
		return err
	}
	if _, err := c.Expect(regexp.MustCompile(`pick up|take`)); err != nil {
		return err
	}
	// The figurine is eligible for either hand, so equip must name the slot
	// (a bare `equip man` prompts "Which slot?").
	if err := c.SendLine("equip man wield"); err != nil {
		return err
	}
	_, err := c.Expect(regexp.MustCompile(`figurine|wield|hold|wear`))
	return err
}

// walkToBoar moves the channeler from the Smithy west twice into the Deep
// Westwood (the-forge → westwood-edge → deep-westwood), confirming arrival by
// the destination room name so a missed step fails loudly rather than leaving
// the later engageBoar polling a wrong room for 45s.
func walkToBoar(c *telnettest.Client) error {
	if err := c.SendLine("west"); err != nil {
		return err
	}
	if _, err := c.ExpectString("Edge of the Westwood"); err != nil {
		return err
	}
	if err := c.SendLine("west"); err != nil {
		return err
	}
	_, err := c.ExpectString("Deep in the Westwood")
	return err
}
