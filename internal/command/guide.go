package command

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// GuideService is the onboarding-guide lifecycle surface (onboarding-guide.md) —
// implemented at the composition root over the mob spawn pipeline, mirroring
// HirelingService but far thinner: a guide has no recruiter catalog, no durable
// ownership, and no combat. Only Materialize / Dematerialize are needed here; the
// spawn/graduate/trail lifecycle is driven engine-side (the session Manager).
type GuideService interface {
	// Materialize spawns the guide template into roomID and stamps ownerID as its
	// owner + marks it a guide. Returns the live guide's entity id.
	Materialize(ctx context.Context, ownerID, templateID string, roomID world.RoomID) (entities.EntityID, error)
	// Dematerialize removes a live guide from the world (shoo / graduation / logout)
	// — the inverse of Materialize. Reports whether one was present and removed.
	Dematerialize(ctx context.Context, id entities.EntityID) bool
	// DematerializeOwnedBy removes EVERY live guide currently owned by ownerID from
	// the world and returns how many were removed (onboarding-guide.md — the
	// one-guide-per-owner invariant). The guide-spawn path calls it before
	// materializing a fresh guide, so a guide stranded by a prior session (a
	// reconnect/relogin that built a new session without draining the old guide) is
	// swept rather than accumulating. Empty ownerID is a no-op.
	DematerializeOwnedBy(ctx context.Context, ownerID string) int
}

// guideOwner is the per-character live-guide surface the connActor satisfies
// (onboarding-guide.md). A guide is never persisted, so there is no durable half:
// DrainLiveGuide atomically reads-and-clears the single live guide (nil-safe,
// double-shoo-safe), returning its entity id. Handlers type-assert c.Actor to this.
type guideOwner interface {
	DrainLiveGuide() (entities.EntityID, bool)
}

// ShooHandler implements `shoo` — send your onboarding guide away for the session
// (onboarding-guide.md). The guide is not persisted and the login gate still
// applies, so a character below the graduation level re-acquires one next login;
// this is a session convenience, not a durable opt-out. Atomic drain so a shoo
// racing the logout drain can't double-remove.
func ShooHandler(ctx context.Context, c *Context) error {
	owner, ok := c.Actor.(guideOwner)
	if !ok || c.Guides == nil {
		return c.Actor.Write(ctx, "You have no one to send away.")
	}
	id, ok := owner.DrainLiveGuide()
	if !ok {
		return c.Actor.Write(ctx, "You have no one to send away.")
	}
	c.Guides.Dematerialize(ctx, id)
	return c.Actor.Write(ctx, "You wave your guide off; they nod and melt back into the sprawl.")
}
