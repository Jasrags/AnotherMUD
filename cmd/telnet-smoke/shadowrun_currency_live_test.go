//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ShadowrunCurrency proves the currency-label seam end-to-end through
// the real boot wiring (main.go → session.Config → command.Env → Context →
// handlers) — the one thing unit tests can't reach. A fresh Street Samurai's
// balance and score sheet must render nuyen with the ¥ mark, not "gold".
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunCurrency -v
func TestLive_ShadowrunCurrency(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":  "Runner:admin", // for the deterministic `spawn gold` seed
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Runner"); err != nil {
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

	// Seed a known positive balance so the `gold` verb has an amount to render
	// (a zero balance says "no nuyen" with no ¥).
	send("spawn gold 1000") // admin conjure into the purse

	// The `gold` verb reskins the amount to ¥. (We assert ¥ presence only — the
	// telnet echo of the typed "gold" command would foil a negative gold check.)
	if out := send("gold"); !strings.Contains(out, "¥") {
		t.Fatalf("`gold` verb did not render nuyen/¥ (got %q) — the currency label isn't threading to the handler", strings.TrimSpace(out))
	}

	// The score purse row is labelled Nuyen and shows ¥.
	out := send("score")
	if !strings.Contains(out, "Nuyen") {
		t.Fatalf("score purse label is not 'Nuyen':\n%s", out)
	}
	if !strings.Contains(out, "¥") {
		t.Fatalf("score purse value has no ¥ mark:\n%s", out)
	}
	t.Log("shadowrun currency verified live: the `gold` verb and score sheet render nuyen with the ¥ mark through the full boot wiring")
}
