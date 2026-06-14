package progression

import "context"

// CastNotifier is the optional cast-lifecycle seam for timed weaves (WoT S2 —
// the channel interrupt game). It is deliberately SEPARATE from AbilitySink:
// the instant-resolution events (used / missed / fizzled / vital-depleted) are
// unchanged, and a ruleset with no timed casts never implements this. The
// ability phase emits:
//
//   - OnCastBegan when a CastTime > 0 ability starts its warmup (so the caster
//     sees "you begin to weave …" and bystanders can be told).
//   - OnCastInterrupted when an in-flight cast is aborted before it resolves
//     (slice 2: a hit on the caster). Completion needs no event here — a cast
//     that survives to resolve emits the normal AbilitySink used/missed events.
//
// nil-safe: the driver runs the timed-cast state machine silently without one.
// Called synchronously on the tick goroutine inside the ability phase; keep
// implementations cheap and non-blocking, like the other sinks.
type CastNotifier interface {
	OnCastBegan(ctx context.Context, ev CastBeganEvent)
	OnCastInterrupted(ctx context.Context, ev CastInterruptedEvent)
}

// CastBeganEvent is published when a timed weave starts its warmup. Rounds is
// the warmup length (CastTime) so the renderer can show a cast bar / ETA.
type CastBeganEvent struct {
	SourceID       string
	AbilityID      string
	AbilityName    string
	TargetEntityID string
	Rounds         int
}

// CastInterruptedEvent is published when an in-flight weave is aborted before
// it resolves. Cause is a free-form lower-case keyword for the disruption
// ("hit", later "stunned"/"moved"); clients SHOULD treat unknown causes as
// opaque.
type CastInterruptedEvent struct {
	SourceID    string
	AbilityID   string
	AbilityName string
	Cause       string
}
