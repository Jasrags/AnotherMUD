package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
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
	"login-look":         scenarioLoginLook,
	"channeler-affinity": scenarioChannelerAffinity,
}

func scenarioNames() []string {
	names := make([]string, 0, len(scenarios))
	for n := range scenarios {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// scenarioLoginLook is the baseline example: log in (creating the character if
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

// scenarioChannelerAffinity is a regression check for WoT S2 Phase 3 (gender-
// derived affinity → soft potency scaling). It creates a FEMALE channeler
// (saidar: Fire is a weak Power), firebolts the wild boar, and asserts the
// weave's damage is pinned to 1 — the weak-affinity floor.
//
// REQUIRES the engine be booted for this assertion to hold:
//
//	ANOTHERMUD_PACKS=wot ANOTHERMUD_START_ROOM=wot:deep-westwood \
//	ANOTHERMUD_AFFINITY_WEAK_FACTOR=0.1 go run ./cmd/anothermud
//
// At weak factor 0.1, a Fire-weak firebolt (2d4 × 0.1, floored at 1) is always
// exactly 1; an UNPENALIZED firebolt is always ≥ 2 (2d4 min). So damage == 1 is
// a dice-proof signal that the affinity penalty fired. TestLive_ChannelerAffinity
// adds the male (Fire strong → ≥ 2) half for the full gender-direction proof.
func scenarioChannelerAffinity(c *telnettest.Client, name string) error {
	if err := createChanneler(c, name, "female"); err != nil {
		return err
	}
	dmg, err := fireboltBoarDamage(c)
	if err != nil {
		return err
	}
	if dmg != 1 {
		return fmt.Errorf("female channeler Firebolt (Fire is weak for saidar) dealt %d; want 1 — the weak-affinity floor at ANOTHERMUD_AFFINITY_WEAK_FACTOR=0.1 (an unpenalized firebolt is always >=2). Affinity scaling may have regressed, or the engine wasn't booted with the documented env", dmg)
	}
	return nil
}

// fireboltBoarDamage engages the wild boar and casts Firebolt at it, returning
// the weave's damage.
//
// Firebolt is a combat weave (it can only be cast while fighting) with a
// cast_time warmup, and the WoT S2 interrupt-on-hit mechanic disrupts a weave
// whenever the caster is struck mid-warmup. So a single in-melee cast is
// unreliable — the boar's swing often lands during the warmup and disrupts it
// (this is why the old single-shot flow began failing once weaves gained cast
// times). The boar misses often enough, though, that retrying yields an
// uninterrupted resolution; this loops until one cast lands (mirroring the
// interrupt smoke's retry structure, but succeeding on resolution rather than
// disruption). Firebolt is variance-0, so the damage line immediately follows
// the cast confirmation, before the next auto-attack swing.
func fireboltBoarDamage(c *telnettest.Client) (int, error) {
	if err := engageBoar(c); err != nil {
		return 0, err
	}
	damageRe := regexp.MustCompile(`hit a wild boar for (\d+) damage`)
	outcome := regexp.MustCompile(`cast Firebolt|is disrupted|isn't here|don't see|no longer|have to be fighting`)
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if err := c.SendLine("channel firebolt boar"); err != nil {
			return 0, err
		}
		out, err := c.ExpectTimeout(outcome, 8*time.Second)
		if err != nil {
			continue // no clear outcome in time; retry
		}
		switch {
		case strings.Contains(out, "cast Firebolt"):
			dmg, derr := c.ExpectTimeout(damageRe, 4*time.Second)
			if derr != nil {
				return 0, fmt.Errorf("Firebolt resolved but no damage line followed: %w", derr)
			}
			m := damageRe.FindStringSubmatch(dmg)
			n, perr := strconv.Atoi(m[1])
			if perr != nil {
				return 0, fmt.Errorf("parse damage from %q: %w", dmg, perr)
			}
			return n, nil
		case strings.Contains(out, "isn't here"), strings.Contains(out, "don't see"),
			strings.Contains(out, "no longer"), strings.Contains(out, "have to be fighting"):
			// Boar slain / fell out of combat — a fresh boar respawns on the area
			// reset; re-engage before the next attempt.
			_ = engageBoar(c)
		}
		// "is disrupted" (or an unmatched line) → just retry the cast.
	}
	return 0, fmt.Errorf("Firebolt never resolved uninterrupted against a wild boar within 45s")
}

// wardingACDelta weaves Warding (a self-buff that installs +ac/+hit modifiers)
// and returns how much the channeler's Armor Class rose. Warding is variance-0
// (always lands) and self-targeted (no save), so the delta is exactly the
// installed ac modifier — full (+2) at affinity, or affinity-scaled when woven
// outside the caster's strength. This is the WoT S2 Phase 4 proof for the
// EFFECT path (the firebolt test covers the damage path). Reads Armor Class off
// the score sheet before and after the weave.
func wardingACDelta(c *telnettest.Client) (int, error) {
	before, err := scoreArmorClass(c)
	if err != nil {
		return 0, fmt.Errorf("AC before warding: %w", err)
	}
	if err := c.SendLine("channel warding"); err != nil {
		return 0, err
	}
	if _, err := c.ExpectString("cast Warding"); err != nil {
		return 0, fmt.Errorf("Warding did not resolve: %w", err)
	}
	after, err := scoreArmorClass(c)
	if err != nil {
		return 0, fmt.Errorf("AC after warding: %w", err)
	}
	return after - before, nil
}

// armorClassRe pulls the signed Armor Class value off the score sheet.
var armorClassRe = regexp.MustCompile(`Armor Class[^\d-]*(-?\d+)`)

// scoreArmorClass sends `score` and parses the Armor Class value off the sheet.
func scoreArmorClass(c *telnettest.Client) (int, error) {
	if err := c.SendLine("score"); err != nil {
		return 0, err
	}
	out, err := c.Expect(armorClassRe)
	if err != nil {
		return 0, fmt.Errorf("no Armor Class line on score sheet: %w", err)
	}
	m := armorClassRe.FindStringSubmatch(out)
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("parse Armor Class from %q: %w", out, err)
	}
	return n, nil
}

// fortitudeRe pulls the signed Fortitude save value off the score sheet's
// "Saves  Fort +N  Ref +N  Will +N" line (saves §2; ANSI markup stripped by the
// telnet client).
var fortitudeRe = regexp.MustCompile(`Fort\s*([+-]?\d+)`)

// scoreFortitude sends `score` and parses the Fortitude save value off the sheet.
// Used by the Initiate/Wilder split test: the two channeling classes diverge on
// the Fortitude save curve (initiate weak, wilder strong), which is RNG-free at a
// fixed level + identical stats, so the value is a deterministic class fingerprint.
func scoreFortitude(c *telnettest.Client) (int, error) {
	if err := c.SendLine("score"); err != nil {
		return 0, err
	}
	out, err := c.Expect(fortitudeRe)
	if err != nil {
		return 0, fmt.Errorf("no Fortitude save on score sheet: %w", err)
	}
	m := fortitudeRe.FindStringSubmatch(out)
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("parse Fortitude from %q: %w", out, err)
	}
	return n, nil
}

// movementRe pulls the current/max movement pool off the score sheet's
// "MV cur/max" cell (the WoT S2 resource columns; ANSI markup stripped by the
// telnet client).
var movementRe = regexp.MustCompile(`MV\s*(\d+)/(\d+)`)

// scoreMovement sends `score` and parses the (current, max) movement pool.
func scoreMovement(c *telnettest.Client) (cur, max int, err error) {
	if err := c.SendLine("score"); err != nil {
		return 0, 0, err
	}
	out, err := c.Expect(movementRe)
	if err != nil {
		return 0, 0, fmt.Errorf("no MV cell on score sheet: %w", err)
	}
	m := movementRe.FindStringSubmatch(out)
	cur, err = strconv.Atoi(m[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse MV current from %q: %w", out, err)
	}
	max, err = strconv.Atoi(m[2])
	if err != nil {
		return 0, 0, fmt.Errorf("parse MV max from %q: %w", out, err)
	}
	return cur, max, nil
}

// engageBoar starts combat with the wild boar, polling until one is present.
// The Westwood spawns its boar on the area reset interval (~30s), not at boot,
// so a freshly-booted engine has an empty room for the first half-minute — the
// deadline covers that initial spawn with margin.
func engageBoar(c *telnettest.Client) error {
	deadline := time.Now().Add(45 * time.Second)
	marker := regexp.MustCompile(`You attack a wild boar|don't see|isn't here|not here`)
	var last string
	for time.Now().Before(deadline) {
		if err := c.SendLine("kill boar"); err != nil {
			return err
		}
		out, err := c.ExpectTimeout(marker, 3*time.Second)
		if err == nil && strings.Contains(out, "You attack") {
			return nil
		}
		last = strings.TrimSpace(out)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("could not engage a wild boar within 45s (last response: %q)", last)
}

// createAndLogin drives the login flow for name, creating the character with
// wizard defaults if it's new, and returns once the in-game prompt appears.
func createAndLogin(c *telnettest.Client, name string) error {
	// Account-first: `name` is both the account username and the character
	// name (the smoke harness keeps them the same).
	isNew, err := doLogin(c, name)
	if err != nil {
		return err
	}
	return finishLogin(c, name, isNew, nil)
}

// createChanneler is createAndLogin with explicit wizard answers: the given
// gender and the Initiate channeler class (everything else takes the first
// option). The Initiate/Wilder split (WoT S2) means there is no longer a single
// "channeler" class id; either channeling class gives the same One Power pool
// and weaves, so the affinity/cast/interrupt smokes pick Initiate arbitrarily.
func createChanneler(c *telnettest.Client, name, gender string) error {
	return createChannelerClass(c, name, gender, "initiate")
}

// createChannelerClass is createChanneler with an explicit channeling class id
// ("initiate" or "wilder") — used by the Initiate/Wilder split test, which must
// create one of each to compare their divergent save sheets.
func createChannelerClass(c *telnettest.Client, name, gender, class string) error {
	isNew, err := doLogin(c, name)
	if err != nil {
		return err
	}
	return finishLogin(c, name, isNew, map[string]string{"gender": gender, "class": class})
}

// doLogin authenticates (or creates) the ACCOUNT named `username` at the
// account-first front door (character-select.md). It writes the username,
// then drives the password branch: a new account chooses + confirms a
// password; an existing one just enters it. Returns true for a freshly
// created account (the caller then creates a character), false for an
// existing one (the caller selects from the roster).
func doLogin(c *telnettest.Client, username string) (bool, error) {
	if _, err := c.ExpectString("Account username:"); err != nil {
		return false, fmt.Errorf("username prompt: %w", err)
	}
	if err := c.SendLine(username); err != nil {
		return false, err
	}
	// A new account is asked to "Choose a password"; an existing one just
	// "Password:". Both contain "password"; the "choose" word disambiguates.
	out, err := c.Expect(regexp.MustCompile(`(?i)password`))
	if err != nil {
		return false, fmt.Errorf("post-username prompt: %w", err)
	}
	if regexp.MustCompile(`(?i)choose`).MatchString(out) {
		if err := c.SendLine(smokePassword); err != nil {
			return false, err
		}
		if _, err := c.ExpectString("Confirm"); err != nil {
			return false, fmt.Errorf("confirm-password prompt: %w", err)
		}
		if err := c.SendLine(smokePassword); err != nil {
			return false, err
		}
		return true, nil
	}
	// Existing account.
	if err := c.SendLine(smokePassword); err != nil {
		return false, err
	}
	return false, nil
}

// finishLogin completes an authenticated session: a new account creates its
// first character (character-name prompt → wizard with the given answers); an
// existing account selects `charName` from the roster. Leaves the connection
// at the in-game prompt. `charName` is used as the character name.
func finishLogin(c *telnettest.Client, charName string, isNew bool, answers map[string]string) error {
	if isNew {
		if _, err := c.ExpectString("new character's name"); err != nil {
			return fmt.Errorf("character-name prompt: %w", err)
		}
		if err := c.SendLine(charName); err != nil {
			return err
		}
		return runWizardWith(c, answers)
	}
	if _, err := c.Expect(regexp.MustCompile("Select a character")); err != nil {
		return fmt.Errorf("roster prompt: %w", err)
	}
	if err := c.SendLine(charName); err != nil {
		return err
	}
	if _, err := c.ExpectTimeout(gamePrompt, 8*time.Second); err != nil {
		return fmt.Errorf("login never reached the game prompt: %w", err)
	}
	return nil
}

// runWizardWith answers the character-creation wizard until the game prompt
// appears. For each "Choose your <field>" menu it sends answers[field] if
// present (e.g. "gender"→"female", "class"→"initiate"), else the first option;
// every "(yes/no)" confirm gets "yes". Being menu-shape-agnostic means it
// survives pack-specific wizard differences without edits.
func runWizardWith(c *telnettest.Client, answers map[string]string) error {
	step := regexp.MustCompile(`Choose your (\w+)|\(yes/no\)|` + gamePrompt.String())
	field := regexp.MustCompile(`Choose your (\w+)`)
	const maxSteps = 20 // generous guard against an unexpected loop
	for i := 0; i < maxSteps; i++ {
		out, err := c.ExpectTimeout(step, 8*time.Second)
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
		default:
			ans := "1"
			if m := field.FindStringSubmatch(out); m != nil {
				if a, ok := answers[strings.ToLower(m[1])]; ok {
					ans = a
				}
			}
			if err := c.SendLine(ans); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("creation wizard did not reach the game prompt within %d steps", maxSteps)
}
