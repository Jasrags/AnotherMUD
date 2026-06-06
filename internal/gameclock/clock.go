// Package gameclock is the in-game hour/day clock from
// docs/specs/time-and-clock.md §3.
//
// Distinct from internal/clock (the wall-clock abstraction) — this
// package owns the simulated time-of-day that content reasons about
// (mob nocturnal schedules, weather rolls, shop hours, sunrise /
// sunset ambience). The wall clock drives the tick loop; the tick
// loop drives this clock; this clock emits the time.hour.change and
// time.period.change events that downstream features subscribe to.
//
// M15.4b₁ scope: the Clock type + Tick driver + accessors + event
// emission. Composition-root wiring (registering Tick as the
// "game-clock" tick handler per spec §4.2) lands in M15.4b₂
// alongside the weather subscriber binding.
//
// Persistence: the state machine is seedable (Config.InitialHour /
// InitialDay) and exposes Snapshot for a caller to persist; the
// global on-disk store lives in store.go (light-and-darkness §7,
// resolving time-and-clock §3.6). Sub-hour position is not
// preserved — a restart resumes at the start of the saved hour.
package gameclock

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// Period names — lowercased per spec §3.3 / §3.4 ("the new period
// name, lowercased"). Exposed as constants so subscribers (weather
// service, content scripts) can compare without string-literal
// drift.
const (
	PeriodNight = "night"
	PeriodDawn  = "dawn"
	PeriodDay   = "day"
	PeriodDusk  = "dusk"
)

// DefaultPeriodBoundaries is the spec §3.2 default ordered array
// [dawn_start, day_start, dusk_start, night_start]. With these
// values night spans 20:00-04:59 (wrapping), dawn 5-7:59, day
// 8-17:59, dusk 18-19:59.
var DefaultPeriodBoundaries = [4]int{5, 8, 18, 20}

// DefaultTicksPerGameHour is the spec §7 configuration default —
// 600 ticks per in-game hour (at the default 100ms tick rate that
// is one in-game hour per minute of wall time).
const DefaultTicksPerGameHour = 600

// Config wires the Clock at composition time.
//
// TicksPerGameHour MUST be > 0; New panics on 0/negative because
// dividing by it every tick would loop forever or panic later.
// Period boundaries follow §3.2: strictly ascending, each in
// [0,23]. New does not validate (spec §3.2 "The clock does not
// validate them"); operators ship sane defaults.
//
// A zero-value PeriodBoundaries ([0,0,0,0]) is treated as
// "unset" and collapses to DefaultPeriodBoundaries. The
// theoretically-valid all-zero boundary set (which would read
// every hour past 0 as Night via the ≥night_start branch) is
// not expressible through Config — author intent is impossible
// to distinguish from a forgotten field. Callers that genuinely
// want a degenerate boundary set can override with e.g.
// [4]int{0, 0, 0, 1} which dodges the zero-check while
// preserving the same effective behavior.
//
// Bus is optional (nil-safe) so tests that only exercise the
// state machine can omit it.
//
// InitialHour / InitialDay seed the clock at boot from persisted
// time (light-and-darkness §7). Both default to zero (the spec
// §3.5 cold-start state). InitialHour outside [0,23] is clamped to
// 0 — New does not panic on it, because a corrupt saved value
// should degrade to a deterministic cold start, not crash the
// server. The period is computed from the seeded hour.
type Config struct {
	TicksPerGameHour int
	PeriodBoundaries [4]int
	Bus              *eventbus.Bus
	InitialHour      int
	InitialDay       uint64
}

// Clock is the in-game hour/day state machine (spec §3).
//
// Single-writer model: Tick is called from one goroutine (the
// tick loop's "game-clock" handler per spec §4.2). Accessors are
// safe for concurrent callers via mu — they're cheap reads
// (verbs, status lines, occasional weather queries), not hot.
type Clock struct {
	mu               sync.Mutex
	tickCount        uint64
	currentHour      int
	dayCount         uint64
	currentPeriod    string
	ticksPerGameHour int
	boundaries       [4]int
	bus              *eventbus.Bus
}

// New constructs a Clock from cfg, applying spec defaults for any
// zero-valued field. Panics on a non-positive TicksPerGameHour
// because runtime division by zero would be worse than failing
// fast at composition. Initial state: hour 0, day 0, period
// computed from hour 0 (defaults → Night).
func New(cfg Config) *Clock {
	tpg := cfg.TicksPerGameHour
	if tpg == 0 {
		tpg = DefaultTicksPerGameHour
	}
	if tpg < 0 {
		panic("gameclock.New: TicksPerGameHour must be positive")
	}
	bounds := cfg.PeriodBoundaries
	if bounds == ([4]int{}) {
		bounds = DefaultPeriodBoundaries
	}
	hour := cfg.InitialHour
	if hour < 0 || hour > 23 {
		hour = 0
	}
	c := &Clock{
		currentHour:      hour,
		dayCount:         cfg.InitialDay,
		ticksPerGameHour: tpg,
		boundaries:       bounds,
		bus:              cfg.Bus,
	}
	c.currentPeriod = periodFor(hour, bounds)
	return c
}

// Tick advances the internal counter (spec §3.1 step 1) and, on
// every TicksPerGameHour-th call, performs an hour advance with
// the §3.1 step-3 publish sequence.
//
// Production topology is single-writer: the spec §4.2 "game-clock"
// tick handler is the sole caller. The mutex still protects
// against accidental concurrent callers (the cadence and event
// emission stay correct if two goroutines race the Tick call),
// but no production path exercises that fallback.
//
// Returns true when the call resulted in an hour advance, false
// otherwise. The bool is for tests and observability hooks; the
// production tick handler ignores it.
func (c *Clock) Tick(ctx context.Context) bool {
	c.mu.Lock()
	c.tickCount++
	if c.tickCount%uint64(c.ticksPerGameHour) != 0 {
		c.mu.Unlock()
		return false
	}
	prevPeriod := c.currentPeriod
	c.currentHour++
	if c.currentHour > 23 {
		c.currentHour = 0
		c.dayCount++
	}
	c.currentPeriod = periodFor(c.currentHour, c.boundaries)
	// Snapshot for publishing outside the lock — the bus must
	// never be called while holding c.mu (subscribers may need
	// to read accessors, which would self-deadlock).
	hour := c.currentHour
	period := c.currentPeriod
	day := c.dayCount
	periodChanged := period != prevPeriod
	c.mu.Unlock()

	if c.bus != nil {
		if periodChanged {
			// Spec §3.1: period-change event fires BEFORE the
			// hour-change event so a subscriber that wants to
			// react to the period transition before any
			// hour-cadence work (weather roll) sees them in
			// "more specific first" order. The spec text says
			// "If the new period differs from the captured
			// one, emit a time.period.change event. Always
			// emit a time.hour.change event." — listing
			// period first.
			c.bus.Publish(ctx, eventbus.TimePeriodChange{
				Period:         period,
				PreviousPeriod: prevPeriod,
				Hour:           hour,
			})
		}
		c.bus.Publish(ctx, eventbus.TimeHourChange{
			Hour:     hour,
			Period:   period,
			DayCount: day,
		})
	}
	logging.From(ctx).Debug("gameclock.hour_advance",
		slog.Int("hour", hour),
		slog.String("period", period),
		slog.Uint64("day", day),
		slog.Bool("period_change", periodChanged))
	return true
}

// CurrentHour returns the current in-game hour [0,23].
func (c *Clock) CurrentHour() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentHour
}

// DayCount returns the number of midnight wraps since boot.
func (c *Clock) DayCount() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dayCount
}

// Snapshot returns the persistable time state (hour + day) under a
// single lock, so a caller writing it to disk gets a consistent
// pair rather than racing two accessors. Sub-hour position is
// deliberately not included (light-and-darkness §7).
func (c *Clock) Snapshot() SavedTime {
	c.mu.Lock()
	defer c.mu.Unlock()
	return SavedTime{CurrentHour: c.currentHour, DayCount: c.dayCount}
}

// CurrentPeriod returns the lowercased period name covering the
// current hour (one of PeriodNight / PeriodDawn / PeriodDay /
// PeriodDusk under the default boundary set).
func (c *Clock) CurrentPeriod() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentPeriod
}

// TickCount returns the internal raw-tick counter — useful for
// tests that want to confirm the Tick driver was actually called
// the right number of times.
func (c *Clock) TickCount() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tickCount
}

// periodFor implements the spec §3.2 top-down lookup:
//
//  1. hour ≥ night_start → Night
//  2. hour ≥ dusk_start  → Dusk
//  3. hour ≥ day_start   → Day
//  4. hour ≥ dawn_start  → Dawn
//  5. else                → Night (pre-dawn hours)
//
// The "else → Night" branch is what makes hour 0 fall into
// Night under the default boundaries (0 < dawn_start=5).
func periodFor(hour int, b [4]int) string {
	switch {
	case hour >= b[3]:
		return PeriodNight
	case hour >= b[2]:
		return PeriodDusk
	case hour >= b[1]:
		return PeriodDay
	case hour >= b[0]:
		return PeriodDawn
	default:
		return PeriodNight
	}
}
