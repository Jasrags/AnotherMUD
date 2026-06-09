package world

// Terrain vocabulary + classifier — the canonical home for the room
// terrain concept (world-rooms-movement §6.4). It lives here, not in a
// consuming feature, because more than one feature now keys off it:
// the weather/time-ambience cascade and the light-and-darkness
// sky-gate must agree on what "indoors" means and on the empty →
// outdoors default. Both delegate here rather than each defining their
// own copy.

// Terrain classifiers the engine knows about. Empty terrain on a room
// is treated as `outdoors`. The two shielding values hide a room from
// sky-driven effects (ambience delivery; ambient light) unless a
// matching exposure flag is set.
const (
	TerrainOutdoors    = "outdoors"
	TerrainIndoors     = "indoors"
	TerrainUnderground = "underground"
)

// TerrainOf returns the effective terrain string for r, applying the
// empty → outdoors default. Centralised so every sky-driven feature
// agrees on the fallback.
func TerrainOf(r *Room) string {
	if r == nil || r.Terrain == "" {
		return TerrainOutdoors
	}
	return r.Terrain
}

// IsShielded reports whether r's terrain is one of the engine's two
// structurally-enclosed classifiers (indoors / underground).
//
// NOTE (biomes.md §3): weather/time *ambience* shielding is no longer
// decided here — it moved to the biome registry's per-axis flags, consulted
// in the weather package's eligibility check (weather.ShieldingFunc), so a
// content biome can declare its own shielding. This hardcoded check remains
// the "is this room enclosed" predicate for callers that mean structural
// enclosure rather than sky-ambience eligibility — today the campfire build
// gate (no fire indoors/underground), which should NOT treat a merely
// weather-shielded canopy as unbuildable.
func IsShielded(r *Room) bool {
	switch TerrainOf(r) {
	case TerrainIndoors, TerrainUnderground:
		return true
	default:
		return false
	}
}
