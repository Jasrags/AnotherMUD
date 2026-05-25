package spawn

import (
	"context"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// PresenceSource is the seam the scheduler uses to count players per
// area. session.Manager satisfies it via an adapter at the
// composition root (cmd/anothermud).
type PresenceSource interface {
	PlayerCountInArea(areaID world.AreaID) int
}

// Scheduler tracks per-area "ticks since last reset" counters and
// emits eventbus.AreaTick when each area's cadence elapses (spec
// mobs-ai-spawning §3.7).
//
// The cadence is `baseInterval × occupiedModifier` where the
// modifier defaults to 1.0 and applies only when an area has at
// least one player present. A modifier > 1 slows resets in
// occupied areas (good for difficulty cycles); a modifier < 1
// speeds them up (good for player-driven repop).
//
// The scheduler is wall-clock-agnostic: it ticks via Step(), called
// by the tick.Loop handler at a fixed cadence. Per-area state
// accumulates Step calls and fires when the accumulator crosses the
// area's effective cadence.
type Scheduler struct {
	world    *world.World
	bus      *eventbus.Bus
	presence PresenceSource

	mu           sync.Mutex
	occupiedMod  float64
	areaMod      map[world.AreaID]float64 // per-area override; 0 → use occupiedMod
	defaultReset uint64                   // engine default in ticks; applies when an area's ResetInterval is 0
	state        map[world.AreaID]*areaState
}

// areaState carries per-area scheduler counters. Lives behind the
// Scheduler.mu lock.
type areaState struct {
	accum     uint64 // step counter since last fire
	tickCount uint64 // monotonic AreaTick count (carried in the event)
}

// SchedulerConfig is the constructor input bundle. DefaultReset is
// required: areas with ResetInterval=0 fall back to this value.
// OccupiedModifier defaults to 1.0 when zero.
type SchedulerConfig struct {
	World            *world.World
	Bus              *eventbus.Bus
	Presence         PresenceSource
	DefaultReset     uint64
	OccupiedModifier float64
}

// NewScheduler returns a configured scheduler. State is empty until
// Step starts populating it on demand.
func NewScheduler(cfg SchedulerConfig) *Scheduler {
	mod := cfg.OccupiedModifier
	if mod <= 0 {
		mod = 1.0
	}
	return &Scheduler{
		world:        cfg.World,
		bus:          cfg.Bus,
		presence:     cfg.Presence,
		occupiedMod:  mod,
		areaMod:      make(map[world.AreaID]float64),
		defaultReset: cfg.DefaultReset,
		state:        make(map[world.AreaID]*areaState),
	}
}

// SetAreaOccupiedModifier overrides the occupied modifier for a
// specific area (spec §3.7: "The modifier MAY be overridden at
// runtime per area"). Pass 0 to clear the override.
func (s *Scheduler) SetAreaOccupiedModifier(area world.AreaID, mod float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mod <= 0 {
		delete(s.areaMod, area)
		return
	}
	s.areaMod[area] = mod
}

// Step advances every area's accumulator by deltaTicks game ticks
// and emits AreaTick events for any area whose accumulator crossed
// its effective cadence. Designed to be the body of a single
// tick.Loop handler; deltaTicks should match the handler's
// registration cadence so accum stays in game-tick units (the same
// units Area.ResetInterval uses).
//
// Per-rule ResetInterval (SpawnRule.ResetInterval) is accepted by
// the loader but NOT yet honored here — Step fires one AreaTick per
// area at the area's cadence, and the manager runs every rule on
// each tick. Per-rule cadence override requires either a per-rule
// accumulator + per-rule event, or the manager filtering rules by
// elapsed time. Deferred to a follow-on slice; see
// m6-6-deferred-fixes.md.
func (s *Scheduler) Step(ctx context.Context, deltaTicks uint64) {
	if s.world == nil || deltaTicks == 0 {
		return
	}
	for _, area := range s.world.Areas() {
		if len(area.SpawnRules) == 0 {
			continue
		}
		base := area.ResetInterval
		if base == 0 {
			base = s.defaultReset
		}
		if base == 0 {
			// Area is intentionally unscheduled — config wants
			// area.tick events delivered manually only.
			continue
		}
		occupants := 0
		if s.presence != nil {
			occupants = s.presence.PlayerCountInArea(area.ID)
		}
		// Snapshot the per-area modifier under the lock, then compute
		// cadence without holding any locks. Keeping cadence math
		// outside the inner state-update critical section avoids any
		// future risk of re-entering s.mu (the helper is now pure
		// and arithmetic-only).
		mod := s.modifierFor(area.ID, occupants)
		cadence := effectiveCadence(base, mod)

		s.mu.Lock()
		st, ok := s.state[area.ID]
		if !ok {
			st = &areaState{}
			s.state[area.ID] = st
		}
		st.accum += deltaTicks
		fire := st.accum >= cadence
		if fire {
			st.accum = 0
			st.tickCount++
		}
		tickNum := st.tickCount
		s.mu.Unlock()

		if !fire {
			continue
		}
		if s.bus != nil {
			s.bus.Publish(ctx, eventbus.AreaTick{
				AreaID:      area.ID,
				TickCount:   tickNum,
				PlayerCount: occupants,
			})
		}
	}
}

// modifierFor returns the occupied modifier that applies to `area`
// for the current step. Returns 1.0 (i.e. no scaling) when the area
// is empty (spec §3.7: "applies only when the area has at least one
// player present"). The locked section is small and serves only to
// snapshot the global + per-area override maps.
func (s *Scheduler) modifierFor(area world.AreaID, occupants int) float64 {
	if occupants == 0 {
		return 1.0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if over, ok := s.areaMod[area]; ok {
		return over
	}
	return s.occupiedMod
}

// effectiveCadence is pure: `base × mod`, floored at 1 so any
// modifier below `1/base` doesn't fire every step. Extracted from
// modifierFor so the arithmetic stays lock-free and easy to test.
func effectiveCadence(base uint64, mod float64) uint64 {
	scaled := float64(base) * mod
	if scaled < 1 {
		return 1
	}
	return uint64(scaled)
}
