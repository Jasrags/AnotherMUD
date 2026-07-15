//go:build unix

package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_TransitSubway rides the Downtown Metro end to end (transit.md §4.2 —
// the SCHEDULED policy). No one calls the train: it runs its own timetable,
// ping-ponging Financial District <-> Pike <-> Waterfront. A runner descends to
// the Financial District platform, waits for a train to pull in, boards during
// its dwell, rides to the next station, and steps out — proving the same
// conveyance machine drives a subway when the queue is a timetable, not calls.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_TransitSubway -v
func TestLive_TransitSubway(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":           "shadowrun",
		"ANOTHERMUD_START_ROOM":      "shadowrun:the-flop",
		"ANOTHERMUD_ROLE_SEED":       "Runner:admin",
		"ANOTHERMUD_TRANSIT_CADENCE": "400ms", // a roomy boarding window for the test
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Straphanger"); err != nil {
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
	has := func(out, want, ctx string) {
		t.Helper()
		if !strings.Contains(out, want) {
			t.Fatalf("%s: expected %q in:\n%s", ctx, want, out)
		}
	}

	// Flop -> Westlake -> Fifth Avenue -> down the stairs to the Metro platform.
	send("down")
	send("north")
	has(send("down"), "Financial District Station", "reached the Financial District platform")

	// A call here does NOT summon a scheduled train — it's informational.
	has(send("call"), "schedule", "call at a scheduled platform is informational, not a summons")

	// Wait for a train to pull in and board on its dwell. Each wait captures the
	// arrival buffer, which includes the platform "approaching" cue that fires
	// one beat before the doors open. Boarding only succeeds during the dwell, so
	// retry across arrivals if the window is missed.
	doorsOpen := regexp.MustCompile(`Downtown Metro doors open`)
	boarded, sawApproaching := false, false
	for attempt := 0; attempt < 4 && !boarded; attempt++ {
		buf, err := c.ExpectTimeout(doorsOpen, 30*time.Second)
		if err != nil {
			t.Fatalf("no train pulled into Financial District: %v", err)
		}
		if strings.Contains(buf, "pulling in") {
			sawApproaching = true
		}
		boarded = strings.Contains(send("north"), "Aboard the Metro")
	}
	if !boarded {
		t.Fatal("could not board a train within its dwell after several arrivals")
	}
	if !sawApproaching {
		t.Error("expected the platform approaching cue before the doors opened")
	}

	// On-board next-stop cue: from the Financial District terminus the train's
	// only next stop is Pike Station, and the PA calls it as the doors close.
	if _, err := c.ExpectTimeout(regexp.MustCompile(`Next stop — Pike Station`), 20*time.Second); err != nil {
		t.Fatalf("no on-board next-stop announcement: %v", err)
	}
	// Ride there (the queue is the timetable, no press needed).
	if _, err := c.ExpectTimeout(regexp.MustCompile(`doors open on Pike Station`), 30*time.Second); err != nil {
		t.Fatalf("train never reached Pike Station: %v", err)
	}
	has(send("out"), "Pike Station", "alighted at Pike Station")
	// And the stairs up surface on Pike Street — a real cross-Downtown shortcut.
	has(send("up"), "Pike Street", "surfaced on Pike Street via the Metro")

	t.Log("subway verified: scheduled train self-ran Financial -> Pike; boarded on dwell, rode, alighted, surfaced")
}
