package weather

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

func TestWeatherEligible(t *testing.T) {
	cases := []struct {
		name string
		room *world.Room
		want bool
	}{
		{"nil room", nil, false},
		{"default terrain is outdoors → eligible", &world.Room{}, true},
		{"forest terrain is unshielded → eligible", &world.Room{Terrain: "forest"}, true},
		{"indoors is shielded → ineligible", &world.Room{Terrain: TerrainIndoors}, false},
		{"indoors + exposure flag → eligible", &world.Room{Terrain: TerrainIndoors, WeatherExposed: true}, true},
		{"underground is shielded → ineligible", &world.Room{Terrain: TerrainUnderground}, false},
		{"underground + exposure flag → eligible", &world.Room{Terrain: TerrainUnderground, WeatherExposed: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// nil resolver → hardcoded indoors/underground fallback (the
			// pre-biomes §6.4 behavior these cases assert).
			if got := weatherEligible(tc.room, nil); got != tc.want {
				t.Errorf("weatherEligible = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTimeEligible_HonorsItsOwnExposureFlag(t *testing.T) {
	// A room may be weather-exposed but not time-exposed (e.g. a
	// covered porch that gets rained on but never sees sky). Spec
	// §6.4 — the two flags are independent.
	r := &world.Room{Terrain: TerrainIndoors, WeatherExposed: true, TimeExposed: false}
	if !weatherEligible(r, nil) {
		t.Error("weather eligible should be true")
	}
	if timeEligible(r, nil) {
		t.Error("time eligible should be false")
	}
}

// TestEligible_BiomeShieldingResolver covers the biomes.md §3 generalization:
// a resolver can declare per-axis shielding for an otherwise-unshielded
// terrain, and the two flags are independent.
func TestEligible_BiomeShieldingResolver(t *testing.T) {
	// A "canopy" biome: weather-shielded (rain doesn't reach) but NOT
	// time-shielded (you still see the sky brighten/darken).
	resolver := func(terrain string) (bool, bool, bool) {
		if terrain == "canopy" {
			return true, false, true // weatherShielded, timeShielded, ok
		}
		return false, false, false // unregistered → hardcoded fallback
	}
	canopy := &world.Room{Terrain: "canopy"}
	if weatherEligible(canopy, resolver) {
		t.Error("canopy is weather-shielded → weather ineligible without exposure")
	}
	if !timeEligible(canopy, resolver) {
		t.Error("canopy is NOT time-shielded → time eligible")
	}
	// A weather-exposed canopy room becomes weather-eligible again.
	exposed := &world.Room{Terrain: "canopy", WeatherExposed: true}
	if !weatherEligible(exposed, resolver) {
		t.Error("canopy + WeatherExposed → weather eligible")
	}
	// Unregistered terrain falls back to the hardcoded set even with a
	// resolver present: indoors stays shielded.
	if weatherEligible(&world.Room{Terrain: TerrainIndoors}, resolver) {
		t.Error("indoors (unregistered by this resolver) → hardcoded shielded")
	}
}

func TestResolveWeatherMessage_FallsBackToOutdoorsThenEmpty(t *testing.T) {
	zone := &Zone{
		WeatherMessages: map[string]map[string]MessageTriple{
			"rain": {
				TerrainOutdoors: {Start: "Rain begins.", End: "Rain stops."},
				"forest":        {Start: "Drops patter on leaves."},
			},
		},
	}
	cases := []struct {
		name      string
		room      *world.Room
		state     string
		wantStart string
		wantEnd   string
	}{
		{"forest hits specific entry", &world.Room{Terrain: "forest"}, "rain", "Drops patter on leaves.", ""},
		{"outdoors hits the outdoor entry", &world.Room{Terrain: TerrainOutdoors}, "rain", "Rain begins.", "Rain stops."},
		{"empty terrain → outdoors fallback", &world.Room{}, "rain", "Rain begins.", "Rain stops."},
		{"unknown terrain falls back to outdoors", &world.Room{Terrain: "swamp"}, "rain", "Rain begins.", "Rain stops."},
		{"unknown state → empty", &world.Room{Terrain: "forest"}, "snow", "", ""},
		{"nil zone → empty", &world.Room{Terrain: "forest"}, "rain", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			z := zone
			if tc.name == "nil zone → empty" {
				z = nil
			}
			got := resolveWeatherMessage(tc.room, z, tc.state)
			if got.Start != tc.wantStart {
				t.Errorf("Start = %q, want %q", got.Start, tc.wantStart)
			}
			if got.End != tc.wantEnd {
				t.Errorf("End = %q, want %q", got.End, tc.wantEnd)
			}
		})
	}
}

func TestResolveTimeMessage_TerrainCascade(t *testing.T) {
	zone := &Zone{
		TimeMessages: map[string]map[string]string{
			"dawn": {
				TerrainOutdoors: "Light leaks across the horizon.",
				"forest":        "Birds begin to call.",
			},
		},
	}
	if got := resolveTimeMessage(&world.Room{Terrain: "forest"}, zone, "dawn"); got != "Birds begin to call." {
		t.Errorf("forest dawn = %q", got)
	}
	if got := resolveTimeMessage(&world.Room{}, zone, "dawn"); got != "Light leaks across the horizon." {
		t.Errorf("default dawn = %q", got)
	}
	if got := resolveTimeMessage(&world.Room{}, zone, "midnight"); got != "" {
		t.Errorf("unknown period should be empty, got %q", got)
	}
	if got := resolveTimeMessage(&world.Room{}, nil, "dawn"); got != "" {
		t.Errorf("nil zone should be empty, got %q", got)
	}
}

// fixedRoller returns a pre-set sequence of values from IntN.
// Index past the end panics so a test that calls IntN more times
// than expected fails loud.
type fixedRoller struct {
	values []int
	idx    int
}

func (f *fixedRoller) IntN(_ int) int {
	v := f.values[f.idx]
	f.idx++
	return v
}

func TestWeightedPick(t *testing.T) {
	row := []TransitionWeight{
		{NextState: "clear", Weight: 1},
		{NextState: "cloudy", Weight: 2},
		{NextState: "rain", Weight: 1},
	} // total weight 4
	cases := []struct {
		pick int
		want string
	}{
		{0, "clear"},
		{1, "cloudy"},
		{2, "cloudy"},
		{3, "rain"},
	}
	for _, tc := range cases {
		got := weightedPick(&fixedRoller{values: []int{tc.pick}}, row)
		if got != tc.want {
			t.Errorf("pick=%d → %q, want %q", tc.pick, got, tc.want)
		}
	}
}

func TestWeightedPick_EmptyAndZeroWeights(t *testing.T) {
	if got := weightedPick(&fixedRoller{values: []int{0}}, nil); got != "" {
		t.Errorf("empty row = %q, want empty", got)
	}
	zero := []TransitionWeight{{NextState: "x", Weight: 0}}
	if got := weightedPick(&fixedRoller{values: []int{0}}, zero); got != "" {
		t.Errorf("all-zero weights = %q, want empty", got)
	}
	if got := weightedPick(nil, []TransitionWeight{{NextState: "x", Weight: 1}}); got != "" {
		t.Errorf("nil roller = %q, want empty", got)
	}
}
