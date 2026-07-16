package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// guideService implements command.GuideService (onboarding-guide.md) over the boot
// spawn pipeline — a thinner sibling of hirelingService. It materializes an owned
// onboarding guide into the world and removes it again; it owns no durable state
// (a guide is never persisted) and no recruiter catalog (guides are not hired).
type guideService struct {
	spawner *bootSpawner
	store   *entities.Store
}

// Materialize spawns the guide template into roomID through the full mob pipeline
// and stamps ownerID as its owner + marks it a guide (SetGuide, NOT SetHireling —
// a guide never fights, incurs no upkeep, and credits no loot/XP). Returns the
// live guide's entity id.
func (g *guideService) Materialize(ctx context.Context, ownerID, templateID string, roomID world.RoomID) (entities.EntityID, error) {
	id, err := g.spawner.spawnMob(ctx, templateID, roomID, "")
	if err != nil {
		return "", err
	}
	e, ok := g.store.GetByID(id)
	if !ok {
		logging.From(ctx).Warn("guide materialize: mob missing after spawn",
			slog.String("template", templateID), slog.String("entity", string(id)))
		return "", fmt.Errorf("guide materialize: mob %s not found after spawn", id)
	}
	inst, ok := e.(*entities.MobInstance)
	if !ok {
		logging.From(ctx).Warn("guide materialize: unexpected entity type",
			slog.String("template", templateID), slog.String("entity", string(id)))
		return "", fmt.Errorf("guide materialize: entity %s is not a mob", id)
	}
	inst.SetOwner(ownerID)
	inst.SetGuide(true)
	return id, nil
}

// DematerializeOwnedBy removes every live guide owned by ownerID (onboarding-guide.md
// — the one-guide-per-owner invariant). SpawnGuideFor calls this before materializing
// a fresh guide so a guide stranded by a prior session (a reconnect/relogin that built
// a new session without draining the old guide) is swept rather than accumulating.
// Returns how many were removed. Reads the owner-scoped guide set straight from the
// store (IsGuide + OwnerID, engine-authoritative), so it finds ALL strays, not just a
// last-tracked one.
func (g *guideService) DematerializeOwnedBy(ctx context.Context, ownerID string) int {
	removed := 0
	for _, id := range g.store.GuidesOwnedBy(ownerID) {
		if g.Dematerialize(ctx, id) {
			removed++
		}
	}
	return removed
}

// Dematerialize removes a live guide from the world (shoo / graduation / logout):
// out of its room and out of the entity store. Reports whether it was present.
func (g *guideService) Dematerialize(ctx context.Context, id entities.EntityID) bool {
	if _, ok := g.store.GetByID(id); !ok {
		return false
	}
	g.spawner.placement.Remove(id)
	if err := g.store.Untrack(id); err != nil {
		logging.From(ctx).Warn("guide dematerialize: untrack failed",
			slog.String("guide", string(id)), slog.Any("err", err))
	}
	return true
}
