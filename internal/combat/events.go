package combat

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Combat publishes through an EventSink rather than directly through
// eventbus.Bus, because eventbus imports entities and entities imports
// combat (MobInstance carries Vitals / Stats fields). A combat →
// eventbus edge would close that cycle. EventSink is the indirection
// that keeps combat free of the eventbus dependency; cmd/anothermud
// wires a real sink that publishes to the bus when a subscriber
// actually needs the events (M7.5 for mob.killed → spawn untrack,
// M7.6 for combat.ended → flee-cooldown clear).
//
// In M7.2 there is no engine subscriber yet — the production sink can
// log-only, and tests use a recording sink to assert the right
// payloads emit.

// EventSink consumes combat events. Implementations live outside the
// combat package. All methods MUST tolerate concurrent calls from the
// Manager's mutation path. Implementations MUST NOT call back into
// the Manager from a handler — combat publishes after releasing its
// mutex, so re-entrant Engage / Disengage would not deadlock, but the
// resulting causal chain (engage → handler → engage → handler) is
// undefined and easy to make recursive.
type EventSink interface {
	OnEngagement(ctx context.Context, e Engagement)
	OnCombatEnded(ctx context.Context, e CombatEnded)
}

// Engagement is dispatched after both sides of an Engage have been
// inserted into each other's combat lists (spec combat §2.1 step 3).
// Symmetric — one Engagement event per Engage call, not one per side.
//
// AttackerID / TargetID carry the pre-engagement roles. RoomID is the
// shared room at engagement time; combat refuses cross-room engages
// in M7.6 once tag checks are in, so for now this is always the
// caller-supplied room.
type Engagement struct {
	AttackerID   CombatantID
	TargetID     CombatantID
	AttackerName string
	TargetName   string
	RoomID       world.RoomID
}

// CombatEnded is dispatched when an entity's combat list becomes
// empty (spec §2.2 and §2.3). One event per side that empties —
// a pairwise Disengage between two combatants whose lists become
// empty dispatches two CombatEnded events; a DisengageAll
// dispatches one per opponent that emptied PLUS one for the entity
// itself.
//
// CombatantID is the entity that left combat. CombatantName is
// included for log convenience; subscribers that need richer state
// resolve through a Locator.
type CombatEnded struct {
	CombatantID   CombatantID
	CombatantName string
	RoomID        world.RoomID
}

// nopSink is the EventSink used when Manager is constructed with a
// nil sink. Centralized so the mutation path always has a non-nil
// dispatch target and doesn't have to nil-guard at every emission
// site.
type nopSink struct{}

func (nopSink) OnEngagement(context.Context, Engagement)   {}
func (nopSink) OnCombatEnded(context.Context, CombatEnded) {}
