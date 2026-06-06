package light

import "github.com/Jasrags/AnotherMUD/internal/world"

// PropRoomLight is the engine-registered room property naming an
// explicit light override (spec §2.4 / §9). Its value is one of the
// four level names ("black"/"gloom"/"dim"/"lit"); the property
// registry validates it is a string, ParseLevel validates the
// vocabulary at read time (a typo is treated as no override, never a
// silent black pin).
const PropRoomLight = "light"

// OverrideFor walks the room → area → zone light-override cascade
// (spec §2.4) and returns the authored level plus true when one is
// present. The override, when present, both floors and ceilings the
// room's ambient term in Resolve.
//
// Only the room tier is wired today, mirroring how the weather cascade
// shipped its zone tier first (M15.4a): Area carries no light default
// field and the zone/biome tier waits on biomes.md. The walk is
// structured so those tiers slot in here without touching callers.
func OverrideFor(r *world.Room) (Level, bool) {
	if r == nil {
		return Black, false
	}
	// Room tier: the authored `light` property.
	if s, ok := r.PropertyString(PropRoomLight); ok {
		if lvl, ok := ParseLevel(s); ok {
			return lvl, true
		}
	}
	// Area tier (future): an area-level light default.
	// Zone/biome tier (future): a biomes.md region default
	//   (e.g. a "cavern" biome defaulting black).
	return Black, false
}
