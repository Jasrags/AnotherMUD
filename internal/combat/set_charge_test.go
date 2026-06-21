package combat

import (
	"context"
	"testing"
)

// special-weapons §4 — set vs a charge. A `set` weapon braced against a foe that
// CHARGED into strike range this round (recorded by recordCharge / the auto-close
// / the advance verb) lands a bonus blow, consumed once per charge. A non-set
// weapon, or a set weapon with no pending charge, is unaffected.

// baseline (no set, no charge): a plain weapon deals its dice only. The contrast
// that isolates the set bonus below — same rolls, +setBonus on the charge hit.
func TestAutoAttack_SetNoChargeNoBonus(t *testing.T) {
	atkStats := Stats{HitMod: 0, STR: 10, Set: true} // set weapon...
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 50, []int{14, 0}) // d20=15 hits AC10, 1d3=1
	rig.setBonus = 4
	// ...but no charge recorded → no braced blow.
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || hits[0].Damage != 1 {
		t.Fatalf("set weapon with no charge deals dice only; want one hit of 1, got %v", hits)
	}
}

// A set weapon answering a charge deals dice + the set bonus.
func TestAutoAttack_SetVsChargeAddsBonus(t *testing.T) {
	atkStats := Stats{HitMod: 0, STR: 10, Set: true}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 50, []int{14, 0}) // d20=15 hits, 1d3=1
	rig.setBonus = 4
	// The target charged into the setter this round.
	rig.mgr.recordCharge(rig.target.id, rig.attacker.id)
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || hits[0].Damage != 5 { // 1 (dice) + 4 (set bonus)
		t.Fatalf("set vs charge: want one hit of 5 (1 dice + 4 set), got %v", hits)
	}
	// The braced moment is spent — no second charge remains.
	if rig.mgr.ConsumeCharge(rig.target.id, rig.attacker.id) {
		t.Error("the charge should be consumed by the set blow, but a charge remained")
	}
}

// A NON-set weapon ignores a pending charge — the bonus is the weapon's, not the
// wielder's. Proves the gate is the `set` tag.
func TestAutoAttack_NonSetIgnoresCharge(t *testing.T) {
	atkStats := Stats{HitMod: 0, STR: 10} // no Set
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 50, []int{14, 0})
	rig.setBonus = 4
	rig.mgr.recordCharge(rig.target.id, rig.attacker.id)
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || hits[0].Damage != 1 {
		t.Fatalf("a non-set weapon ignores a charge; want one hit of 1, got %v", hits)
	}
}

// recordCharge / ConsumeCharge are directional and order-independent on the key:
// only the actual charger's pending charge is consumed, and only once.
func TestManager_ChargeRecordConsume(t *testing.T) {
	m := NewManager(MapLocator{}, &recordingSink{})
	a := NewMobCombatantID("a")
	b := NewPlayerCombatantID("b")

	// a charged toward b. b (a setter) consumes it; a (the charger) does not.
	m.recordCharge(a, b)
	if m.ConsumeCharge(b, a) {
		t.Error("b is the victim, not the charger — b's own charge should not exist")
	}
	if !m.ConsumeCharge(a, b) {
		t.Error("a charged toward b; b should consume a's charge")
	}
	if m.ConsumeCharge(a, b) {
		t.Error("a charge is consumed exactly once")
	}
}

// A pending charge dies with the engagement (no stale braced blow into a later,
// unrelated fight reusing the same combatant ids).
func TestManager_DisengageClearsCharge(t *testing.T) {
	m := NewManager(MapLocator{}, &recordingSink{})
	a := NewMobCombatantID("a")
	b := NewPlayerCombatantID("b")
	m.Engage(context.Background(), a, b, roomA)
	m.recordCharge(a, b)
	m.Disengage(context.Background(), a, b, roomA)
	if m.ConsumeCharge(a, b) {
		t.Error("disengage should clear the pending charge")
	}
}
