package economy

import (
	"context"
	"sync"
)

// Rest is the M11.4 rest-state machine (spec economy-survival §5): a
// transient per-entity state (awake / resting / sleeping) with a
// per-state regen multiplier other features compose with the sustenance
// multiplier (§4.3). Unlike sustenance, rest DOES emit an observable
// event — a cancellable entity.rest_state.changed pre-event on the
// player-initiated path (§5.3 step 3) and the same event as an
// informational notification on the combat-wake path (§5.4) — so this
// service carries a Sink, bridged to the bus at the composition root.
//
// Rest state is transient: it never persists (§5.1). A disconnect that
// leaves an entity resting/sleeping restores as awake on next login,
// which the session layer gets for free by not persisting the field.

// RestState is one of the three §5.1 states. The string values are the
// stable wire names (event payloads, render strings).
type RestState string

const (
	// StateAwake is the default (also when the property is unset).
	StateAwake RestState = "awake"
	// StateResting is seated / lying on furniture.
	StateResting RestState = "resting"
	// StateSleeping is deep rest.
	StateSleeping RestState = "sleeping"
)

// IsRestState reports whether s is one of the three known states. Used
// to reject a malformed transition request defensively.
func IsRestState(s RestState) bool {
	switch s {
	case StateAwake, StateResting, StateSleeping:
		return true
	default:
		return false
	}
}

// normalizeRestState maps the empty/unset value to awake (§5.1 — "also
// when the property is unset").
func normalizeRestState(s string) RestState {
	if s == "" {
		return StateAwake
	}
	return RestState(s)
}

// RestConfig holds the spec §5.5/§5.6 parameters: the per-state regen
// multipliers and the well-rested sleep threshold.
type RestConfig struct {
	// RestingMultiplier / SleepingMultiplier scale regen for those
	// states (§5.5 defaults 2.0 / 3.0). Awake (and any other state) is
	// always 1.0 and is not configurable.
	RestingMultiplier  float64
	SleepingMultiplier float64
	// MinSleepTicksForWellRested is the minimum sleep duration that
	// qualifies for a "well-rested" bonus (§5.6 default 120). The rest
	// feature only records the sleep-start tick; the bonus logic lives
	// wherever it is applied (M11.5 regen).
	MinSleepTicksForWellRested uint64
}

// DefaultRestConfig returns the spec §5 documented defaults.
func DefaultRestConfig() RestConfig {
	return RestConfig{
		RestingMultiplier:          2.0,
		SleepingMultiplier:         3.0,
		MinSleepTicksForWellRested: 120,
	}
}

// GetRestMultiplier returns the regen scalar for state (§5.5).
// Resting → RestingMultiplier, sleeping → SleepingMultiplier, anything
// else (awake / unset) → 1.0. The regen-driving feature composes this
// with the sustenance multiplier, typically by multiplying.
func (c RestConfig) GetRestMultiplier(state RestState) float64 {
	switch state {
	case StateResting:
		return c.RestingMultiplier
	case StateSleeping:
		return c.SleepingMultiplier
	default:
		return 1.0
	}
}

// RestEntity is the holder a RestService reads and writes rest state on
// (spec §5.1/§5.2 — the state and its auxiliary fields live directly on
// the holder, all transient). The connActor satisfies it. Setters are
// only ever called by the service; implementations own their locking so
// the combat-wake (tick goroutine) and a rest verb (command goroutine)
// don't race.
type RestEntity interface {
	// ID returns the stable identity (bare id, engine convention).
	ID() string
	// RestState returns the current state ("" == awake).
	RestState() string
	// SetRestState writes the transient state field.
	SetRestState(state string)
	// SetRestTarget sets (or clears, with "") the furniture id being
	// rested on (§5.2).
	SetRestTarget(furnitureID string)
	// SetSleepStart records the tick sleeping began (§5.2), consulted
	// by the M11.5 well-rested credit.
	SetSleepStart(tick uint64)
}

// RestSink is the event seam, mirroring ShopSink: the composition root
// bridges it to the bus. OnRestStateChange fires the cancellable
// entity.rest_state.changed pre-event and returns whether a listener
// vetoed it. The combat-wake path calls it too but ignores the veto
// (§5.4 — you don't get to refuse to wake up).
type RestSink interface {
	// OnRestStateChange publishes the cancellable change event and
	// returns cancelled (true == a listener vetoed). reason is empty on
	// the player-initiated path and "combat" on the wake path.
	OnRestStateChange(ctx context.Context, entityID string, oldState, newState RestState, reason string) (cancelled bool)
}

// nopRestSink discards every event and never cancels. Default when
// NewRestService receives a nil sink (keeps tests free of bus
// boilerplate).
type nopRestSink struct{}

func (nopRestSink) OnRestStateChange(context.Context, string, RestState, RestState, string) bool {
	return false
}

// RestService owns the spec §5.3/§5.4 operations. The mutex makes the
// read-current-state → publish → write sequence atomic against a
// concurrent transition on the same entity (a rest verb racing the
// combat-wake tick). The sink fires INSIDE the lock here — unlike
// currency — because the cancellable result gates the write, so the
// decision and the write must not be split by another transition; the
// bridge's PublishCancellable is synchronous and must not re-enter
// SetRestState / ForceAwake (mirrors the AlignmentSink re-entrancy
// contract).
type RestService struct {
	mu   sync.Mutex
	cfg  RestConfig
	sink RestSink
	// now returns the current engine tick for the sleep-start stamp
	// (§5.2). Supplied by the composition root (loop.TickCount). Nil is
	// tolerated as "tick 0" so tests need no clock.
	now func() uint64
}

// NewRestService returns a service over cfg. A nil sink becomes a nop;
// a nil now becomes a zero-tick source.
func NewRestService(cfg RestConfig, sink RestSink, now func() uint64) *RestService {
	if sink == nil {
		sink = nopRestSink{}
	}
	if now == nil {
		now = func() uint64 { return 0 }
	}
	return &RestService{cfg: cfg, sink: sink, now: now}
}

// Config returns the service's config so callers holding only the
// service can read the well-rested threshold / multipliers.
func (s *RestService) Config() RestConfig { return s.cfg }

// GetRestMultiplier exposes the config derivation on the service.
func (s *RestService) GetRestMultiplier(state RestState) float64 {
	return s.cfg.GetRestMultiplier(state)
}

// State returns the entity's current rest state, awake for a nil
// entity or unset field.
func (s *RestService) State(e RestEntity) RestState {
	if e == nil {
		return StateAwake
	}
	return normalizeRestState(e.RestState())
}

// SetRestState applies the spec §5.3 transition. Returns (ok, reason):
//
//   - (false, "entity_not_found") — nil entity.
//   - (false, "invalid_state")    — newState is not a known state.
//   - (false, "already_in_state") — entity is already in newState.
//   - (false, "cancelled")        — a listener vetoed the change event.
//   - (true, "")                  — applied.
//
// On success: the new state is written; a transition to awake clears
// the rest target; a transition to resting/sleeping with a non-empty
// furnitureID sets the rest target; a transition to sleeping records
// the current tick as the sleep start.
func (s *RestService) SetRestState(ctx context.Context, e RestEntity, newState RestState, furnitureID string) (bool, string) {
	if e == nil {
		return false, "entity_not_found"
	}
	if !IsRestState(newState) {
		return false, "invalid_state"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := normalizeRestState(e.RestState())
	if cur == newState {
		return false, "already_in_state"
	}
	if s.sink.OnRestStateChange(ctx, e.ID(), cur, newState, "") {
		return false, "cancelled"
	}
	s.applyLocked(e, newState, furnitureID)
	return true, ""
}

// ForceAwake forces the entity to awake, bypassing the cancellable
// check (spec §5.4 combat wake). Returns true if the entity was
// actually resting/sleeping (and thus woken); false if it was already
// awake (no event fired). The change event is published with the given
// reason (typically "combat") as an informational notification — the
// veto result is discarded.
func (s *RestService) ForceAwake(ctx context.Context, e RestEntity, reason string) bool {
	if e == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := normalizeRestState(e.RestState())
	if cur == StateAwake {
		return false
	}
	// Fire the event BEFORE applying, mirroring SetRestState's
	// pending-change semantics: a listener inspecting the entity during
	// the event sees the still-current (resting/sleeping) state with the
	// new state in the payload. The veto result is ignored — combat wake
	// is not refusable (spec §5.4).
	_ = s.sink.OnRestStateChange(ctx, e.ID(), cur, StateAwake, reason)
	s.applyLocked(e, StateAwake, "")
	return true
}

// applyLocked writes the new state and its auxiliary effects (§5.3 step
// 5). Caller holds s.mu.
func (s *RestService) applyLocked(e RestEntity, newState RestState, furnitureID string) {
	e.SetRestState(string(newState))
	switch newState {
	case StateAwake:
		e.SetRestTarget("")
	case StateResting:
		if furnitureID != "" {
			e.SetRestTarget(furnitureID)
		}
	case StateSleeping:
		if furnitureID != "" {
			e.SetRestTarget(furnitureID)
		}
		e.SetSleepStart(s.now())
	}
}
