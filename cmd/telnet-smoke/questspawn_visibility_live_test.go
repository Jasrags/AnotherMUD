//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_QuestSpawnVisibility proves quest-spawns.md Phase 2 (per-observer
// visibility) end to end: two runners both accept the Johnson run and both
// reach Avondale, so each activation spawns its OWN paydata chip into the
// shared room. With the owner gate, each runner sees exactly ONE chip (their
// own) — never the other's — and each can still pick up their own. Without the
// gate (Phase 1) a `look` here would show two chips.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_QuestSpawnVisibility -v
func TestLive_QuestSpawnVisibility(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner",
		// Both admin so each can teleport to the meet and the site.
		"ANOTHERMUD_ROLE_SEED": "Alfa:admin;Brava:admin",
	})

	dial := func(name string) *telnettest.Client {
		t.Helper()
		c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
		if err != nil {
			t.Fatalf("dial %s: %v", name, err)
		}
		t.Cleanup(func() { c.Close() })
		if err := createAndLogin(c, name); err != nil {
			t.Fatalf("create+login %s: %v", name, err)
		}
		return c
	}
	alfa := dial("Alfa")
	brava := dial("Brava")

	send := func(c *telnettest.Client, line string) string {
		t.Helper()
		c.Drain(150 * time.Millisecond)
		if err := c.SendLine(line); err != nil {
			t.Fatalf("send %q: %v", line, err)
		}
		out, err := c.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
		return out
	}

	alfa.Drain(800 * time.Millisecond)
	brava.Drain(800 * time.Millisecond)

	// Both accept the run at the meet, then both walk into Avondale — each
	// stage-2 activation spawns that runner's own chip + gangers.
	accept := func(c *telnettest.Client) {
		t.Helper()
		send(c, "teleport shadowrun:dantes-inferno")
		if out := send(c, "accept Redmond Retrieval"); strings.Contains(strings.ToLower(out), "requirements") {
			t.Fatalf("Redmond Retrieval was prereq-refused:\n%s", out)
		}
		send(c, "teleport shadowrun:avondale") // visit objective -> stage 2 spawns
	}
	accept(alfa)
	accept(brava)

	// Give the shared room a beat to settle both spawn sets, then look.
	alfa.Drain(400 * time.Millisecond)
	brava.Drain(400 * time.Millisecond)

	// Each runner sees exactly one paydata chip: their own. The foreign chip
	// is gated out (Phase 2). Under Phase 1 this count would be 2.
	if got := strings.Count(strings.ToLower(send(alfa, "look")), "paydata chip"); got != 1 {
		t.Fatalf("Alfa should see exactly her own chip, saw %d paydata chips", got)
	}
	if got := strings.Count(strings.ToLower(send(brava, "look")), "paydata chip"); got != 1 {
		t.Fatalf("Brava should see exactly her own chip, saw %d paydata chips", got)
	}

	// Each can still collect her own chip (owner visibility intact end to end).
	if out := send(alfa, "get chip"); !strings.Contains(strings.ToLower(out), "paydata") &&
		!strings.Contains(strings.ToLower(out), "pick up") {
		t.Fatalf("Alfa could not pick up her own paydata chip:\n%s", out)
	}
	// Brava's chip is untouched by Alfa's pickup — she still sees exactly one.
	if got := strings.Count(strings.ToLower(send(brava, "look")), "paydata chip"); got != 1 {
		t.Fatalf("Brava's chip should be unaffected by Alfa's pickup, saw %d", got)
	}
}
