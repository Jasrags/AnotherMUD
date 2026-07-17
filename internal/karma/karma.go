// Package karma is the spendable-advancement currency for the karma-ledger
// advancement strategy (shadowrun-mvp.md SR-M5, Decision D3 Option B). It is a
// dependency-free leaf: a per-character ledger holding two figures that mirror
// Shadowrun's character sheet —
//
//   - Current: karma available to SPEND (raising skills/attributes, buying
//     qualities via the `improve` verb).
//   - Total: lifetime karma EARNED, never decreased by spending. It is the
//     figure initiation grade / karma-quality thresholds read against, and the
//     "how far has this runner come" number shown on the score sheet.
//
// A karma-ledger world routes advancement rewards (kill/quest) here instead of
// onto a progression track; a level-track world never constructs a Ledger. The
// type is safe for concurrent use — the connActor is touched from the combat
// tick, command dispatch, and persistence, so the lock lives here rather than
// being threaded through every caller (mirrors progression.ProgressionState).
package karma

import "sync"

// Ledger is a character's karma balance. The zero value is not usable; call
// NewLedger. Grant/Spend mutate under an internal mutex.
type Ledger struct {
	mu      sync.Mutex
	current int64
	total   int64
}

// NewLedger returns an empty ledger (0 current, 0 total). A freshly-created
// karma-ledger character starts here; SR5 grants no starting karma.
func NewLedger() *Ledger {
	return &Ledger{}
}

// Grant adds an earned reward. It raises BOTH current (spendable) and total
// (lifetime) by amount. A non-positive amount is a no-op — earning is the only
// way current rises, and it never lowers total. Returns the post-grant current.
func (l *Ledger) Grant(amount int64) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	if amount > 0 {
		l.current += amount
		l.total += amount
	}
	return l.current
}

// Spend deducts amount from current when the balance covers it, leaving total
// untouched (lifetime earned is a monotone record). Returns true when the spend
// happened. A non-positive amount, or an amount exceeding current, is refused
// with false and no mutation — the caller (the improve verb) reports the
// shortfall.
func (l *Ledger) Spend(amount int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if amount <= 0 || amount > l.current {
		return false
	}
	l.current -= amount
	return true
}

// Current returns the spendable balance.
func (l *Ledger) Current() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.current
}

// Total returns the lifetime karma earned.
func (l *Ledger) Total() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.total
}

// Snapshot is the persisted shape of a Ledger. A list would be overkill — two
// scalars round-trip cleanly under a `karma:` block on the player save.
type Snapshot struct {
	Current int64 `yaml:"current"`
	Total   int64 `yaml:"total"`
}

// Snapshot serializes the ledger for persistence.
func (l *Ledger) Snapshot() Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Snapshot{Current: l.current, Total: l.total}
}

// Restore replaces the ledger state from a snapshot at login. Negative values
// are clamped to 0 — a hand-edited save cannot make a character owe karma.
func (l *Ledger) Restore(snap Snapshot) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.current = max(snap.Current, 0)
	l.total = max(snap.Total, 0)
}
