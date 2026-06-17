package combat

import (
	"context"
	"testing"
)

// An attacker whose Stats carry an OffHand profile makes TWO swings in a round
// (two-weapon-fighting §3): the main weapon, then the off-hand weapon — each
// with its own dice/name/damage bonus. The off-hand swing reuses the same
// resolver, so it lands as an ordinary hit on a surviving target.
func TestAutoAttack_OffHandSwing(t *testing.T) {
	atkStats := Stats{
		HitMod:     50, // always hits
		STR:        10,
		Damage:     DiceExpr{Count: 1, Sides: 6},
		WeaponName: "a sword",
		OffHand: &OffHandProfile{
			Damage:      DiceExpr{Count: 1, Sides: 4},
			WeaponName:  "a dagger",
			HitMod:      50, // always hits
			DamageBonus: 5,  // distinct so the off-hand hit is identifiable
		},
	}
	defStats := Stats{AC: 10}
	// Two swings: main (d20, 1d6) then off-hand (d20, 1d4). d20 rolls land well
	// below the threat range so neither crits; damage rolls are deterministic.
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 100, []int{
		9, 2, // main: d20 raw 10 (hit, no crit), 1d6 → 3
		9, 1, // off-hand: d20 raw 10 (hit, no crit), 1d4 → 2 (+5 bonus = 7)
	})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 2 {
		t.Fatalf("want 2 hits (main + off-hand), got %d (misses=%d)", len(hits), len(rig.sink.snapshotMisses()))
	}
	if hits[0].WeaponName != "a sword" || hits[0].Damage != 3 {
		t.Errorf("main hit = %q dmg %d, want a sword dmg 3", hits[0].WeaponName, hits[0].Damage)
	}
	if hits[1].WeaponName != "a dagger" || hits[1].Damage != 7 {
		t.Errorf("off-hand hit = %q dmg %d, want a dagger dmg 7 (2 dice + 5 bonus)", hits[1].WeaponName, hits[1].Damage)
	}
}

// No OffHand profile ⇒ exactly one swing, the pre-feature behavior.
func TestAutoAttack_NoOffHandSingleSwing(t *testing.T) {
	atkStats := Stats{HitMod: 50, STR: 10, Damage: DiceExpr{Count: 1, Sides: 6}, WeaponName: "a sword"}
	rig := newAutoAttackRig(t, atkStats, Stats{AC: 10}, 10, 100, []int{9, 2})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)
	if hits := rig.sink.snapshotHits(); len(hits) != 1 {
		t.Fatalf("want 1 hit (no off-hand), got %d", len(hits))
	}
}

// The off-hand swing is suppressed when the main weapon fires as a projectile
// (two-weapon-fighting §3 — melee only). With no ammo gate a projectile fires
// once; the OffHand profile must not add a second swing.
func TestAutoAttack_RangedMainSuppressesOffHand(t *testing.T) {
	atkStats := Stats{
		HitMod:      50,
		STR:         10,
		Damage:      DiceExpr{Count: 1, Sides: 6},
		WeaponName:  "a bow",
		RangedClass: RangedProjectile,
		OffHand:     &OffHandProfile{Damage: DiceExpr{Count: 1, Sides: 4}, WeaponName: "a dagger", HitMod: 50},
	}
	rig := newAutoAttackRig(t, atkStats, Stats{AC: 10}, 10, 100, []int{9, 2})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)
	if hits := rig.sink.snapshotHits(); len(hits) != 1 {
		t.Fatalf("want 1 hit (ranged main, off-hand suppressed), got %d", len(hits))
	}
}
