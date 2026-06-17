//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ThrowVerb proves the thrown-weapon path (ranged-combat §3) is wired
// end to end in a real engine: the starter-world throwing knife loads and
// equips, and the `throw` verb dispatches — refusing when nothing throwable is
// wielded, then resolving a target once the knife is in hand. The hit / land /
// destroy mechanics are unit-covered (internal/command, internal/combat); this
// smoke is deliberately target-agnostic so it doesn't depend on live mob spawn
// timing or combat RNG.
func TestLive_ThrowVerb(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // default starter-world boot, start = town-square
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Hurler"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	// With nothing throwable wielded, throw is refused.
	if err := c.SendLine("throw nobody"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := c.ExpectStringTimeout("aren't wielding anything you can throw", 4*time.Second); err != nil {
		t.Fatalf("pre-equip throw should be refused: %v", err)
	}

	// Pick up + wield the throwing knife from Town Square (proves the content
	// loaded and the thrown weapon is equippable).
	if err := c.SendLine("get knife"); err != nil {
		t.Fatalf("send: %v", err)
	}
	c.Drain(500 * time.Millisecond)
	if err := c.SendLine("equip knife"); err != nil {
		t.Fatalf("send: %v", err)
	}
	c.Drain(500 * time.Millisecond)

	// Now throw finds the wielded knife and proceeds to target resolution; an
	// unknown target yields the not-here message (proving the verb ran past the
	// wielded-weapon check).
	if err := c.SendLine("throw nobody"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := c.ExpectStringTimeout("don't see them here", 4*time.Second); err != nil {
		t.Fatalf("post-equip throw should reach target resolution: %v", err)
	}
	t.Log("throw verb wired: refused with no thrown weapon; resolves a target once the knife is wielded")
}

// TestLive_BandVerbs proves the advance/withdraw kiting verbs (ranged-combat
// §5.4) are wired in a real engine. Kept deterministic by exercising the
// not-fighting path (no live mob / combat RNG) — the band-move mechanics
// themselves are unit-covered (internal/combat, internal/command).
func TestLive_BandVerbs(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil)
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Kiter"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	for _, verb := range []string{"advance", "withdraw"} {
		if err := c.SendLine(verb); err != nil {
			t.Fatalf("send %s: %v", verb, err)
		}
		if _, err := c.ExpectStringTimeout("aren't fighting anyone", 4*time.Second); err != nil {
			t.Fatalf("%s out of combat should be refused: %v", verb, err)
		}
	}
	t.Log("advance/withdraw verbs wired (refuse cleanly out of combat)")
}
