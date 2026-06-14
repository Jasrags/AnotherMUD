package progression

import (
	"strings"
)

// ValidationEntity is the per-entity surface ValidationPipeline
// reads during §4.3 checks. Players (session-side connActor) and
// mobs (M9.4 once StatBlock wiring lands) implement this. Each
// method is intentionally narrow so test fakes can supply only the
// fields a given check exercises.
//
// All methods MUST be cheap and safe for concurrent access — the
// pipeline calls them from the ability resolution phase on every
// pulse for every queueing entity.
type ValidationEntity interface {
	// EntityID returns the stable id the manager keys on.
	EntityID() string

	// IsResting reports whether the entity is asleep or in any
	// non-standing rest state (spec §4.3 step 1). Sleeping +
	// resting collapse to a single check here; renderers that
	// need to distinguish them do so elsewhere.
	IsResting() bool

	// Alignment returns the entity's current alignment value.
	// Read only when the ability declares HasAlignmentRange
	// (spec §4.3 step 2).
	Alignment() int

	// EquippedTags returns the tag list of the item equipped in
	// slot (spec §4.3 step 4). The second return is false when
	// the slot is empty; (nil, true) means "item equipped but
	// no tags". Tag matching is case-insensitive.
	//
	// The returned slice MAY alias internal state — callers MUST
	// treat it as read-only and MUST NOT retain it beyond the
	// current Validate call. The pipeline only iterates the slice
	// inside containsFold.
	EquippedTags(slot string) ([]string, bool)

	// InCombat reports whether the entity is engaged in combat
	// (spec §4.3 steps 5, 6).
	InCombat() bool

	// CurrentTarget returns the entity's current primary combat
	// target id (spec §4.4 step 2). Second return is false when
	// no target is set.
	CurrentTarget() (string, bool)

	// Movement / Mana return the current pool values used for
	// the resource check (spec §4.3 step 9, §4.7).
	Movement() int
	Mana() int

	// Race returns the entity's race for cost adjustment (spec
	// §4.7). nil yields an unadjusted cost — AdjustCost handles
	// the nil case.
	Race() *Race
}

// TargetLookup resolves an explicit target id to "does the target
// exist?". The validation pipeline uses this for §4.3 step 6 /
// §4.4 step 1 — explicit target id supplied but unresolvable
// fizzles as invalid_target. Implementations typically consult the
// entities.Store / session manager / mob store.
//
// Targeting an entity by id that resolves does NOT carry any
// liveness/visibility/room-shared check here; those are policy
// the host applies in TargetLookup itself.
type TargetLookup interface {
	// ResolveID reports whether targetID names a live entity.
	ResolveID(targetID string) bool
}

// TargetLookupFunc adapts a closure to TargetLookup.
type TargetLookupFunc func(targetID string) bool

// ResolveID implements TargetLookup.
func (f TargetLookupFunc) ResolveID(targetID string) bool { return f(targetID) }

// nopTargetLookup treats every id as unresolvable. The default when
// the pipeline is constructed without a lookup — offensive
// abilities with explicit targets then fizzle as invalid_target,
// which is the safe outcome for tests that don't wire a real entity
// store.
type nopTargetLookup struct{}

func (nopTargetLookup) ResolveID(string) bool { return false }

// ValidationPipeline orchestrates spec §4.3's nine ordered checks
// against an entity's queued ability invocation. Returns the first
// failing check as a FizzleReason; FizzleOK when every check passes.
//
// The pipeline is read-only — it never mutates the entity, the
// queue, the proficiency map, the pulse-delay tracker, or the
// effect manager. Resolution (M9.4) consumes the OK return and
// performs the actual mutation pass.
type ValidationPipeline struct {
	abilities  *AbilityRegistry
	proficient ProficiencyReader
	effects    EffectPresence
	pulseDelay PulseDelayReader
	targets    TargetLookup

	// reserveMultiple is the §Power "reserve-to-begin" gate (WoT S2 /
	// WoTMUD): a mana/spell ability requires the caller to HOLD this
	// multiple of its cost before it may begin, even though only the cost
	// itself is spent — a "you need headroom to channel safely" gate.
	// Defaults to 1 (the historic Mana() >= cost check); a channeling
	// ruleset sets 2 via SetReserveMultiple. Applies to the mana/spell
	// resource only — movement abilities keep the plain cost gate.
	reserveMultiple int
}

// SetReserveMultiple sets the spell-resource reserve-to-begin multiple
// (WoT S2). A value < 1 is clamped to 1 (the no-op default). Mirrors the
// resolver's SetSaveResolver: an optional knob set post-construction by the
// composition root from ruleset config, leaving the constructor unchanged.
func (p *ValidationPipeline) SetReserveMultiple(mult int) {
	if mult < 1 {
		mult = 1
	}
	p.reserveMultiple = mult
}

// ProficiencyReader is the read-only seam ValidationPipeline needs
// from a ProficiencyManager. Mirrors the AbilityProficiency seam
// in shape but strips the mutation surface so tests can hand the
// pipeline a tiny fake.
type ProficiencyReader interface {
	Has(entityID, abilityID string) bool
}

// EffectPresence is the read-only seam the pipeline needs from an
// EffectManager — spec §4.3 step 7 only asks "does the source
// already carry an active effect with this id?".
type EffectPresence interface {
	Has(entityID, effectID string) bool
}

// PulseDelayReader is the read-only seam the pipeline needs from a
// PulseDelayTracker.
type PulseDelayReader interface {
	IsCoolingDown(entityID, abilityID string, currentPulse int64) bool
}

// NewValidationPipeline constructs a pipeline. abilities is
// required. The other seams may be nil — a nil ProficiencyReader
// causes every ability to fail the proficiency check (safe
// default); a nil EffectPresence skips the effect-present check; a
// nil PulseDelayReader skips the pulse-delay check; a nil
// TargetLookup yields the unresolvable default (every explicit
// target fizzles as invalid_target).
func NewValidationPipeline(
	abilities *AbilityRegistry,
	proficient ProficiencyReader,
	effects EffectPresence,
	pulseDelay PulseDelayReader,
	targets TargetLookup,
) *ValidationPipeline {
	if targets == nil {
		targets = nopTargetLookup{}
	}
	return &ValidationPipeline{
		abilities:       abilities,
		proficient:      proficient,
		effects:         effects,
		pulseDelay:      pulseDelay,
		targets:         targets,
		reserveMultiple: 1, // no-op default; SetReserveMultiple opts in
	}
}

// ValidationResult is the structured outcome of one Validate call.
// Reason == FizzleOK means every check passed; Ability holds the
// resolved registry entry; ResolvedTarget holds the spec §4.4
// outcome (self / explicit / current-target). When Reason !=
// FizzleOK, Ability MAY still be non-nil (every reason after
// unknown_ability has the ability resolved) but ResolvedTarget is
// always empty on failure.
type ValidationResult struct {
	Reason         FizzleReason
	Ability        *Ability
	ResolvedTarget string
}

// Validate runs the §4.3 pipeline for one queued action invoked by
// source at currentPulse. The pipeline never mutates state.
func (p *ValidationPipeline) Validate(source ValidationEntity, action QueuedAction, currentPulse int64) ValidationResult {
	if source == nil {
		return ValidationResult{Reason: FizzleUnknownAbility}
	}
	abilityID := strings.ToLower(strings.TrimSpace(action.AbilityID))
	if abilityID == "" {
		return ValidationResult{Reason: FizzleUnknownAbility}
	}

	// Step 0 (spec §4.2 step 1): registry resolution.
	if p.abilities == nil {
		return ValidationResult{Reason: FizzleUnknownAbility}
	}
	ability, ok := p.abilities.Get(abilityID)
	if !ok {
		return ValidationResult{Reason: FizzleUnknownAbility}
	}

	entityID := source.EntityID()

	// 1. Rest-state.
	if source.IsResting() {
		return ValidationResult{Reason: FizzleAsleep, Ability: ability}
	}

	// 2. Alignment range.
	if ability.HasAlignmentRange {
		a := source.Alignment()
		if a < ability.AlignmentMin || a > ability.AlignmentMax {
			return ValidationResult{Reason: FizzleAlignmentRestricted, Ability: ability}
		}
	}

	// 3. Proficiency.
	if p.proficient == nil || !p.proficient.Has(entityID, ability.ID) {
		return ValidationResult{Reason: FizzleNoProficiency, Ability: ability}
	}

	// 4. Equipment slot + optional tag.
	if ability.EquipmentSlot != "" {
		tags, equipped := source.EquippedTags(ability.EquipmentSlot)
		if !equipped {
			return ValidationResult{Reason: FizzleEquipmentRequired, Ability: ability}
		}
		if ability.EquipmentTag != "" && !containsFold(tags, ability.EquipmentTag) {
			return ValidationResult{Reason: FizzleEquipmentRequired, Ability: ability}
		}
	}

	// 5. Initiate-only.
	if ability.InitiateOnly && source.InCombat() {
		return ValidationResult{Reason: FizzleInitiateOnly, Ability: ability}
	}

	// 6. Target validity (offensive only). Spec §4.3 step 6 allows
	// either `invalid_target` or `not_in_combat` as the reported
	// reason. We check in-combat FIRST for offensive abilities so a
	// player who hasn't engaged combat sees the "you're not in
	// combat" diagnostic rather than the more confusing "invalid
	// target" (which would be the consequence of an empty
	// CurrentTarget fallback in the non-in-combat case). Target
	// resolution still runs after, so explicit-target-unresolvable
	// fizzles as `invalid_target` when the entity IS in combat.
	if IsOffensive(ability) && !source.InCombat() {
		return ValidationResult{Reason: FizzleNotInCombat, Ability: ability}
	}
	resolvedTarget, targetReason := p.resolveTarget(source, ability, action)
	if targetReason != FizzleOK {
		return ValidationResult{Reason: targetReason, Ability: ability}
	}

	// 7. Effect-present (source already carries this effect's id).
	if ability.Effect != nil && p.effects != nil {
		effectID := strings.ToLower(strings.TrimSpace(ability.Effect.ID))
		if effectID != "" && p.effects.Has(entityID, effectID) {
			return ValidationResult{Reason: FizzleEffectPresent, Ability: ability}
		}
	}

	// 8. Pulse-delay cooldown.
	if ability.PulseDelay > 0 && p.pulseDelay != nil && p.pulseDelay.IsCoolingDown(entityID, ability.ID, currentPulse) {
		return ValidationResult{Reason: FizzlePulseDelay, Ability: ability}
	}

	// 9. Resource pool vs race-adjusted cost. The mana/spell resource adds
	// the reserve-to-begin gate (default multiple 1 = plain cost check):
	// a channeler must HOLD reserveMultiple × cost to start, though only
	// cost is spent. Movement abilities keep the plain cost gate.
	if ability.Cost > 0 {
		cost := AdjustCost(ability.Cost, source.Race())
		if cost > 0 {
			switch ResourceFor(ability) {
			case ResourceMana:
				mult := p.reserveMultiple
				if mult < 1 {
					mult = 1 // tolerate a zero-value pipeline (test fakes)
				}
				if source.Mana() < cost*mult {
					return ValidationResult{Reason: FizzleInsufficientResources, Ability: ability}
				}
			default:
				if source.Movement() < cost {
					return ValidationResult{Reason: FizzleInsufficientResources, Ability: ability}
				}
			}
		}
	}

	return ValidationResult{Reason: FizzleOK, Ability: ability, ResolvedTarget: resolvedTarget}
}

// resolveTarget implements spec §4.4 target resolution against the
// validation pipeline's read-only surface. Returns the resolved
// target id (empty for self-targeted resolution) and a fizzle
// reason; FizzleOK signals the target step passes.
//
// Resolution order:
//   1. Explicit action.TargetEntityID — must resolve via TargetLookup.
//      Unresolvable explicit id ⇒ FizzleInvalidTarget.
//   2. Offensive ability without explicit target ⇒ current combat
//      target; missing ⇒ FizzleInvalidTarget.
//   3. Self / buff ability ⇒ source entity id, no fizzle.
func (p *ValidationPipeline) resolveTarget(source ValidationEntity, ability *Ability, action QueuedAction) (string, FizzleReason) {
	if action.TargetEntityID != "" {
		if !p.targets.ResolveID(action.TargetEntityID) {
			return "", FizzleInvalidTarget
		}
		return action.TargetEntityID, FizzleOK
	}
	if IsOffensive(ability) {
		if t, ok := source.CurrentTarget(); ok && p.targets.ResolveID(t) {
			return t, FizzleOK
		}
		return "", FizzleInvalidTarget
	}
	return source.EntityID(), FizzleOK
}

// containsFold reports whether haystack contains target under a
// case-insensitive comparison. Used by the equipment-tag check.
func containsFold(haystack []string, target string) bool {
	t := strings.ToLower(strings.TrimSpace(target))
	for _, h := range haystack {
		if strings.ToLower(strings.TrimSpace(h)) == t {
			return true
		}
	}
	return false
}
