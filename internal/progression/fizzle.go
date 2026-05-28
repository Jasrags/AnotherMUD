package progression

// FizzleReason is the structured reason a queued ability invocation
// was rejected by the validation pipeline (spec abilities-and-effects
// §4.8). Values are lower-case keywords; clients SHOULD treat unknown
// reasons as opaque strings rather than failing.
//
// FizzleOK is the sentinel returned by ValidationPipeline.Validate
// when every check passed; it is NOT emitted on the bus.
type FizzleReason string

const (
	// FizzleOK signals that validation succeeded; no fizzle event
	// should be emitted. Distinct value from the spec's keyword
	// set so callers can branch on (== "" == ok).
	FizzleOK FizzleReason = ""

	// FizzleUnknownAbility — the queued ability id doesn't resolve
	// in the registry (spec §4.2 step 1, §4.8).
	FizzleUnknownAbility FizzleReason = "unknown_ability"
	// FizzleAsleep — entity is sleeping or resting (spec §4.3 step 1).
	FizzleAsleep FizzleReason = "asleep"
	// FizzleAlignmentRestricted — entity alignment outside the
	// ability's permitted range (spec §4.3 step 2).
	FizzleAlignmentRestricted FizzleReason = "alignment_restricted"
	// FizzleNoProficiency — entity hasn't learned the ability
	// (spec §4.3 step 3).
	FizzleNoProficiency FizzleReason = "no_proficiency"
	// FizzleEquipmentRequired — required slot empty or equipped
	// item lacks the required tag (spec §4.3 step 4).
	FizzleEquipmentRequired FizzleReason = "equipment_required"
	// FizzleInitiateOnly — initiate-only ability used while in
	// combat (spec §4.3 step 5).
	FizzleInitiateOnly FizzleReason = "initiate_only"
	// FizzleInvalidTarget — offensive ability had an explicit
	// target that doesn't resolve (spec §4.3 step 6, §4.4).
	FizzleInvalidTarget FizzleReason = "invalid_target"
	// FizzleNotInCombat — offensive ability invoked while the
	// source isn't in combat (spec §4.3 step 6).
	FizzleNotInCombat FizzleReason = "not_in_combat"
	// FizzleEffectPresent — ability would apply an effect the
	// source already carries (spec §4.3 step 7, §5.2).
	FizzleEffectPresent FizzleReason = "effect_present"
	// FizzlePulseDelay — per-entity cooldown hasn't expired
	// (spec §4.3 step 8).
	FizzlePulseDelay FizzleReason = "pulse_delay"
	// FizzleInsufficientResources — race-adjusted cost exceeds
	// the entity's pool (spec §4.3 step 9, §4.7).
	FizzleInsufficientResources FizzleReason = "insufficient_resources"
)

// IsOffensive reports whether ability is offensive per spec §4.6.
// An ability is offensive when:
//   - Its category is skill; OR
//   - Its category is spell AND it has no effect template AND its
//     metadata declares damage dice.
//
// M9.6b wired the damage-dice metadata (Ability.DamageDice), so a
// damage spell with no effect now classifies offensive. A heal spell
// (HealDice set, DamageDice empty) stays non-offensive even if it can
// target an enemy — the conservative branch never auto-routes a
// non-damage spell into the "must be in combat" check.
func IsOffensive(a *Ability) bool {
	if a == nil {
		return false
	}
	if a.Category == AbilitySkill {
		return true
	}
	// Spell branch (§4.6): offensive only when it has no effect
	// template AND declares damage dice. DamageDice is trimmed at
	// registration, so a bare != "" check is sufficient.
	return a.Effect == nil && a.DamageDice != ""
}

// ResourcePool names the entity-side resource pool an ability
// charges (spec §4.7). Skills draw movement; spells draw mana.
type ResourcePool string

const (
	// ResourceMovement is the per-entity stamina/movement pool.
	ResourceMovement ResourcePool = "movement"
	// ResourceMana is the per-entity spell pool.
	ResourceMana ResourcePool = "mana"
)

// ResourceFor returns the pool an ability charges (spec §4.7).
// Skills → movement, spells → mana. Defaults to movement when
// category is unrecognized so the validator can still apply the
// cost check rather than silently letting an exotic ability bypass
// the pool entirely.
func ResourceFor(a *Ability) ResourcePool {
	if a != nil && a.Category == AbilitySpell {
		return ResourceMana
	}
	return ResourceMovement
}
