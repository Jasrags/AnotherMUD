package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// AnotherMUD-specific helpers + scenarios. These live ONE LAYER ABOVE the
// generic telnettest core: they compose its Send/Expect primitives and encode
// engine knowledge (the login flow, the in-game prompt, the creation wizard).
// Adding a scenario is adding a function to the registry below — the core never
// changes.

// gamePrompt matches the in-game status prompt "[HP 20/20] ...> ". Reaching it
// is how a scenario knows login/creation finished and the world is live.
var gamePrompt = regexp.MustCompile(`\[HP \d+/\d+\]`)

// smokePassword is the password used for smoke-created characters. Fixed so a
// returning login (same name on a re-run against the same engine) still works.
const smokePassword = "smoketest-pw"

// scenarios is the named-scenario registry the binary and tests dispatch on.
var scenarios = map[string]func(*telnettest.Client, string) error{
	"login-look": scenarioLoginLook,
}

func scenarioNames() []string {
	names := make([]string, 0, len(scenarios))
	for n := range scenarios {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// scenarioLoginLook is the day-one example: log in (creating the character if
// it's new), then a `look` command round-trip, asserting the room's "Exits:"
// line comes back — proving both directions of the connection work end to end.
func scenarioLoginLook(c *telnettest.Client, name string) error {
	if err := createAndLogin(c, name); err != nil {
		return err
	}
	if err := c.SendLine("look"); err != nil {
		return err
	}
	if _, err := c.ExpectString("Exits:"); err != nil {
		return fmt.Errorf("look round-trip (no \"Exits:\" line came back): %w", err)
	}
	return nil
}

// createAndLogin drives the AnotherMUD login flow for name, transparently
// handling both a returning character (name → password) and a brand-new one
// (name → email → password → confirm → creation wizard). It returns once the
// in-game prompt appears.
func createAndLogin(c *telnettest.Client, name string) error {
	if _, err := c.ExpectString("name shall we know you"); err != nil {
		return fmt.Errorf("name prompt: %w", err)
	}
	if err := c.SendLine(name); err != nil {
		return err
	}
	// Returning characters are asked for a password; new ones for an email.
	out, err := c.Expect(regexp.MustCompile(`(?i)password|email`))
	if err != nil {
		return fmt.Errorf("post-name prompt (password|email): %w", err)
	}
	if strings.Contains(strings.ToLower(out), "email") {
		return createNewCharacter(c, name)
	}
	// Returning character.
	if err := c.SendLine(smokePassword); err != nil {
		return err
	}
	if _, err := c.ExpectTimeout(gamePrompt, 8*time.Second); err != nil {
		return fmt.Errorf("returning login never reached the game prompt: %w", err)
	}
	return nil
}

// createNewCharacter completes the new-account + creation-wizard path, picking
// up right after the "Email address:" prompt.
func createNewCharacter(c *telnettest.Client, name string) error {
	if err := c.SendLine(name + "@smoke.test"); err != nil {
		return err
	}
	if _, err := c.ExpectString("password"); err != nil { // "Choose a password for …:"
		return fmt.Errorf("choose-password prompt: %w", err)
	}
	if err := c.SendLine(smokePassword); err != nil {
		return err
	}
	if _, err := c.ExpectString("Confirm"); err != nil {
		return fmt.Errorf("confirm-password prompt: %w", err)
	}
	if err := c.SendLine(smokePassword); err != nil {
		return err
	}
	return runWizard(c)
}

// runWizard answers the character-creation wizard generically — every "Choose
// your X" menu gets the first option, every "(yes/no)" confirm gets "yes" —
// until the game prompt appears. Being menu-shape-agnostic means it survives
// pack-specific wizard differences (gender/race/class/background/…) without
// edits.
func runWizard(c *telnettest.Client) error {
	marker := regexp.MustCompile(`Choose your|\(yes/no\)|` + gamePrompt.String())
	const maxSteps = 20 // generous guard against an unexpected loop
	for i := 0; i < maxSteps; i++ {
		out, err := c.ExpectTimeout(marker, 8*time.Second)
		if err != nil {
			return fmt.Errorf("wizard step %d: %w", i, err)
		}
		switch {
		case gamePrompt.MatchString(out):
			return nil
		case strings.Contains(out, "(yes/no)"):
			if err := c.SendLine("yes"); err != nil {
				return err
			}
		case strings.Contains(out, "Choose your"):
			if err := c.SendLine("1"); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("creation wizard did not reach the game prompt within %d steps", maxSteps)
}
