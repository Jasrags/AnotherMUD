//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_RivalFixer proves the second job source: Kestrel, the street fixer at
// the Big Rhino, offers her own open starter run (Protection) while her follow-up
// (Care Package) stays prereq-gated — and the two-sided rivalry dialogue works
// (ask Kestrel about Johnson, and Johnson about Kestrel).
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_RivalFixer -v
func TestLive_RivalFixer(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:dantes-inferno",
		"ANOTHERMUD_ROLE_SEED":  "Streetwise:admin", // teleport between the two hubs
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Streetwise"); err != nil {
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

	// Kestrel holds court at the Big Rhino.
	send("teleport shadowrun:the-big-rhino")

	// Her starter run is open to a fresh runner; her follow-up is gated behind it.
	board := send("talk kestrel")
	if !strings.Contains(board, "Protection") {
		t.Fatalf("Kestrel should offer her open starter run Protection:\n%s", board)
	}
	if strings.Contains(board, "Care Package") {
		t.Fatalf("Care Package should be prereq-gated behind Protection, not offered to a fresh runner:\n%s", board)
	}

	// The rivalry runs both ways: Kestrel on Johnson...
	if out := send("ask kestrel about johnson"); !strings.Contains(out, "Kestrel") || !strings.Contains(out, "Johnson") {
		t.Fatalf("ask kestrel about johnson did not speak her take on the corp fixer:\n%s", out)
	}
	// ...and Johnson on Kestrel.
	send("teleport shadowrun:dantes-inferno")
	if out := send("ask johnson about kestrel"); !strings.Contains(out, "Rhino") {
		t.Fatalf("ask johnson about kestrel did not speak his take on the rival:\n%s", out)
	}
}
