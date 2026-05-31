// Package weather is the M15.4a substrate for area-scoped weather
// state machines and ambience-message delivery.
//
// Spec: docs/specs/world-rooms-movement.md §6.
//
// The package ships three things:
//
//   - Zone: a content-defined climate definition — its transition
//     table (currentState → weighted nextState distribution) plus
//     the per-terrain ambience tables (weather start/ongoing/end
//     triples; time-period one-shot lines).
//   - Registry: zone id → *Zone lookup populated at composition.
//   - Service: the runtime that holds per-area current state and
//     exposes the seams the future in-game clock will call
//     (HourChanged, PeriodChanged) along with the spec §6.6
//     query/setter pair.
//
// M15.4a is intentionally callable-only — nothing in the
// composition root subscribes the Service to the (also deferred)
// time.hour.change / time.period.change events. M15.4b lands the
// wiring when the in-game clock implementation lands. Until then
// tests drive the Service directly.
package weather

// MessageTriple is the (start, ongoing, end) message set for one
// weather state. Any field may be empty: empty messages are simply
// not sent (spec §6.2 step 7 "absent messages are simply not
// sent").
//
//   - Start is broadcast to eligible rooms when the area transitions
//     INTO this state.
//   - End is broadcast to eligible rooms when the area transitions
//     OUT of this state (delivered on the next state's hour roll).
//   - Ongoing is reserved for the future render-hook layer (spec
//     §6.6 — "what's the room's current ambience?"); the M15.4a
//     dispatch path does not deliver it.
type MessageTriple struct {
	Start   string
	Ongoing string
	End     string
}

// TransitionWeight is one outcome in a per-state transition row.
// Weight MUST be > 0; the weighted pick treats the table as the
// full probability distribution (no implicit "stay in current
// state" — content authors who want that include the current
// state explicitly in the row).
type TransitionWeight struct {
	NextState string
	Weight    int
}

// Zone is one named climate definition (spec §6).
//
// Transitions maps current state → weighted next-state distribution.
// A current state missing from the table is a no-op for the roll —
// the area keeps its current state (spec §6.2 step 3).
//
// WeatherMessages maps state → terrain → MessageTriple. The
// outer key is the weather state name (matching a key in
// Transitions); the inner key is the terrain string the room
// carries (`outdoors`, `forest`, etc.). The resolution cascade
// (spec §6.3) reads this as the third fallback after room and
// area overrides.
//
// TimeMessages mirrors WeatherMessages for time-of-day periods
// (`dawn`, `midday`, …); the value is a single string, not a
// triple — time periods are one-shot transitions.
//
// Both maps may be partial: an absent state/terrain entry resolves
// to the empty message (skipped by the dispatcher).
//
// InitialState is the area's state before the first roll. Empty
// defaults to `clear` (spec §6.6).
//
// RollIntervalHours is the spec §6.2 "every N in-game hours"
// cadence. Zero defaults to 1 (roll every hour). Negative values
// are invalid; loaders should reject them.
type Zone struct {
	ID                string
	InitialState      string
	RollIntervalHours int
	Transitions       map[string][]TransitionWeight
	WeatherMessages   map[string]map[string]MessageTriple
	TimeMessages      map[string]map[string]string
}

// initialState returns the zone's declared InitialState or the
// engine default (`clear` per spec §6.6) when unset.
func (z *Zone) initialState() string {
	if z == nil || z.InitialState == "" {
		return DefaultWeatherState
	}
	return z.InitialState
}

// rollInterval returns the zone's declared cadence or 1 when
// unset (every hour). Negative values are clamped to 1 — invalid
// configurations should have been rejected upstream, but the
// service refuses to divide-by-zero or roll on impossible
// intervals at runtime.
func (z *Zone) rollInterval() int {
	if z == nil || z.RollIntervalHours <= 0 {
		return 1
	}
	return z.RollIntervalHours
}

// DefaultWeatherState is the spec §6.6 default for any area whose
// zone declares no InitialState.
const DefaultWeatherState = "clear"
