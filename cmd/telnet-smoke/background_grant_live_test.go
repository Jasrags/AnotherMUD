package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_BackgroundGrant drives the background-grant scenario against a
// RUNNING WoT engine: it creates an Aiel character and asserts the background's
// starting package (shortbow + buckler items, the Stealthy feat) actually
// landed, plus the channeling-gift row on the score sheet. Env-gated so the
// normal `go test ./...` run (no engine) skips it.
//
// To run it:
//
//	# terminal 1 — a WoT engine on a known port with a throwaway save dir
//	ANOTHERMUD_PACKS=wot ANOTHERMUD_START_ROOM=wot:the-green \
//	    ANOTHERMUD_ADDR=127.0.0.1:14530 ANOTHERMUD_SAVE_DIR=$(mktemp -d) \
//	    go run ./cmd/anothermud
//
//	# terminal 2
//	ANOTHERMUD_TELNET_ADDR=127.0.0.1:14530 \
//	    go test ./cmd/telnet-smoke -run TestLive_BackgroundGrant -v
func TestLive_BackgroundGrant(t *testing.T) {
	addr := os.Getenv("ANOTHERMUD_TELNET_ADDR")
	if addr == "" {
		t.Skip("set ANOTHERMUD_TELNET_ADDR=host:port (a running WoT engine) to run this live test")
	}
	c := telnettest.DialT(t, addr, telnettest.WithTimeout(8*time.Second))
	if err := scenarioBackgroundGrant(c, "Bgtest"); err != nil {
		t.Fatalf("background grant against %s: %v", addr, err)
	}
}
