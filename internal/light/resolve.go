package light

import "github.com/Jasrags/AnotherMUD/internal/world"

// Inputs is the gathered set of light contributions for one
// (room, viewer) pair (spec §2.2). Call sites assemble it; Resolve is
// pure over it. Levels default to their zero value (Black), which is
// the correct "no contribution" for Sources and ViewerFloor.
type Inputs struct {
	// Ambient is the sky's ambient level for the current period
	// (Config.AmbientFor) — never Black.
	Ambient Level
	// Terrain is the room's terrain string (world.TerrainOf); the
	// sky-gate keys off it. Empty is treated as outdoors.
	Terrain string
	// IndoorCap is the ceiling ambient may reach in an `indoors`
	// room (Config.IndoorCap).
	IndoorCap Level
	// Override is the authored room→area→zone `light` PIN level, or nil
	// when none is set. When present it both floors and ceilings the
	// AMBIENT term for the room (§2.4) — it replaces the sky entirely
	// (a sealed vault pinned `black` defeats daylight), but light
	// sources and the viewer floor still combine over it.
	Override *Level
	// AmbientFloor is the authored room→area→zone `light_floor` level,
	// or nil when none is set. Unlike Override (a pin that replaces the
	// sky), the floor MAX-combines with the terrain-gated ambient: it
	// lifts a dark night (gloom → dim) without capping a bright day
	// (noon stays lit). This is the "lamp-lit settlement" knob (§2.4) —
	// a street the lamps keep navigable after dark. Ignored when
	// Override is set (a pin is the stricter directive and wins). Not
	// gated by terrain, like Override (§2.3).
	AmbientFloor *Level
	// Sources is the best level contributed by lit sources (the
	// viewer's held light + luminous items/mobs in the room). Black
	// when nothing is lit. Not gated by terrain (§2.3).
	Sources Level
	// ViewerFloor is the per-viewer minimum (darkvision / sight
	// effect). Applied last, after everything else (§2.2 viewerCap).
	ViewerFloor Level
}

// throughTerrain applies the §2.3 sky-gate: outdoors (and any unknown
// terrain) gets full ambient; indoors is capped; underground gets
// none. Only ambient is gated here — overrides and sources are not.
func throughTerrain(ambient, indoorCap Level, terrain string) Level {
	switch terrain {
	case world.TerrainUnderground:
		return Black
	case world.TerrainIndoors:
		if ambient < indoorCap {
			return ambient
		}
		return indoorCap
	default:
		// outdoors, empty, or any non-shielding terrain string →
		// full ambient (matches the weather eligibility rule that
		// unknown terrain is always sky-eligible).
		return ambient
	}
}

// Resolve computes the effective light level for one (room, viewer)
// from gathered Inputs (spec §2.2):
//
//	ambientTerm = override (pin), if present;
//	              else max(throughTerrain(ambient), ambientFloor)
//	effective   = clamp( max(ambientTerm, sources, viewerFloor) )
//
// The override (pin) replacing the ambient term — rather than
// max-combining with it — is what lets a room pin `black` to defeat
// daylight (§2.4) while a carried torch (Sources) still lights it
// (§2.3). The ambient floor, in contrast, max-combines: it lifts a
// dark night but never caps a bright day, so a lamp-lit street stays
// `lit` at noon and `dim` (not `gloom`) at midnight. A pin outranks a
// floor. The result is never persisted — recompute on demand.
func Resolve(in Inputs) Level {
	ambientTerm := throughTerrain(in.Ambient, in.IndoorCap, in.Terrain)
	switch {
	case in.Override != nil:
		// Pin: replace the sky entirely (floors and ceilings).
		ambientTerm = *in.Override
	case in.AmbientFloor != nil:
		// Floor: lift a dark ambient, never cap a bright one. Ungated
		// by terrain (a lamp floor reaches an indoors/underground room).
		ambientTerm = max(ambientTerm, *in.AmbientFloor)
	}
	eff := max(ambientTerm, in.Sources)
	eff = max(eff, in.ViewerFloor)
	return clamp(eff)
}
