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
