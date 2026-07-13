//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_QuestSpawnReDerive proves the Phase-1b session lifecycle of quest-
// scoped spawns (quest-spawns.md §7): a run's spawned content is cleaned up when
// the player logs out and re-derived when they log back in, so a disconnect
// mid-run doesn't soft-lock it.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_QuestSpawnReDerive -v
//
// Flow: accept "Redmond Retrieval", reach Avondale (stage 2 activates → the
// paydata chip + scavengers spawn), then `quit` (Manager.Remove → CleanupPlayer
// despawns them) and reconnect on the same account (Manager.Add →
// ReactivatePlayer re-spawns the active stage). Back in Avondale, the chip is
// there again — proof the re-derive recreated it.
func TestLive_QuestSpawnReDerive(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:dantes-inferno", // the meet is here
		"ANOTHERMUD_ROLE_SEED":  "Redux:admin",              // teleport
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := createAndLogin(c, "Redux"); err != nil {
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

	// Meet the Johnson, take the run, and reach Avondale — stage 2 activates and
	// spawns the chip + scavengers there.
	send("talk johnson")
	send("accept Redmond Retrieval")
	send("teleport shadowrun:avondale")

	// Disconnect: `quit` routes through Manager.Remove -> CleanupPlayer, which
	// despawns everything this run owns.
	_ = c.SendLine("quit")
	time.Sleep(500 * time.Millisecond)
	c.Close()

	// Reconnect on the same account and select the character from the roster.
	c2, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("re-dial: %v", err)
	}
	defer c2.Close()
	if _, err := doLogin(c2, "Redux"); err != nil {
		t.Fatalf("relogin: %v", err)
	}
	if err := finishLogin(c2, "Redux", false, nil); err != nil {
		t.Fatalf("roster select: %v", err)
	}
	send2 := func(line string) string {
		t.Helper()
		_ = c2.SendLine(line)
		out, _ := c2.ExpectTimeout(gamePrompt, 8*time.Second)
		return out
	}

	// Login fired ReactivatePlayer, re-spawning the active stage's content in
	// Avondale (the spawn's declared room, regardless of where we landed). Go
	// there and confirm the chip was recreated.
	send2("teleport shadowrun:avondale")
	send2("restore")
	if out := send2("get chip"); !strings.Contains(strings.ToLower(out), "paydata") &&
		!strings.Contains(strings.ToLower(out), "pick up") {
		t.Fatalf("re-derive did not recreate the paydata chip after relogin:\n%s", out)
	}
}
