package combat

import "sync"

// Vitals holds a combatant's hit-point state. The combat round (which
// runs on the heartbeat-bucket tick) applies damage from a tick
// goroutine; a status command or autosave may read concurrently from a
// session goroutine; an AI heal-over-time effect may add hit points
// from yet another. The internal mutex serializes all of that without
// callers having to thread their own lock through.
//
// Vitals is constructed via NewVitals / NewVitalsAt; the zero value is
// not meaningful (Max=0 would force every combatant into dead-on-spawn).
// All public methods are safe to call from any goroutine.
type Vitals struct {
	mu  sync.Mutex
	hp  int
	max int
}

// NewVitals returns a fresh Vitals at full HP. Used for mob spawns and
// for player login when no persisted HP exists yet. A non-positive
// maxHP is clamped to 1 — a combatant must have at least one hit point
// to be a meaningful combat participant.
func NewVitals(maxHP int) *Vitals {
	if maxHP < 1 {
		maxHP = 1
	}
	return &Vitals{hp: maxHP, max: maxHP}
}

// NewVitalsAt returns Vitals with current HP set explicitly. Used by
// the player respawn / load path once vitals persistence lands (M7.5+).
// hp is clamped to [0, maxHP]; maxHP is clamped to >= 1 the same way
// NewVitals does.
func NewVitalsAt(hp, maxHP int) *Vitals {
	if maxHP < 1 {
		maxHP = 1
	}
	if hp < 0 {
		hp = 0
	}
	if hp > maxHP {
		hp = maxHP
	}
	return &Vitals{hp: hp, max: maxHP}
}

// Current returns the current HP under the internal lock. Callers that
// also need Max in the same expression MUST use Snapshot instead — two
// separate Current/Max calls open a TOCTOU window in which a combat-
// tick damage application or a SetMax can interleave between them,
// producing an internally inconsistent (current, max) pair.
func (v *Vitals) Current() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.hp
}

// Max returns the current maximum HP. See Current for the TOCTOU
// warning: a caller that wants both current and max should reach for
// Snapshot, not Current+Max.
func (v *Vitals) Max() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.max
}

// Snapshot returns (current, max) atomically — cheaper and race-free
// versus two separate Current/Max calls when both are needed (status
// rendering, percent-of-max checks under one lock).
func (v *Vitals) Snapshot() (current, max int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.hp, v.max
}

// IsDead reports whether current HP is at or below zero. Combat-side
// code uses this as the canonical liveness check.
func (v *Vitals) IsDead() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.hp <= 0
}

// Percent returns current HP as a fraction of max in [0, 1]. Returns 0
// when max is zero (defensive — NewVitals prevents this, but a future
// SetMax that drives max to 0 should not produce a divide-by-zero
// panic in the wimpy check).
func (v *Vitals) Percent() float64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.max <= 0 {
		return 0
	}
	if v.hp <= 0 {
		return 0
	}
	return float64(v.hp) / float64(v.max)
}

// ApplyDamage subtracts amount from current HP and returns the new
// current HP. amount is clamped to >= 0 (a negative damage roll is a
// caller bug, not a heal — callers that want a heal should call Heal
// explicitly). Current HP is clamped to >= 0; ApplyDamage(big) on a
// living combatant returns 0, never a negative number, so downstream
// "is dead" checks have only one comparison to make.
//
// Combat §4.5 requires that a successful swing land at least 1 point
// of damage; that minimum is the caller's responsibility (the damage
// roll). Vitals is the dumb counter — it does not invent damage from
// a zero amount.
func (v *Vitals) ApplyDamage(amount int) int {
	if amount < 0 {
		amount = 0
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.hp -= amount
	if v.hp < 0 {
		v.hp = 0
	}
	return v.hp
}

// Heal adds amount to current HP, capped at max. Returns the new
// current HP. Negative amounts are clamped to zero (callers that want
// to deal damage call ApplyDamage). Healing past zero from a dead
// combatant works — Vitals does not enforce "no resurrection from
// healing"; the combat-side death flow owns whether a corpse is still
// addressable as a heal target.
func (v *Vitals) Heal(amount int) int {
	if amount < 0 {
		amount = 0
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.hp += amount
	if v.hp > v.max {
		v.hp = v.max
	}
	return v.hp
}

// SetMax adjusts the maximum HP. If the new max is below current HP,
// current is also clamped down. If the new max is above current HP,
// current is left alone — leveling up does not auto-heal; the
// progression layer (M8) decides whether to follow a SetMax with a
// Heal-to-full. newMax is clamped to >= 1 like the constructors.
func (v *Vitals) SetMax(newMax int) {
	if newMax < 1 {
		newMax = 1
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.max = newMax
	if v.hp > v.max {
		v.hp = v.max
	}
}
