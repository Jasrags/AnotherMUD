package light

import "github.com/Jasrags/AnotherMUD/internal/world"

// PeriodSource is the slice of the in-game clock the resolver needs:
// the current time-of-day period name. *gameclock.Clock satisfies it.
// Kept as a one-method interface so light does not depend on the
// concrete clock and tests can supply a fixed period.
type PeriodSource interface {
	CurrentPeriod() string
}

// Resolver is the composed light surface: it pairs the tunable Config
// with a period source and gathers the per-viewer Inputs that Resolve
// consumes. This is the seam the render, combat, and movement call
// sites use — they hand it the room plus the two per-viewer terms they
// own (lit-source contribution and viewer floor) and get back an
// effective Level.
type Resolver struct {
	cfg   Config
	clock PeriodSource
}

// NewResolver builds a Resolver from cfg and a period source. The
// clock may be nil (tests that pin behavior to overrides/terrain);
// a nil clock resolves the period to "" which AmbientFor floors at
// Gloom.
func NewResolver(cfg Config, clock PeriodSource) *Resolver {
	return &Resolver{cfg: cfg, clock: clock}
}

// Config exposes the resolver's policy so call sites can read tunables
// (e.g. the combat penalty table, examination minimum) without a
// second copy.
func (r *Resolver) Config() Config { return r.cfg }

// Period returns the clock's current time-of-day period name (the
// `daylight` probe reports it), or "" when no clock is wired.
func (r *Resolver) Period() string {
	if r.clock == nil {
		return ""
	}
	return r.clock.CurrentPeriod()
}

// Effective computes the effective light level for viewing room.
//
//   - sources is the best level contributed by lit sources for this
//     viewer (their held light + luminous items/mobs in the room);
//     Black when nothing is lit. Wired in Phase 3.
//   - viewerFloor is this viewer's darkvision/sight-effect floor;
//     Black for an ordinary viewer. Wired in Phase 4.
//
// Until those phases land, call sites pass Black for both and get the
// ambient + terrain + override result, which is the correct partial
// behavior.
func (r *Resolver) Effective(room *world.Room, sources, viewerFloor Level) Level {
	period := ""
	if r.clock != nil {
		period = r.clock.CurrentPeriod()
	}
	return r.EffectiveForPeriod(room, sources, viewerFloor, period)
}

// EffectiveForPeriod is Effective with an explicit time-of-day period
// instead of the clock's current one. The §6 transition driver uses it
// to compute a viewer's level under the previous period vs. the new one
// and message only when the level crosses. All non-ambient terms
// (sources, override, terrain, viewer floor) are period-independent, so
// the period is the only thing that differs between the two calls.
func (r *Resolver) EffectiveForPeriod(room *world.Room, sources, viewerFloor Level, period string) Level {
	var override *Level
	if lvl, ok := OverrideFor(room); ok {
		override = &lvl
	}
	return Resolve(Inputs{
		Ambient:     r.cfg.AmbientFor(period),
		Terrain:     world.TerrainOf(room),
		IndoorCap:   r.cfg.IndoorCap,
		Override:    override,
		Sources:     sources,
		ViewerFloor: viewerFloor,
	})
}
