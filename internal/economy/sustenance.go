package economy

import "sync"

// Sustenance is the M11.3 hunger pool (spec economy-survival §4): an
// integer in [0, MaxSustenance] on the player, three derived tiers, and
// a per-tier regen multiplier other features consult when computing
// regen. Unlike currency, sustenance emits NO observable events (spec
// §7 — "a value plus tier-derivation helpers"), so this slice carries
// no Sink. The drain pipeline (the world-tick subscriber that calls
// Drain) lives at the composition root; this package only owns the
// value semantics and the config it reads.

// MaxSustenance is the engine ceiling for the pool (spec §4.5 — "an
// engine constant in the consume path, not part of the config; content
// cannot raise the cap"). Set / Add clamp to it.
const MaxSustenance = 100

// Tier names a sustenance band (spec §4.2). The string values are
// content-visible — render strings and the consider/score surface show
// them — so they are the stable wire names, not display-only labels.
type Tier string

const (
	// TierFull — value at or above TierFullMin; baseline regen.
	TierFull Tier = "full"
	// TierHungry — value in [TierHungryMin, TierFullMin); regen halved.
	TierHungry Tier = "hungry"
	// TierFamished — value below TierHungryMin; no regen.
	TierFamished Tier = "famished"
)

// SustenanceConfig holds the spec §4.2/§4.3/§4.4 parameters: the tier
// thresholds, the per-tier regen multipliers, and the drain pipeline's
// cadence/amount/reminder knobs the world-tick subscriber reads.
type SustenanceConfig struct {
	// TierFullMin is the lowest value still counted Full (§4.2).
	TierFullMin int
	// TierHungryMin is the lowest value still counted Hungry; below it
	// is Famished (§4.2).
	TierHungryMin int

	// FullMultiplier / HungryMultiplier / FamishedMultiplier are the
	// per-tier regen scalars (§4.3 defaults 1.0 / 0.5 / 0.0).
	FullMultiplier     float64
	HungryMultiplier   float64
	FamishedMultiplier float64

	// DrainAmount is the points removed per drain tick (§4.4).
	DrainAmount int
	// DrainCadence is the drain interval in engine ticks (§4.4). The
	// world-tick subscriber registers at this cadence.
	DrainCadence uint64
	// ReminderIntervalTicks is the minimum gap between hunger reminder
	// messages to a single player (§4.4), throttling the per-drain
	// nudge so a hungry player isn't spammed every drain tick.
	ReminderIntervalTicks uint64
}

// DefaultSustenanceConfig returns the spec §4 documented defaults.
func DefaultSustenanceConfig() SustenanceConfig {
	return SustenanceConfig{
		TierFullMin:           67,
		TierHungryMin:         34,
		FullMultiplier:        1.0,
		HungryMultiplier:      0.5,
		FamishedMultiplier:    0.0,
		DrainAmount:           1,
		DrainCadence:          300,
		ReminderIntervalTicks: 3000,
	}
}

// TierOf derives the tier from a raw value (spec §4.2). Boundaries are
// inclusive at the low end of each band: value == TierFullMin is Full.
func (c SustenanceConfig) TierOf(value int) Tier {
	switch {
	case value >= c.TierFullMin:
		return TierFull
	case value >= c.TierHungryMin:
		return TierHungry
	default:
		return TierFamished
	}
}

// GetRegenMultiplier returns the regen scalar for the value's tier
// (spec §4.3). Regen-driving features (the M11.5 heartbeat, room
// healing, abilities) multiply their per-tick regen by this. Sustenance
// itself never touches vitals — it only exposes the scalar.
func (c SustenanceConfig) GetRegenMultiplier(value int) float64 {
	switch c.TierOf(value) {
	case TierFull:
		return c.FullMultiplier
	case TierHungry:
		return c.HungryMultiplier
	default:
		return c.FamishedMultiplier
	}
}

// SustenanceEntity is the holder a SustenanceService reads and writes
// the pool on (spec §4.1 — the value lives directly on the holder). The
// connActor satisfies it. Implementations own their own locking:
// Sustenance / SetSustenance may be called from the drain (tick)
// goroutine and the consume (command) goroutine, and must be safe
// against the autosave path reading the same field.
type SustenanceEntity interface {
	// ID returns the stable identity (bare id, engine convention).
	ID() string
	// Sustenance returns the current pool value.
	Sustenance() int
	// SetSustenance writes the pool value. The service only ever
	// passes a value already clamped to [0, MaxSustenance], so
	// implementations need not re-clamp.
	SetSustenance(value int)
}

// SustenanceService owns the spec §4 operations over a config. A single
// mutex makes each read-modify-write (Add / Drain) atomic against a
// concurrent mutation on the same entity — the drain tick and a consume
// command can otherwise both read the same value and lose one write.
// The lock is process-wide rather than per-entity because sustenance
// mutations are infrequent; contention is negligible at MUD scale,
// matching CurrencyService's choice.
type SustenanceService struct {
	mu  sync.Mutex
	cfg SustenanceConfig
}

// NewSustenanceService returns a service over cfg.
func NewSustenanceService(cfg SustenanceConfig) *SustenanceService {
	return &SustenanceService{cfg: cfg}
}

// Config returns the service's config so the drain subscriber can read
// DrainCadence / ReminderIntervalTicks without a second copy.
func (s *SustenanceService) Config() SustenanceConfig { return s.cfg }

// Read returns the entity's pool value, zero for a nil entity. (The
// spec §6.2 consume path's "default to 100 when absent" is the consume
// verb's concern, landing in M11.5; the pool itself is seeded to
// MaxSustenance at character creation so a live entity always carries a
// real value.)
func (s *SustenanceService) Read(e SustenanceEntity) int {
	if e == nil {
		return 0
	}
	return e.Sustenance()
}

// TierOf / GetRegenMultiplier expose the config's derivations on the
// service so callers holding only the service need not also hold the
// config.
func (s *SustenanceService) TierOf(value int) Tier { return s.cfg.TierOf(value) }
func (s *SustenanceService) GetRegenMultiplier(value int) float64 {
	return s.cfg.GetRegenMultiplier(value)
}

// Set forces the pool to value, clamped to [0, MaxSustenance] (spec
// §4.5). This is the character-created seed path (value == MaxSustenance)
// and the quest/admin direct-set path. Returns the clamped value.
// Nil-safe.
func (s *SustenanceService) Set(e SustenanceEntity, value int) int {
	if e == nil {
		return 0
	}
	value = clampSustenance(value)
	s.mu.Lock()
	e.SetSustenance(value)
	s.mu.Unlock()
	return value
}

// Add applies delta (replenishment is positive, drain-like is negative)
// to the pool, clamped to [0, MaxSustenance] (spec §4.5 — "any value
// above 100 is silently capped"). This is the M11.5 consume
// replenishment primitive. Returns the new value. Nil-safe.
func (s *SustenanceService) Add(e SustenanceEntity, delta int) int {
	if e == nil {
		return 0
	}
	s.mu.Lock()
	next := clampSustenance(e.Sustenance() + delta)
	e.SetSustenance(next)
	s.mu.Unlock()
	return next
}

// Drain decrements the pool by the configured DrainAmount, floored at
// zero, and returns the new value and its derived tier (spec §4.4). The
// world-tick subscriber calls this once per logged-in player per drain
// tick. Nil-safe: a nil entity reports (0, Famished).
func (s *SustenanceService) Drain(e SustenanceEntity) (int, Tier) {
	if e == nil {
		return 0, TierFamished
	}
	s.mu.Lock()
	next := e.Sustenance() - s.cfg.DrainAmount
	if next < 0 {
		next = 0
	}
	e.SetSustenance(next)
	s.mu.Unlock()
	return next, s.cfg.TierOf(next)
}

func clampSustenance(v int) int {
	if v < 0 {
		return 0
	}
	if v > MaxSustenance {
		return MaxSustenance
	}
	return v
}
