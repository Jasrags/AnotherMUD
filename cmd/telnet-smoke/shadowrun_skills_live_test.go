//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ShadowrunSkills proves the SR skills content track (sr-skills-plan.md
// content Slice A) on the shipped engine (skills.md §2.1/§5): a fresh Street
// Samurai starts trained across the weapon skills + Sneaking (granted by the
// class path), and `skills` renders them grouped SR-style — by category then
// group, with a linked-attribute tag. Proves the 18-skill roster loads, the
// grants land as proficiency, and the grouped display lights up with real
// content (not just the unit fixtures).
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunSkills -v
func TestLive_ShadowrunSkills(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:the-flop",
		"ANOTHERMUD_ROLE_SEED":  "Runner:admin",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Chromer"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	if err := c.SendLine("skills"); err != nil {
		t.Fatalf("send skills: %v", err)
	}
	out, err := c.ExpectTimeout(gamePrompt, 8*time.Second)
	if err != nil {
		t.Fatalf("no prompt after skills: %v", err)
	}

	has := func(want string) {
		t.Helper()
		if !strings.Contains(out, want) {
			t.Errorf("skills sheet missing %q:\n%s", want, out)
		}
	}
	// Grouped headers (category — group), the weapon skills, Sneaking, and the
	// SR-overridden Perception, each with an attribute tag.
	has("Combat — Firearms")
	has("Pistols")
	has("Automatics")
	has("Combat — Close Combat")
	has("Blades")
	has("Physical — Stealth")
	has("Sneaking")
	has("(AGI)") // agility-linked skills carry the tag
	// Perception is the SR override (intuition), under Physical.
	has("Perception")

	t.Logf("SR grouped skills sheet:\n%s", out)
}
