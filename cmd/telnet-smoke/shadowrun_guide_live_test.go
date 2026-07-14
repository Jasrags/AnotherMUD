//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ShadowrunGuide exercises the onboarding guide end to end: a new runner
// spawning in the safehouse is met by Patch (the street-guide), who trails them as
// they move and leaves when `shoo`ed.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunGuide -v
func TestLive_ShadowrunGuide(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":          "shadowrun",
		"ANOTHERMUD_START_ROOM":     "shadowrun:the-flop",
		"ANOTHERMUD_GUIDE_TEMPLATE": "shadowrun:street-guide",
		"ANOTHERMUD_ROLE_SEED":      "Runner:admin",
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
	hasPatch := func(out string) bool { return strings.Contains(strings.ToLower(out), "patch") }

	// A fresh runner is met by Patch in the flop.
	if !hasPatch(send("look")) {
		t.Fatal("the guide (Patch) was not present at the spawn room")
	}

	// Patch trails the runner down the stairwell into Downtown.
	if !hasPatch(send("down")) {
		t.Fatal("the guide did not trail the runner into Westlake Plaza")
	}
	if !hasPatch(send("look")) {
		t.Fatal("the guide is not in the runner's new room after trailing")
	}

	// `shoo` sends the guide away for the session.
	if out := send("shoo"); !strings.Contains(strings.ToLower(out), "wave") {
		t.Fatalf("shoo did not confirm sending the guide off:\n%s", out)
	}
	if hasPatch(send("look")) {
		t.Fatal("the guide is still present after `shoo`")
	}
	t.Logf("guide verified live: Patch met the runner, trailed them to Westlake, and left on `shoo`")
}

// TestLive_ShadowrunGuideGraduates proves the graduation gate: reaching the
// default graduation level (3) retires the guide with a farewell. An admin `xp`
// top-up (300 XP crosses street.yaml's level-3 threshold) triggers it without
// grinding a real fight.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunGuideGraduates -v
func TestLive_ShadowrunGuideGraduates(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":          "shadowrun",
		"ANOTHERMUD_START_ROOM":     "shadowrun:the-flop",
		"ANOTHERMUD_GUIDE_TEMPLATE": "shadowrun:street-guide",
		"ANOTHERMUD_ROLE_SEED":      "Runner:admin",
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
	hasPatch := func(out string) bool { return strings.Contains(strings.ToLower(out), "patch") }

	if !hasPatch(send("look")) {
		t.Fatal("the guide (Patch) was not present at the spawn room")
	}

	// Cross the level-3 threshold on the street track (the default cap) — the
	// guide graduates.
	grad := send("xp 300 street")
	// The farewell may print on the xp line or be visible via a follow-up look.
	if !strings.Contains(strings.ToLower(grad), "hang of it") && hasPatch(send("look")) {
		t.Fatalf("the guide did not graduate on reaching the level cap:\n%s", grad)
	}
	if hasPatch(send("look")) {
		t.Fatal("the guide is still present after graduating to the level cap")
	}
	t.Logf("guide verified live: Patch graduated the runner and departed at the level cap")
}
