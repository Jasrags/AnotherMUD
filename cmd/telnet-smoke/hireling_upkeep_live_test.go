//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_HirelingUpkeepDeparts proves hireable-mobs.md §7: a hireling whose
// upkeep its owner cannot pay departs on the upkeep tick. Booted with a short
// upkeep interval so the test doesn't wait minutes.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_HirelingUpkeepDeparts -v
func TestLive_HirelingUpkeepDeparts(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_HIRELING_UPKEEP_INTERVAL": "2s",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Pincha"); err != nil {
		t.Fatalf("create+login: %v", err)
	}
	send := func(line string) string {
		t.Helper()
		c.Drain(150 * time.Millisecond)
		if err := c.SendLine(line); err != nil {
			t.Fatalf("send %q: %v", line, err)
		}
		out, err := c.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
		return out
	}

	send("set gold amount self 500")
	if out := send("hire sellsword"); !strings.Contains(out, "hire a grizzled sellsword") {
		t.Fatalf("hire failed:\n%s", out)
	}
	if out := send("hirelings"); !strings.Contains(out, "a grizzled sellsword") {
		t.Fatalf("sellsword should be on the roster:\n%s", out)
	}

	// Drain the owner's gold below the upkeep cost; the next upkeep tick can't be
	// paid, so the hireling departs.
	send("set gold amount self 0")

	departed := false
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(send("hirelings"), "no hirelings") {
			departed = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !departed {
		t.Fatal("the unpaid hireling never departed on the upkeep tick")
	}
	t.Log("upkeep-departs verified live: an owner who couldn't pay lost the hireling")
}
