//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_Lockpicking exercises PLAYTEST §26 (skills / lockpicking) on a fresh
// default boot: a fighter descends to the Forge Cellar and picks the locked iron
// door WITHOUT the key. The pick is a d20 + Open Lock vs difficulty 15 check, so
// it loops until a success lands (or the door opens). Booting fresh guarantees
// the door starts locked.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_Lockpicking -v
func TestLive_Lockpicking(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil)

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Picklock"); err != nil {
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

	// town-square → forge (north) → open oak → cellar (down).
	send("north")
	if out := send("open down"); !strings.Contains(strings.ToLower(out), "open") {
		t.Fatalf("PLAYTEST FAIL [§12]: could not open the oak door:\n%s", out)
	}
	if out := send("down"); !strings.Contains(strings.ToLower(out), "cellar") {
		t.Fatalf("PLAYTEST FAIL [§12]: did not descend to the cellar:\n%s", out)
	}

	// §26 — pick the iron door (no key in hand). Loop until a success lands.
	deadline := time.Now().Add(45 * time.Second)
	picked := false
	for time.Now().Before(deadline) {
		out := send("pick iron")
		lo := strings.ToLower(out)
		switch {
		case strings.Contains(lo, "pick an iron door's lock"), strings.Contains(lo, "deftly pick"):
			picked = true
		case strings.Contains(lo, "isn't locked"):
			// A prior attempt already opened it — success.
			picked = true
		case strings.Contains(lo, "fail to pick"), strings.Contains(lo, "fumble"):
			// expected miss; retry
		case strings.Contains(lo, "huh?"), strings.Contains(lo, "no lock"):
			t.Fatalf("PLAYTEST FAIL [§26]: pick verb misbehaved:\n%s", out)
		}
		if picked {
			break
		}
	}
	if !picked {
		t.Fatalf("PLAYTEST FAIL [§26]: never picked the iron door within 45s (Open Lock vs DC15)")
	}

	// §26 — the skill climbs with use; the listing shows Open Lock.
	if out := send("skills"); !strings.Contains(out, "Open Lock") {
		t.Errorf("PLAYTEST FAIL [§26 skills]: Open Lock not listed:\n%s", out)
	}
	t.Log("lockpicking verified live: descended → picked iron door (no key) → skill listed")
}
