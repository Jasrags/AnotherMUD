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
func NewAbilityPhaseDriver(
	queue *ActionQueueManager,
	pipeline *ValidationPipeline,
	resolver *AbilityResolver,
	sources ResolutionSourceLookup,
	sink AbilitySink,
	overchannel OverchannelFunc,
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

		// §4.2 step 3: valid → resolve + drop + stop.
		d.resolver.Resolve(ctx, source, result.Ability, result.ResolvedTarget, currentPulse)
		// WoT S2: a deliberate overdraw resolved — exact the consequence
		// (Fortitude save + cascade) AFTER the weave's own effects landed and
		// the pool was spent. The deficit was captured pre-spend by validation.
		if result.Overchannel && d.overchannel != nil {
			d.overchannel(ctx, entityID, result.Ability.ID, result.OverchannelDeficit)
		}
		d.queue.Pop(entityID)
		return
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
