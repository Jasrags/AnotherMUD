package weather

import (
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Terrain classifiers the engine knows about. The canonical
// definitions live in package world (world.TerrainOf / IsShielded)
// now that light-and-darkness also keys off terrain; these aliases
// preserve weather's existing public surface (and its content tests)
// while pointing at the one shared source of truth. Spec §6.4 default:
// empty terrain → outdoors; `indoors`/`underground` shield from
// sky-driven ambience unless the matching exposure flag is set.
const (
	TerrainOutdoors    = world.TerrainOutdoors
	TerrainIndoors     = world.TerrainIndoors
	TerrainUnderground = world.TerrainUnderground
)

// terrainOf delegates to the world classifier so the weather cascade and
// the light sky-gate agree on the empty → outdoors default.
func terrainOf(r *world.Room) string { return world.TerrainOf(r) }

// ShieldingFunc reports a terrain's weather/time shielding from the biome
// registry (biomes.md §3). The terrain argument is a room terrain string;
// the production resolver self-normalizes (empty → outdoors), so callers
// may pass either a TerrainOf-normalized value or a raw room.Terrain.
// ok=false means the terrain has no registered biome — the caller falls
// back to the engine's hardcoded shielding set (indoors/underground), so
// unregistered terrain and a nil resolver (tests / no-biomes build) behave
// exactly as before this feature.
type ShieldingFunc func(terrain string) (weatherShielded, timeShielded bool, ok bool)

// weatherShieldedRoom / timeShieldedRoom resolve a room's per-axis
// shielding (biomes.md §3). A registered biome's flag wins; otherwise the
// hardcoded indoors/underground set applies (the §6.4 pre-biomes default).
// The two engine-baseline shielding biomes carry both flags, so wiring the
// resolver changes nothing for existing terrain — it only lets a content
// biome (a canopy, a sealed vault) declare its own shielding.
func weatherShieldedRoom(r *world.Room, fn ShieldingFunc) bool {
	t := terrainOf(r) // shared normalizer — same source the message cascade uses
	if fn != nil {
		if ws, _, ok := fn(t); ok {
			return ws
		}
	}
	return t == TerrainIndoors || t == TerrainUnderground
}

func timeShieldedRoom(r *world.Room, fn ShieldingFunc) bool {
	t := terrainOf(r)
	if fn != nil {
		if _, ts, ok := fn(t); ok {
			return ts
		}
	}
	return t == TerrainIndoors || t == TerrainUnderground
}

// weatherEligible implements §6.4 for the weather path. An
// unshielded room is always eligible; a shielded room needs
// WeatherExposed=true. fn resolves biome shielding (§3); nil = hardcoded.
func weatherEligible(r *world.Room, fn ShieldingFunc) bool {
	if r == nil {
		return false
	}
	if !weatherShieldedRoom(r, fn) {
		return true
	}
	return r.WeatherExposed
}

// timeEligible mirrors weatherEligible for the time-period path, keyed on
// the biome's time-shielded flag.
func timeEligible(r *world.Room, fn ShieldingFunc) bool {
	if r == nil {
		return false
	}
	if !timeShieldedRoom(r, fn) {
		return true
	}
	return r.TimeExposed
}

// resolveWeatherMessage walks the §6.3 cascade for one weather
// state. Order: room override → area override → zone default by
// terrain. Empty room/area override maps fall through to the next
// layer. The first layer to yield a non-empty triple wins; an
// empty zone entry (or nil zone) yields a zero MessageTriple
// (every field empty), which the dispatcher then skips.
//
// M15.4a wires only the zone-by-terrain layer because Room and
// Area don't yet carry the per-state override maps the spec
// describes — those land alongside the YAML loader extension in
// M15.4b. The cascade is structured here so M15.4b adds two
// `if room.Weather...; if area.Weather...` lookups at the top
// without disturbing the zone fallback.
func resolveWeatherMessage(room *world.Room, zone *Zone, state string) MessageTriple {
	if zone == nil || state == "" {
		return MessageTriple{}
	}
	terrainTable, ok := zone.WeatherMessages[state]
	if !ok || terrainTable == nil {
		return MessageTriple{}
	}
	if t, ok := terrainTable[terrainOf(room)]; ok {
		return t
	}
	// Per spec §6.3 the terrain dimension of the zone layer is
	// the fallback; if the room's terrain has no entry, fall
	// back to the engine default terrain (outdoors). This keeps
	// authors from having to copy the outdoor message under
	// every terrain key in the common case.
	if t, ok := terrainTable[TerrainOutdoors]; ok {
		return t
	}
	return MessageTriple{}
}

// resolveTimeMessage mirrors resolveWeatherMessage for time-of-
// day periods. Single-string return (no triple — time periods
// are one-shot).
func resolveTimeMessage(room *world.Room, zone *Zone, period string) string {
	if zone == nil || period == "" {
		return ""
	}
	terrainTable, ok := zone.TimeMessages[period]
	if !ok || terrainTable == nil {
		return ""
	}
	if s, ok := terrainTable[terrainOf(room)]; ok {
		return s
	}
	if s, ok := terrainTable[TerrainOutdoors]; ok {
		return s
	}
	return ""
}

// weightedPick chooses a NextState from row by weight (spec §6.2
// step 3). Returns "" when row is empty or every weight is non-
// positive — the caller treats that as a no-op.
//
// Algorithm: sum positive weights, draw IntN(total), walk and
// stop. O(n) per pick; n is small (a typical row has ≤4 entries).
func weightedPick(r Roller, row []TransitionWeight) string {
	total := 0
	for _, w := range row {
		if w.Weight > 0 {
			total += w.Weight
		}
	}
	if total <= 0 || r == nil {
		return ""
	}
	pick := r.IntN(total)
	for _, w := range row {
		if w.Weight <= 0 {
			continue
		}
		if pick < w.Weight {
			return w.NextState
		}
		pick -= w.Weight
	}
	// Unreachable: pick < total and weights sum to total.
	return ""
}
