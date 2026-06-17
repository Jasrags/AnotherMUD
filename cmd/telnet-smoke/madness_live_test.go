//go:build unix

package main

import (
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_SaidinTaintAccruesAndCures is the WoT S2 Phase 4+ regression test for
// taint/madness. It boots its own engine subprocess with deterministic tuning,
// drives a male and a female channeler, and proves the three pillars of the
// curse off the score sheet's Madness band:
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_SaidinTaint -v
//
//   - ASYMMETRY: a man who weaves accrues taint; a woman never does (saidar is
//     clean). With MADNESS_PER_CAST=50, one Warding pushes the man to band
//     "voices clamor"; the woman shows no Madness row at all.
//   - CURE: Heal the Mind reduces the man's madness — one cast (2d6, max 12)
//     drops him strictly below 50, so the band falls from "voices clamor" to
//     "shadow on your mind". (The cure weave is excluded from accrual, so it
//     never deepens what it heals.)
//
// MADNESS_THRESHOLD is set absurdly high and MADNESS_DECAY to 0 so the
// manifestation tick and the slow drift never perturb the deterministic bands
// mid-test.
func TestLive_SaidinTaintAccruesAndCures(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":             "wot",
		"ANOTHERMUD_START_ROOM":        "wot:deep-westwood",
		"ANOTHERMUD_MADNESS_PER_CAST":  "50",
		"ANOTHERMUD_MADNESS_THRESHOLD": "100000", // never manifest mid-test
		"ANOTHERMUD_MADNESS_DECAY":     "0",       // no drift between reads
	})

	// Male — saidin — accrues taint as he weaves.
	cm, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial male: %v", err)
	}
	defer cm.Close()
	if err := createChanneler(cm, "Madman", "male"); err != nil {
		t.Fatalf("create male channeler: %v", err)
	}
	if band, _ := scoreMadnessBand(cm); band != "" {
		t.Errorf("fresh male channeler already tainted (band %q); want clean", band)
	}
	if err := castWarding(cm); err != nil {
		t.Fatalf("male warding: %v", err)
	}
	band, err := scoreMadnessBand(cm)
	if err != nil {
		t.Fatalf("male madness band: %v", err)
	}
	if band != "voices clamor" {
		t.Errorf("male band after one weave (madness 50) = %q, want \"voices clamor\"", band)
	}

	// Heal the Mind cures: one cast drops him below the 50 band.
	if err := castHealMind(cm); err != nil {
		t.Fatalf("heal the mind: %v", err)
	}
	cured, err := scoreMadnessBand(cm)
	if err != nil {
		t.Fatalf("male madness band after cure: %v", err)
	}
	if cured != "shadow on your mind" {
		t.Errorf("male band after Heal the Mind = %q, want \"shadow on your mind\" (50 - 2d6 < 50)", cured)
	}

	// Female — saidar — clean: weaving never taints her.
	cf, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial female: %v", err)
	}
	defer cf.Close()
	if err := createChanneler(cf, "Cleanwoman", "female"); err != nil {
		t.Fatalf("create female channeler: %v", err)
	}
	if err := castWarding(cf); err != nil {
		t.Fatalf("female warding: %v", err)
	}
	if fband, _ := scoreMadnessBand(cf); fband != "" {
		t.Errorf("female channeler accrued taint (band %q) — saidar must stay clean", fband)
	}
	t.Logf("taint verified: male 1 weave → %q, cured → %q; female → clean", band, cured)
}

// castWarding weaves Warding (self-buff, variance-0, always lands) and waits for
// resolution — a convenient generic "weave something" that accrues taint.
func castWarding(c *telnettest.Client) error {
	if err := c.SendLine("channel warding"); err != nil {
		return err
	}
	_, err := c.ExpectString("cast Warding")
	return err
}

// castHealMind weaves Heal the Mind on the caster (the saidin-taint cure) and
// waits for resolution.
func castHealMind(c *telnettest.Client) error {
	if err := c.SendLine("channel heal-the-mind"); err != nil {
		return err
	}
	_, err := c.ExpectString("cast Heal the Mind")
	return err
}

// madnessBandRe pulls the saidin-taint band label off the score sheet's Madness
// row. `[^\n]*?` absorbs the panel padding + any ANSI color markup between the
// "Madness" label and its band value (the row is colored danger-red). The row is
// present only once the channeler has accrued taint.
var madnessBandRe = regexp.MustCompile(`Madness[^\n]*?(faint whisper|shadow on your mind|voices clamor|the madness has you)`)

// scoreMadnessBand sends `score`, drains the whole framed sheet, and returns the
// Madness band label — or "" when no Madness row is shown (a clean channeler / a
// woman / a non-channeler). Drains rather than Expect-on-AC because the Madness
// row renders several lines BELOW Armor Class, so an AC anchor would stop short
// of it.
func scoreMadnessBand(c *telnettest.Client) (string, error) {
	if err := c.SendLine("score"); err != nil {
		return "", err
	}
	out := c.Drain(700 * time.Millisecond)
	if !armorClassRe.MatchString(out) {
		return "", fmt.Errorf("score sheet did not render (no Armor Class line in %q)", out)
	}
	if m := madnessBandRe.FindStringSubmatch(out); m != nil {
		return m[1], nil
	}
	return "", nil
}
