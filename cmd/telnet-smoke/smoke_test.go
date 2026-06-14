package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestSmoke_LoginLook is the live end-to-end integration test: it drives the
// login-look scenario against a RUNNING engine. It is env-gated so the normal
// `go test ./...` run (which has no engine) skips it rather than failing.
//
// To run it:
//
//	# terminal 1 — boot an engine on a known port with a throwaway save dir
//	ANOTHERMUD_ADDR=127.0.0.1:14500 ANOTHERMUD_SAVE_DIR=$(mktemp -d) \
//	    go run ./cmd/anothermud
//
//	# terminal 2
//	ANOTHERMUD_TELNET_ADDR=127.0.0.1:14500 \
//	    go test ./cmd/telnet-smoke -run TestSmoke_LoginLook -v
//
// The same scenario function (scenarioLoginLook) backs the standalone binary,
// so the binary and this test never drift.
func TestSmoke_LoginLook(t *testing.T) {
	addr := os.Getenv("ANOTHERMUD_TELNET_ADDR")
	if addr == "" {
		t.Skip("set ANOTHERMUD_TELNET_ADDR=host:port (a running engine) to run the live smoke test")
	}
	c := telnettest.DialT(t, addr, telnettest.WithTimeout(8*time.Second))
	if err := scenarioLoginLook(c, "Smoketest"); err != nil {
		t.Fatalf("login+look smoke against %s: %v", addr, err)
	}
}
