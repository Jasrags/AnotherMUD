package combat

import (
	"context"
	"testing"
)

func bandPair(rangedClass string) (*liveCombatant, *liveCombatant, *Manager) {
	atk := &liveCombatant{id: NewPlayerCombatantID("archer"), name: "Archer", vitals: NewVitals(20), stats: Stats{RangedClass: rangedClass}}
	tgt := &liveCombatant{id: NewMobCombatantID("boar"), name: "a boar", vitals: NewVitals(20), stats: Stats{}}
	loc := MapLocator{atk.id: atk, tgt.id: tgt}
	return atk, tgt, NewManager(loc, &recordingSink{})
}

// ranged-combat §5.2: a fight begun by a ranged-wielding initiator opens at the
// far band; the band is symmetric (per unordered pair) and clears on disengage.
func TestEngage_RangedInitiatorOpensFar(t *testing.T) {
	atk, tgt, m := bandPair(RangedProjectile)
	if _, ok := m.EngageWithReason(context.Background(), atk.id, tgt.id, roomA); !ok {
		t.Fatal("engage failed")
	}
	if got := m.BandOf(atk.id, tgt.id); got != farBand() {
		t.Errorf("ranged-initiated band = %d (%s), want far (%d)", got, BandName(got), farBand())
	}
	// Symmetric: distance is mutual, so the reverse pairing reads the same band.
	if got := m.BandOf(tgt.id, atk.id); got != farBand() {
		t.Errorf("reverse-pair band = %d, want far (symmetric)", got)
	}
	// Disengage ends the engagement and clears the band (reads melee again).
	m.Disengage(context.Background(), atk.id, tgt.id, roomA)
	if got := m.BandOf(atk.id, tgt.id); got != meleeBand {
		t.Errorf("post-disengage band = %d, want melee (cleared)", got)
	}
}

func TestEngage_MeleeInitiatorOpensMelee(t *testing.T) {
	atk, tgt, m := bandPair("") // no ranged weapon = melee
	if _, ok := m.EngageWithReason(context.Background(), atk.id, tgt.id, roomA); !ok {
		t.Fatal("engage failed")
	}
	if got := m.BandOf(atk.id, tgt.id); got != meleeBand {
		t.Errorf("melee-initiated band = %d (%s), want melee", got, BandName(got))
	}
}

// DisengageAll clears every band the combatant was part of (ranged-combat §5).
func TestDisengageAll_ClearsBands(t *testing.T) {
	atk, tgt, m := bandPair(RangedProjectile)
	if _, ok := m.EngageWithReason(context.Background(), atk.id, tgt.id, roomA); !ok {
		t.Fatal("engage failed")
	}
	m.DisengageAll(context.Background(), atk.id, roomA)
	if got := m.BandOf(atk.id, tgt.id); got != meleeBand {
		t.Errorf("post-DisengageAll band = %d, want melee (cleared)", got)
	}
}

// AdjustBand advances (toward melee) and withdraws (toward far), clamped to the
// vocabulary ends (ranged-combat §5.4).
func TestAdjustBand_AdvanceWithdrawClamp(t *testing.T) {
	atk, tgt, m := bandPair(RangedProjectile)
	m.EngageWithReason(context.Background(), atk.id, tgt.id, roomA)
	if got := m.BandOf(atk.id, tgt.id); got != farBand() {
		t.Fatalf("opening band = %d, want far", got)
	}
	// Advance (delta -1) steps toward melee, one band at a time.
	if got := m.AdjustBand(atk.id, tgt.id, -1); got != farBand()-1 {
		t.Errorf("advance once = %d, want %d", got, farBand()-1)
	}
	// Advancing past melee clamps at melee.
	m.AdjustBand(atk.id, tgt.id, -5)
	if got := m.BandOf(atk.id, tgt.id); got != meleeBand {
		t.Errorf("advance past melee = %d, want clamp at melee (0)", got)
	}
	// Withdrawing past far clamps at far.
	m.AdjustBand(atk.id, tgt.id, +5)
	if got := m.BandOf(atk.id, tgt.id); got != farBand() {
		t.Errorf("withdraw past far = %d, want clamp at far (%d)", got, farBand())
	}
}

// MoveBand is the verb-facing kiting move (ranged-combat §5.4): it steps the
// band, emits a BandChange, and reports false at the extremes / when unengaged.
func TestMoveBand_AdvanceWithdraw(t *testing.T) {
	atk, tgt, m := bandPair(RangedProjectile) // opens at far
	sink := m.sink.(*recordingSink)
	m.EngageWithReason(context.Background(), atk.id, tgt.id, roomA)

	// Withdraw at far → already at the extreme, no move.
	if _, moved := m.MoveBand(context.Background(), atk.id, tgt.id, roomA, false); moved {
		t.Error("withdraw at far should report no move (already at the extreme)")
	}
	// Advance (closing) steps toward melee and emits a closing BandChange.
	if band, moved := m.MoveBand(context.Background(), atk.id, tgt.id, roomA, true); !moved || band != farBand()-1 {
		t.Errorf("advance = (%d,%v), want (%d,true)", band, moved, farBand()-1)
	}
	bc := sink.snapshotBandChanges()
	if len(bc) != 1 || !bc[0].Closing || bc[0].SubjectID != atk.id {
		t.Errorf("want 1 closing BandChange by the archer, got %+v", bc)
	}
	// Not engaged → no move.
	other := NewMobCombatantID("stranger")
	if _, moved := m.MoveBand(context.Background(), atk.id, other, roomA, true); moved {
		t.Error("MoveBand against an unengaged opponent should report no move")
	}
}
