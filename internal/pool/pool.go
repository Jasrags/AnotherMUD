// Package pool is the generalized resource-pool primitive: a named
// current/max counter with content-declared rules (floor, overflow,
// channel-degradation, depletion signalling). It generalizes the
// HP-only combat.Vitals into the substrate every resource pool shares —
// HP, mana / the One Power, Essence, Edge, Shadowrun's two condition
// monitors — so adding a pool is content + wiring, not a new Go type.
//
// pool is a LEAF package (the role srckey plays for the entities↔stats
// cycle): it imports nothing from the engine, so both combat and
// progression can depend on it without a cycle. A Pool owns NO events
// and NO clock — it is a dumb, goroutine-safe counter, the same
// property that makes combat.Vitals safe to touch from the tick, a
// session, and an effect goroutine at once. Coordination that needs
// more than one pool (overflow routing) lives on Set; coordination that
// needs the event bus or the channel layer (emitting a depletion event,
// capping a "degraded" channel) lives in the owner — exactly as combat
// already owns combat.VitalDepleted emission while Vitals only counts.
//
// Design note: regeneration is deliberately NOT a Pool feature. Regen is
// owner-driven (a rest / tick handler calls Restore), so the pool needs
// no clock and keeps the "safe from any goroutine, no time dependency"
// property F3 relies on.
package pool

import "sync"

// Kind is a pool's stable lowercase identity: "hp", "mana", "one_power",
// "essence", "edge", "stun". Content declares the set; the engine only
// has consumers for the kinds something reads (depletion, spend, cap).
// Kept a typed string (not an enum) so YAML round-trips cleanly and an
// unknown content-declared kind coexists with the engine's known ones —
// the same convention progression.StatType uses.
type Kind string

// Rules is the content-declared, mostly-static behavior of a pool. The
// zero value is an inert pool: floors at 0, never overflows, caps no
// channel, signals no depletion. A setting sets only the fields it needs.
//
// Rules are NOT persisted with the pool's value — they are re-derived
// from content at load (see RestoreSet), so rebalancing a pool's
// behavior never needs a save migration.
type Rules struct {
	// Floor is the value Current clamps to (0 for HP and most pools).
	Floor int

	// OverflowTo names another pool in the same Set that receives the
	// excess when damage would drive this pool below Floor (Shadowrun's
	// Physical monitor overflowing to a death track). Empty ⇒ clamp at
	// Floor and discard the excess. Routing is performed by Set, never
	// by the Pool itself.
	OverflowTo Kind

	// Degrades names a derived channel whose MAX this pool's Current
	// value caps — Essence's Current caps the "magic" channel. Empty ⇒
	// caps nothing. The Pool only stores the name; the owner reads it
	// and pushes Current into the channel (the one Essence-specific
	// line, kept out of the generic counter).
	Degrades string

	// DepletionEvent advertises that the owner SHOULD emit a depletion
	// signal when this pool crosses to Floor (HP, both Shadowrun
	// monitors). The Pool never emits — it reports the crossing via
	// ApplyDamage's `crossed` return and Set's []Crossing; the owner
	// turns that into combat.VitalDepleted. A pool that never kills
	// anything (mana) leaves this false.
	DepletionEvent bool
}

// Pool is one named current/max counter with an internal mutex. The
// concurrency contract is identical to combat.Vitals: the tick goroutine
// applies damage while a session goroutine reads for `score` while an
// effect goroutine heals, all without caller-side locking. Constructed
// via New / NewAt; the zero value is not meaningful (Max 0 below Floor
// would force empty-on-spawn).
type Pool struct {
	mu      sync.Mutex
	kind    Kind
	current int
	max     int
	rules   Rules
}

// New returns a full pool (Current == Max) of the given kind. A Max below
// the rules' Floor is raised to Floor — a pool must have at least its
// floor of headroom to be meaningful, mirroring NewVitals clamping Max to
// at least 1.
func New(kind Kind, max int, rules Rules) *Pool {
	if max < rules.Floor {
		max = rules.Floor
	}
	return &Pool{kind: kind, current: max, max: max, rules: rules}
}

// NewAt restores a pool at an explicit Current — the load path
// (combat.NewVitalsAt's generalization). Max is raised to Floor as in
// New; Current is clamped to [Floor, Max].
func NewAt(kind Kind, current, max int, rules Rules) *Pool {
	if max < rules.Floor {
		max = rules.Floor
	}
	if current < rules.Floor {
		current = rules.Floor
	}
	if current > max {
		current = max
	}
	return &Pool{kind: kind, current: current, max: max, rules: rules}
}

// Kind returns the pool's identity.
func (p *Pool) Kind() Kind { return p.kind }

// Rules returns a copy of the pool's rules. Cheap (small value struct);
// lets the Set read OverflowTo / DepletionEvent without locking the
// counter.
func (p *Pool) Rules() Rules { return p.rules }

// Snapshot returns (current, max) atomically — the race-free way to read
// both, as combat.Vitals.Snapshot is. Two separate Current/Max reads
// open a TOCTOU window a concurrent ApplyDamage/SetMax can tear.
func (p *Pool) Snapshot() (current, max int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current, p.max
}

// Current returns the current value. Callers that also need Max in the
// same expression MUST use Snapshot (see its TOCTOU note).
func (p *Pool) Current() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}

// Max returns the current maximum. See Current for the TOCTOU warning.
func (p *Pool) Max() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.max
}

// IsEmpty reports whether Current is at or below Floor — the generalized
// liveness check (combat.Vitals.IsDead is IsEmpty on the "hp" pool).
func (p *Pool) IsEmpty() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current <= p.rules.Floor
}

// Percent returns Current as a fraction of the usable range
// [Floor, Max], clamped to [0, 1]. Returns 0 for an empty pool or a
// degenerate range (Max <= Floor), so a wimpy-style check never divides
// by zero. For HP (Floor 0) this is current/max, matching Vitals.Percent.
func (p *Pool) Percent() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	span := p.max - p.rules.Floor
	if span <= 0 || p.current <= p.rules.Floor {
		return 0
	}
	return float64(p.current-p.rules.Floor) / float64(span)
}

// ApplyDamage subtracts amount (clamped to >= 0), flooring Current at
// Rules.Floor. It is the unification of Vitals.ApplyDamage,
// ApplyDamageIfAlive, and Deplete:
//
//	current  – the new value after the hit.
//	overflow – the amount that would have driven Current below Floor
//	           (>= 0). The Set routes this to Rules.OverflowTo; 0 when
//	           the pool absorbed the whole hit.
//	crossed  – true ONLY for the call that transitions the pool from
//	           >Floor to ==Floor. The owner uses this to emit a depletion
//	           event exactly once, even when two killers (a swing and a
//	           DoT) race — only the call that enters the lock with
//	           Current>Floor and drives it to Floor observes crossed=true.
func (p *Pool) ApplyDamage(amount int) (current, overflow int, crossed bool) {
	if amount < 0 {
		amount = 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	wasAbove := p.current > p.rules.Floor
	p.current -= amount
	if p.current < p.rules.Floor {
		overflow = p.rules.Floor - p.current
		p.current = p.rules.Floor
	}
	crossed = wasAbove && p.current <= p.rules.Floor
	return p.current, overflow, crossed
}

// Deplete drives Current to Floor outright and reports whether the pool
// was above Floor when the call entered the lock. The primitive for
// save-gated instant death (massive damage) and a future coup-de-grace.
// Like ApplyDamage's `crossed`, only the caller that observes
// wasAbove=true should emit a depletion event, so a concurrent killer
// (a racing swing, a DoT effect) cannot double-emit the death.
func (p *Pool) Deplete() (wasAbove bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current <= p.rules.Floor {
		return false
	}
	p.current = p.rules.Floor
	return true
}

// TrySpend deducts amount only if it leaves Current >= Floor, atomically.
// Returns false WITHOUT mutating when insufficient — for costs that must
// not partially drain (a spell that needs its full mana, an Edge point).
// Closes the check-then-deduct race a Snapshot+Deduct pair would open.
// A non-positive amount is a no-op that succeeds.
func (p *Pool) TrySpend(amount int) bool {
	if amount <= 0 {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current-amount < p.rules.Floor {
		return false
	}
	p.current -= amount
	return true
}

// Deduct subtracts amount, flooring at Floor and returning the new
// Current — the existing DeductMana / DeductMovement contract, where
// validation already proved sufficiency and a mid-pulse change must not
// underflow. Negative amounts are clamped to 0 (a caller wanting to add
// calls Restore).
func (p *Pool) Deduct(amount int) int {
	if amount < 0 {
		amount = 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current -= amount
	if p.current < p.rules.Floor {
		p.current = p.rules.Floor
	}
	return p.current
}

// Restore adds amount, capped at Max, returning the new Current
// (combat.Vitals.Heal). Owner-called by the rest / regen tick handler —
// the pool has no clock of its own. Negative amounts clamp to 0.
func (p *Pool) Restore(amount int) int {
	if amount < 0 {
		amount = 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current += amount
	if p.current > p.max {
		p.current = p.max
	}
	return p.current
}

// SetCurrent writes Current to an explicit value, clamped to [Floor, Max]
// in one lock acquisition (the admin `set vital` write). Returns the new
// Current.
func (p *Pool) SetCurrent(v int) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if v < p.rules.Floor {
		v = p.rules.Floor
	}
	if v > p.max {
		v = p.max
	}
	p.current = v
	return p.current
}

// SetMax adjusts Max, clamping Current down if it now exceeds the new
// maximum; a raise leaves Current alone (leveling up does not auto-fill —
// Vitals.SetMax semantics). newMax below Floor is raised to Floor. Wired
// to the "<kind>_max" derived channel via StatBlock.OnMaxChange by the
// owner.
func (p *Pool) SetMax(newMax int) {
	if newMax < p.rules.Floor {
		newMax = p.rules.Floor
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.max = newMax
	if p.current > p.max {
		p.current = p.max
	}
}
