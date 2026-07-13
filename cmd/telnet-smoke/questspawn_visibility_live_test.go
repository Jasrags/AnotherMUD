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
// visibility) end to end, both the owner gate and the §10 admin bypass:
//
//   - Two runners (Alfa, Brava — admin, so they can teleport to drive the run)
//     both accept the Johnson run and both reach Avondale, so each activation
//     spawns its OWN paydata chip into the shared room.
//
//   - A non-admin bystander (Cleric) stands in Avondale the whole time. She is
//     on no run and owns nothing, so the gate hides BOTH chips from her — she
//     sees zero. Under Phase 1 she would have seen two.
//
//   - An admin runner (Alfa) bypasses the gate for moderation, so she sees BOTH
//     chips (her own + the foreign one) — proving the staff bypass is wired.
//
//     ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_QuestSpawnVisibility -v
func TestLive_QuestSpawnVisibility(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS": "shadowrun",
		// Everyone starts in Avondale so the non-admin bystander can stand at the
		// spawn site without needing (admin-only) teleport; the runners teleport
		// away to the meet and back.
		"ANOTHERMUD_START_ROOM": "shadowrun:avondale",
		// Only the two runners are admin (teleport to drive the run); Cleric is a
		// plain player, the non-owner the gate must hide the spawns from.
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
	cleric := dial("Cleric") // non-admin bystander, stays in Avondale

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
	chipsSeen := func(c *telnettest.Client) int {
		return strings.Count(strings.ToLower(send(c, "look")), "paydata chip")
	}

	for _, c := range []*telnettest.Client{alfa, brava, cleric} {
		c.Drain(800 * time.Millisecond)
	}

	// Each runner accepts the run at the meet, then teleports back into Avondale
	// — the visit objective completes and stage 2 spawns that runner's own chip.
	runTo := func(c *telnettest.Client) {
		t.Helper()
		send(c, "teleport shadowrun:dantes-inferno")
		if out := send(c, "accept Redmond Retrieval"); strings.Contains(strings.ToLower(out), "requirements") {
			t.Fatalf("Redmond Retrieval was prereq-refused:\n%s", out)
		}
		send(c, "teleport shadowrun:avondale") // visit objective -> stage 2 spawns
	}
	runTo(alfa)
	runTo(brava)

	// Let both spawn sets settle in the shared room.
	for _, c := range []*telnettest.Client{alfa, brava, cleric} {
		c.Drain(400 * time.Millisecond)
	}

	// The gate: the non-admin bystander owns nothing, so BOTH runners' chips are
	// hidden from her — she sees zero. (Phase 1 would show two.)
	if got := chipsSeen(cleric); got != 0 {
		t.Fatalf("non-owner bystander must see no foreign quest spawns, saw %d paydata chips", got)
	}

	// The admin bypass: Alfa is staff, so she sees BOTH chips (her own + Brava's).
	if got := chipsSeen(alfa); got != 2 {
		t.Fatalf("staff runner should bypass the gate and see both chips, saw %d", got)
	}
}
