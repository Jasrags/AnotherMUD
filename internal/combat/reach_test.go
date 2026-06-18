package combat

import (
	"context"
	"testing"
)

// special-weapons §3 — reach. A reach melee weapon (Reach > 0) strikes at the
// `near` band as well as melee; a non-reach melee weapon at `near` must close
// first. Reach is one band, not unlimited — at `far` even a reach weapon closes.

// A reach weapon swings at the near band instead of auto-closing.
func TestAutoAttack_ReachStrikesAtNear(t *testing.T) {
	atkStats := Stats{HitMod: 0, STR: 10, Reach: 1} // reach melee weapon
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{14, 0}) // d20=15 hits AC10, then 1d3 dmg
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, +1)               // melee → near
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if bc := rig.sink.snapshotBandChanges(); len(bc) != 0 {
		t.Fatalf("a reach weapon strikes at near, it does not close: %+v", bc)
	}
	if h := len(rig.sink.snapshotHits()); h != 1 {
		t.Fatalf("want 1 hit at near (reach), got %d hit / %d miss", h, len(rig.sink.snapshotMisses()))
	}
}

// A reach weapon at the melee band swings normally (reach adds the near band, it
// does not change melee-band behavior).
func TestAutoAttack_ReachStrikesAtMelee(t *testing.T) {
	atkStats := Stats{HitMod: 0, STR: 10, Reach: 1}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{14, 0}) // d20=15 hits, 1d3 dmg
	// No band adjustment — the engagement opens at melee (band 0).
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if bc := rig.sink.snapshotBandChanges(); len(bc) != 0 {
		t.Fatalf("a reach weapon at melee swings, no band change: %+v", bc)
	}
	if h := len(rig.sink.snapshotHits()); h != 1 {
		t.Fatalf("want 1 hit at melee, got %d hit / %d miss", h, len(rig.sink.snapshotMisses()))
	}
}

// A non-reach melee weapon at near closes a band instead of swinging (today's
// behavior — the contrast that proves reach is what changed the outcome).
func TestAutoAttack_NonReachAtNearCloses(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10} // no reach
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, nil) // no swing expected
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, +1)      // near
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	bc := rig.sink.snapshotBandChanges()
	if len(bc) != 1 || !bc[0].Closing {
		t.Fatalf("a non-reach melee weapon at near closes: %+v", bc)
	}
	if h := len(rig.sink.snapshotHits()); h != 0 {
		t.Errorf("no swing while closing, got %d hits", h)
	}
}

// Reach is one band: a reach weapon at far still auto-closes (no sniping across
// the whole engagement).
func TestAutoAttack_ReachStillClosesFromFar(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10, Reach: 1}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, nil) // closes, no swing
	rig.mgr.AdjustBand(rig.attacker.id, rig.target.id, +farBand())
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	bc := rig.sink.snapshotBandChanges()
	if len(bc) != 1 || !bc[0].Closing {
		t.Fatalf("reach is one band — a reach weapon at far still closes: %+v", bc)
	}
	if got := rig.mgr.BandOf(rig.attacker.id, rig.target.id); got != farBand()-1 {
		t.Errorf("band after close = %d, want %d (one step toward melee)", got, farBand()-1)
	}
}
