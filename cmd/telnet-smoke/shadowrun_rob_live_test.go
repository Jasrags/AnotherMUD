//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_RobDownedHalloweener drives the non-lethal rob path end-to-end: spawn
// a Halloweener, prove rob refuses it while conscious, knock it out, rob it, and
// prove a second rob finds it picked clean.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_RobDownedHalloweener -v
func TestLive_RobDownedHalloweener(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:westlake-plaza", // safe hub, no combat noise
		"ANOTHERMUD_ROLE_SEED":  "Robber:admin",             // spawn/afflict are admin verbs
	})

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	if err := createAndLogin(c, "Robber"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	send := func(cmd string) string {
		t.Helper()
		if err := c.SendLine(cmd); err != nil {
			t.Fatalf("send %q: %v", cmd, err)
		}
		return c.Drain(700 * time.Millisecond)
	}

	send("spawn mob halloweener")

	// Conscious → rob refused.
	if out := send("rob halloweener"); !strings.Contains(strings.ToLower(out), "isn't helpless") {
		t.Fatalf("rob of a conscious Halloweener should refuse; got:\n%s", out)
	}

	// Knock it out, then rob.
	send("afflict halloweener unconscious")
	robbed := send("rob halloweener")
	t.Logf("ROB OUTPUT:\n%s", robbed)
	low := strings.ToLower(robbed)
	if !strings.Contains(low, "you loot") && !strings.Contains(low, "nothing worth taking") {
		t.Fatalf("rob of a downed Halloweener produced no loot line:\n%s", robbed)
	}

	// Second rob → already picked clean (the single-claim guard).
	if out := send("rob halloweener"); !strings.Contains(strings.ToLower(out), "picked clean") {
		t.Fatalf("second rob should report already picked clean; got:\n%s", out)
	}
	t.Logf("rob verified live: refused while conscious -> robbed while down -> picked clean on re-rob")
}
