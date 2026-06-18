//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_MountLifecycle drives the whole mount loop end to end against a real
// engine: buy a mount from the gate stablemaster, retrieve it, ride it out into
// the meadow and back (proving co-located travel — the mount follows the
// rider), dismount, and stable it again. The first character is the bootstrap
// admin, so it can `set gold` to fund the purchase.
//
// Run: ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_MountLifecycle -v
func TestLive_MountLifecycle(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, start = town-square

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Rider"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	// Fund the purchase (bootstrap admin can set its own gold).
	mountCmd(t, c, "set gold amount self 500", "Gold set to 500")

	// No stable at town-square: the verb refuses cleanly.
	mountCmd(t, c, "buymount horse", "no stable here")

	// To the gate stable (south).
	walkStep(t, c, "south") // town-square -> village-gate

	mountCmd(t, c, "mounts", "own no mounts")
	mountCmd(t, c, "buymount horse", "stabled here")
	mountCmd(t, c, "unstable horse", "brings out")
	mountCmd(t, c, "mount horse", "climb onto")

	// Ride north into the town square — the mount must FOLLOW the rider
	// (co-located travel). Proof that doesn't depend on the (dark) room render:
	// dismount and re-mount. `mount horse` succeeds ONLY if the horse is in
	// this room — i.e. it came along. Had it stayed at the gate, this would be
	// "You don't see that mount here."
	walkStep(t, c, "north") // village-gate -> town-square (mounted)
	mountCmd(t, c, "dismount", "climb down")
	mountCmd(t, c, "mount horse", "climb onto") // co-location proof

	// Ride back to the gate and put the horse away.
	walkStep(t, c, "south") // town-square -> village-gate (mounted, mount follows)
	mountCmd(t, c, "dismount", "climb down")
	mountCmd(t, c, "stable horse", "You stable")
	mountCmd(t, c, "mounts", "stabled")

	t.Log("mount lifecycle verified: buy -> retrieve -> mount -> co-located ride -> dismount -> stable")
}

// mountCmd sends a command and waits for want to appear in the response,
// returning the captured output. Fails the test if want never arrives.
func mountCmd(t *testing.T, c *telnettest.Client, line, want string) string {
	t.Helper()
	if err := c.SendLine(line); err != nil {
		t.Fatalf("send %q: %v", line, err)
	}
	out, err := c.ExpectStringTimeout(want, 5*time.Second)
	if err != nil {
		t.Fatalf("after %q: never saw %q: %v", line, want, err)
	}
	return out
}
