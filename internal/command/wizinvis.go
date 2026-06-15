package command

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/visibility"
)

// adminInvisible is the optional actor capability the `wizinvis` verb and the
// visibility filter need (visibility.md §3.4). connActor implements it; a test
// actor that doesn't simply can't go wizinvis. Optional interface (the
// concealer/sneaker pattern) so the broad Actor iface + fakes need not grow.
type adminInvisible interface {
	// IsAdminInvisible reports current admin (wizinvis) concealment (the
	// target-read the visibility filter + who roster use).
	IsAdminInvisible() bool
	// ToggleAdminInvisible flips the state under one lock and returns the NEW
	// state, so the verb's check-and-act is atomic.
	ToggleAdminInvisible() bool
}

// WizinvisHandler implements `wizinvis` (visibility.md §3.4): an admin toggles
// a flag-gated concealment that hides them from the room render, target
// resolution, and `who` for every observer of lower admin rank. Unlike
// hide/sneak it is NOT roll-gated and does NOT break on action. Admin-gated at
// dispatch (registered with Admin: true), so reaching the handler means the
// actor already holds the admin role.
func WizinvisHandler(ctx context.Context, c *Context) error {
	ai, ok := c.Actor.(adminInvisible)
	if !ok {
		return c.Actor.Write(ctx, "You can't go invisible.")
	}
	roomID := roomIDOf(c)
	// Atomic flip (single lock) — the new state decides which event fires.
	if ai.ToggleAdminInvisible() {
		if c.Bus != nil {
			c.Bus.Publish(ctx, eventbus.EntityConcealed{
				EntityID:   c.Actor.PlayerID(),
				SourceType: string(visibility.SourceAdminInvis),
				Room:       roomID,
			})
		}
		return c.Actor.Write(ctx, "You wink out of sight; only your peers can see you now.")
	}
	if c.Bus != nil {
		c.Bus.Publish(ctx, eventbus.EntityRevealed{
			EntityID:   c.Actor.PlayerID(),
			SourceType: string(visibility.SourceAdminInvis),
			Reason:     "emerged",
			Room:       roomID,
		})
	}
	return c.Actor.Write(ctx, "You fade back into view.")
}

// actorIsAdmin reports whether the actor holds the admin role (adminRole,
// defaulting to the engine default when empty). The shared "is this viewer
// staff?" test the visibility admin-rank path and other admin gates reuse.
func actorIsAdmin(actor Actor, adminRole string) bool {
	role := adminRole
	if role == "" {
		role = defaultAdminRole
	}
	rh, ok := actor.(RoleHolder)
	return ok && rh.HasRole(role)
}
