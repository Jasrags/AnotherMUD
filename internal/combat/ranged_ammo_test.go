package combat

import (
	"context"
	"testing"
)

// ranged-combat §3 — a projectile swing with no matching ammo is skipped
// (a RangedDry event) and the attacker stays engaged; no hit/miss is
// emitted for that swing.
func TestAutoAttack_ProjectileOutOfAmmo_SkipsSwing(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10, RangedClass: RangedProjectile, AmmoKind: "arrow"}
	defStats := Stats{AC: 10}
	// HitMod 100 would always hit, so a recorded hit (or miss) means the
	// swing fired; we assert it did NOT — the dry gate pre-empts it. No
	// rolls should be consumed (the gate returns before rollHit).
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, nil)
	rig.ammoFor = func(CombatantID) (bool, int) { return false, 0 }
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	dry := rig.sink.snapshotRangedDry()
	if len(dry) != 1 {
		t.Fatalf("want 1 RangedDry, got %d", len(dry))
	}
	if dry[0].AmmoKind != "arrow" {
		t.Errorf("RangedDry = %+v, want AmmoKind=arrow", dry[0])
	}
	if h, m := len(rig.sink.snapshotHits()), len(rig.sink.snapshotMisses()); h != 0 || m != 0 {
		t.Errorf("no swing should land/miss when dry: hits=%d misses=%d", h, m)
	}
	// Still engaged — combat did not disengage on a dry swing.
	if _, ok := rig.mgr.PrimaryTargetOf(rig.attacker.id); !ok {
		t.Error("attacker should remain engaged after a dry swing")
	}
}

// A projectile swing WITH ammo fires, and the masterwork-ammo to-hit bonus
// is folded into the swing's roll: a roll that would miss without the bonus
// hits with it.
func TestAutoAttack_ProjectileWithAmmo_FoldsToHitBonus(t *testing.T) {
	// HitMod 0 vs AC 15: a raw d20 of 14 (+1 = 15) ties AC. Without the ammo
	// bonus that misses (need to MEET AC? use a margin). Roll 13 → 14 total:
	// misses AC 15 by 1; +2 ammo bonus → 16 ≥ 15 hits. One swing, one damage
	// roll (1d3 unarmed since no Damage set).
	atkStats := Stats{HitMod: 0, STR: 10, RangedClass: RangedProjectile, AmmoKind: "arrow"}
	defStats := Stats{AC: 15}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{
		13, // d20: 13+1 = 14; +2 ammo = 16 ≥ 15 → hit
		0,  // damage 1d3: 0+1 = 1
	})
	rig.ammoFor = func(CombatantID) (bool, int) { return true, 2 }
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if h := len(rig.sink.snapshotHits()); h != 1 {
		t.Fatalf("want 1 hit (ammo bonus carries the swing over AC), got %d (misses=%d, dry=%d)",
			h, len(rig.sink.snapshotMisses()), len(rig.sink.snapshotRangedDry()))
	}
	if d := len(rig.sink.snapshotRangedDry()); d != 0 {
		t.Errorf("a swing with ammo should not be dry, got %d dry", d)
	}
}

// A melee weapon never consults the ammo hook (the hook would force a dry
// swing if called) — it swings normally.
func TestAutoAttack_MeleeIgnoresAmmoHook(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10} // RangedClass empty = melee
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{
		10, // d20: 10+1+100 → hit
		0,  // damage 1d3
	})
	called := false
	rig.ammoFor = func(CombatantID) (bool, int) { called = true; return false, 0 }
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if called {
		t.Error("AmmoFor must not be called for a melee weapon")
	}
	if h := len(rig.sink.snapshotHits()); h != 1 {
		t.Errorf("melee swing should land, got %d hits", h)
	}
}
