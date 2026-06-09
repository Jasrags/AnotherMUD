package biome

import "fmt"

// Engine-baseline biome ids. These mirror world's terrain classifiers
// (world.TerrainOutdoors/Indoors/Underground) — kept in sync by value, not
// import, so the biome package stays free of a world dependency. The two
// shielding biomes carry the flags that generalize world-rooms-movement
// §6.4 (biomes.md §3 / PD-2): registering them preserves today's exact
// shielding behavior with no content change.
const (
	BaselineOutdoors    = "outdoors"
	BaselineIndoors     = "indoors"
	BaselineUnderground = "underground"
)

// RegisterEngineBaseline installs the engine-known biomes (biomes.md PD-2):
// outdoors (no shielding — the default), and indoors / underground (both
// weather- and time-shielded). A pack may add biomes but MUST NOT shadow
// these. Idempotent only across distinct registries; calling twice on the
// same registry returns a duplicate error (mirrors slot.RegisterEngineBaseline).
func RegisterEngineBaseline(r *Registry) error {
	if r == nil {
		return fmt.Errorf("biome.RegisterEngineBaseline: nil registry")
	}
	baseline := []*Biome{
		{ID: BaselineOutdoors, DisplayName: "open ground"},
		{ID: BaselineIndoors, DisplayName: "indoors", WeatherShielded: true, TimeShielded: true},
		{ID: BaselineUnderground, DisplayName: "underground", WeatherShielded: true, TimeShielded: true},
	}
	for _, b := range baseline {
		if err := r.RegisterEngine(b); err != nil {
			return fmt.Errorf("biome.RegisterEngineBaseline %q: %w", b.ID, err)
		}
	}
	return nil
}
