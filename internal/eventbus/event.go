// Package eventbus is the engine's observable-event substrate per the
// cross-cutting events convention in docs/specs/README.md §Events.
//
// Every spec's §Observable events section lists named events; the bus
// is what publishers send them through and what scripts / engine
// subsystems subscribe to. Two semantics live side by side:
//
//   - Post-fact notifications (the common case): publisher fires
//     after the operation succeeded. Listeners react but cannot
//     change behavior.
//   - Cancellable pre-events: publisher fires before the operation
//     commits; any listener can flip the event's cancel flag to
//     abort. M5 has exactly one (`container.item_adding`); other
//     specs (combat, progression, economy) add ~5 more.
//
// The bus is intentionally synchronous and sequential. Cancellable
// events require it (the caller needs the verdict before acting),
// tests stay deterministic without goroutine bookkeeping, and the
// emit sites today already run under handler locks that an async
// queue would invert. We will revisit when a listener's workload
// justifies offloading; we are not committing to sync forever.
package eventbus

// Event is the marker every published value satisfies. Name returns
// the dotted event name; spec text uses spaces (e.g. "entity
// equipped") for readability, the bus uses dots ("entity.equipped")
// because identifiers carry better in code. The mapping is
// one-to-one and documented at each event type's declaration.
type Event interface {
	Name() string
}

// CancellableEvent is the additional surface a pre-event needs. A
// listener calls Cancel() to veto the operation; the publisher
// (PublishCancellable) checks Cancelled() after the dispatch loop
// returns.
//
// Implementations should embed a pointer to a small struct that
// carries the cancel flag so that the cancel call from a listener
// is visible to siblings later in the dispatch loop — the spec
// allows later listeners to observe "someone already cancelled"
// and react accordingly (e.g. log + skip their own work).
type CancellableEvent interface {
	Event
	Cancelled() bool
	Cancel()
}

// CancelFlag is the shared cancel-flag scaffolding. Concrete
// cancellable event types embed *CancelFlag so the cancel state is
// visible across the dispatch loop without each event re-implementing
// the flag. The type name is CancelFlag rather than Cancel so the
// embedded field doesn't shadow the promoted Cancel() method.
type CancelFlag struct {
	cancelled bool
}

// Cancelled reports whether any handler has already called Cancel
// on this event.
func (c *CancelFlag) Cancelled() bool {
	if c == nil {
		return false
	}
	return c.cancelled
}

// Cancel flips the cancel flag. Idempotent.
func (c *CancelFlag) Cancel() {
	if c == nil {
		return
	}
	c.cancelled = true
}
