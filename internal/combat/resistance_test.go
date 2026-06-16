package combat

import (
	"context"
	"testing"
)

// armor-depth §4: per-damage-type resistance. The defender's resistance to
// the attacker's weapon type reduces damage, composing additively with the
// type-agnostic Mitigation, with the per-swing minimum-1 floor last.

func TestTypedResistance(t *testing.T) {
	res := map[string]int{"slashing": 3, "piercing": 1}
	cases := []struct {
		name  string
		res   map[string]int
		types []string
		want  int
	}{
		{"nil map", nil, []string{"slashing"}, 0},
		{"untyped attacker", res, nil, 0},
		{"matching type", res, []string{"slashing"}, 3},
		{"other matching type", res, []string{"piercing"}, 1},
		{"unmatched type", res, []string{"bludgeoning"}, 0},
		{"first-match precedence", res, []string{"slashing", "piercing"}, 3},
		{"first miss then match", res, []string{"bludgeoning", "piercing"}, 1},
	}
	for _, tc := range cases {
		if got := TypedResistance(tc.res, tc.types); got != tc.want {
			t.Errorf("%s: TypedResistance(%v, %v) = %d, want %d", tc.name, tc.res, tc.types, got, tc.want)
		}
	}
}

func TestAutoAttack_TypedResistanceReducesDamage(t *testing.T) {
	atk := Stats{HitMod: 100, DamageBonus: 5, Damage: DiceExpr{1, 1, 0}, WeaponDamageTypes: []string{"slashing"}}
	def := Stats{AC: 10, Resistances: map[string]int{"slashing": 3}}
	rig := newAutoAttackRig(t, atk, def, 10, 50, []int{9, 0}) // always hit; 1d1 → 1
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d", len(hits))
	}
	if hits[0].Damage != 3 { // 1 die + 5 bonus − 3 slashing resistance
		t.Errorf("damage = %d, want 3 (1 + 5 − 3 resistance)", hits[0].Damage)
	}
}

func TestAutoAttack_ResistanceComposesWithMitigation(t *testing.T) {
	// Scalar Mitigation (type-agnostic) and per-type Resistance compose
	// additively: 1 + 5 − 2 (mitigation) − 3 (slashing resistance) = 1.
	atk := Stats{HitMod: 100, DamageBonus: 5, Damage: DiceExpr{1, 1, 0}, WeaponDamageTypes: []string{"slashing"}}
	def := Stats{AC: 10, Mitigation: 2, Resistances: map[string]int{"slashing": 3}}
	rig := newAutoAttackRig(t, atk, def, 10, 50, []int{9, 0})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || hits[0].Damage != 1 {
		t.Fatalf("want 1 hit of 1 damage (6 − 2 − 3 = 1), got %+v", hits)
	}
}

func TestAutoAttack_ResistanceSkippedOnUnmatchedType(t *testing.T) {
	// A bludgeoning attacker is unaffected by slashing-only armor: damage is
	// the full 1 + 5 = 6 (no resistance, no mitigation).
	atk := Stats{HitMod: 100, DamageBonus: 5, Damage: DiceExpr{1, 1, 0}, WeaponDamageTypes: []string{"bludgeoning"}}
	def := Stats{AC: 10, Resistances: map[string]int{"slashing": 3}}
	rig := newAutoAttackRig(t, atk, def, 10, 50, []int{9, 0})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || hits[0].Damage != 6 {
		t.Fatalf("want 1 hit of 6 damage (slashing armor does not soak bludgeoning), got %+v", hits)
	}
}

func TestAutoAttack_ResistanceFlooredAtOne(t *testing.T) {
	atk := Stats{HitMod: 100, DamageBonus: 0, Damage: DiceExpr{1, 1, 0}, WeaponDamageTypes: []string{"slashing"}}
	def := Stats{AC: 10, Resistances: map[string]int{"slashing": 50}} // dwarfs the 1 damage
	rig := newAutoAttackRig(t, atk, def, 10, 50, []int{9, 0})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || hits[0].Damage != 1 {
		t.Fatalf("want 1 hit of 1 damage (floored under full resistance), got %+v", hits)
	}
}
