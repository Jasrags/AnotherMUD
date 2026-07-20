//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ShadowrunRoleOrigin exercises role×origin character creation end to
// end: a Face × Corporate Dropout and a Street Samurai × Street Kid, proving the
// role-floor weapon, the universal commlink (tier by origin), and the SIN
// spectrum (a real SIN vs SINless) all land on a freshly-created runner.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunRoleOrigin -v
func TestLive_ShadowrunRoleOrigin(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:westlake-plaza", // safe start, no fixer noise
	})

	// createRunner dials a fresh connection, creates a character with the given
	// role/origin picks (the wizard resolves a case-insensitive label prefix), and
	// returns a send() helper bound to that session.
	createRunner := func(name string, answers map[string]string) func(string) string {
		t.Helper()
		c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
		if err != nil {
			t.Fatalf("dial %s: %v", name, err)
		}
		t.Cleanup(func() { c.Close() })
		isNew, err := doLogin(c, name)
		if err != nil {
			t.Fatalf("login %s: %v", name, err)
		}
		if !isNew {
			t.Fatalf("%s: expected a fresh character on a clean save", name)
		}
		if err := finishLogin(c, name, isNew, answers); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return func(line string) string {
			t.Helper()
			if err := c.SendLine(line); err != nil {
				t.Fatalf("%s send %q: %v", name, line, err)
			}
			out, err := c.ExpectTimeout(gamePrompt, 8*time.Second)
			if err != nil {
				t.Fatalf("%s: no prompt after %q: %v", name, line, err)
			}
			return out
		}
	}
	has := func(out, want string) bool { return strings.Contains(strings.ToLower(out), want) }

	// --- Face × Corporate Dropout: holdout floor, top-tier commlink, real SIN. ---
	face := createRunner("Faceman", map[string]string{"class": "face", "background": "corporate"})
	inv := face("inventory")
	for _, want := range []string{"streetline special", "hermes ikon", "corporate sin", "actioneer", "ares light fire"} {
		if !has(inv, want) {
			t.Errorf("Face×Corporate inventory missing %q; got:\n%s", want, inv)
		}
	}
	if lic := face("licenses"); !has(lic, "corporate") || !has(lic, "firearms") {
		t.Errorf("Face×Corporate `licenses` should show the corporate SIN's firearms+corporate permits; got:\n%s", lic)
	}
	if sk := face("skills"); !has(sk, "negotiation") || !has(sk, "con") {
		t.Errorf("Face `skills` should show the social spread (negotiation/con); got:\n%s", sk)
	}

	// --- Street Samurai × Street Kid: stun-baton floor, cheap commlink, SINless. ---
	sam := createRunner("Razor", map[string]string{"class": "street samurai", "background": "street kid"})
	inv = sam("inventory")
	for _, want := range []string{"stun baton", "meta link"} {
		if !has(inv, want) {
			t.Errorf("Sam×StreetKid inventory missing %q; got:\n%s", want, inv)
		}
	}
	if lic := sam("licenses"); !has(lic, "sinless") {
		t.Errorf("Street Kid should be running SINless; got:\n%s", lic)
	}

	t.Logf("role×origin verified live: Face got a Streetline+Hermes+corporate SIN, Sam a stun baton+Meta Link+SINless")
}
