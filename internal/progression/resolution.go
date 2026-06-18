package progression

import (
	"context"
	"strings"
)

// Roller is the source of randomness for ability resolution rolls
// (hit/miss in §4.5 step 4, proficiency gain in §3.5). It mirrors
// combat.Roller exactly — *math/rand/v2.Rand satisfies both via
// IntN — so the production wiring in cmd/anothermud can hand the
// resolver and the auto-attack phase the same seeded generator.
// progression depends on this local interface rather than importing
// combat (which would close progression → combat → progression once
// combat holds progression state).
//
// CONCURRENCY CONTRACT: identical to combat.Roller. Implementations
// are NOT required to be safe for concurrent use. The resolver runs
// serially inside the combat round's ability phase on the tick-loop
// goroutine, which is the single-goroutine guarantee math/rand/v2
// requires.
type Roller interface {
	// IntN returns a non-negative pseudo-random integer in [0, n).
	// Implementations MUST panic on n <= 0, matching
	// math/rand/v2.Rand.IntN.
	IntN(n int) int
}

// ResolutionSource is the per-entity mutation surface the resolver
// needs from the invoking entity during §4.5. It embeds
// ValidationEntity (so the resolver can re-read race for cost
// adjustment, alignment, etc. without a second seam) and adds the
// three mutating operations resolution performs on the source:
// resource deduction, last-ability recording, and a stat read for
// the proficiency-gain stat factor (§3.5 step 3).
//
// Players (session.connActor) implement this directly; mobs gain a
// thin adapter in M9.4b once the StatBlock-backed resource pools are
// wired.
type ResolutionSource interface {
	ValidationEntity

	// DeductMovement subtracts amount from the entity's movement
	// pool (spec §4.7, skills). amount is the already-race-adjusted
	// cost; the source MUST NOT re-apply the race multiplier.
	// Floors at zero — validation already guaranteed sufficiency,
	// but the floor keeps a mid-pulse pool change from underflowing.
	DeductMovement(amount int)

	// DeductMana subtracts amount from the entity's mana pool
	// (spec §4.7, spells). Same contract as DeductMovement.
	DeductMana(amount int)

	// SetLastAbility records the entity's "last ability used"
	// property (spec §4.5 step 2). Stored lower-cased by the
	// resolver before the call.
	SetLastAbility(abilityID string)

	// StatValue returns the entity's current effective value for
	// stat, used by the §3.5 step 3 gain stat factor. Unknown or
	// empty stat ⇒ 0 (the resolver then applies no stat factor).
	StatValue(stat StatType) int
}

// TargetHPLookup resolves an entity id to its current HP for the
// §4.5 step 9 post-resolution death check. The resolver consults it
// AFTER effect application so an effect (or a handler's damage, once
// M9.6 lands damage-bearing abilities) that drops the target to zero
// produces a vital-depleted emission. Second return is false when
// the target is gone (logged out / despawned) — the resolver then
// skips the death check.
//
// nil-safe: a resolver built without a lookup never emits
// vital-depleted, which is the correct default for M9.4a (no
// ability applies damage yet).
type TargetHPLookup interface {
	HP(entityID string) (int, bool)
}

// TargetHPLookupFunc adapts a closure to TargetHPLookup.
type TargetHPLookupFunc func(entityID string) (int, bool)

// HP implements TargetHPLookup.
func (f TargetHPLookupFunc) HP(entityID string) (int, bool) { return f(entityID) }

// ProficiencyMutator is the narrow mutation seam the resolver needs
// to apply a §3.5 proficiency gain. ProficiencyManager satisfies it
// via its existing AddProficiency method. Split from the broader
// manager surface so resolver tests can supply a tiny recording fake.
type ProficiencyMutator interface {
	// AddProficiency increments (entityID, abilityID) by delta,
	// clamped to the effective cap. The resolver always calls with
	// delta == 1 (spec §3.5 "incremented by one").
	AddProficiency(entityID, abilityID string, delta int)
}

// effectApplier is the narrow slice of EffectManager.Apply the
// resolver uses on a hit-with-effect (spec §4.5 step 7 / §5.1).
// *EffectManager satisfies it; the seam keeps resolver tests free of
// a full manager + resolver wiring when they only assert that Apply
// was called with the right template.
type effectApplier interface {
	Apply(ctx context.Context, entityID string, tpl EffectTemplate, sourceEntityID, sourceAbilityID string) bool
}

// ResolutionConfig is the host-side configuration for the resolver
// (spec §8 "engine configuration"). Construction-time only.
type ResolutionConfig struct {
	// DefaultMaxHitChance caps a rolled hit chance when the ability
	// declares no MaxHitChance of its own. Keeps even a
	// fully-proficient invocation fallible. <=0 or >100 falls back
	// to 100 (no cap). Spec §8 lists the hit-chance ceiling as
	// configurable.
	DefaultMaxHitChance int

	// SpendOnSuccess defers the resource spend to a successful hit
	// (WoT S2 / WoTMUD's channeling model): a missed or interrupted
	// invocation costs tempo, not Power. Default false = the historic
	// deduct-on-cast behavior (the resource is spent the moment the
	// ability resolves, miss or hit), which the fantasy ruleset keeps.
	// A channeling ruleset sets it true so a fizzled weave refunds.
	SpendOnSuccess bool
}

// DefaultResolutionConfig returns the engine defaults: no hit-chance
// ceiling beyond the natural 100. Content that wants a miss floor on
// high-proficiency invocations sets Variance + MaxHitChance per
// ability, or the host overrides DefaultMaxHitChance.
func DefaultResolutionConfig() ResolutionConfig {
	return ResolutionConfig{DefaultMaxHitChance: 100}
}

// AbilityResolver executes spec §4.5 resolution for a single
// validated invocation. It is the mutation half of the ability
// phase: the ValidationPipeline returns an OK result, then the
// resolver deducts the resource, records last-used + pulse-delay,
// rolls hit/miss, applies the effect on hit, and emits the
// used/missed/vital-depleted events. The per-pulse driver wiring
// (queue inspection + at-most-one-execution) lands in M9.4b.
//
// The resolver holds no per-entity state — every per-entity datum
// lives in the managers it is handed (proficiency, pulse-delay,
// effects). It is safe to share one resolver across the whole
// process; concurrency is bounded by the Roller contract (single
// goroutine).
type AbilityResolver struct {
	cfg         ResolutionConfig
	proficient  ProficiencyReader
	gainer      ProficiencyMutator
	pulseDelay  *PulseDelayTracker
	effects     effectApplier
	targetHP    TargetHPLookup
	sink        AbilitySink
	roller      Roller
	saves       SaveResolver    // optional; conditions §4 entry save (resist-on-apply)
	potency     PotencyFunc     // optional; setting-specific weave potency (WoT affinity)
	saveDCBonus SaveDCBonusFunc // optional; per-caster additive entry-save DC (special-weapons trip/disarm)
}

// SaveDCBonusFunc is the host's optional hook for an ADDITIVE entry-save DC
// adjustment that depends on the caster, not the ability content — e.g. a
// special-weapon maneuver where the wielded weapon raises the DC (a trip weapon
// trips harder; a disarm weapon disarms harder — special-weapons §4/§5). It
// returns the points to ADD to the ability's base ApplySave.DC for this
// (caster, ability) pair, applied AFTER the potency scale. Returns 0 (no
// adjustment) when the caster wields no matching weapon. nil-safe: with no hook
// the DC is the content value, leaving every existing maneuver unchanged.
type SaveDCBonusFunc func(sourceID, abilityID string) int

// SetSaveResolver wires the entry-save resolver an ability with an ApplySave
// rolls before installing its effect (conditions §4). The host's emitting
// implementation rolls combat.ResolveSave over the target's effective save
// bonus AND emits the SaveResolved event so the resist reads in-game.
// nil-safe: with no resolver an ApplySave ability always lands its effect
// (the pre-conditions behavior).
func (r *AbilityResolver) SetSaveResolver(s SaveResolver) { r.saves = s }

// SetPotencyProvider wires the host's optional weave-potency hook (WoT S2
// One-Power affinity). On a landed effect the resolver scales the entry-save
// DC, the effect's stat-modifier magnitudes, and its recurring-save DC by the
// returned factor. nil-safe: with no provider every weave lands at full
// potency — the setting-agnostic default that leaves fantasy packs unchanged.
// See PotencyFunc.
func (r *AbilityResolver) SetPotencyProvider(f PotencyFunc) { r.potency = f }

// SetSaveDCBonus wires the host's optional per-caster entry-save DC bonus
// (special-weapons §4/§5 — a trip/disarm weapon raises the maneuver's DC). The
// returned points are added to ApplySave.DC after the potency scale. nil-safe.
// See SaveDCBonusFunc.
func (r *AbilityResolver) SetSaveDCBonus(f SaveDCBonusFunc) { r.saveDCBonus = f }

// NewAbilityResolver builds a resolver. proficient + roller are
// required for hit/miss + gain rolls; the others are nil-safe:
//   - nil gainer skips proficiency gain (no AddProficiency call)
//   - nil pulseDelay skips the §4.5 step 3 cooldown record
//   - nil effects skips effect application (a hit-with-effect still
//     emits ability-used, just installs nothing)
//   - nil targetHP skips the §4.5 step 9 death check
//   - nil sink runs silently
//
// A nil roller is replaced by a panic-on-use guard only at the call
// site — callers MUST supply one; resolution is meaningless without
// a hit/miss source. We default it to alwaysHitRoller so a resolver
// built for an effect-only test (every Variance == 0) still works
// without injecting randomness.
func NewAbilityResolver(
	cfg ResolutionConfig,
	proficient ProficiencyReader,
	gainer ProficiencyMutator,
	pulseDelay *PulseDelayTracker,
	effects effectApplier,
	targetHP TargetHPLookup,
	sink AbilitySink,
	roller Roller,
) *AbilityResolver {
	if cfg.DefaultMaxHitChance <= 0 || cfg.DefaultMaxHitChance > 100 {
		cfg.DefaultMaxHitChance = 100
	}
	if roller == nil {
		roller = alwaysHitRoller{}
	}
	return &AbilityResolver{
		cfg:        cfg,
		proficient: proficient,
		gainer:     gainer,
		pulseDelay: pulseDelay,
		effects:    effects,
		targetHP:   targetHP,
		sink:       sink,
		roller:     roller,
	}
}

// ResolveOutcome is the structured result of one Resolve call. It is
// the resolver's report to the driver: whether the invocation hit,
// how much resource was deducted, whether an effect was installed,
// and whether the post-hit death check fired. The driver uses Hit
// only for logging today; the outcome exists so M9.4b's driver and
// future tests can assert resolution behavior without re-deriving it
// from sink emissions.
type ResolveOutcome struct {
	AbilityID     string
	Hit           bool
	ResourceSpent int
	EffectApplied bool
	// EffectResisted is true when the ability hit but the target made the
	// ability's entry save (conditions §4), so the effect was NOT applied.
	// EffectApplied is false in that case.
	EffectResisted bool
	TargetDepleted bool
	ResolvedTarget string
}

// Resolve executes spec §4.5 for source invoking ability against
// resolvedTarget (the §4.4 outcome the validation pass already
// computed; empty means self-targeted). currentPulse is the pulse
// number the round is resolving, used to record the pulse-delay
// expiration (§4.5 step 3).
//
// Resolve assumes validation already passed — it does NOT re-run the
// §4.3 checks. The driver MUST call ValidationPipeline.Validate
// first and only invoke Resolve on an OK result. Calling Resolve on
// a nil ability or nil source is a no-op returning the zero outcome.
func (r *AbilityResolver) Resolve(ctx context.Context, source ResolutionSource, ability *Ability, resolvedTarget string, currentPulse int64) ResolveOutcome {
	if source == nil || ability == nil {
		return ResolveOutcome{}
	}
	entityID := source.EntityID()
	// Defensive normalization: production ids arrive registry-lowered,
	// but a direct caller (test fake) could pass a mixed-case ID. Key
	// every seam (pulse-delay, proficiency, events) on the normalized
	// form so reads and writes stay consistent. DisplayName is left
	// untouched for rendering.
	abilityID := normalizeAbilityID(ability.ID)
	out := ResolveOutcome{AbilityID: abilityID, ResolvedTarget: resolvedTarget}

	// 1. Resource spend (race-adjusted, skipped when cost is zero). The
	// deduction is captured in a closure so the timing is config-driven:
	// the default deduct-on-cast model spends here (miss or hit), while the
	// SpendOnSuccess model defers the spend to a successful hit below — a
	// missed/interrupted invocation then costs tempo, not Power (WoT S2).
	var spendResource func()
	if ability.Cost > 0 {
		if cost := AdjustCost(ability.Cost, source.Race()); cost > 0 {
			resource := ResourceFor(ability)
			spendResource = func() {
				switch resource {
				case ResourceMana:
					source.DeductMana(cost)
				default:
					source.DeductMovement(cost)
				}
				out.ResourceSpent = cost
			}
		}
	}
	if spendResource != nil && !r.cfg.SpendOnSuccess {
		spendResource()
	}

	// 2. Record last-used. (Unlike the resource, last-used applies on a
	// miss in BOTH models — attempting the weave is what's recorded.)
	source.SetLastAbility(abilityID)

	// 3. Roll hit/miss. (Pulse delay is recorded AFTER the roll, on
	// the hit branch only — spec §4.5's narrative lists the record as
	// step 3, but the §4.8 acceptance criterion "pulse delay is
	// recorded on success, not on miss or fizzle" is authoritative.
	// In the default model the resource is already spent (step 1) so it
	// applies on a miss too; under SpendOnSuccess it is spent only on the
	// hit branch below. The cooldown is success-gated in both.)
	hit := r.rollHit(entityID, abilityID, ability)
	out.Hit = hit

	// 4. Resolve target — already computed by validation; passed in.
	targetName := resolvedTarget // M9.4a has no name registry; id doubles as name.

	// 5. On miss: emit, roll gain (failure multiplier), stop. No
	// pulse-delay record (spec §4.8 acceptance criterion).
	if !hit {
		if r.sink != nil {
			r.sink.OnAbilityMissed(ctx, AbilityMissedEvent{
				SourceID:    entityID,
				AbilityID:   abilityID,
				AbilityName: ability.DisplayName,
				TargetID:    resolvedTarget,
				TargetName:  targetName,
			})
		}
		r.rollGain(source, abilityID, ability, false)
		return out
	}

	// 6. On hit under the SpendOnSuccess model: NOW spend the resource (the
	// cast completed). Deduct-on-cast already spent it in step 1, so this is
	// a no-op there (spendResource is invoked exactly once across the two
	// models).
	if spendResource != nil && r.cfg.SpendOnSuccess {
		spendResource()
	}

	// 6b. On hit: record pulse delay (next-ready = currentPulse +
	// delay + 1, spec §4.5 step 3 / §4.8 success-gated).
	if ability.PulseDelay > 0 && r.pulseDelay != nil {
		r.pulseDelay.Record(entityID, abilityID, currentPulse+int64(ability.PulseDelay)+1)
	}

	// 7. On hit, with effect: roll the entry save (conditions §4) then build
	// + apply to the target. An ability with an ApplySave (a save-gated
	// condition like trip/bash) lets the target resist on a made save — the
	// effect is then NOT applied. The resolver emits SaveResolved (the
	// emitting host bridge) so the resist reads in-game. No ApplySave, or no
	// save resolver, ⇒ the effect always lands (the pre-conditions path).
	if ability.Effect != nil && r.effects != nil {
		target := resolvedTarget
		if target == "" {
			target = entityID // self-targeted buff
		}
		// WoT S2 Phase 4 — affinity potency on the effect path. A weave woven
		// outside the caster's affinity lands weaker: its entry-save DC drops
		// (easier to resist) and, once installed, its stat modifiers and
		// recurring-save DC are scaled down. nil provider (or a factor ≥ 1) ⇒
		// the pre-affinity behavior, so this is inert for fantasy. The same
		// host factor scales the damage/heal payload in the ability.used
		// handler — this seam covers the effect/DC half the resolver owns.
		potency := 1.0
		if r.potency != nil {
			potency = r.potency(entityID, abilityID)
		}
		resisted := false
		if ability.ApplySave != nil && r.saves != nil && target != entityID {
			// Only a hostile (non-self) effect is save-gated; a self-buff
			// with an ApplySave would otherwise let you resist your own buff.
			dc := scaleDC(ability.ApplySave.DC, potency)
			// special-weapons §4/§5: a trip/disarm weapon raises the maneuver's
			// DC by its bonus — additive, on top of the (affinity) potency scale.
			// nil hook ⇒ no change, so every non-weapon maneuver is unchanged. The
			// bonus is contracted non-negative (the loader validates trip_bonus /
			// disarm_bonus >= 0), so this only raises the floored DC; a future
			// SUBTRACTING hook would need to re-floor the sum at 1.
			if r.saveDCBonus != nil {
				dc += r.saveDCBonus(entityID, abilityID)
			}
			resisted = r.saves.ResolveSave(ctx, target, ability.ApplySave.Axis, dc, abilityID)
		}
		if resisted {
			out.EffectResisted = true
		} else {
			out.EffectApplied = r.effects.Apply(ctx, target, ability.Effect.scaledBy(potency), entityID, abilityID)
		}
	}

	// 8. On hit, always: emit ability-used, roll gain (success).
	if r.sink != nil {
		r.sink.OnAbilityUsed(ctx, AbilityUsedEvent{
			SourceID:     entityID,
			AbilityID:    abilityID,
			AbilityName:  ability.DisplayName,
			Category:     ability.Category,
			HandlerToken: ability.HandlerToken,
			TargetID:     resolvedTarget,
			TargetName:   targetName,
		})
	}
	r.rollGain(source, abilityID, ability, true)

	// 9. Post-hit death check. Only meaningful once an ability
	// applies damage (M9.6); today effects install stat modifiers
	// that don't reduce current HP, so this is emit-only plumbing
	// that stays dark in M9.4a. resolvedTarget == "" (self-cast)
	// never death-checks.
	if resolvedTarget != "" && r.targetHP != nil {
		if hp, ok := r.targetHP.HP(resolvedTarget); ok && hp <= 0 {
			out.TargetDepleted = true
			if r.sink != nil {
				r.sink.OnVitalDepleted(ctx, VitalDepletedEvent{
					VictimID: resolvedTarget,
					KillerID: entityID,
					Vital:    VitalHP,
				})
			}
		}
	}

	return out
}

// rollHit implements spec §4.5 step 4. Variance zero ⇒ always hits
// (no roll consumed). Otherwise chance = clamp(proficiency ×
// variance / 100, 1, ceiling) where ceiling is the ability's
// MaxHitChance or the configured default. The roll is IntN(100)+1
// (uniform 1..100); hit when roll ≤ chance.
//
// Luck scaling (spec §4.5 "luck × luckScale") is deferred: the
// ResolutionSource surface has no luck read yet, and spec §8 lists
// the luck scale factor as configuration that lands with the stat
// surface. Until then the formula is proficiency-only, which is the
// conservative subset.
func (r *AbilityResolver) rollHit(entityID, abilityID string, ability *Ability) bool {
	if ability.Variance <= 0 {
		return true
	}
	prof := r.proficiencyOf(entityID, abilityID)
	chance := prof * ability.Variance / 100
	ceiling := ability.MaxHitChance
	if ceiling <= 0 || ceiling > 100 {
		ceiling = r.cfg.DefaultMaxHitChance
	}
	if chance > ceiling {
		chance = ceiling
	}
	if chance < 1 {
		chance = 1
	}
	roll := r.roller.IntN(100) + 1
	return roll <= chance
}

// rollGain implements spec §3.5 proficiency gain. Chance starts from
// the ability's GainBaseChance, is scaled by (1 - prof/100) so gains
// taper toward the cap, optionally scaled by a stat factor, and
// scaled by the failure multiplier on a miss. A successful roll adds
// one proficiency point via the gainer. No gain when the ability has
// no base chance, the entity is already at its effective cap (the
// manager re-clamps so the AddProficiency is a no-op there), or no
// gainer is wired.
func (r *AbilityResolver) rollGain(source ResolutionSource, abilityID string, ability *Ability, hit bool) {
	if r.gainer == nil {
		return
	}
	entityID := source.EntityID()
	prof := r.proficiencyOf(entityID, abilityID)
	// step 3: stat factor when the ability declares a gain stat.
	statFactor := 1.0
	if ability.GainStat != "" && ability.GainStatScale != 0 {
		statFactor = 1 + float64(source.StatValue(ability.GainStat))*ability.GainStatScale
	}
	threshold := gainThreshold(
		ability.GainBaseChance, prof, r.effectiveCapOf(entityID, abilityID),
		statFactor, ability.GainFailureMultiplier, hit,
	)
	if threshold == 0 {
		return
	}
	if r.roller.IntN(100)+1 <= threshold {
		r.gainer.AddProficiency(entityID, abilityID, 1)
	}
}

// gainThreshold computes the spec §3.5 proficiency-gain probability as
// an integer 1..100 roll threshold (0 ⇒ no roll). Shared by the active
// resolver and the passive resolver so the taper / failure-multiplier
// / cap-guard semantics stay identical. statFactor is the §3.5 step-3
// multiplier (1 + statVal×scale); callers that omit the stat factor
// pass 1.0. Returns 0 when base is non-positive or proficiency has
// reached the effective cap (gain collapses to zero at min(cap,100)).
func gainThreshold(baseChance, prof, effectiveCap int, statFactor, failureMult float64, hit bool) int {
	if baseChance <= 0 || prof >= effectiveCap {
		return 0
	}
	chance := float64(baseChance) * (1 - float64(prof)/100) // step 2: taper.
	chance *= statFactor
	if !hit { // step 4: failure multiplier on a miss.
		mult := failureMult
		if mult <= 0 {
			mult = 1 // unset ⇒ miss gains at the same rate as hit.
		}
		chance *= mult
	}
	if chance <= 0 {
		return 0
	}
	threshold := int(chance)
	if threshold < 1 {
		threshold = 1 // a positive fractional chance still rolls.
	}
	if threshold > 100 {
		threshold = 100
	}
	return threshold
}

// proficiencyOf reads the source's current proficiency for abilityID
// through the ProficiencyReader when it also exposes the richer
// Proficiency accessor (ProficiencyManager does). A bare Has-only
// reader yields 0 — which floors the hit chance at 1% and lets gain
// roll at the full base rate, the conservative default for a fake
// that didn't bother wiring values.
func (r *AbilityResolver) proficiencyOf(entityID, abilityID string) int {
	return proficiencyValueOf(r.proficient, entityID, abilityID)
}

// effectiveCapOf returns min(cap, 100) for (entityID, abilityID).
func (r *AbilityResolver) effectiveCapOf(entityID, abilityID string) int {
	return effectiveCapValueOf(r.proficient, entityID, abilityID)
}

// proficiencyValueOf reads the current proficiency for (entityID,
// abilityID) through a ProficiencyReader that also exposes the richer
// Proficiency accessor (ProficiencyManager does). A bare Has-only
// reader yields 0 — the conservative default. Free function so both
// the active resolver and the passive resolver share one implementation.
func proficiencyValueOf(reader ProficiencyReader, entityID, abilityID string) int {
	if pr, ok := reader.(interface {
		Proficiency(entityID, abilityID string) (int, bool)
	}); ok {
		if v, known := pr.Proficiency(entityID, abilityID); known {
			return v
		}
	}
	return 0
}

// effectiveCapValueOf returns min(cap, 100) for (entityID, abilityID),
// read through the richer ProficiencyReader surface when it exposes
// Cap (ProficiencyManager does). A bare Has-only reader yields 100 —
// the global ceiling — so the §3.5 gain guard still fires at prof 100
// even without per-ability cap knowledge.
func effectiveCapValueOf(reader ProficiencyReader, entityID, abilityID string) int {
	capValue := 100
	if cr, ok := reader.(interface {
		Cap(entityID, abilityID string) int
	}); ok {
		capValue = cr.Cap(entityID, abilityID)
	}
	if capValue > 100 {
		capValue = 100
	}
	if capValue < 1 {
		capValue = 1
	}
	return capValue
}

// alwaysHitRoller is the resolver's default Roller — every IntN
// returns 0, which makes rollHit's `roll = 0+1 = 1 ≤ chance` always
// true (chance is floored at 1) and makes rollGain always fire when
// the threshold is ≥ 1. Used only when NewAbilityResolver is handed
// a nil roller (effect-only tests). Production always injects a real
// generator.
type alwaysHitRoller struct{}

func (alwaysHitRoller) IntN(n int) int {
	if n <= 0 {
		panic("progression: alwaysHitRoller.IntN n<=0")
	}
	return 0
}

// normalizeAbilityID is the resolver-side defensive lowercase used
// before any seam call that keys on ability id. Real paths arrive
// pre-normalized (registry-owned ids), but a test constructing an
// Ability with a mixed-case ID and calling Resolve directly would
// otherwise key the pulse-delay / proficiency maps inconsistently.
// Kept package-private; the registry's Register already lowercases
// on the production path.
func normalizeAbilityID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
