package biome

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Biome ambience (biomes.md §4): idle ecological flavor — birdsong in a
// forest, dripping water in a cavern — delivered to occupied rooms of a
// biome at a configured cadence. It is the biome's OWN flavor, so unlike
// weather/time ambience it is NOT gated by shielding (a sheltered cavern
// still drips, §4.1) and it emits NO bus event (pure presentation, like
// time ambience).

// RoomLister enumerates the world's rooms (world.World satisfies it).
type RoomLister interface {
	Rooms() []*world.Room
}

// Broadcaster delivers a one-shot message to a room — the same shape as
// session.Manager.SendToRoom (redeclared so biome doesn't import session).
type Broadcaster interface {
	SendToRoom(ctx context.Context, roomID world.RoomID, text string, excludePlayerIDs ...string)
}

// Roller picks a random index (math/rand/v2.Rand.IntN satisfies it; same
// shape as combat/weather Roller). Panics on n<=0 by contract, so callers
// guard against empty pools.
type Roller interface {
	IntN(n int) int
}

// OccupiedFunc reports whether a room currently holds at least one player
// — §4.1's occupied-only rule (no point flavoring an empty room). Injected
// from the session manager so biome stays free of a session import. A nil
// func treats every room as occupied (flavor still no-ops on empty rooms at
// the broadcaster, but the configured default wires this).
type OccupiedFunc func(roomID world.RoomID) bool

// AmbienceService delivers biome ambience on a tick. All deps are
// interfaces so tests need no real world/session.
type AmbienceService struct {
	registry *Registry
	rooms    RoomLister
	occupied OccupiedFunc
	bcast    Broadcaster
	roller   Roller
}

// NewAmbienceService wires the service. A nil registry / rooms / bcast /
// roller makes Tick a no-op (defensive — the composition root supplies all
// four in production).
func NewAmbienceService(registry *Registry, rooms RoomLister, occupied OccupiedFunc, bcast Broadcaster, roller Roller) *AmbienceService {
	return &AmbienceService{registry: registry, rooms: rooms, occupied: occupied, bcast: bcast, roller: roller}
}

// Tick delivers one random ambience line to each occupied room whose biome
// declares a non-empty ambience pool (biomes.md §4.1). One pass over the
// world's rooms: resolve each room's biome, skip rooms with no biome /
// empty pool / no occupants, then pick and broadcast a line. Shielding is
// deliberately NOT consulted (§4.1). No bus event is emitted.
func (s *AmbienceService) Tick(ctx context.Context) {
	if s == nil || s.registry == nil || s.rooms == nil || s.bcast == nil || s.roller == nil {
		return
	}
	for _, room := range s.rooms.Rooms() {
		if room == nil {
			continue
		}
		b, ok := s.registry.Resolve(room.Terrain)
		if !ok || len(b.Ambience) == 0 {
			continue
		}
		if s.occupied != nil && !s.occupied(room.ID) {
			continue
		}
		line := b.Ambience[s.roller.IntN(len(b.Ambience))]
		if line == "" {
			continue
		}
		s.bcast.SendToRoom(ctx, room.ID, line)
	}
}
