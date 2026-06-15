package command

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/visibility"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// concealer is the optional actor capability the hide/reveal verbs need
// (visibility.md §3.1). connActor implements it; test actors that don't
// simply cannot hide (the verbs report a graceful refusal). Kept an
// optional interface — like LightViewer's capability assertions — so the
// broad Actor interface (and its many test fakes) need not grow.
type concealer interface {
	// IsHidden reports current hide concealment.
	IsHidden() bool
	// HideScore computes the would-be concealment difficulty (§4.2).
	HideScore() int
	// Hide commits concealment at score and returns the new instance id (§4.1).
	Hide(score int) uint64
	// Reveal clears hide concealment, returning whether it was hidden.
	Reveal() bool
}

// HideHandler implements `hide` (visibility.md §3.1): a stationary attempt
// to conceal the actor in its current room. Publishes the cancellable
// concealment.before so packs may forbid hiding (no cover, full light,
// sanctuary); on success sets the hide concealment and emits
// entity.concealed. Actor-only messaging — discovery is per-observer.
func HideHandler(ctx context.Context, c *Context) error {
	h, ok := c.Actor.(concealer)
	if !ok {
		return c.Actor.Write(ctx, "You can't hide.")
	}
	if h.IsHidden() {
		return c.Actor.Write(ctx, "You are already hidden.")
	}

	roomID := roomIDOf(c)

	// Cancellable pre-event (§3.1 step 1 / §6): a veto aborts with a generic
	// refusal so a subscribing pack owns the specific reason.
	pre := eventbus.NewConcealmentBefore(c.Actor.PlayerID(), string(visibility.SourceHide), roomID)
	if c.Bus != nil && c.Bus.PublishCancellable(ctx, pre) {
		return c.Actor.Write(ctx, "You can't hide here.")
	}

	score := h.HideScore()
	h.Hide(score)
	if c.Bus != nil {
		c.Bus.Publish(ctx, eventbus.EntityConcealed{
			EntityID:   c.Actor.PlayerID(),
			SourceType: string(visibility.SourceHide),
			Room:       roomID,
		})
	}
	return c.Actor.Write(ctx, "You slip into the shadows and go still.")
}

// RevealHandler implements `unhide` / `reveal`: the actor voluntarily
// steps out of hiding (visibility.md §3.1). Emits entity.revealed
// (reason = emerged) when it actually dropped a concealment.
func RevealHandler(ctx context.Context, c *Context) error {
	h, ok := c.Actor.(concealer)
	if !ok || !h.IsHidden() {
		return c.Actor.Write(ctx, "You aren't hidden.")
	}
	h.Reveal()
	if c.Bus != nil {
		c.Bus.Publish(ctx, eventbus.EntityRevealed{
			EntityID:   c.Actor.PlayerID(),
			SourceType: string(visibility.SourceHide),
			Reason:     "emerged",
			Room:       roomIDOf(c),
		})
	}
	return c.Actor.Write(ctx, "You step out of hiding.")
}

// breakConcealmentOnAction reveals a hidden actor when a breaks_concealment
// command runs (visibility §4.5: attacking/casting/speaking/loud manipulation
// drops roll-based concealment so the action is observed). The dispatcher
// calls this BEFORE the handler, after any typed-arg resolution succeeded, so
// the action is seen the instant it resolves. A no-op unless the actor is a
// hidden concealer; flag-gated invisibility (S5) is exempt and not handled
// here. Sneak (S4) will extend the same hook to drop the sneaking tag.
func breakConcealmentOnAction(ctx context.Context, c *Context) {
	h, ok := c.Actor.(concealer)
	if !ok || !h.IsHidden() {
		return
	}
	h.Reveal()
	if c.Bus != nil {
		c.Bus.Publish(ctx, eventbus.EntityRevealed{
			EntityID:   c.Actor.PlayerID(),
			SourceType: string(visibility.SourceHide),
			Reason:     "acted",
			Room:       roomIDOf(c),
		})
	}
	_ = c.Actor.Write(ctx, "Your sudden action gives you away; you are no longer hidden.")
}

// roomIDOf returns the actor's current room id, or empty when roomless
// (tests / pre-spawn).
func roomIDOf(c *Context) world.RoomID {
	if r := c.Actor.Room(); r != nil {
		return r.ID
	}
	return ""
}
