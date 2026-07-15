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

// TestLive_TransitElevator rides the ACHE express elevator end to end
// (transit.md): a runner walks from the flop out to Fifth Avenue, into the
// arcology, boards the car through its doorway, presses for the top floor, rides
// up while the car announces the floors it passes, and steps out onto Corporate
// Suites. It proves the moving-room model live — the doorway binds at the
// arrival floor, and `out` lands the rider on that floor's landing.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_TransitElevator -v
func TestLive_TransitElevator(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":           "shadowrun",
		"ANOTHERMUD_START_ROOM":      "shadowrun:the-flop",
		"ANOTHERMUD_ROLE_SEED":       "Runner:admin",
		"ANOTHERMUD_TRANSIT_CADENCE": "200ms", // a brisk ride for the test
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Rider"); err != nil {
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

	// Flop -> Westlake -> Fifth Avenue -> into the arcology.
	send("down")
	send("north")
	has(send("east"), "Ground Concourse", "entered the ACHE concourse")

	// The car seeds at the Ground Concourse with its doors open: the north
	// elevator door is open, so walk north to board.
	has(send("north"), "Inside the Elevator", "boarded the car through the open doors")

	// Press the top-floor button by its code and ride up.
	has(send("press X"), "Corporate Suites", "pressed the top-floor button [X]")

	// On-board next-stop cue: the panel names the destination as it departs.
	if _, err := c.ExpectTimeout(regexp.MustCompile(`panel lights: Corporate Suites`), 15*time.Second); err != nil {
		t.Fatalf("no on-board next-stop announcement: %v", err)
	}

	// The ride is asynchronous (the transit tick handler drives it): wait for the
	// passing-floor flavor and the arrival chime.
	arr, err := c.ExpectTimeout(regexp.MustCompile(`doors open on Corporate Suites`), 15*time.Second)
	if err != nil {
		t.Fatalf("elevator never arrived at Corporate Suites: %v", err)
	}
	if !strings.Contains(arr, "passes Commercial Concourse") && !strings.Contains(arr, "passes Residential Enclave") {
		t.Logf("note: no passing-floor line captured (timing); arrival ok:\n%s", arr)
	}

	// Step out onto the top-floor landing — the doors opened there on arrival.
	has(send("out"), "Corporate Suites", "alighted on the top floor")

	// Take the fire stairs down one floor to Residential. The car stayed up at
	// Corporate, so the elevator door here is closed — you can't board.
	has(send("down"), "Residential Enclave", "took the fire stairs down")
	has(send("north"), "Elevator door is closed", "elevator door closed on a floor the car isn't at")

	// Call the car to this floor; while waiting, the platform shows it approaching
	// before the doors open, and then you board.
	has(send("call"), "call button", "summoned the car")
	appr, err := c.ExpectTimeout(regexp.MustCompile(`doors open`), 15*time.Second)
	if err != nil {
		t.Fatalf("called car never arrived at Residential: %v", err)
	}
	has(appr, "the car is arriving", "approaching cue on the platform before the doors opened")
	has(send("north"), "Inside the Elevator", "boarded after the called car arrived")

	t.Log("elevator verified: rode Ground->Corporate, closed-door refusal on the wrong floor, call+board after arrival")
}
