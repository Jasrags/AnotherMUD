package combat

import (
	"context"
	"testing"
)

// action-economy.md §7.1 — a reload-gated projectile (ReloadTicks > 0) that is
// NOT chambered skips its swing (a RangedDry with Unloaded=true) and stays
// engaged; no ammo hook is consulted and no shot lands.
func TestAutoAttack_CrossbowUnloaded_SkipsSwing(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10, RangedClass: RangedProjectile, AmmoKind: "bolt", ReloadTicks: 20}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, nil)
	ammoCalled := false
	rig.ammoFor = func(CombatantID) (bool, int) { ammoCalled = true; return true, 0 }
	rig.takeLoadedShot = func(CombatantID) bool { return false } // unloaded

	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	dry := rig.sink.snapshotRangedDry()
	if len(dry) != 1 || !dry[0].Unloaded {
		t.Fatalf("want 1 RangedDry{Unloaded:true}, got %+v", dry)
	}
	if h, m := len(rig.sink.snapshotHits()), len(rig.sink.snapshotMisses()); h != 0 || m != 0 {
		t.Errorf("an unloaded crossbow lands no swing: hits=%d misses=%d", h, m)
	}
	if ammoCalled {
		t.Error("a reload-gated weapon must NOT consult the ammo hook (ammo spent at load)")
	}
	if _, ok := rig.mgr.PrimaryTargetOf(rig.attacker.id); !ok {
		t.Error("attacker should remain engaged after an unloaded swing")
	}
}

// A loaded crossbow fires (no ammo consume) and takes its loaded shot via
// TakeLoadedShot — exactly once, whether the shot hits or misses.
func TestAutoAttack_CrossbowLoaded_FiresAndDischarges(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10, RangedClass: RangedProjectile, AmmoKind: "bolt", ReloadTicks: 20}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{10, 0}) // d20 hit + 1d3 dmg
	ammoCalled := false
	rig.ammoFor = func(CombatantID) (bool, int) { ammoCalled = true; return true, 0 }
	takes := 0
	rig.takeLoadedShot = func(CombatantID) bool { takes++; return true }

	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if h := len(rig.sink.snapshotHits()); h != 1 {
		t.Fatalf("want 1 hit from a loaded crossbow, got %d (dry=%d)", h, len(rig.sink.snapshotRangedDry()))
	}
	if takes != 1 {
		t.Errorf("TakeLoadedShot calls = %d, want 1 (the loaded bolt is loosed)", takes)
	}
	if ammoCalled {
		t.Error("a reload-gated weapon must not consume ammo on fire (spent at load)")
	}
}

// A nil TakeLoadedShot hook (un-wired / a mob) fires freely — a reload-gated weapon
// then behaves like an ordinary projectile, never blocked for want of a load.
func TestAutoAttack_CrossbowNilHook_FiresFreely(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10, RangedClass: RangedProjectile, AmmoKind: "bolt", ReloadTicks: 20}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{10, 0})
	// takeLoadedShot + ammoFor left nil.
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if h := len(rig.sink.snapshotHits()); h != 1 {
		t.Fatalf("a reload-gated weapon with a nil loaded hook should fire freely, got %d hits / %d dry",
			h, len(rig.sink.snapshotRangedDry()))
	}
}
