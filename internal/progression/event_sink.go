package progression

import "context"

// EventSink is the seam between Manager and the host's event bus.
// Mirrors combat.EventSink: the progression package does not import
// eventbus (which would close progression → eventbus → entities →
// progression once entities holds progression state). A production
// adapter living in cmd/anothermud bridges the sink to bus.Publish.
//
// All methods are called synchronously from inside Manager
// operations. Implementations MUST be cheap and non-blocking;
// long-running work should be queued elsewhere.
//
// Every method takes the granting request's context so subscribers
// can propagate request-scoped fields (logger, deadline) and so
// cancellation propagates from the granter down through any
// downstream work the subscriber kicks off. Mirrors
// combat.EventSink's ctx-first convention.
type EventSink interface {
	// OnXPGained fires after a GrantExperience grant is applied.
	// amount is the grant; newTotal is the post-grant XP on this
	// track. source is the free-form attribution string (e.g.
	// "kill:mob:wolf-12", "quest:rescue-victor") the caller passes
	// in.
	OnXPGained(ctx context.Context, entityID, track string, amount, newTotal int64, source string)

	// OnLevelUp fires once per level-up step inside a cascade.
	// oldLevel + 1 == newLevel always; the field pair is kept for
	// subscribers that want to render the transition.
	OnLevelUp(ctx context.Context, entityID, track string, oldLevel, newLevel int)

	// OnXPLost fires after a DeductExperience removal — only when
	// actual loss > 0. amount is the actual loss (may be less
	// than what the caller asked for if floored at the current-
	// level threshold); newTotal is the post-deduction XP.
	OnXPLost(ctx context.Context, entityID, track string, amount, newTotal int64)

	// OnTrackReset fires after ResetTrack. No level/xp fields —
	// the resulting state is always (level=1, xp=0).
	OnTrackReset(ctx context.Context, entityID, track string)
}

// nopSink discards every event. The default when NewManager is
// called with a nil sink — keeps tests that don't care about
// emissions free of subscriber boilerplate.
type nopSink struct{}

func (nopSink) OnXPGained(context.Context, string, string, int64, int64, string) {}
func (nopSink) OnLevelUp(context.Context, string, string, int, int)              {}
func (nopSink) OnXPLost(context.Context, string, string, int64, int64)           {}
func (nopSink) OnTrackReset(context.Context, string, string)                     {}

// AbilitySink is the optional event-emission seam consumed by the
// ability resolution phase (spec abilities-and-effects §7). It
// mirrors the EffectSink / combat.EventSink shape: cheap,
// non-blocking, ctx-first. Implementations adapt to the production
// eventbus.Bus in cmd/anothermud; tests use a recording fake.
// nil-safe: a resolver constructed without a sink runs silently.
//
// The four methods cover the four resolution-phase event names spec
// §7 enumerates:
//   - ability used (hit) — OnAbilityUsed
//   - ability missed (miss) — OnAbilityMissed
//   - ability fizzled (validation failure) — OnAbilityFizzled,
//     emitted by the per-pulse driver in M9.4b
//   - vital-depleted (hp ≤ 0 during resolution) — OnVitalDepleted
type AbilitySink interface {
	OnAbilityUsed(ctx context.Context, ev AbilityUsedEvent)
	OnAbilityMissed(ctx context.Context, ev AbilityMissedEvent)
	OnAbilityFizzled(ctx context.Context, ev AbilityFizzledEvent)
	OnVitalDepleted(ctx context.Context, ev VitalDepletedEvent)
}

// AbilityUsedEvent is the payload published on a hit (spec §4.5
// step 8). Category is the engine's classification, useful for
// per-pool ("you cast …" vs "you …") rendering downstream.
type AbilityUsedEvent struct {
	SourceID    string
	AbilityID   string
	AbilityName string
	Category    AbilityCategory
	TargetID    string
	TargetName  string
}

// AbilityMissedEvent is the payload published on a miss (spec §4.5
// step 6). Same shape minus Category — the renderer typically uses
// the source's pronoun + the ability name.
type AbilityMissedEvent struct {
	SourceID    string
	AbilityID   string
	AbilityName string
	TargetID    string
	TargetName  string
}

// AbilityFizzledEvent is the payload published when the per-pulse
// driver drops a queued invocation because validation rejected it
// (spec §4.2 step 2, §4.8). Reason carries the lower-case keyword
// surface; clients SHOULD treat unknown reasons as opaque strings.
// Emitted by the M9.4b driver; defined here so the AbilitySink
// surface is stable before that wiring lands.
type AbilityFizzledEvent struct {
	SourceID    string
	AbilityID   string
	AbilityName string
	Reason      FizzleReason
}

// VitalDepletedEvent is the payload published when the resolver's
// post-hit death-check observes the target's HP at or below zero
// (spec §4.5 step 9). The progression layer never applies damage
// itself; this event signals combat that a queued resolution
// landed a lethal blow so combat can run its cancellable death
// check (combat §6.1). KillerID == SourceID always today.
//
// Distinct from combat.VitalDepleted (which is emitted by the
// damage-application path inside combat) — keeping the two
// separate avoids a progression → combat dependency. The production
// bus-bridge in cmd/anothermud forwards both onto the same bus
// topic.
type VitalDepletedEvent struct {
	VictimID string
	KillerID string
	Vital    string
}

// VitalHP is the canonical vital identifier emitted in
// VitalDepletedEvent.Vital today. Mirrors combat.VitalHP so
// subscribers comparing across the two event families don't need
// to re-spell the literal.
const VitalHP = "hp"

func (nopSink) OnAbilityUsed(context.Context, AbilityUsedEvent)       {}
func (nopSink) OnAbilityMissed(context.Context, AbilityMissedEvent)   {}
func (nopSink) OnAbilityFizzled(context.Context, AbilityFizzledEvent) {}
func (nopSink) OnVitalDepleted(context.Context, VitalDepletedEvent)   {}
