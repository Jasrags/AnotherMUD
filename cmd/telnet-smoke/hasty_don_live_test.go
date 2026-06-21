//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_HastyDon proves the hastily-donned armor escape (armor-depth §7) end
// to end on the WoT boot: a character buys a breastplate (medium = slow armor)
// at the forge and `hastydon`s it — a faster timed don that announces the haste,
// completes, and warns the piece sits poorly (the degraded protection).
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_HastyDon -v
func TestLive_HastyDon(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "wot",
		"ANOTHERMUD_START_ROOM": "wot:the-green",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Hasty"); err != nil {
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

	send("teleport wot:the-forge")
	send("set gold amount Hasty 3000")
	if out := send("buy breastplate"); strings.Contains(strings.ToLower(out), "don't") || strings.Contains(strings.ToLower(out), "can't") {
		t.Fatalf("could not buy a breastplate at the forge:\n%s", out)
	}

	// Hasty don: the begin announces the haste...
	if out := send("hastydon breastplate"); !strings.Contains(strings.ToLower(out), "hastily") {
		t.Fatalf("hastydon should begin a hasty don, got:\n%s", out)
	}
	// ...and the timed completion both equips it and warns it sits poorly.
	out, err := c.ExpectStringTimeout("sits poorly", 6*time.Second)
	if err != nil {
		t.Fatalf("the hasty don never completed with a degradation note: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "breastplate") {
		t.Errorf("completion should name the breastplate:\n%s", out)
	}
}
