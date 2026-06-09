package weather

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Roller is the random-pick surface the weighted transition table
// consumes. math/rand/v2.Rand.IntN satisfies it (mirrors
// combat.Roller — same single-method shape, same panic-on-n<=0
// contract).
type Roller interface {
	IntN(n int) int
}

// Broadcaster is the per-room delivery surface — same shape as
// command.Broadcaster but redeclared here so the weather package
// doesn't import command. session.Manager satisfies both.
type Broadcaster interface {
	SendToRoom(ctx context.Context, roomID world.RoomID, text string, excludePlayerIDs ...string)
}

// WorldRooms is the subset of *world.World the service uses to
// enumerate the rooms of an area for ambience delivery (spec §6.2
// step 7, §6.5) and to resolve an area's zone in O(1) for
// Ambience reads. Defined as an interface here so test fixtures
// don't have to build a real *world.World.
type WorldRooms interface {
	RoomsInArea(id world.AreaID) []*world.Room
	Areas() []*world.Area
	Area(id world.AreaID) (*world.Area, error)
}

// Config wires the Service at composition time. Bus and
// Broadcaster are optional (nil-safe): a tests-only Service that
// only wants to exercise state-machine transitions can leave both
// nil and inspect CurrentWeather directly.
type Config struct {
	Registry    *Registry
	World       WorldRooms
	Bus         *eventbus.Bus
	Broadcaster Broadcaster
	Roller      Roller
	// Shielding resolves a room's weather/time shielding from the biome
	// registry (biomes.md §3), generalizing the hardcoded §6.4
	// indoors/underground set. nil-tolerant: when nil (tests, or a build
	// without biomes), the eligibility check falls back to the hardcoded
	// shielding terrains, preserving pre-biomes behavior exactly.
	Shielding ShieldingFunc
}

// Service is the runtime state holder + dispatcher (spec §6).
//
// Single-writer model: HourChanged + PeriodChanged + SetWeather
// all serialize on s.mu for the state mutation, then release the
// lock before broadcasting (broadcaster I/O must not stall the
// tick). Concurrent callers are safe but the expected topology is
// one driver (the future hour-tick handler).
//
// Consistency contract: weather.changed describes the transition
// that was canonical at the moment s.mu was released. If two
// drivers mutate the same area concurrently (e.g. an admin
// SetWeather races the hour-tick roll), subscribers may observe
// events whose NewState is no longer the current state by the
// time the handler runs. Subscribers should be transition-aware
// (react to "what just changed") rather than absolute-state-aware
// (read CurrentWeather inside the handler when truth matters).
type Service struct {
	cfg Config

	mu     sync.Mutex
	states map[world.AreaID]string // current weather state per area
}

// New builds a Service. Registry MUST be non-nil; the other
// Config fields are nil-tolerant as documented above. World may
// be nil when the caller only intends to drive CurrentWeather /
// SetWeather (no area enumeration needed); HourChanged with a
// nil World is a no-op.
func New(cfg Config) *Service {
	if cfg.Registry == nil {
		panic("weather.New: nil Registry")
	}
	return &Service{cfg: cfg, states: make(map[world.AreaID]string)}
}

// CurrentWeather returns the area's current state, defaulting to
// the configured zone's initial state (or DefaultWeatherState
// when no zone) when the service has never rolled for this area.
//
// Lock-free for the common case (state already set) would require
// duplicating the fallback chain; the cost of taking s.mu here is
// negligible and the read is rare (rendering / queries).
//
// Spec: §6.6.
func (s *Service) CurrentWeather(areaID world.AreaID) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.states[areaID]; ok {
		return v
	}
	return s.initialStateForLocked(areaID)
}

// SetWeather force-updates an area's weather state (spec §6.6:
// "It also exposes a setter used by content that needs to force
// weather"). Identical-state writes are no-ops and do NOT publish
// weather.changed (matches the §6.2 step 4 invariant — only real
// transitions are events). Unknown / empty area ids are accepted
// silently because content scripting may set weather on any area
// id the caller knows; validation belongs to the caller.
func (s *Service) SetWeather(ctx context.Context, areaID world.AreaID, state string) {
	if state == "" {
		state = DefaultWeatherState
	}
	s.mu.Lock()
	prev, present := s.states[areaID]
	if !present {
		prev = s.initialStateForLocked(areaID)
	}
	if prev == state {
		s.mu.Unlock()
		return
	}
	s.states[areaID] = state
	s.mu.Unlock()

	s.dispatchTransition(ctx, areaID, prev, state)
}

// HourChanged is the seam future time-and-clock §3.3 will call on
// every hour advancement (spec §6.2). The caller passes the new
// in-game hour; the service decides which areas roll based on
// each area's zone's RollIntervalHours.
//
// A nil World makes this a no-op (no areas to enumerate). A nil
// Roller falls back to "no-op roll" — the service can't pick a
// transition without randomness, so it skips rather than picking
// deterministically and pretending it rolled.
func (s *Service) HourChanged(ctx context.Context, hour int) {
	if s.cfg.World == nil {
		return
	}
	if s.cfg.Roller == nil {
		logging.From(ctx).Debug("weather.HourChanged: nil Roller, skipping rolls",
			slog.Int("hour", hour))
		return
	}
	for _, area := range s.cfg.World.Areas() {
		if area == nil || area.WeatherZone == "" {
			continue
		}
		zone, err := s.cfg.Registry.Get(area.WeatherZone)
		if err != nil {
			logging.From(ctx).Warn("weather.HourChanged: unknown zone on area",
				slog.String("area", string(area.ID)),
				slog.String("zone", area.WeatherZone))
			continue
		}
		// Spec §6.2 step 1 — only roll on the configured interval.
		// Hour 0 always rolls (covers fresh-boot first hour and the
		// day-wrap case identically).
		if hour%zone.rollInterval() != 0 {
			continue
		}
		s.rollArea(ctx, area, zone)
	}
}

// rollArea performs steps 2-7 of §6.2 for one area.
func (s *Service) rollArea(ctx context.Context, area *world.Area, zone *Zone) {
	s.mu.Lock()
	current, present := s.states[area.ID]
	if !present {
		current = zone.initialState()
	}
	row, ok := zone.Transitions[current]
	if !ok || len(row) == 0 {
		s.mu.Unlock()
		return
	}
	next := weightedPick(s.cfg.Roller, row)
	if next == "" || next == current {
		s.mu.Unlock()
		return
	}
	s.states[area.ID] = next
	s.mu.Unlock()

	s.dispatchTransition(ctx, area.ID, current, next)
}

// dispatchTransition fires the weather.changed bus event and
// broadcasts the end-then-start messages to eligible rooms in the
// area (spec §6.2 steps 6-7). Centralised so SetWeather and the
// roll path share the same observable behavior.
func (s *Service) dispatchTransition(ctx context.Context, areaID world.AreaID, prev, next string) {
	if s.cfg.Bus != nil {
		s.cfg.Bus.Publish(ctx, eventbus.WeatherChanged{
			AreaID:        areaID,
			PreviousState: prev,
			NewState:      next,
		})
	}
	logging.From(ctx).Debug("weather.changed",
		slog.String("area", string(areaID)),
		slog.String("from", prev),
		slog.String("to", next))

	if s.cfg.World == nil || s.cfg.Broadcaster == nil {
		return
	}
	// Look up the zone once for the cascade fallbacks. Error
	// discarded by design: nil zone is handled downstream by
	// resolveWeatherMessage, which returns an empty triple that
	// the dispatcher then skips.
	zone, _ := s.cfg.Registry.Get(zoneIDForArea(s.cfg.World, areaID))
	for _, room := range s.cfg.World.RoomsInArea(areaID) {
		if room == nil || !weatherEligible(room, s.cfg.Shielding) {
			continue
		}
		end := resolveWeatherMessage(room, zone, prev).End
		if end != "" {
			s.cfg.Broadcaster.SendToRoom(ctx, room.ID, end)
		}
		start := resolveWeatherMessage(room, zone, next).Start
		if start != "" {
			s.cfg.Broadcaster.SendToRoom(ctx, room.ID, start)
		}
	}
}

// Ambience returns the resolved "ongoing" weather message for room
// — the per-look ambience line a renderer appends to the room
// description (spec world-rooms-movement §6.6 "ongoing" leg of the
// message triple).
//
// Returns "" when:
//   - room is nil, has no AreaID, or fails weather eligibility
//     (shielded terrain without WeatherExposed);
//   - the area has no weather_zone;
//   - the zone has no entry for the area's current state under the
//     room's terrain (and no outdoor fallback).
//
// Callers MUST tolerate "" and skip rendering — empty is the
// "nothing to say right now" signal, not an error. Safe for
// concurrent callers (the underlying state read takes s.mu only
// briefly via CurrentWeather; the cascade resolver is pure).
func (s *Service) Ambience(room *world.Room) string {
	if room == nil || room.AreaID == "" || !weatherEligible(room, s.cfg.Shielding) {
		return ""
	}
	zoneID := zoneIDForArea(s.cfg.World, room.AreaID)
	if zoneID == "" {
		return ""
	}
	zone, err := s.cfg.Registry.Get(zoneID)
	if err != nil || zone == nil {
		return ""
	}
	state := s.CurrentWeather(room.AreaID)
	return resolveWeatherMessage(room, zone, state).Ongoing
}

// PeriodChanged is the §6.5 seam future time-and-clock §3.4 will
// call on every period advancement. Delivers the period's
// resolved message to each eligible room in every area. Unlike
// HourChanged, period changes do NOT publish their own engine
// event from this feature (the time feature already emitted
// time.period.change — spec §6.5).
func (s *Service) PeriodChanged(ctx context.Context, period string) {
	if s.cfg.World == nil || s.cfg.Broadcaster == nil || period == "" {
		return
	}
	for _, area := range s.cfg.World.Areas() {
		if area == nil {
			continue
		}
		// Period messages cascade through the zone even when an
		// area has no weather (a calendar-only ambience zone is
		// legitimate). zone may be nil — resolveTimeMessage
		// handles that.
		var zone *Zone
		if area.WeatherZone != "" {
			// Error discarded by design: nil zone is handled
			// downstream by resolveTimeMessage (empty result →
			// dispatcher skips the room).
			zone, _ = s.cfg.Registry.Get(area.WeatherZone)
		}
		for _, room := range s.cfg.World.RoomsInArea(area.ID) {
			if room == nil || !timeEligible(room, s.cfg.Shielding) {
				continue
			}
			msg := resolveTimeMessage(room, zone, period)
			if msg == "" {
				continue
			}
			s.cfg.Broadcaster.SendToRoom(ctx, room.ID, msg)
		}
	}
}

// initialStateForLocked returns the area's initial state per its
// registered zone, defaulting to DefaultWeatherState when no zone
// is set or the zone is unknown. Caller MUST hold s.mu.
func (s *Service) initialStateForLocked(areaID world.AreaID) string {
	zoneID := zoneIDForArea(s.cfg.World, areaID)
	if zoneID == "" {
		return DefaultWeatherState
	}
	zone, err := s.cfg.Registry.Get(zoneID)
	if err != nil {
		return DefaultWeatherState
	}
	return zone.initialState()
}

// zoneIDForArea looks up the area's WeatherZone field via the
// WorldRooms abstraction. Returns "" when world is nil, the area
// is unknown, or the area declares no zone. O(1) lookup via the
// world's area index (closes the M15.4a-review O(n) deferral).
func zoneIDForArea(w WorldRooms, areaID world.AreaID) string {
	if w == nil {
		return ""
	}
	area, err := w.Area(areaID)
	if err != nil || area == nil {
		return ""
	}
	return area.WeatherZone
}
