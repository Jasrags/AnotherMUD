package progression

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// ResolutionSourceLookup maps a combat CombatantID to the
// ResolutionSource that owns the matching action queue. The host
// supplies it: players resolve through the session manager, mobs
// through the mob store. The second return is false when the
// combatant has no ability-capable source (logged out, despawned,
// or a combatant kind that can't queue abilities) — the driver then
// skips that combatant for this pulse.
//
// The combatantID is passed as the raw combat string (e.g.
// "player:alice", "mob:wolf-3"); the lookup is responsible for the
// prefix-to-entity translation. The returned source's EntityID()
// MUST be the same id the action queue / proficiency / pulse-delay
// managers key on (no prefix), so the driver can use it uniformly.
type ResolutionSourceLookup interface {
	ResolveSource(combatantID string) (ResolutionSource, bool)
}

// ResolutionSourceLookupFunc adapts a closure to the interface.
type ResolutionSourceLookupFunc func(combatantID string) (ResolutionSource, bool)

// ResolveSource implements ResolutionSourceLookup.
func (f ResolutionSourceLookupFunc) ResolveSource(combatantID string) (ResolutionSource, bool) {
	return f(combatantID)
}

// AbilityPhaseDriver implements spec abilities-and-effects §4.2
// per-pulse processing as a combat.PhaseFunc. The combat heartbeat
// invokes the returned closure once per combatant per round (the
// ability phase, first in §3 order). For each combatant the driver
// inspects the front of its action queue and:
//
//   - empty queue ⇒ nothing to do (return).
//   - front entry fails validation (§4.3, including the
//     unknown-ability case §4.2 step 1) ⇒ emit a fizzle carrying the
//     lower-case reason keyword, drop the entry, and CONTINUE to the
//     next entry. Invalid entries do NOT consume the pulse's single
//     execution slot.
//   - front entry passes validation ⇒ resolve it (§4.5), drop the
//     entry, and STOP. At most one valid execution per entity per
//     pulse.
//
// The loop terminates because every iteration Pops exactly one entry,
// so the queue strictly shrinks; it is additionally bounded by the
// queue's configured depth limit.
type AbilityPhaseDriver struct {
	queue       *ActionQueueManager
	pipeline    *ValidationPipeline
	resolver    *AbilityResolver
	sources     ResolutionSourceLookup
	sink        AbilitySink
	overchannel OverchannelFunc
	casts       *CastTracker // optional; WoT S2 timed-cast warmups (nil ⇒ all instant)
	notifier    CastNotifier // optional; cast-lifecycle messaging
}

// OverchannelFunc is the host hook invoked after a deliberately overdrawn
// weave resolves (WoT S2). entityID is the caster (no combat prefix),
// abilityID the resolved weave, and deficit how far below the safe reserve
// the caster was (ValidationResult.OverchannelDeficit). The host exacts the
// consequence — a Fortitude save scaled by deficit, with a condition cascade
// on failure. It runs on the tick goroutine, in the driver's serial loop,
// AFTER the weave's own effects resolved and the pool was spent. nil disables
// the consequence (a non-channeling ruleset never flags an action).
type OverchannelFunc func(ctx context.Context, entityID, abilityID string, deficit int)

// NewAbilityPhaseDriver builds the driver and returns it as a
// combat.PhaseFunc ready to register as combat.Phases.Ability.
//
// queue, pipeline, resolver, and sources are required — a nil any of
// them makes the phase meaningless, so we panic at construction
// (mirrors combat.NewAutoAttack's fail-fast) rather than nil-deref on
// the first pulse. sink is nil-safe: a driver built without one
// resolves silently (no fizzle emission), which the resolver's own
// nil-sink handling already permits for the used/missed/depleted
// events.
//
// CONCURRENCY: the returned closure runs serially inside
// Heartbeat.runPhase on the tick-loop goroutine — the same
// single-goroutine guarantee the resolver's Roller contract relies
// on. The managers it touches (queue, pulse-delay, proficiency,
// effects) each carry their own locks, so a verb handler pushing to
// the queue from a connection goroutine is safe against the driver
// popping from it.
// casts and notifier are optional (WoT S2 — the channel interrupt game): a nil
// casts tracker disables timed warmups entirely, so every ability resolves in
// the pulse it validates (the pre-S2 behavior every non-channeling ruleset
// keeps); a nil notifier just suppresses the begin/interrupt messaging.
func NewAbilityPhaseDriver(
	queue *ActionQueueManager,
	pipeline *ValidationPipeline,
	resolver *AbilityResolver,
	sources ResolutionSourceLookup,
	sink AbilitySink,
	overchannel OverchannelFunc,
	casts *CastTracker,
	notifier CastNotifier,
) combat.PhaseFunc {
	if queue == nil {
		panic("progression.NewAbilityPhaseDriver: nil queue")
	}
	if pipeline == nil {
		panic("progression.NewAbilityPhaseDriver: nil pipeline")
	}
	if resolver == nil {
		panic("progression.NewAbilityPhaseDriver: nil resolver")
	}
	if sources == nil {
		panic("progression.NewAbilityPhaseDriver: nil sources")
	}
	d := &AbilityPhaseDriver{
		queue:       queue,
		pipeline:    pipeline,
		resolver:    resolver,
		sources:     sources,
		sink:        sink,
		overchannel: overchannel,
		casts:       casts,
		notifier:    notifier,
	}
	return d.run
}

// run is the combat.PhaseFunc. mgr is unused today — target
// resolution flows through the ValidationEntity's CurrentTarget()
// (which the source reads from combat itself), not through the
// manager directly. The parameter is retained to satisfy the
// PhaseFunc shape and so a future §4.4 refinement that needs the
// manager (e.g. validating the target is still in the same combat)
// has it on hand.
func (d *AbilityPhaseDriver) run(ctx context.Context, combatantID combat.CombatantID, _ *combat.Manager, pulse uint64) {
	source, ok := d.sources.ResolveSource(string(combatantID))
	if !ok {
		return
	}
	entityID := source.EntityID()
	// pulse is the round's tick count; the int64 narrowing is safe
	// (overflow is ~29 billion years at 100ms/tick).
	currentPulse := int64(pulse)

	// WoT S2 — the channel interrupt game. If this entity has a timed weave
	// in flight, advance its warmup by one round instead of touching the
	// queue. Still warming up ⇒ done for this round (the cast occupies the
	// caster). Warmup elapsed ⇒ resolve it now. Advance is one locked op, so
	// it can't race a concurrent Interrupt (a hit on the caster) — whichever
	// wins, the cast is consumed exactly once.
	if d.casts != nil {
		if cast, ready, active := d.casts.Advance(entityID); active {
			if ready {
				d.resolveCast(ctx, source, cast, currentPulse)
			}
			return
		}
	}

	// Peek-then-Pop is intentionally not atomic. It is safe because
	// the driver is the SOLE consumer of an entity's queue (only the
	// ability phase Pops) and Push only ever appends to the back, so
	// the front entry the driver Peeked, Validated, and then Pops is
	// always the same entry — a concurrent verb-goroutine Push can
	// add behind it but cannot displace or reorder it. When the M9.6
	// enqueue verb lands this invariant still holds; if a second
	// consumer is ever added, this loop must take the queue lock
	// across Peek→Pop.
	for {
		action, ok := d.queue.Peek(entityID)
		if !ok {
			return // empty queue — nothing queued this pulse.
		}

		result := d.pipeline.Validate(source, action, currentPulse)
		if result.Reason != FizzleOK {
			// §4.2 step 1/2: invalid entry → fizzle + drop + continue.
			// The slot is NOT consumed; the loop tries the next entry.
			d.emitFizzle(ctx, entityID, action, result)
			d.queue.Pop(entityID)
			continue
		}

		// WoT S2: a weave with a warmup does not resolve now — it BEGINS a
		// timed cast that the Advance branch above resolves CastTime rounds
		// later (if it survives). The queue entry is consumed (popped) at
		// begin; the in-flight state lives in the cast tracker, where a hit
		// can interrupt it. Validation has already passed (reserve/cost/target
		// checked at begin), and it runs again at resolve so a target that
		// dies or a caster stilled mid-cast collapses cleanly.
		if d.casts != nil && result.Ability.CastTime > 0 {
			d.casts.Begin(entityID, Cast{
				AbilityID:          result.Ability.ID,
				AbilityName:        result.Ability.DisplayName,
				TargetEntityID:     action.TargetEntityID,
				Overchannel:        action.Overchannel,
				OverchannelDeficit: result.OverchannelDeficit, // lock the begin-time reach
				Remaining:          result.Ability.CastTime,
			})
			if d.notifier != nil {
				d.notifier.OnCastBegan(ctx, CastBeganEvent{
					SourceID:       entityID,
					AbilityID:      result.Ability.ID,
					AbilityName:    result.Ability.DisplayName,
					TargetEntityID: result.ResolvedTarget,
					Rounds:         result.Ability.CastTime,
				})
			}
			d.queue.Pop(entityID)
			return
		}

		// §4.2 step 3: instant ability → resolve + drop + stop.
		d.resolveValidated(ctx, source, result, currentPulse)
		d.queue.Pop(entityID)
		return
	}
}

// resolveCast resolves a timed weave whose warmup has elapsed. It RE-validates
// the stored action first (so a weave whose target died, or whose caster was
// stilled or drained mid-warmup, collapses into a fizzle rather than resolving
// against stale state) and then resolves it through the shared path. The queue
// entry was already popped at begin, so there is nothing to pop here.
// The stored cast.TargetEntityID is re-resolved here (not snapshotted): a
// weave fires at whoever the caster is fighting WHEN it completes, so retargeting
// during a warmup redirects an offensive weave rather than wasting it. A
// self-buff has no target to drift.
func (d *AbilityPhaseDriver) resolveCast(ctx context.Context, source ResolutionSource, cast Cast, currentPulse int64) {
	action := QueuedAction{
		AbilityID:      cast.AbilityID,
		TargetEntityID: cast.TargetEntityID,
		Overchannel:    cast.Overchannel,
	}
	result := d.pipeline.Validate(source, action, currentPulse)
	// Re-validation collapses the weave on any failure that arose during the
	// warmup — target gone, caster stilled, Power drained. The recovery
	// cooldown (FizzlePulseDelay) is NOT a concern here: it is recorded only at
	// a PRIOR cast's resolution and, measured in pulses (≈ delay+1), always
	// expires within one combat round (which advances the pulse by the combat
	// cadence), so an in-flight weave cannot trip its own ability's cooldown.
	// (A cadence faster than the cooldown would break that — call it a content
	// constraint, not a runtime guard.)
	if result.Reason != FizzleOK {
		d.emitFizzle(ctx, source.EntityID(), action, result)
		return
	}
	// Lock the overchannel consequence to what the player committed at begin —
	// mana may have regenerated across the warmup, which would otherwise soften
	// (or under spend-on-success, with a re-derived deficit, mis-scale) the
	// Fortitude DC. Deficit > 0 ⇔ it was a genuine overchannel.
	result.OverchannelDeficit = cast.OverchannelDeficit
	result.Overchannel = cast.OverchannelDeficit > 0
	d.resolveValidated(ctx, source, result, currentPulse)
}

// resolveValidated resolves an already-validated action through the resolver
// and exacts the overchannel consequence when applicable. Shared by the instant
// path and the timed-cast completion path.
//
// WoT S2: a deliberate overdraw exacts the consequence (Fortitude save +
// cascade) AFTER the weave's own effects landed — but ONLY when the Power was
// actually drawn past the reserve (ResourceSpent > 0). That keeps the two spend
// models coherent: under deduct-on-cast a miss still drew the Power (risk
// applies), while under spend-on-success a missed weave drew nothing — it cost
// tempo, not Power, so it carries no stilling risk. The deficit was captured
// pre-spend by validation.
func (d *AbilityPhaseDriver) resolveValidated(ctx context.Context, source ResolutionSource, result ValidationResult, currentPulse int64) {
	outcome := d.resolver.Resolve(ctx, source, result.Ability, result.ResolvedTarget, currentPulse)
	if result.Overchannel && d.overchannel != nil && outcome.ResourceSpent > 0 {
		d.overchannel(ctx, source.EntityID(), result.Ability.ID, result.OverchannelDeficit)
	}
}

// emitFizzle publishes the §4.8 fizzle event for a dropped invalid
// entry. AbilityName is the registered display name when the ability
// resolved (every reason after unknown_ability has result.Ability
// set); it falls back to the raw queued id for the unknown-ability
// case so the renderer still has something to show.
func (d *AbilityPhaseDriver) emitFizzle(ctx context.Context, entityID string, action QueuedAction, result ValidationResult) {
	if d.sink == nil {
		return
	}
	abilityID := action.AbilityID
	abilityName := abilityID
	if result.Ability != nil {
		abilityID = result.Ability.ID
		abilityName = result.Ability.DisplayName
	}
	d.sink.OnAbilityFizzled(ctx, AbilityFizzledEvent{
		SourceID:    entityID,
		AbilityID:   abilityID,
		AbilityName: abilityName,
		Reason:      result.Reason,
	})
}
