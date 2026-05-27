package combat

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// FleeOutcome explains what happened to a flee attempt. Three of the
// four values carry a matching bus event (see eventbus.Flee /
// FleePrevented / FleeFailed); FleeOutcomeNoCombatant is the
// "combatant wasn't even resolvable" branch and emits nothing — the
// caller has already filtered to live combatants in the heartbeat,
// so this is mostly a defensive return value for verb-driven flee.
type FleeOutcome int

const (
	FleeOutcomeNoCombatant FleeOutcome = iota
	FleeOutcomePrevented               // no-flee tag
	FleeOutcomeFailedNoExits
	FleeOutcomeFailedUnknownRoom
	FleeOutcomeSuccess
)

// Mover is the seam combat consumes to actually relocate a fleeing
// combatant. Implementations live outside combat (cmd/anothermud
// composition root for mobs + session connActors) so combat doesn't
// import session or entities for movement.
//
// Move MUST be safe to call from the tick goroutine (the heartbeat's
// wimpy phase runs there) and MUST handle a non-existent destination
// gracefully — return false rather than panic. A successful Move
// means the entity is now in dst; the caller's combat-side state has
// already been cleared via DisengageAll before Move runs.
type Mover interface {
	Move(ctx context.Context, id CombatantID, dst *world.Room) bool
}

// RoomSource is the seam combat consumes to look up a Room (not just
// a RoomID). Used by the flee primitive to enumerate exits and to
// hand a *world.Room to Mover. Defined here as a tiny interface so
// tests can substitute a map-backed stub instead of a full
// world.World.
type RoomSource interface {
	Room(id world.RoomID) (*world.Room, error)
}

// FleeConfig bundles the inputs the flee primitive needs beyond
// Manager. Every field is required at the production call site; tests
// substitute fakes.
//
// CooldownTicks is the duration a successful flee stamps on the
// FleeCooldowns tracker. Configured at boot from the same config
// surface combat-cadence and tick-interval live on.
//
// Rand returns a non-negative integer in [0, n). The standard
// math/rand/v2.Rand.IntN satisfies it. The heartbeat-tick goroutine
// is the sole caller in production, so concurrency is serialized;
// the Roller interface (used by auto-attack) documents the same
// contract and this is the same shape.
type FleeConfig struct {
	Mgr           *Manager
	Locator       Locator
	RoomLocator   RoomLocator
	Rooms         RoomSource
	Mover         Mover
	Sink          EventSink // currently used for DisengageAll's CombatEnded
	Bus           FleeBus
	Cooldowns     *FleeCooldowns
	Tags          TagSource
	Rand          Roller
	CooldownTicks uint64
}

// FleeBus is the narrow surface flee uses for the three §5.2 bus
// emissions. Separated from a full eventbus.Bus so the combat package
// doesn't import eventbus — cmd/anothermud supplies an adapter that
// translates these calls into bus.Publish.
type FleeBus interface {
	EmitFlee(ctx context.Context, entityID CombatantID, entityName string, from, to world.RoomID, direction string)
	EmitFleePrevented(ctx context.Context, entityID CombatantID, entityName string, room world.RoomID)
	EmitFleeFailed(ctx context.Context, entityID CombatantID, entityName string, room world.RoomID, reason string)
}

// nopFleeBus is the placeholder used when FleeConfig.Bus is nil.
// Mirrors nopSink for EventSink.
type nopFleeBus struct{}

func (nopFleeBus) EmitFlee(context.Context, CombatantID, string, world.RoomID, world.RoomID, string) {
}
func (nopFleeBus) EmitFleePrevented(context.Context, CombatantID, string, world.RoomID) {}
func (nopFleeBus) EmitFleeFailed(context.Context, CombatantID, string, world.RoomID, string) {
}

// Reason constants on FleeFailed events. Duplicated here (rather
// than importing eventbus) so the combat package stays free of the
// eventbus dependency. Production sink translates these to the
// matching eventbus.FleeFailed* constants.
const (
	FleeReasonNoExits     = "no-exits"
	FleeReasonUnknownRoom = "unknown-room"
)

// Flee executes the spec §5.2 flee attempt for c against cfg. Returns
// the outcome so verb-driven flee can render a player-facing message.
//
// Sequence (matches §5.2 1-3 verbatim):
//
//  1. Resolve combatant (locator). Missing → FleeOutcomeNoCombatant,
//     no events.
//  2. Resolve room (room locator + room source). Missing →
//     FleeOutcomeFailedUnknownRoom + flee_failed event.
//  3. no-flee tag check → FleeOutcomePrevented + flee_prevented
//     event.
//  4. Enumerate exits. None → FleeOutcomeFailedNoExits + flee_failed
//     event.
//  5. Pick a uniformly random exit; resolve destination room. If the
//     destination doesn't resolve, treat as unknown-room (the spec
//     says "If the move fails, the spec does not define the
//     outcome"; we choose flee_failed/unknown-room over a silent
//     no-op so operators see evidence).
//  6. DisengageAll (clears combat-side state on both sides BEFORE the
//     move so combat-ended events emit with the from-room, not the
//     post-move to-room).
//  7. Move via the supplied Mover.
//  8. Stamp the cooldown via FleeCooldowns.Start (if configured).
//  9. Emit the flee event.
//
// Ordering note: cooldown stamping happens AFTER the move so a
// failed move doesn't burn the cooldown timer. Mover failure here
// is unspecified by the spec; today the production Mover never
// fails for a valid room, but the defensive ordering means a future
// Mover that gates on (e.g.) closed doors degrades cleanly.
func Flee(ctx context.Context, c CombatantID, cfg FleeConfig) FleeOutcome {
	bus := cfg.Bus
	if bus == nil {
		bus = nopFleeBus{}
	}

	cb, ok := cfg.Locator.LookupCombatant(c)
	if !ok {
		return FleeOutcomeNoCombatant
	}
	entityName := cb.Name()

	roomID, hasRoom := cfg.RoomLocator.RoomOf(c)
	if !hasRoom {
		bus.EmitFleeFailed(ctx, c, entityName, "", FleeReasonUnknownRoom)
		return FleeOutcomeFailedUnknownRoom
	}
	room, err := cfg.Rooms.Room(roomID)
	if err != nil || room == nil {
		bus.EmitFleeFailed(ctx, c, entityName, roomID, FleeReasonUnknownRoom)
		return FleeOutcomeFailedUnknownRoom
	}

	if cfg.Tags != nil && cfg.Tags.EntityHasTag(c, TagNoFlee) {
		bus.EmitFleePrevented(ctx, c, entityName, roomID)
		return FleeOutcomePrevented
	}

	// Snapshot exits into a deterministic slice — map iteration is
	// non-deterministic and the §5.2 "uniformly random" pick must be
	// reproducible under a deterministic Roller in tests.
	dirs := exitDirections(room)
	if len(dirs) == 0 {
		bus.EmitFleeFailed(ctx, c, entityName, roomID, FleeReasonNoExits)
		return FleeOutcomeFailedNoExits
	}

	idx := 0
	if len(dirs) > 1 && cfg.Rand != nil {
		idx = cfg.Rand.IntN(len(dirs))
	}
	dir := dirs[idx]
	exit := room.Exits[dir]
	dst, err := cfg.Rooms.Room(exit.Target)
	if err != nil || dst == nil {
		bus.EmitFleeFailed(ctx, c, entityName, roomID, FleeReasonUnknownRoom)
		return FleeOutcomeFailedUnknownRoom
	}

	// §2.3 DisengageAll BEFORE move so CombatEnded payloads carry the
	// from-room (spec §6.3 step 3 ordering for death analogously
	// applies — disengage in the room the combat was happening in).
	if cfg.Mgr != nil {
		cfg.Mgr.DisengageAll(ctx, c, roomID)
	}

	if cfg.Mover != nil {
		if !cfg.Mover.Move(ctx, c, dst) {
			// Move refused — treat as unknown-room rather than silent
			// no-op so operators see evidence. The DisengageAll above
			// already ran, so the entity is now out of combat but
			// stuck in the original room. Acceptable: spec §5.2
			// explicitly says "If the move fails, the spec does not
			// define the outcome and the world's movement rules
			// apply." Stuck-out-of-combat is a movement-rule outcome.
			bus.EmitFleeFailed(ctx, c, entityName, roomID, FleeReasonUnknownRoom)
			return FleeOutcomeFailedUnknownRoom
		}
	}

	if cfg.Cooldowns != nil && cfg.CooldownTicks > 0 {
		cfg.Cooldowns.Start(c, cfg.CooldownTicks)
	}

	bus.EmitFlee(ctx, c, entityName, roomID, dst.ID, dir.Long())
	return FleeOutcomeSuccess
}

// exitDirections returns the room's exits in a stable, deterministic
// order so a uniform-random pick over the slice is reproducible.
// Sort key is the canonical Long() direction name so the order is
// alphabetical (east, north, south, west, ...) regardless of how the
// underlying Direction enum is numbered.
func exitDirections(r *world.Room) []world.Direction {
	if r == nil || len(r.Exits) == 0 {
		return nil
	}
	out := make([]world.Direction, 0, len(r.Exits))
	for d := range r.Exits {
		out = append(out, d)
	}
	// Insertion sort by Long() — len is at most ~8 directions, so
	// the sort package import is overkill.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].Long() > out[j].Long(); j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
