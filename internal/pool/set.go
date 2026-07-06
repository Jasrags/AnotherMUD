package pool

import (
	"sort"
	"sync"
)

// Set is the per-entity collection of pools. Single-pool operations go
// straight to the Pool (each carries its own lock); the Set exists for
// the operations that span pools — overflow routing and
// snapshot/restore. The map is guarded by an RWMutex so a pool can be
// added at login while combat reads concurrently; the pools themselves
// are not re-locked by the Set beyond their own methods.
type Set struct {
	mu    sync.RWMutex
	pools map[Kind]*Pool
}

// NewSet returns an empty Set. Callers Add pools as content/derivation
// produces them.
func NewSet() *Set {
	return &Set{pools: make(map[Kind]*Pool)}
}

// Add installs (or replaces) the pool under its Kind.
func (s *Set) Add(p *Pool) {
	if p == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pools[p.kind] = p
}

// Fill restores every pool in the set to its max (Pool.Refill). The
// "start full" used once at character creation after a pool's max is
// endowed; a no-op for pools already at max or with a zero max.
func (s *Set) Fill() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.pools {
		p.Refill()
	}
}

// Get returns the pool of the given kind and whether it exists.
func (s *Set) Get(k Kind) (*Pool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.pools[k]
	return p, ok
}

// Crossing reports one pool that transitioned to its Floor during an
// ApplyDamage call. The owner emits one depletion event per Crossing
// (the primary pool AND any overflow target the spill filled). Nonlethal
// mirrors the crossed pool's Rules.Nonlethal so the owner can pick
// knock-out vs kill without re-looking-up the pool (shadowrun-mvp SR-M2:
// a Stun-monitor crossing is a KO).
type Crossing struct {
	Kind      Kind
	Nonlethal bool
}

// ApplyDamage applies amount to the pool of kind k, then routes any
// overflow down the OverflowTo chain (Shadowrun's Physical monitor
// spilling into a death track). It returns every DepletionEvent pool
// that crossed to empty so the owner emits exactly one
// combat.VitalDepleted per crossing.
//
// The routing is two sequential single-pool operations, deliberately NOT
// atomic across the chain — a transient observer seeing the first pool at
// Floor before the spill lands is harmless, the same kind of accepted
// TOCTOU the combat death flow already tolerates. A visited-set guards
// against a misconfigured OverflowTo cycle (A→B→A) so the loop always
// terminates.
//
// An unknown starting kind, or a zero amount, is a no-op returning no
// crossings.
//
// escaped/escapedTo surface overflow the chain could NOT route to a pool:
// when an OverflowTo names a kind that is not a pool in this Set — e.g.
// Shadowrun's Stun monitor overflowing to `hp`, where the Physical monitor is
// the Vitals track (Design 1), not a declared pool — the pending amount is
// returned as escaped (its intended kind as escapedTo) rather than silently
// dropped, so the owner (combat) can apply it to Vitals. escaped is 0 when the
// chain terminates normally (no OverflowTo, or overflow fully absorbed).
func (s *Set) ApplyDamage(k Kind, amount int) (crossed []Crossing, escaped int, escapedTo Kind) {
	visited := make(map[Kind]bool)
	for amount > 0 && !visited[k] {
		visited[k] = true
		p, ok := s.Get(k)
		if !ok {
			// k is not a pool in this Set. Two cases reach here: (1) mid-chain, an
			// OverflowTo target that isn't a pool — Shadowrun's stun overflowing to
			// `hp` (the Physical monitor is Vitals, not a pool); (2) first iteration,
			// the caller passed an initial kind with no matching pool (a weapon whose
			// target_pool names no declared pool). Both surface the pending amount as
			// escaped so the owner can route it (combat spills escapedTo==hp onto
			// Vitals) or ignore it — never a silent drop.
			escaped, escapedTo = amount, k
			break
		}
		_, overflow, didCross := p.ApplyDamage(amount)
		if didCross && p.rules.DepletionEvent {
			crossed = append(crossed, Crossing{Kind: p.kind, Nonlethal: p.rules.Nonlethal})
		}
		next := p.rules.OverflowTo
		if overflow == 0 || next == "" {
			break
		}
		k, amount = next, overflow
	}
	return crossed, escaped, escapedTo
}

// Entry is one pool's persisted value. Rules are intentionally absent —
// they are re-derived from content at load (see RestoreSet).
type Entry struct {
	Kind    Kind `yaml:"kind"`
	Current int  `yaml:"current"`
	Max     int  `yaml:"max"`
}

// Snapshot is the persisted shape of a Set: an ordered list of entries
// (sorted by Kind) so YAML round-trips and save diffs are deterministic.
type Snapshot []Entry

// Snapshot returns the Set's pools as a deterministically-ordered list.
func (s *Set) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.pools) == 0 {
		return nil
	}
	kinds := make([]string, 0, len(s.pools))
	for k := range s.pools {
		kinds = append(kinds, string(k))
	}
	sort.Strings(kinds)
	out := make(Snapshot, 0, len(kinds))
	for _, k := range kinds {
		p := s.pools[Kind(k)]
		cur, max := p.Snapshot()
		out = append(out, Entry{Kind: Kind(k), Current: cur, Max: max})
	}
	return out
}

// RestoreSet rebuilds a Set from a persisted Snapshot, looking up each
// pool's Rules from content via rulesFor (rules are not persisted, so a
// rebalance never needs a migration). A nil rulesFor yields zero-value
// (inert) rules for every pool.
func RestoreSet(snap Snapshot, rulesFor func(Kind) Rules) *Set {
	s := NewSet()
	for _, e := range snap {
		rules := Rules{}
		if rulesFor != nil {
			rules = rulesFor(e.Kind)
		}
		s.Add(NewAt(e.Kind, e.Current, e.Max, rules))
	}
	return s
}
