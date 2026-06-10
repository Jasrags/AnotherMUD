package combat

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Roller is the source of randomness for combat rolls. *math/rand/v2.Rand
// from the standard library satisfies it directly via its IntN method, so
// the production wiring passes a seeded *rand.Rand and tests inject a
// deterministic implementation. Combat depends on this interface rather
// than a concrete *rand.Rand so the per-package import surface stays at
// the seam between combat and its caller.
//
// CONCURRENCY CONTRACT: Roller implementations are NOT required to be
// safe for concurrent use. *math/rand/v2.Rand is explicitly documented
// as single-goroutine; combat callers MUST guarantee single-goroutine
// access. Today every consumer of Roller (the auto-attack phase, future
// damage-over-time and ability phases) runs serially inside
// Heartbeat.runPhase on the tick-loop goroutine, which is the
// guarantee. A future phase that wants to roll from a separate
// goroutine MUST either receive its own Roller or wrap a shared one
// in a sync.Mutex; the auto-attack-phase wiring in cmd/anothermud
// documents this contract at the call site.
type Roller interface {
	// IntN returns a non-negative pseudo-random integer in [0, n).
	// Implementations MUST panic on n <= 0, matching math/rand/v2.Rand.IntN.
	IntN(n int) int
}

// DiceExpr is a parsed NdM±K dice expression (combat §4.5). The zero
// value is invalid; callers MUST use ParseDice or compose explicitly
// (Count and Sides both > 0). The struct is immutable in practice —
// passed by value everywhere, no setters.
type DiceExpr struct {
	Count    int
	Sides    int
	Modifier int
}

var diceRe = regexp.MustCompile(`^(\d+)d(\d+)([+-]\d+)?$`)

const (
	// maxDiceCount caps the dice count to prevent a pathological mob
	// template (`9999d6`) from burning the round loop on a single roll.
	// 99 is generous — most fantasy dice expressions cap at 10d10.
	maxDiceCount = 99
	// maxDiceSides caps the per-die range similarly. d1000 has room to
	// grow without inviting a 32-bit overflow in the sum.
	maxDiceSides = 1000
)

// ParseDice parses an NdM±K expression. Whitespace around the
// expression is tolerated; whitespace inside is not. Returns a
// descriptive error on malformed input so content-pack loaders can
// surface the bad template by name rather than panicking at runtime.
func ParseDice(s string) (DiceExpr, error) {
	m := diceRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return DiceExpr{}, fmt.Errorf("combat.ParseDice %q: not in NdM[±K] form", s)
	}
	count, err := strconv.Atoi(m[1])
	if err != nil || count < 1 || count > maxDiceCount {
		return DiceExpr{}, fmt.Errorf("combat.ParseDice %q: count out of range [1,%d]", s, maxDiceCount)
	}
	sides, err := strconv.Atoi(m[2])
	if err != nil || sides < 2 || sides > maxDiceSides {
		return DiceExpr{}, fmt.Errorf("combat.ParseDice %q: sides out of range [2,%d]", s, maxDiceSides)
	}
	var mod int
	if m[3] != "" {
		mod, err = strconv.Atoi(m[3])
		if err != nil {
			return DiceExpr{}, fmt.Errorf("combat.ParseDice %q: modifier parse: %w", s, err)
		}
	}
	return DiceExpr{Count: count, Sides: sides, Modifier: mod}, nil
}

// Average returns the integer arithmetic mean of the dice expression
// (mobs-ai-spawning §3.2 mob class-growth derivation). For NdM±K the
// mean is `count*(sides+1)/2 + modifier` using integer division —
// 1d6 → 3, 2d6 → 7, 1d10+2 → 7. A zero expression returns 0.
//
// Used at mob spawn for class-bound stat growth: `Average() × level`
// is added to the relevant base stats once and stored on the
// StatBlock under srckey.ClassGrowth so the result is observable
// to combat and to renderers without re-deriving each tick.
func (d DiceExpr) Average() int {
	if d.Count <= 0 || d.Sides <= 0 {
		return 0
	}
	return d.Count*(d.Sides+1)/2 + d.Modifier
}

// Roll evaluates the expression using r. Each die contributes a value
// in [1, Sides]. The flat Modifier is added once at the end. A nil
// receiver-equivalent (zero DiceExpr) returns 0; callers that want a
// guaranteed-positive damage roll MUST clamp the result (combat §4.5
// "final damage MUST be at least 1 on a hit") — Roll is a pure math
// operation and does not invent the clamp.
func (d DiceExpr) Roll(r Roller) int {
	if d.Count <= 0 || d.Sides <= 0 {
		return 0
	}
	sum := d.Modifier
	for i := 0; i < d.Count; i++ {
		sum += r.IntN(d.Sides) + 1
	}
	return sum
}

// IsZero reports whether the expression is unconfigured (zero value).
// Used by Stats.EffectiveDamage to choose between the configured damage
// and the unarmed default.
func (d DiceExpr) IsZero() bool {
	return d.Count == 0 && d.Sides == 0 && d.Modifier == 0
}

// String renders back to NdM±K form. Symmetric with ParseDice so a
// round-trip through ParseDice(d.String()) returns the same expression
// for any non-zero DiceExpr.
func (d DiceExpr) String() string {
	switch {
	case d.Modifier == 0:
		return fmt.Sprintf("%dd%d", d.Count, d.Sides)
	case d.Modifier > 0:
		return fmt.Sprintf("%dd%d+%d", d.Count, d.Sides, d.Modifier)
	default:
		return fmt.Sprintf("%dd%d%d", d.Count, d.Sides, d.Modifier) // -K embeds the sign
	}
}

// defaultUnarmedDamage is the dice expression used when a combatant has
// no weapon-damage configured (combat §4.5 "default unarmed expression").
// 1d3 produces 1-3 damage before the STR bonus — small but never zero,
// so a no-weapon fight still progresses. Unexported because a mutable
// package-level Stats input on the hot round path is a global-test-
// state hazard; callers go through DefaultUnarmedDamage().
var defaultUnarmedDamage = DiceExpr{Count: 1, Sides: 3}

// DefaultUnarmedDamage returns the engine's unarmed damage expression.
// Returned by value (DiceExpr is a value type) so the package-level
// state cannot be mutated through the return.
func DefaultUnarmedDamage() DiceExpr { return defaultUnarmedDamage }

// DefaultCritMultiplier is the §4.5 "doubled dice" crit policy default:
// on a critical hit the rolled weapon/natural dice are multiplied by
// this factor (the STR bonus is not). A multiplier of 1 disables the
// bonus (crit = normal damage, the original M7.4 policy); the flag still
// flows on the hit event for renderers. Overridable via
// AutoAttackConfig.CritMultiplier (env ANOTHERMUD_CRIT_MULTIPLIER).
const DefaultCritMultiplier = 2

// DefaultNonProficientPenalty is the to-hit penalty applied when a
// character wields a weapon outside their class proficiency set
// (weapon-identity §3 — the WoT "−4 to attack" rule). Applied as a
// negative delta through the AutoAttackConfig.HitModAdjust seam (the same
// seam the darkness penalty uses). Overridable via env
// ANOTHERMUD_NONPROFICIENT_PENALTY.
const DefaultNonProficientPenalty = 4

// STRBonus maps a STR score to a damage modifier. Spec §4.5 leaves the
// scaling formula explicitly as policy ("scaled by an attacker stat
// (strength) modifier"); the M7.4 default is (STR-10)/2 with Go's
// truncation-toward-zero on negatives. M8 progression replaces this
// with a real derivation when ability-score tables exist.
//
// Truncation rather than floor was a conscious choice: every default
// combatant ships with STR 10, so the formula returns 0 for both
// players and mobs by default and balance is decided at the template
// level. Negative STR is unreachable from the default pipelines.
func STRBonus(str int) int {
	return (str - 10) / 2
}
