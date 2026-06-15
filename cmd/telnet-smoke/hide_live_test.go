//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_HideVerbs is a deterministic single-player live smoke for the
// visibility hide verbs (S3a/S3b) — it boots a real engine, creates a
// player, and exercises hide → look → unhide end to end:
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_HideVerbs -v
//
// The 2-player "an observer can't see a hidden player" outcome is NOT live-
// tested: it rides the per-observer perception contest (a d20 roll), so the
// result is non-deterministic without an RNG hook — that logic is covered
// deterministically by the unit tests (internal/command visObserver/predicate
// + resolver-filter tests). This smoke proves the verbs, the events, and the
// visibility predicate (which now runs on every render + target resolve) all
// work in a real boot without crashing the hot paths.
func TestLive_HideVerbs(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // default starter-world boot

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Shadower"); err != nil {
		t.Fatalf("create player: %v", err)
	}

	// hide → concealment message.
	if err := c.SendLine("hide"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExpectStringTimeout("shadows", 6*time.Second); err != nil {
		t.Fatalf("hide did not confirm concealment: %v", err)
	}

	// look still works while hidden (self is always visible; the predicate on
	// the render path must not error) — the room name/prompt returns.
	if err := c.SendLine("look"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExpectTimeout(gamePrompt, 6*time.Second); err != nil {
		t.Fatalf("look while hidden did not render: %v", err)
	}

	// unhide → emerge message.
	if err := c.SendLine("unhide"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExpectStringTimeout("out of hiding", 6*time.Second); err != nil {
		t.Fatalf("unhide did not confirm: %v", err)
	}

	// unhide again → already-not-hidden.
	if err := c.SendLine("unhide"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExpectStringTimeout("aren't hidden", 6*time.Second); err != nil {
		t.Fatalf("second unhide did not report not-hidden: %v", err)
	}
	t.Log("hide verbs verified live: hide → look → unhide → unhide")
}

// TestLive_RevealOnAction is a deterministic live check for reveal-on-action
// (visibility §4.5): a hidden player who runs a "loud" command is revealed.
// `get` is hand-parsed and flagged BreaksConcealment, so it reveals on attempt
// even with no matching item — a deterministic trigger (no contest, no target
// needed).
func TestLive_RevealOnAction(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil)

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Lurker"); err != nil {
		t.Fatalf("create player: %v", err)
	}

	if err := c.SendLine("hide"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExpectStringTimeout("shadows", 6*time.Second); err != nil {
		t.Fatalf("hide did not confirm: %v", err)
	}

	// A loud action reveals — even an item that isn't there (get is hand-parsed
	// and breaks concealment before its handler runs).
	if err := c.SendLine("get nonexistentthing"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExpectStringTimeout("no longer hidden", 6*time.Second); err != nil {
		t.Fatalf("a loud action did not reveal the hidden player: %v", err)
	}

	// Confirm the reveal stuck: unhide now reports not-hidden.
	if err := c.SendLine("unhide"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExpectStringTimeout("aren't hidden", 6*time.Second); err != nil {
		t.Fatalf("player was not actually revealed by the loud action: %v", err)
	}
	t.Log("reveal-on-action verified live: hide → loud action → revealed")
}
