package main

import (
	"context"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// mountService implements command.MountService (mounts.md §2/§3) over the boot
// spawn pipeline: it materializes an owned mount into the world and removes it
// again. It is the composition-root glue between the stable verbs and the mob
// spawner — owning none of the durable state (that lives on the player save).
type mountService struct {
	spawner *bootSpawner
	store   *entities.Store
}

// MountName returns a mount template's display name and whether the id resolves
// to a real mount (a mob template carrying a mount block). A non-mount or
// unknown id returns ("", false), so the stable verbs never sell or retrieve a
// plain mob.
func (m *mountService) MountName(templateID string) (string, bool) {
	tpl, err := m.spawner.mobTemplates.Get(mob.TemplateID(templateID))
	if err != nil || tpl.Mount == nil {
		return "", false
	}
	return tpl.Name, true
}

// Materialize spawns the owned mount into roomID through the full mob pipeline
// (the same path area spawning uses) and stamps ownerID as its owner. Returns
// the live mount's entity id.
func (m *mountService) Materialize(ctx context.Context, ownerID, templateID string, roomID world.RoomID) (entities.EntityID, error) {
	id, err := m.spawner.spawnMob(ctx, templateID, roomID, "")
	if err != nil {
		return "", err
	}
	if e, ok := m.store.GetByID(id); ok {
		if inst, ok := e.(*entities.MobInstance); ok {
			inst.SetOwner(ownerID)
		}
	}
	return id, nil
}

// Dematerialize removes a live mount from the world (stabling / §9): out of its
// room and out of the entity store. Ownership (the save record) is untouched.
// Reports whether the mount was present and removed. (v1 mounts carry no
// saddlebag cargo; when tack lands the carried contents will be persisted into
// the owner's record here before the creature is untracked.)
func (m *mountService) Dematerialize(ctx context.Context, mountID entities.EntityID) bool {
	if _, ok := m.store.GetByID(mountID); !ok {
		return false
	}
	// Remove from placement first (the mount is immediately unreachable by room
	// queries), then untrack from the store. The mount-travel-regen tick may
	// still see it once via GetByTag between these two calls, but RestoreTravel
	// on its about-to-be-discarded pool is a capped no-op — safe.
	m.spawner.placement.Remove(mountID)
	if err := m.store.Untrack(mountID); err != nil {
		// Already untracked (a concurrent remove) — the creature is gone
		// either way; log so a real bookkeeping leak is visible rather than
		// silently swallowed.
		logging.From(ctx).Warn("mount dematerialize: untrack failed",
			slog.String("mount", string(mountID)), slog.Any("err", err))
	}
	return true
}
