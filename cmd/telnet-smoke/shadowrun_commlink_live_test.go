//go:build unix

package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ShadowrunCommlinkCall proves the first-entry commlink onboarding call:
// a fresh runner carrying a commlink gets Rook's IC transmission on entry, and it
// is shown-once — a relog delivers no second call.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunCommlinkCall -v
func TestLive_ShadowrunCommlinkCall(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":          "shadowrun",
		"ANOTHERMUD_START_ROOM":     "shadowrun:the-flop",
		"ANOTHERMUD_COMMLINK_FIXER": "shadowrun:fixer-mentor",
	})

	// Fresh runner (defaults grant a commlink whatever the role/origin): the
	// commlink chimes with Rook's transmission on entry.
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	entry, err := createCapturingCommlink(c, "Wired", nil)
	if err != nil {
		t.Fatalf("create+capture entry: %v", err)
	}
	low := strings.ToLower(entry)
	for _, want := range []string{"commlink chimes", "transmission", "rook"} {
		if !strings.Contains(low, want) {
			t.Errorf("first-entry commlink call missing %q; got:\n%s", want, entry)
		}
	}

	// Clean quit (saves the shown-once record), then reconnect: no second chime.
	if err := c.SendLine("quit"); err == nil {
		_, _ = c.ExpectTimeout(regexp.MustCompile("Goodbye"), 4*time.Second)
	}
	c.Close()

	c2, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("re-dial: %v", err)
	}
	defer c2.Close()
	reentry, err := reloginCapturingEntry(c2, "Wired")
	if err != nil {
		t.Fatalf("relogin+capture entry: %v", err)
	}
	if strings.Contains(strings.ToLower(reentry), "commlink chimes") {
		t.Errorf("commlink call fired again on relogin (must be shown-once); got:\n%s", reentry)
	}
	t.Logf("commlink onboarding verified live: Rook's transmission fired once on first entry, silent on relog")
}

// createCapturingCommlink drives the creation wizard (defaults for any unset
// step) and returns the enter-world stream up to and including the commlink call.
func createCapturingCommlink(c *telnettest.Client, name string, answers map[string]string) (string, error) {
	isNew, err := doLogin(c, name)
	if err != nil {
		return "", err
	}
	if !isNew {
		return "", fmt.Errorf("expected a fresh character on a clean save")
	}
	if _, err := c.ExpectString("new character's name"); err != nil {
		return "", fmt.Errorf("character-name prompt: %w", err)
	}
	if err := c.SendLine(name); err != nil {
		return "", err
	}
	step := regexp.MustCompile(`Choose your [\w ]+:|\(yes/no\)`)
	field := regexp.MustCompile(`Choose your ([\w ]+):`)
	for i := 0; i < 20; i++ {
		out, err := c.ExpectTimeout(step, 8*time.Second)
		if err != nil {
			return "", fmt.Errorf("wizard step %d: %w", i, err)
		}
		if strings.Contains(out, "(yes/no)") {
			if err := c.SendLine("yes"); err != nil {
				return "", err
			}
			// The confirm enters the world; the commlink call streams in next.
			// Anchor on the CLOSING frame so the whole transmission (chime line +
			// Rook's message) is captured, not just the opening words.
			return c.ExpectTimeout(regexp.MustCompile(`(?i)transmission ends`), 8*time.Second)
		}
		ans := "1"
		if m := field.FindStringSubmatch(out); m != nil {
			if a, ok := answers[strings.ToLower(m[1])]; ok {
				ans = a
			}
		}
		if err := c.SendLine(ans); err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("wizard did not reach the confirm step")
}

// reloginCapturingEntry logs an existing character back in and returns the
// enter-world stream up to the game prompt (where a wrongly-repeated commlink
// call would land).
func reloginCapturingEntry(c *telnettest.Client, name string) (string, error) {
	isNew, err := doLogin(c, name)
	if err != nil {
		return "", err
	}
	if isNew {
		return "", fmt.Errorf("expected an existing character")
	}
	if _, err := c.Expect(regexp.MustCompile("Select a character")); err != nil {
		return "", fmt.Errorf("roster prompt: %w", err)
	}
	if err := c.SendLine(name); err != nil {
		return "", err
	}
	out, err := c.ExpectTimeout(regexp.MustCompile("Make your choice:|"+gamePrompt.String()), 8*time.Second)
	if err != nil {
		return "", fmt.Errorf("post-select prompt: %w", err)
	}
	if strings.Contains(out, "Make your choice:") {
		if err := c.SendLine("1"); err != nil {
			return "", err
		}
		out, err = c.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			return "", fmt.Errorf("enter-world prompt: %w", err)
		}
	}
	return out, nil
}
