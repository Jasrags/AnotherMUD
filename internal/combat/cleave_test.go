package combat

import (
	"context"
	"testing"
)

// cleaveScene builds an attacker engaged with TWO foes in the same room and runs
// one auto-attack round with the given CleaveFor capability. The roll sequence
// drives the swings (each swing = one d20 hit roll + one 1d3 unarmed damage
// roll). foe1 is the primary target (engaged first); foe2 is the cleave target.
func cleaveScene(t *testing.T, cleaveFor func(CombatantID) (bool, bool), foe1HP, foe2HP int, rollSeq []int) (*recordingSink, CombatantID, CombatantID) {
	t.Helper()
	atkID := NewPlayerCombatantID("atk")
	f1 := NewMobCombatantID("foe1")
	f2 := NewMobCombatantID("foe2")
	attacker := &liveCombatant{id: atkID, name: "attacker", vitals: NewVitals(20), stats: Stats{STR: 10}}
	foe1 := &liveCombatant{id: f1, name: "foe1", vitals: NewVitals(foe1HP), stats: Stats{AC: 10}}
	foe2 := &liveCombatant{id: f2, name: "foe2", vitals: NewVitals(foe2HP), stats: Stats{AC: 10}}
	loc := MapLocator{atkID: attacker, f1: foe1, f2: foe2}
	rooms := MapRoomLocator{atkID: roomA, f1: roomA, f2: roomA}
	sink := &recordingSink{}
	mgr := NewManager(loc, sink)
	mgr.Engage(context.Background(), atkID, f1, roomA) // foe1 is primary (list[0])
	mgr.Engage(context.Background(), atkID, f2, roomA)
	roller := &scriptedRoller{t: t, seq: rollSeq}

	phase := NewAutoAttack(AutoAttackConfig{
		Locator:     loc,
		RoomLocator: rooms,
		Sink:        sink,
		Roller:      roller,
		CleaveFor:   cleaveFor,
	})
	phase(context.Background(), atkID, mgr, 0)
	return sink, f1, f2
}

// hitRoll lands a hit (d20 = 15 vs AC 10, not a crit); dmgRoll 0 → 1 damage.
const cleaveHitRoll = 14

// Cleave grants ONE bonus melee swing at another engaged foe when a swing drops
// the primary target. The bonus swing is aimed at foe2 (the second engaged foe).
func TestCleave_BonusSwingOnKill(t *testing.T) {
	// Kill foe1 (1 HP), then the bonus swing strikes foe2 (50 HP, survives).
	sink, f1, f2 := cleaveScene(t, func(CombatantID) (bool, bool) { return true, false }, 1, 50,
		[]int{cleaveHitRoll, 0 /* kill foe1 */, cleaveHitRoll, 0 /* cleave foe2 */})

	hits := sink.snapshotHits()
	if len(hits) != 2 {
		t.Fatalf("want 2 hits (kill + cleave), got %d", len(hits))
	}
	if hits[1].TargetID != f2 {
		t.Errorf("cleave swing targeted %v, want foe2 %v", hits[1].TargetID, f2)
	}
	deaths := sink.snapshotDeaths()
	if len(deaths) != 1 || deaths[0].VictimID != f1 {
		t.Errorf("want exactly foe1 dead, got %+v", deaths)
	}
}

// Without the Cleave capability, a kill ends the round with no bonus swing — the
// pre-feat behavior. Proves the seam is inert when CleaveFor reports false.
func TestCleave_NoCapabilityNoBonus(t *testing.T) {
	sink, f1, _ := cleaveScene(t, func(CombatantID) (bool, bool) { return false, false }, 1, 50,
		[]int{cleaveHitRoll, 0 /* kill foe1 */})

	if h := len(sink.snapshotHits()); h != 1 {
		t.Fatalf("want 1 hit (no cleave), got %d", h)
	}
	if d := sink.snapshotDeaths(); len(d) != 1 || d[0].VictimID != f1 {
		t.Errorf("want only foe1 dead, got %+v", d)
	}
}

// Great Cleave keeps cleaving while each bonus swing also drops a foe: killing
// foe1 cleaves into foe2, killing foe2 looks for a third foe and (finding none)
// stops. Two kills from one round.
func TestGreatCleave_ChainsUntilNoFoe(t *testing.T) {
	sink, f1, f2 := cleaveScene(t, func(CombatantID) (bool, bool) { return true, true }, 1, 1,
		[]int{cleaveHitRoll, 0 /* kill foe1 */, cleaveHitRoll, 0 /* cleave-kill foe2 */})

	if h := len(sink.snapshotHits()); h != 2 {
		t.Fatalf("want 2 hits (chain), got %d", h)
	}
	deaths := sink.snapshotDeaths()
	if len(deaths) != 2 {
		t.Fatalf("want 2 deaths (great cleave chain), got %d", len(deaths))
	}
	if deaths[0].VictimID != f1 || deaths[1].VictimID != f2 {
		t.Errorf("death order = %v then %v, want foe1 then foe2", deaths[0].VictimID, deaths[1].VictimID)
	}
}

// Plain Cleave is capped at one bonus swing per round: killing foe1 cleaves once
// into foe2 and stops even though that bonus swing ALSO kills foe2 — no third
// swing is sought. (Contrast TestGreatCleave_ChainsUntilNoFoe.)
func TestCleave_CapsAtOneBonusSwing(t *testing.T) {
	sink, _, _ := cleaveScene(t, func(CombatantID) (bool, bool) { return true, false }, 1, 1,
		[]int{cleaveHitRoll, 0 /* kill foe1 */, cleaveHitRoll, 0 /* cleave-kill foe2 */})

	// Two hits / two deaths is fine — the point is the round STOPPED after the
	// single bonus swing (the scriptedRoller would fatal on a 3rd swing's rolls).
	if h := len(sink.snapshotHits()); h != 2 {
		t.Fatalf("want exactly 2 hits (kill + one bonus), got %d", h)
	}
}
