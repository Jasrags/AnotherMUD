//go:build unix

package main

import (
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_MultipleCharactersPerAccount confirms the substrate behind a
// per-account roster (character-identity): one account (email) can hold
// several characters, each created and logged into independently. A first
// character creates the account; a second is created on the SAME email via
// the existing-account path (no password confirmation — proof the email
// matched an existing account); then both log back in by name.
//
// On a one-world-per-process server both characters are in the active world,
// so this proves the multi-character substrate; the cross-world gate (a
// character refused when its world isn't active) is covered by the login
// world-gate unit tests.
func TestLive_MultipleCharactersPerAccount(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, town-square
	const email = "house@smoke.test"

	// 1. First character — creates a new account at `email`.
	c1, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial 1: %v", err)
	}
	if err := createWithEmail(c1, "Aymara", email, true); err != nil {
		c1.Close()
		t.Fatalf("create Aymara (new account): %v", err)
	}
	c1.Close()

	// 2. Second character on the SAME account (existing-email path).
	c2, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial 2: %v", err)
	}
	if err := createWithEmail(c2, "Bronn", email, false); err != nil {
		c2.Close()
		t.Fatalf("create Bronn on existing account: %v", err)
	}
	c2.Close()

	// 3. Both characters log back in by name — proof both persist and are
	//    independently playable.
	for _, name := range []string{"Aymara", "Bronn"} {
		c, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
		if err != nil {
			t.Fatalf("dial returning %s: %v", name, err)
		}
		if err := loginReturning(c, name); err != nil {
			c.Close()
			t.Fatalf("returning login %s: %v", name, err)
		}
		c.Close()
	}
}

// createWithEmail drives character creation at the name prompt. When
// newAccount is true it expects the new-account dance (Choose password +
// Confirm); when false it expects the existing-account path (Password for
// <email>, no confirm) — which only fires when email matches an account, so
// reaching the wizard proves the new character bound to the existing account.
func createWithEmail(c *telnettest.Client, name, email string, newAccount bool) error {
	if _, err := c.ExpectString("name shall we know you"); err != nil {
		return fmt.Errorf("name prompt: %w", err)
	}
	if err := c.SendLine(name); err != nil {
		return err
	}
	if _, err := c.Expect(regexp.MustCompile("(?i)email")); err != nil {
		return fmt.Errorf("email prompt: %w", err)
	}
	if err := c.SendLine(email); err != nil {
		return err
	}
	if newAccount {
		if _, err := c.ExpectString("Choose a password"); err != nil {
			return fmt.Errorf("choose-password prompt: %w", err)
		}
		if err := c.SendLine(smokePassword); err != nil {
			return err
		}
		if _, err := c.ExpectString("Confirm"); err != nil {
			return fmt.Errorf("confirm prompt: %w", err)
		}
		if err := c.SendLine(smokePassword); err != nil {
			return err
		}
	} else {
		// Existing-account path: a plain "Password for <email>:" with no
		// confirmation. If we instead saw "Choose"/"Confirm" the email did
		// not match an account and the test premise is broken.
		out, err := c.Expect(regexp.MustCompile(`(?i)password`))
		if err != nil {
			return fmt.Errorf("existing-account password prompt: %w", err)
		}
		if regexp.MustCompile(`(?i)choose`).MatchString(out) {
			return fmt.Errorf("expected existing-account path, got new-account prompt: %q", out)
		}
		if err := c.SendLine(smokePassword); err != nil {
			return err
		}
	}
	return runWizardWith(c, nil)
}

// loginReturning logs in an existing character by name + password.
func loginReturning(c *telnettest.Client, name string) error {
	if _, err := c.ExpectString("name shall we know you"); err != nil {
		return fmt.Errorf("name prompt: %w", err)
	}
	if err := c.SendLine(name); err != nil {
		return err
	}
	// The returning-character prompt is "Password:" (capital P); match
	// case-insensitively, as doLogin does.
	if _, err := c.Expect(regexp.MustCompile("(?i)password")); err != nil {
		return fmt.Errorf("password prompt: %w", err)
	}
	if err := c.SendLine(smokePassword); err != nil {
		return err
	}
	if _, err := c.ExpectTimeout(gamePrompt, 8*time.Second); err != nil {
		return fmt.Errorf("game prompt after login: %w", err)
	}
	return nil
}
