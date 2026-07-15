//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_GmcpVitalsPools proves the web-client prereq END TO END: a real
// character's resource pools reach the wire in the Char.Vitals `pools` map. It
// runs a Shadowrun boot because SR carries an **Essence** pool that has NO fixed
// mp/mv slot — so seeing `essence` on the wire proves the generalized map
// delivers what the fixed fields can't (the whole point of the map). Movement
// (a fixed-slot pool) rides both the `mv` field and the map.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_GmcpVitalsPools -v
func TestLive_GmcpVitalsPools(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner",
	})
	frames := &frameLog{}
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second), telnettest.WithGMCPCapture(frames.add))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := c.ActivateGMCP(); err != nil {
		t.Fatalf("activate gmcp: %v", err)
	}
	if err := createAndLogin(c, "Chrome"); err != nil {
		t.Fatalf("create+login: %v", err)
	}
	// Let the per-tick vitals flusher emit the spawn Char.Vitals snapshot.
	c.Drain(2500 * time.Millisecond)

	v, ok := frames.lastAfter("Char.Vitals", 0)
	if !ok {
		t.Fatalf("no Char.Vitals frame captured — GMCP not activating or vitals not flushed")
	}
	// The generalized pools map carries Essence (no fixed slot) + movement.
	for _, want := range []string{`"pools"`, `"essence"`, `"movement"`} {
		if !strings.Contains(v.JSON, want) {
			t.Errorf("Char.Vitals missing %s:\n%s", want, v.JSON)
		}
	}
	t.Logf("Char.Vitals on the wire: %s", v.JSON)
}
