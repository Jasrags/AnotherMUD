//go:build unix

package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ColdHazard proves the `cryo` biome's cold ambient hazard end to end
// (area-effects.md §4.6), the cold counterpart to the Glow's radiation:
//
//   - An UNSHIELDED runner who steps into the deep freeze takes the cold payload
//     each hazard tick — the killing-cold room line and dropping HP.
//
//   - A runner WEARING the thermal survival suit (tag `cold-sealed`, the hazard's
//     protection key) takes no cold and no HP loss.
//
//     ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ColdHazard -v
func TestLive_ColdHazard(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":  "Runner:admin",
		// Tick the hazard fast so the window is short and deterministic.
		"ANOTHERMUD_BIOME_HAZARD_INTERVAL": "1s",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Runner"); err != nil {
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
	hpFrom := func(s string) (int, bool) {
		m := hpRe.FindStringSubmatch(s)
		if m == nil {
			return 0, false
		}
		hp, _ := strconv.Atoi(m[1])
		return hp, true
	}

	// --- Phase 1: unshielded → the deep freeze bites. ---
	send("restore")
	base, ok := hpFrom(send("look"))
	if !ok {
		t.Fatal("could not read baseline HP from the prompt")
	}
	send("teleport shadowrun:cold-storage-vault")
	frozen := c.Drain(2500 * time.Millisecond)
	after := send("teleport shadowrun:westlake-plaza") // back to safety before any death

	if !strings.Contains(strings.ToLower(frozen), "cold sinks its teeth") &&
		!strings.Contains(strings.ToLower(frozen), "whiten") {
		t.Fatalf("unshielded runner saw no cold line in the deep freeze:\n%s", frozen)
	}
	if hp, ok := hpFrom(after); ok && hp >= base {
		if hp2, ok := hpFrom(send("look")); ok && hp2 >= base {
			t.Fatalf("unshielded HP did not drop in the deep freeze: baseline %d, after %d", base, hp)
		}
	}

	// --- Phase 2: WEARING the thermal suit → immunity. ---
	send("restore")
	send("spawn item coldsuit me")
	if out := send("equip suit"); !strings.Contains(strings.ToLower(out), "suit") {
		t.Fatalf("could not equip the thermal survival suit:\n%s", out)
	}
	shielded, _ := hpFrom(send("look"))

	send("teleport shadowrun:cold-storage-vault")
	quiet := c.Drain(3000 * time.Millisecond)
	back := send("teleport shadowrun:westlake-plaza")

	if strings.Contains(strings.ToLower(quiet), "cold sinks its teeth") {
		t.Fatalf("worn thermal suit did not protect against the freeze (protection key ignored):\n%s", quiet)
	}
	if hp, ok := hpFrom(back); ok && shielded > 0 && hp < shielded {
		t.Fatalf("worn-suit HP dropped in the freeze: before %d, after %d (immunity failed)", shielded, hp)
	}
}
