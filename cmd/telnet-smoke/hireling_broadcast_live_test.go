//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_HirelingFollowBroadcast proves hireable-mobs.md §5 relocate broadcasts:
// a bystander left in the room sees a bound hireling leave when its owner walks
// off. Deterministic — owner-driven movement, no RNG/timing. The first character
// is the bootstrap admin, so it can fund the hire.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_HirelingFollowBroadcast -v
func TestLive_HirelingFollowBroadcast(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, town-square

	owner, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial owner: %v", err)
	}
	defer owner.Close()
	if err := createAndLogin(owner, "Marcher"); err != nil {
		t.Fatalf("create owner: %v", err)
	}

	watcher, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial watcher: %v", err)
	}
	defer watcher.Close()
	if err := createAndLogin(watcher, "Watcher"); err != nil {
		t.Fatalf("create watcher: %v", err)
	}

	send := func(line string) {
		t.Helper()
		if err := owner.SendLine(line); err != nil {
			t.Fatalf("send %q: %v", line, err)
		}
		if _, err := owner.ExpectTimeout(gamePrompt, 8*time.Second); err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
	}

	// Owner hires a sellsword in the town square (where the watcher is), then
	// walks north. The bound sellsword follows — the watcher, left behind, sees it.
	send("set gold amount self 500")
	send("hire sellsword")
	watcher.Drain(500 * time.Millisecond)
	send("north")

	if _, err := watcher.ExpectStringTimeout("a grizzled sellsword follows Marcher north", 6*time.Second); err != nil {
		t.Fatalf("watcher did not see the hireling follow its owner away: %v", err)
	}
	t.Log("relocate broadcast verified live: a bystander saw the bound hireling follow its owner out")
}
