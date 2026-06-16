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

// TestLive_MultipleCharactersPerAccount exercises the account-first roster
// (character-select.md): one account username holds several characters, a
// second character is created from the roster's "new" option, and each is
// then selectable from the roster.
//
// On a one-world-per-process server both characters are in the active world,
// so this proves the multi-character roster substrate; the cross-world gate
// (a greyed, unselectable character whose world isn't active) is covered by
// the login world-gate unit tests.
func TestLive_MultipleCharactersPerAccount(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, town-square
	const acct = "housetest"   // account username (shared by both characters)

	// 1. New account + its first character.
	c1, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial 1: %v", err)
	}
	if err := createAccountWithChar(c1, acct, "Aymara"); err != nil {
		c1.Close()
		t.Fatalf("create account + first character: %v", err)
	}
	c1.Close()

	// 2. Same account → roster → create a second character.
	c2, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
	if err != nil {
		t.Fatalf("dial 2: %v", err)
	}
	if err := addCharToAccount(c2, acct, "Bronn"); err != nil {
		c2.Close()
		t.Fatalf("create second character from roster: %v", err)
	}
	c2.Close()

	// 3. Both characters are selectable from the account's roster.
	for _, ch := range []string{"Aymara", "Bronn"} {
		c, err := telnettest.Dial(addr, telnettest.WithTimeout(20*time.Second))
		if err != nil {
			t.Fatalf("dial select %s: %v", ch, err)
		}
		if err := selectCharFromAccount(c, acct, ch); err != nil {
			c.Close()
			t.Fatalf("select %s from roster: %v", ch, err)
		}
		c.Close()
	}
}

// createAccountWithChar registers a new account and creates its first
// character (account-first new path).
func createAccountWithChar(c *telnettest.Client, username, charName string) error {
	isNew, err := doLogin(c, username)
	if err != nil {
		return err
	}
	if !isNew {
		return fmt.Errorf("expected a new account for %q", username)
	}
	return finishLogin(c, charName, true, nil)
}

// addCharToAccount logs into an existing account and creates an additional
// character via the roster's "new" option.
func addCharToAccount(c *telnettest.Client, username, charName string) error {
	isNew, err := doLogin(c, username)
	if err != nil {
		return err
	}
	if isNew {
		return fmt.Errorf("expected an existing account for %q", username)
	}
	if _, err := c.Expect(regexp.MustCompile("Select a character")); err != nil {
		return fmt.Errorf("roster prompt: %w", err)
	}
	if err := c.SendLine("n"); err != nil {
		return err
	}
	if _, err := c.ExpectString("new character's name"); err != nil {
		return fmt.Errorf("character-name prompt: %w", err)
	}
	if err := c.SendLine(charName); err != nil {
		return err
	}
	return runWizardWith(c, nil)
}

// selectCharFromAccount logs into an existing account and selects a
// character from the roster by name.
func selectCharFromAccount(c *telnettest.Client, username, charName string) error {
	isNew, err := doLogin(c, username)
	if err != nil {
		return err
	}
	if isNew {
		return fmt.Errorf("expected an existing account for %q", username)
	}
	return finishLogin(c, charName, false, nil)
}
