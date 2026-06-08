// Package campfire implements the Tier-1 improvised crafting station
// (crafting-and-cooking §4): a temporary container entity a player builds
// in a room that, while it exists, makes the room a Tier-1 cooking station,
// and decays after a TTL. It reuses the M15.2 temporary-entity decay shape
// (a tagged placed entity swept by a tick handler) — there is no furniture
// system. The build action's gates (terrain/weather/fuel) live in the
// command layer; this package owns the entity's creation, its station
// contribution, and its decay.
package campfire

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

const (
	// Tag marks a campfire entity so the decay sweep can find every one
	// (mirrors corpse.TagCorpse). Also carries no_get so a fire can't be
	// scooped into a pack.
	Tag      = "campfire"
	TagNoGet = "no_get"

	// PropCreatedTick is the tick the campfire was built on (uint64); the
	// decay sweep compares it against now to find expired fires.
	PropCreatedTick = "campfire_created_tick"

	// PropCraftStations is the per-discipline station-tier map the craft
	// path reads off a room-placed entity — the same key a room uses, so
	// the station-tier computation treats a campfire symmetrically with a
	// fixed room station (crafting-and-cooking §4).
	PropCraftStations = "craft_stations"

	// Name / keywords for the placed fire.
	displayName = "a crackling campfire"
)

// Tier is the station tier a campfire provides for cooking.
const Tier = 1

// Place mints a campfire container, stamps its created-tick + station map,
// and files it into roomID. Returns the new entity id. The caller (the
// build command) owns the terrain/weather/fuel gates.
func Place(store *entities.Store, placement *entities.Placement, roomID world.RoomID, nowTick uint64) (entities.EntityID, error) {
	inst, err := store.SpawnContainer(
		displayName,
		[]string{Tag, TagNoGet},
		[]string{"campfire", "fire"},
		map[string]any{
			PropCreatedTick:   nowTick,
			PropCraftStations: map[string]int{"cooking": Tier},
		},
	)
	if err != nil {
		return "", err
	}
	placement.Place(inst.ID(), roomID)
	return inst.ID(), nil
}

// CreatedTick returns the tick a campfire was built on (0 if absent).
func CreatedTick(it *entities.ItemInstance) uint64 {
	if it == nil {
		return 0
	}
	if v, ok := it.Property(PropCreatedTick); ok {
		switch n := v.(type) {
		case uint64:
			return n
		case int:
			return uint64(n)
		case int64:
			return uint64(n)
		case float64:
			return uint64(n)
		}
	}
	return 0
}

// Service runs the campfire decay sweep over the entity store.
type Service struct {
	store     *entities.Store
	placement *entities.Placement
}

// NewService wires a decay service.
func NewService(store *entities.Store, placement *entities.Placement) *Service {
	return &Service{store: store, placement: placement}
}

// DecaySweep removes every campfire whose lifetime (in ticks) has elapsed,
// returning the rooms a fire burned out in (so the caller can announce it).
// Mirrors corpse.DecaySweep: subtract-first avoids uint64 overflow, and
// placement.Remove is the single-winner claim against a concurrent sweep.
func (s *Service) DecaySweep(ctx context.Context, nowTick, lifetime uint64) []world.RoomID {
	if s.store == nil || s.placement == nil {
		return nil
	}
	var rooms []world.RoomID
	for _, e := range s.store.GetByTag(Tag) {
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		created := CreatedTick(it)
		if nowTick < created || nowTick-created < lifetime {
			continue
		}
		roomID, _ := s.placement.RoomOf(it.ID())
		if !s.placement.Remove(it.ID()) {
			continue // lost the race to another sweep
		}
		_ = s.store.Untrack(it.ID())
		rooms = append(rooms, roomID)
	}
	return rooms
}
