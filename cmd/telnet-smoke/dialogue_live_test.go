//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_DialogueAndBarQuest exercises the `ask <npc> about <topic>` dialogue
// verb and the quest-giver wiring for the two Cocktail bartenders in Dante's
// Inferno. Booting straight into the club (a safe room), a runner:
//   - asks Doug about his Laws (a LIST topic → speaks a Coughlin's Law),
//   - asks Doug about Brian (a single-line topic),
//   - `list`s Doug's drink shop (synthahol on the menu),
//   - `talk`s to Brian and sees the "Flanagan's Own" quest offer.
//
// This proves the whole content+engine path end-to-end: the split `ask` verb,
// dialogue lookup from the mob's Properties bag, the drinks shop block, and the
// quest giver surfacing an offer.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_DialogueAndBarQuest -v
func TestLive_DialogueAndBarQuest(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:dantes-inferno",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Barfly"); err != nil {
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

	// A list topic speaks one of Coughlin's Laws.
	if out := send("ask doug about laws"); !strings.Contains(out, "Coughlin's Law") {
		t.Fatalf("ask doug about laws did not speak a Law:\n%s", out)
	}
	// A single-line topic resolves too.
	if out := send("ask doug about brian"); !strings.Contains(strings.ToLower(out), "flash") {
		t.Fatalf("ask doug about brian did not speak the brian line:\n%s", out)
	}
	// Doug runs the drink shop.
	if out := send("list"); !strings.Contains(strings.ToLower(out), "synthahol") {
		t.Fatalf("Doug's `list` did not show the drink menu (no synthahol):\n%s", out)
	}
	// Brian offers his dream-bar quest.
	if out := send("talk brian"); !strings.Contains(out, "Flanagan's Own") {
		t.Fatalf("talk brian did not surface the Flanagan's Own offer:\n%s", out)
	}
}
