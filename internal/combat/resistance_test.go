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
		// Fire is a first-class type: a fire weapon vs fire-resistant gear is soaked
		// by that resistance (a flamethrower vs a fire-resistance liner).
		{"fire resistance soaks fire", map[string]int{"fire": 4}, []string{"fire"}, 4},
		{"no fire resistance", map[string]int{"radiation": 2}, []string{"fire"}, 0},
		// The energy types are symmetric: insulation soaks cold, nonconductivity
		// soaks the electrical jolt of a stun baton / taser.
		{"insulation soaks cold", map[string]int{"cold": 3}, []string{"cold"}, 3},
		{"nonconductivity soaks electrical", map[string]int{"electrical": 2}, []string{"electrical"}, 2},
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

// Armor penetration reduces the defender's Mitigation, but only up to their
// worn-armor rating — it bypasses armor, never the innate toughness (body).
func TestAutoAttack_ArmorPenReducesSoakCappedAtArmor(t *testing.T) {
	// Defender: Mitigation 5 (think body 2 + armor 3), worn-armor rating 3.
	def := Stats{AC: 1, Mitigation: 5, ArmorRating: 3}
	dmgWithAP := func(ap int) int {
		atk := Stats{HitMod: 100, DamageBonus: 10, Damage: DiceExpr{1, 1, 0}, ArmorPen: ap}
		rig := newAutoAttackRig(t, atk, def, 10, 500, []int{9, 0}) // always hit; 1d1 → 1
		rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)
		hits := rig.sink.snapshotHits()
		if len(hits) != 1 {
			t.Fatalf("want 1 hit, got %d", len(hits))
		}
		return hits[0].Damage
	}
	if noAP := dmgWithAP(0); noAP != 6 { // 1 + 10 − 5 soak
		t.Fatalf("baseline damage = %d, want 6 (1 + 10 − 5 soak)", noAP)
	}
	if got := dmgWithAP(3); got != 9 { // soak 5 − min(3,3) = 2 → 1 + 10 − 2
		t.Errorf("AP 3 vs armor 3: damage = %d, want 9 (3 soak bypassed)", got)
	}
	if got := dmgWithAP(10); got != 9 { // capped at armor 3 — body soak intact
		t.Errorf("surplus AP must not penetrate body soak: damage = %d, want 9", got)
	}
}

// AP bypasses the general armor soak but NOT a typed resistance — a fire-
// resistant liner still soaks a high-AP fire weapon (the flamethrower vs a fire
// liner interaction).
func TestAutoAttack_ArmorPenLeavesTypedResistance(t *testing.T) {
	atk := Stats{HitMod: 100, DamageBonus: 10, Damage: DiceExpr{1, 1, 0}, WeaponDamageTypes: []string{"fire"}, ArmorPen: 10}
	def := Stats{AC: 1, Mitigation: 4, ArmorRating: 4, Resistances: map[string]int{"fire": 3}}
	rig := newAutoAttackRig(t, atk, def, 10, 500, []int{9, 0})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	// AP 10 (capped at armor 4) removes all Mitigation → soak = 0 + 3 fire = 3.
	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || hits[0].Damage != 8 { // 1 + 10 − 3
		t.Fatalf("fire resistance must survive AP: got %+v, want damage 8", hits)
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
