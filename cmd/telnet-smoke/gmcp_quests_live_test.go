//go:build unix

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// questObjective / questEntry / questsPayload decode the Char.Quests wire frame
// (web-client-plan P3 Slice C — the journal form).
type questObjective struct {
	Desc     string `json:"desc"`
	Current  int    `json:"current"`
	Required int    `json:"required"`
	Complete bool   `json:"complete"`
}
type questEntry struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Classification string           `json:"classification"`
	Stage          string           `json:"stage"`
	Hint           string           `json:"hint"`
	Objectives     []questObjective `json:"objectives"`
	AwaitingTurnIn bool             `json:"awaitingTurnIn"`
	Abandonable    bool             `json:"abandonable"`
	AbandonCmd     string           `json:"abandonCmd"`
}
type questsPayload struct {
	Quests []questEntry `json:"quests"`
}

// TestLive_GmcpQuests proves the web-client P3 Slice C journal package reaches
// the wire: on login the server emits a Char.Quests GMCP frame (the additive
// journal form). A fresh character has no active quests, so the journal is empty;
// every entry that appears is well-formed and each abandonable entry carries an
// `abandon <id>` submit command — the data a browser client renders a journal
// panel from.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_GmcpQuests -v
func TestLive_GmcpQuests(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil)

	frames := &frameLog{}
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second), telnettest.WithGMCPCapture(frames.add))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := c.ActivateGMCP(); err != nil {
		t.Fatalf("activate gmcp: %v", err)
	}
	if err := createAndLogin(c, "Journaler"); err != nil {
		t.Fatalf("create+login: %v", err)
	}
	c.Drain(2000 * time.Millisecond)

	f, ok := frames.lastAfter("Char.Quests", 0)
	if !ok {
		t.Fatalf("no Char.Quests frame captured on login (web-client P3 Slice C journal form)")
	}
	var journal questsPayload
	if err := json.Unmarshal([]byte(f.JSON), &journal); err != nil {
		t.Fatalf("unmarshal Char.Quests %q: %v", f.JSON, err)
	}

	// A fresh character starts with no active quests; if the start world DOES
	// auto-grant one, every entry must still be well-formed.
	for _, q := range journal.Quests {
		if q.ID == "" {
			t.Errorf("quest entry missing id: %+v", q)
		}
		if q.Abandonable && !strings.HasPrefix(q.AbandonCmd, "abandon ") {
			t.Errorf("abandonable quest %q abandonCmd = %q, want an `abandon <id>` command", q.ID, q.AbandonCmd)
		}
	}
	t.Logf("login Char.Quests: %d active", len(journal.Quests))
}
