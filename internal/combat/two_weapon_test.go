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

// Improved Two-Weapon Fighting (two-weapon-fighting §3.1): an OffHand profile
// with Attacks=2 makes the main swing plus TWO off-hand strikes — three hits on
// a surviving target.
func TestAutoAttack_ImprovedTwoOffHandSwings(t *testing.T) {
	atkStats := Stats{
		HitMod: 50, STR: 10,
		Damage: DiceExpr{Count: 1, Sides: 6}, WeaponName: "a sword",
		OffHand: &OffHandProfile{
			Damage: DiceExpr{Count: 1, Sides: 4}, WeaponName: "a dagger",
			HitMod: 50, DamageBonus: 5, Attacks: 2,
		},
	}
	rig := newAutoAttackRig(t, atkStats, Stats{AC: 10}, 10, 100, []int{
		9, 2, // main: raw 10 hit, 1d6 → 3
		9, 1, // off-hand 1: raw 10 hit, 1d4 → 2 (+5 = 7)
		9, 1, // off-hand 2: raw 10 hit, 1d4 → 2 (+5 = 7)
	})
	rig.secOff = 0 // isolate the swing count from the secondary penalty
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 3 {
		t.Fatalf("want 3 hits (main + two off-hand), got %d", len(hits))
	}
	if hits[1].WeaponName != "a dagger" || hits[2].WeaponName != "a dagger" {
		t.Errorf("off-hand hits = %q,%q, want both a dagger", hits[1].WeaponName, hits[2].WeaponName)
	}
}

// The secondary off-hand penalty (two-weapon-fighting §4.3) degrades the SECOND
// off-hand strike: with the off hand at +0 against AC 15, the first off-hand
// strike (raw 16) lands and the second (16 − 5 = 11) misses. The first strike is
// unpenalized; only later strikes take the cumulative penalty.
func TestAutoAttack_SecondaryOffHandPenaltyApplied(t *testing.T) {
	atkStats := Stats{
		HitMod: 50, STR: 10,
		Damage: DiceExpr{Count: 1, Sides: 6}, WeaponName: "a sword",
		OffHand: &OffHandProfile{
			Damage: DiceExpr{Count: 1, Sides: 4}, WeaponName: "a dagger",
			HitMod: 0, Attacks: 2,
		},
	}
	rig := newAutoAttackRig(t, atkStats, Stats{AC: 15}, 10, 100, []int{
		9, 2, // main: raw 10, +50 hits, 1d6 → 3
		15, 1, // off-hand 1: raw 16, +0 ≥ 15 hits, 1d4 → 2
		15, // off-hand 2: raw 16, −5 = 11 < 15 → miss (no damage roll)
	})
	rig.secOff = 5
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if hits := rig.sink.snapshotHits(); len(hits) != 2 {
		t.Fatalf("want 2 hits (main + first off-hand), got %d", len(hits))
	}
	if misses := rig.sink.snapshotMisses(); len(misses) != 1 {
		t.Fatalf("want 1 miss (the penalized second off-hand strike), got %d", len(misses))
	}
}

// An off-hand strike that KILLS the target ends the round — no further off-hand
// strikes are made (two-weapon-fighting §3.1, mirroring a killing main swing).
// The main swing leaves the target alive; the first off-hand strike kills, so
// the second is never rolled (a stray swing would exhaust the scripted roller).
func TestAutoAttack_OffHandKillStopsRemainingSwings(t *testing.T) {
	atkStats := Stats{
		HitMod: 50, STR: 10,
		Damage: DiceExpr{Count: 1, Sides: 6}, WeaponName: "a sword",
		OffHand: &OffHandProfile{
			Damage: DiceExpr{Count: 1, Sides: 4}, WeaponName: "a dagger",
			HitMod: 50, DamageBonus: 5, Attacks: 2,
		},
	}
	rig := newAutoAttackRig(t, atkStats, Stats{AC: 10}, 10, 5, []int{
		9, 2, // main: raw 10 hit, 1d6 → 3 (HP 5 → 2, survives)
		9, 1, // off-hand 1: raw 10 hit, 1d4 → 2 (+5 = 7) → kills; round ends
	})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if hits := rig.sink.snapshotHits(); len(hits) != 2 {
		t.Fatalf("want 2 hits (main + the killing off-hand strike), got %d", len(hits))
	}
	if !rig.target.vitals.IsDead() {
		t.Errorf("target should be dead, HP = %d", rig.target.vitals.Current())
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
