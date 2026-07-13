package biome

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Biome ambient hazards (area-effects.md §4.6): intrinsic environmental
// damage declared on a biome — a `toxic` zone's radiation, a `vacuum`
// zone's pressure — that harms every creature present unless they carry or
// wear a protection key. Unlike a placed hazard (caltrops, §4.1–4.5) it has
// no placer (environmental death is credited to no one, §4.5) and is
// DERIVED from content, never persisted (§5): re-reading the pack
// reconstructs it, exactly like weather and biome ambience.
//
// This is the on-tick-while-present half of the area-effect primitive
// (§2). The service mirrors AmbienceService: one pass over the world's
// rooms, resolve each room's biome, and for a hazardous biome apply the
// payload to each unprotected occupant. All engine-coupled work (listing
// occupants, checking the protection key, applying typed damage + routing
// death) rides the HazardSink interface so this package stays free of the
// session/combat imports that would close a cycle.

// Hazard is a biome's intrinsic ambient payload (area-effects.md §2.1 +
// §4.6). v1 delivers flat typed damage on the on-tick-while-present
// trigger; per-type resistance soak (§4.6(b)) and an on-enter first jolt
// are deferred (see the build-log memory).
type Hazard struct {
	// Damage is the flat amount inflicted per hazard tick on an unprotected
	// creature present. Zero (or negative) makes the hazard inert.
	Damage int

	// DamageType is the weapon-identity damage type carried on the harm
	// (e.g. "radiation", "toxic"), passed through for logging and any
	// future per-type resistance step. Empty falls back to the engine's
	// physical default at the application site.
	DamageType string

	// ProtectionKey is the content-declared tag that grants a creature
	// carrying or wearing a bearing item TOTAL immunity (§4.6(b): immunity
	// negates, distinct from resistance which mitigates). Empty means no
	// gate — everyone present takes the payload.
	ProtectionKey string

	// Message is the per-victim room copy delivered when the hazard bites
	// ("The air itself sears your lungs."). Empty suppresses the flavor
	// line; the damage still applies.
	Message string
}

// Active reports whether the hazard actually inflicts anything. A nil
// hazard or a non-positive Damage is inert, so the tick can skip it.
func (h *Hazard) Active() bool { return h != nil && h.Damage > 0 }

// HazardSink is the engine-coupled surface the HazardService drives. The
// production impl (cmd/anothermud) lists room occupants via the session
// manager, scans carried/worn item tags for the protection key (mirroring
// gathering.hasToolTag), and applies typed damage that routes an
// attacker-less death through the normal combat death path.
type HazardSink interface {
	// OccupantsInRoom returns the victim ids the hazard should consider in
	// the room. v1 lists players; mobs are a deferred fast-follow.
	OccupantsInRoom(roomID world.RoomID) []string

	// HasProtection reports whether victimID carries or wears an item
	// bearing protectionKey (§4.6(b) immunity). A caller passes a non-empty
	// key; an empty key is filtered by the service before it calls this.
	HasProtection(victimID, protectionKey string) bool

	// Harm applies the payload to victimID and routes an attacker-less
	// death (§4.5), delivering the room-copy message to the victim. It owns
	// the double-death race guard (Vitals.ApplyDamageIfAlive) and messaging.
	Harm(ctx context.Context, victimID string, roomID world.RoomID, amount int, damageType, message string)
}

// HazardService applies biome ambient hazards on a tick. All deps are
// interfaces so tests need no real world/session/combat.
type HazardService struct {
	registry *Registry
	rooms    RoomLister
	sink     HazardSink
}

// NewHazardService wires the service. A nil registry / rooms / sink makes
// Tick a no-op (defensive — the composition root supplies all three in
// production).
func NewHazardService(registry *Registry, rooms RoomLister, sink HazardSink) *HazardService {
	return &HazardService{registry: registry, rooms: rooms, sink: sink}
}

// Tick applies each hazardous biome's payload to the unprotected occupants
// of its rooms (area-effects.md §4.6). One pass over the world's rooms:
// resolve each room's biome, skip harmless biomes, then for each occupant
// skip the protected and harm the rest. Intrinsic hazards are content-
// derived, so nothing here persists.
func (s *HazardService) Tick(ctx context.Context) {
	if s == nil || s.registry == nil || s.rooms == nil || s.sink == nil {
		return
	}
	for _, room := range s.rooms.Rooms() {
		if room == nil {
			continue
		}
		b, ok := s.registry.Resolve(room.Terrain)
		if !ok || !b.Hazard.Active() {
			continue
		}
		h := b.Hazard
		for _, victimID := range s.sink.OccupantsInRoom(room.ID) {
			if h.ProtectionKey != "" && s.sink.HasProtection(victimID, h.ProtectionKey) {
				continue // §4.6(b): the protection key negates the payload entirely.
			}
			s.sink.Harm(ctx, victimID, room.ID, h.Damage, h.DamageType, h.Message)
		}
	}
}
