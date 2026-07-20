//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_FinishDownedHalloweener drives the coup-de-grace path end-to-end:
// spawn a Halloweener, prove finish refuses it while conscious, knock it out,
// finish it (guaranteed lethal), and confirm it left a corpse (a real death, not
// a knock-out).
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_FinishDownedHalloweener -v
func TestLive_FinishDownedHalloweener(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:westlake-plaza",
		"ANOTHERMUD_ROLE_SEED":  "Butcher:admin", // spawn/afflict are admin verbs
	})

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	if err := createAndLogin(c, "Butcher"); err != nil {
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

	// Conscious → finish refused (no free instakill).
	if out := send("finish halloweener"); !strings.Contains(strings.ToLower(out), "isn't helpless") {
		t.Fatalf("finish of a conscious Halloweener should refuse; got:\n%s", out)
	}

	// Knock it out, then finish it.
	send("afflict halloweener unconscious")
	killed := send("finish halloweener")
	t.Logf("FINISH OUTPUT:\n%s", killed)
	low := strings.ToLower(killed)
	blow := strings.Index(low, "killing blow")
	if blow < 0 {
		t.Fatalf("finish did not report a killing blow:\n%s", killed)
	}
	// The action line must read BEFORE the death pipeline's kill announcement.
	if slain := strings.Index(low, "slain"); slain >= 0 && blow > slain {
		t.Fatalf("ordering: 'killing blow' should precede the kill announcement:\n%s", killed)
	}

	// A real death leaves a corpse in the room (a knock-out would not).
	look := send("look")
	if !strings.Contains(strings.ToLower(look), "corpse") {
		t.Fatalf("finish left no corpse — expected a death, got:\n%s", look)
	}
	t.Logf("finish verified live: refused while conscious -> lethal coup-de-grace while down -> corpse in the room")
}
