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

// A holder-fed firearm (a pistol) that can't fire is UNLOADED, not dry: the
// RangedDry carries Unloaded=true so the sink narrates "reload first" rather
// than "out of ammo". The distinction is what tells the player the fix is a
// reload (the clip is out / absent), not scavenging loose rounds.
func TestAutoAttack_HolderFedEmpty_MarksUnloaded(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10, RangedClass: RangedProjectile, AmmoKind: "bullet", AcceptsHolder: "heavy-pistol"}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, nil)
	rig.ammoFor = func(CombatantID) (bool, int) { return false, 0 }
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	dry := rig.sink.snapshotRangedDry()
	if len(dry) != 1 || !dry[0].Unloaded {
		t.Fatalf("want 1 RangedDry{Unloaded:true} for an empty firearm, got %+v", dry)
	}
}

// An internally-fed magazine weapon with an empty magazine is likewise
// Unloaded (reload it), not dry.
func TestAutoAttack_MagazineEmpty_MarksUnloaded(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10, RangedClass: RangedProjectile, AmmoKind: "bullet", Magazine: 15}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, nil)
	rig.ammoFor = func(CombatantID) (bool, int) { return false, 0 }
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	dry := rig.sink.snapshotRangedDry()
	if len(dry) != 1 || !dry[0].Unloaded {
		t.Fatalf("want 1 RangedDry{Unloaded:true} for an empty magazine, got %+v", dry)
	}
}

// A loose-ammo weapon (a bow drawing from the quiver) that runs out is DRY, not
// unloaded: Unloaded stays false so the message reads "out of arrows", not
// "reload first" — there is no clip to reseat.
func TestAutoAttack_BowOutOfAmmo_NotUnloaded(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10, RangedClass: RangedProjectile, AmmoKind: "arrow"} // no holder, no magazine
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, nil)
	rig.ammoFor = func(CombatantID) (bool, int) { return false, 0 }
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	dry := rig.sink.snapshotRangedDry()
	if len(dry) != 1 || dry[0].Unloaded {
		t.Fatalf("want 1 RangedDry{Unloaded:false} for a loose-ammo bow, got %+v", dry)
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

// ResolveSingleAttack (the throw one-shot, ranged-combat §3) resolves exactly
// one swing through the shared resolver: a thrown weapon's full-STR damage is
// applied and a Hit is emitted through the Manager's sink.
func TestResolveSingleAttack_OneShotHit(t *testing.T) {
	knife, err := ParseDice("1d4")
	if err != nil {
		t.Fatalf("ParseDice: %v", err)
	}
	atkStats := Stats{HitMod: 100, DamageBonus: 2, Damage: knife, WeaponName: "a throwing knife", RangedClass: RangedThrown}
	defStats := Stats{AC: 10}
	// roll seq: d20 index 5 (=6, +100 → hit, not a crit), then 1d4 index 0 (=1).
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{5, 0})
	alive := rig.mgr.ResolveSingleAttack(context.Background(), rig.attacker.id, rig.target.id, roomA, rig.roller, 0)

	if !alive {
		t.Error("target should survive a 3-damage knife at 20 HP")
	}
	hits := rig.sink.snapshotHits()
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d (misses=%d)", len(hits), len(rig.sink.snapshotMisses()))
	}
	if hits[0].Damage != 3 { // 1 die + 2 full-STR bonus
		t.Errorf("damage = %d, want 3 (1 die + 2 thrown STR bonus)", hits[0].Damage)
	}
	if hits[0].WeaponName != "a throwing knife" {
		t.Errorf("weapon = %q, want the knife", hits[0].WeaponName)
	}
}

// ranged-combat §5.2/§5.3 — band effectiveness in the round loop.

// A melee combatant out of melee range CLOSES one band this round instead of
// swinging (the auto-close); no hit/miss is emitted, a BandChange is.
func TestAutoAttack_MeleeOutOfRangeCloses(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10} // melee (no RangedClass)
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, nil) // no rolls — no swing expected
	// Push the engagement out to the far band.
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, +farBand())
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	bc := rig.sink.snapshotBandChanges()
	if len(bc) != 1 || !bc[0].Closing {
		t.Fatalf("want 1 closing BandChange, got %+v", bc)
	}
	if got := rig.mgr.BandOf(rig.attacker.id, rig.target.id); got != farBand()-1 {
		t.Errorf("band after close = %d, want %d (one step toward melee)", got, farBand()-1)
	}
	if h, m := len(rig.sink.snapshotHits()), len(rig.sink.snapshotMisses()); h != 0 || m != 0 {
		t.Errorf("a closing round lands no swing: hits=%d misses=%d", h, m)
	}
}

// A projectile SHOOTS from range (doesn't auto-close); the per-band falloff
// applies — a roll that hits at melee misses at far.
func TestAutoAttack_ProjectileShootsFromRangeWithFalloff(t *testing.T) {
	atkStats := Stats{HitMod: 0, RangedClass: RangedProjectile, AmmoKind: "arrow"}
	defStats := Stats{AC: 10}
	// d20 index 14 = raw 15. At melee (no falloff) 15 ≥ 10 hits; at far with
	// falloff 100, 15 - 200 misses. Same roll, opposite outcome by band.
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{14})
	rig.falloff = 100
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, +farBand())
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if bc := rig.sink.snapshotBandChanges(); len(bc) != 0 {
		t.Fatalf("a projectile shoots from range, it does not auto-close: %+v", bc)
	}
	if m := len(rig.sink.snapshotMisses()); m != 1 {
		t.Fatalf("want 1 miss (falloff drops the shot under AC), got %d miss / %d hit",
			m, len(rig.sink.snapshotHits()))
	}
}

// Vision Magnification (ranged-combat §5.3): the SAME far shot that misses under
// full falloff HITS when the attacker's optics treat the target one band closer
// (magBands 1 → far becomes the nearer band's smaller falloff). Same roll, same
// band, opposite outcome — proving magnification cut the range penalty.
func TestAutoAttack_ProjectileMagnificationReducesFalloff(t *testing.T) {
	// AC 25 sits between the far-band penalty and the magnified (one-band-closer)
	// penalty for a raw-15 roll with falloff 10: far (band 2) → 15-20 misses;
	// magnified to band 1 → 15-10 = 5 ... so pick numbers that straddle AC.
	// raw 15, falloff 10, farBand = 2: unmagnified 15-20 = -5; magnified 15-10 = 5.
	// AC 0 → magnified (5) hits, unmagnified (-5) misses.
	atkStats := Stats{HitMod: 0, RangedClass: RangedProjectile, AmmoKind: "arrow", HasRangeMagnification: true}
	defStats := Stats{AC: 0}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{14, 0}) // d20=15, then dmg
	rig.falloff = 10
	rig.magBands = 1
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, +farBand())
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if h := len(rig.sink.snapshotHits()); h != 1 {
		t.Fatalf("magnification should pull a far shot into hitting range: got %d hit / %d miss",
			h, len(rig.sink.snapshotMisses()))
	}

	// Control: without magnification the identical far shot misses.
	atkNoMag := Stats{HitMod: 0, RangedClass: RangedProjectile, AmmoKind: "arrow"}
	rig2 := newAutoAttackRig(t, atkNoMag, defStats, 10, 20, []int{14})
	rig2.falloff = 10
	rig2.magBands = 1 // config present, but the attacker lacks the capability
	rig2.mgr.AdjustBand(rig2.attacker.id, rig2.target.id, +farBand())
	rig2.phase()(context.Background(), rig2.attacker.id, rig2.mgr, 0)
	if m := len(rig2.sink.snapshotMisses()); m != 1 {
		t.Fatalf("without magnification the far shot must still miss: got %d miss / %d hit",
			m, len(rig2.sink.snapshotHits()))
	}
}

// Invariant: Vision Magnification does NOT reduce the point-blank penalty — it
// helps at range, not up close. At the melee band a magnified projectile still
// eats the full point-blank penalty and misses.
func TestAutoAttack_MagnificationDoesNotHelpPointBlank(t *testing.T) {
	atkStats := Stats{HitMod: 0, RangedClass: RangedProjectile, AmmoKind: "arrow", HasRangeMagnification: true}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{14}) // raw 15
	rig.pblank = 10                                                   // 15 - 10 = 5 < AC 10 → miss
	rig.magBands = 5                                                  // generous, but must not touch point-blank
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, -farBand())    // pull to melee band
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if m := len(rig.sink.snapshotMisses()); m != 1 {
		t.Fatalf("magnification must not offset the point-blank penalty: got %d miss / %d hit",
			m, len(rig.sink.snapshotHits()))
	}
}

// The same projectile roll HITS at the melee band (no falloff there) — proving
// the miss above was the band falloff, not the roll.
func TestAutoAttack_ProjectileHitsAtMeleeBand(t *testing.T) {
	atkStats := Stats{HitMod: 0, RangedClass: RangedProjectile, AmmoKind: "arrow"}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{14, 0}) // d20=15 hit, then 1d3 dmg
	rig.falloff = 100                                                    // irrelevant at the melee band
	// A projectile engage auto-opens at far; pull it in to the melee band
	// (pblank stays 0, so no point-blank penalty for this roll).
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, -farBand())
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if h := len(rig.sink.snapshotHits()); h != 1 {
		t.Fatalf("want 1 hit at the melee band, got %d hit / %d miss", h, len(rig.sink.snapshotMisses()))
	}
}

// ranged-combat §5.4 (MR2 kiting AI): a projectile combatant with room to open
// the band withdraws this round instead of shooting when KitePolicy says so —
// no shot, a band-opening BandChange, and the band steps toward far.
func TestAutoAttack_ProjectileKitesInsteadOfShooting(t *testing.T) {
	atkStats := Stats{HitMod: 100, RangedClass: RangedProjectile, AmmoKind: "arrow"}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, nil) // no rolls — no shot expected
	// Pull the band in to near (room to withdraw toward far).
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, +farBand()) // far
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, -1)         // near
	rig.kite = func(CombatantID, CombatantID, int) bool { return true }
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	bc := rig.sink.snapshotBandChanges()
	if len(bc) != 1 || bc[0].Closing {
		t.Fatalf("want 1 opening (withdraw) BandChange, got %+v", bc)
	}
	if got := rig.mgr.BandOf(rig.attacker.id, rig.target.id); got != farBand() {
		t.Errorf("band after kite = %d, want far (%d)", got, farBand())
	}
	if h, m := len(rig.sink.snapshotHits()), len(rig.sink.snapshotMisses()); h != 0 || m != 0 {
		t.Errorf("a kiting round fires no shot: hits=%d misses=%d", h, m)
	}
}

// KitePolicy returning false (or at the far band, no room) means the projectile
// shoots normally — kiting never blocks a shot it shouldn't.
func TestAutoAttack_ProjectileShootsWhenNotKiting(t *testing.T) {
	atkStats := Stats{HitMod: 100, RangedClass: RangedProjectile, AmmoKind: "arrow"}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{5, 0}) // d20 hit + 1d3 dmg
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, +farBand())      // far
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, -1)              // near (room to kite)
	rig.kite = func(CombatantID, CombatantID, int) bool { return false }
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if len(rig.sink.snapshotBandChanges()) != 0 {
		t.Error("a non-kiting projectile should not move the band")
	}
	if h := len(rig.sink.snapshotHits()); h != 1 {
		t.Fatalf("want 1 hit when not kiting, got %d", h)
	}
}

// At the far band there is no room to withdraw, so KitePolicy isn't even
// consulted — the projectile shoots.
func TestAutoAttack_NoKiteAtFarBand(t *testing.T) {
	atkStats := Stats{HitMod: 100, RangedClass: RangedProjectile, AmmoKind: "arrow"}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{5, 0})
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, +farBand()) // far — no room to kite
	called := false
	rig.kite = func(CombatantID, CombatantID, int) bool { called = true; return true }
	rig.falloff = 0 // shoot lands
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if called {
		t.Error("KitePolicy must not be consulted at the far band (no room to withdraw)")
	}
	if h := len(rig.sink.snapshotHits()); h != 1 {
		t.Errorf("want 1 hit at far, got %d", h)
	}
}
