package combat

import "github.com/Jasrags/AnotherMUD/internal/pool"

// Vitals holds a combatant's hit-point state. As of the generalized-pool
// migration it is a thin facade over a pool.Pool of kind "hp": the dumb,
// goroutine-safe counter now lives in internal/pool (shared with mana,
// the One Power, Essence, …) and Vitals preserves the HP-specific API and
// policy on top of it.
//
// The concurrency contract is unchanged — the combat round applies damage
// from a tick goroutine while a status command reads and an effect heals
// from others; pool.Pool carries its own mutex so callers never thread a
// lock. Vitals keeps two HP-specific rules that are NOT general pool
// behavior: a combatant must have at least 1 max HP (you can have 0 mana,
// but 0 max HP means dead-on-spawn), enforced in NewVitals/NewVitalsAt/
// SetMax; and the "hp" pool flags DepletionEvent so the combat death flow
// emits VitalDepleted once per death.
//
// Vitals is constructed via NewVitals / NewVitalsAt; the zero value is not
// meaningful.
type Vitals struct {
	p *pool.Pool
}

// hpKind / hpRules name the backing pool. Floor 0 (HP bottoms out at zero)
// and DepletionEvent so a crossing is reportable death. Uses the canonical
// pool.KindHP so Vitals, the VitalDepleted event vocabulary, and the pool
// kind are all the one string.
const hpKind = pool.KindHP

func hpRules() pool.Rules { return pool.Rules{Floor: 0, DepletionEvent: true} }

// NewVitals returns a fresh Vitals at full HP. A non-positive maxHP is
// clamped to 1 — a combatant must have at least one hit point to be a
// meaningful participant (HP-specific policy, not a pool rule).
func NewVitals(maxHP int) *Vitals {
	if maxHP < 1 {
		maxHP = 1
	}
	return &Vitals{p: pool.New(hpKind, maxHP, hpRules())}
}

// NewVitalsAt returns Vitals with current HP set explicitly (the load
// path). maxHP is clamped to >= 1 as in NewVitals; hp is clamped to
// [0, maxHP] by pool.NewAt (floor 0).
func NewVitalsAt(hp, maxHP int) *Vitals {
	if maxHP < 1 {
		maxHP = 1
	}
	return &Vitals{p: pool.NewAt(hpKind, hp, maxHP, hpRules())}
}

// Current returns the current HP. Callers needing both current and max in
// one expression MUST use Snapshot (TOCTOU — see pool.Pool.Current).
func (v *Vitals) Current() int { return v.p.Current() }

// Max returns the current maximum HP. See Current for the TOCTOU note.
func (v *Vitals) Max() int { return v.p.Max() }

// Snapshot returns (current, max) atomically.
func (v *Vitals) Snapshot() (current, max int) { return v.p.Snapshot() }

// IsDead reports whether current HP is at or below zero — the canonical
// liveness check (pool floor is 0).
func (v *Vitals) IsDead() bool { return v.p.IsEmpty() }

// Percent returns current HP as a fraction of max in [0, 1], 0 when max is
// non-positive or HP is depleted.
func (v *Vitals) Percent() float64 { return v.p.Percent() }

// ApplyDamage subtracts amount (clamped >= 0) from current HP and returns
// the new current HP, clamped at 0. Combat's per-swing minimum-1 rule is
// the caller's responsibility (the damage roll); Vitals is the dumb
// counter.
func (v *Vitals) ApplyDamage(amount int) int {
	cur, _, _ := v.p.ApplyDamage(amount)
	return cur
}

// ApplyDamageIfAlive is the atomic IsDead+ApplyDamage pair. It returns
// (remaining, wasAlive):
//   - wasAlive=false means the combatant was already at 0 HP on entry; no
//     observable damage is applied (the pool floors at 0 either way) and
//     the caller treats the swing as landing on a corpse.
//   - wasAlive=true means it was living; remaining is the new current.
//     remaining==0 here is the killing-blow signal, and pool.ApplyDamage's
//     `crossed` guarantees exactly one racing caller observes it — so
//     VitalDepleted is emitted once.
//
// wasAlive is reconstructed from the pool return: the combatant was living
// iff this call drove it to the floor (crossed) OR it still has HP left
// (current > 0). An already-dead pool reports neither, yielding false.
func (v *Vitals) ApplyDamageIfAlive(amount int) (remaining int, wasAlive bool) {
	cur, _, crossed := v.p.ApplyDamage(amount)
	return cur, crossed || cur > 0
}

// Deplete drives current HP to zero and reports whether the combatant was
// alive on entry — the primitive for save-gated instant death (saves §4
// massive damage) and a future coup-de-grace. Only the caller observing
// wasAlive=true should emit VitalDepleted (pool.Pool.Deplete's once-only
// guarantee).
func (v *Vitals) Deplete() (wasAlive bool) { return v.p.Deplete() }

// Heal adds amount to current HP, capped at max, returning the new
// current. Negative amounts clamp to zero. Healing a dead combatant works
// — the combat death flow owns whether a corpse is still a heal target.
func (v *Vitals) Heal(amount int) int { return v.p.Restore(amount) }

// HealAmount heals like Heal but returns, atomically, the HP ACTUALLY
// restored (0 when already at max) plus the resulting current and max — so
// a heal's player-facing "+N HP" and its EntityHealed.Amount reflect the
// true delta rather than the raw Heal return (which is the new current).
// Use this wherever the healed amount is reported or emitted.
func (v *Vitals) HealAmount(amount int) (restored, current, max int) {
	return v.p.RestoreDelta(amount)
}

// SetCurrent sets current HP to an explicit value, clamped to [0, max],
// returning the new current (admin `set vital hp` / the `restore` verb).
func (v *Vitals) SetCurrent(hp int) int { return v.p.SetCurrent(hp) }

// SetMax adjusts max HP, clamping current down if it now exceeds the new
// max; a raise leaves current alone (leveling up does not auto-heal). The
// progression layer (StatBlock.OnMaxChange → here) decides whether to
// follow a SetMax with a Heal. newMax is clamped to >= 1.
func (v *Vitals) SetMax(newMax int) {
	if newMax < 1 {
		newMax = 1
	}
	v.p.SetMax(newMax)
}
