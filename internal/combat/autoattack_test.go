package combat

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// liveCombatant is a writable Combatant whose Vitals are mutable for
// the duration of a test. The manager-test staticCombatant returns a
// nil Vitals, which the auto-attack phase would crash on; this rig
// supplies a real Vitals so swing-by-swing damage is observable.
type liveCombatant struct {
	id     CombatantID
	name   string
	vitals *Vitals
	stats  Stats
}

func (l *liveCombatant) CombatantID() CombatantID { return l.id }
func (l *liveCombatant) Name() string             { return l.name }
func (l *liveCombatant) Vitals() *Vitals          { return l.vitals }
func (l *liveCombatant) Stats() Stats             { return l.stats }

// scriptedRoller serves a programmed sequence of IntN results. Reused
// from damage_test.go's fixedRoller pattern but kept distinct so the
// auto-attack tests don't share state with the damage tests.
type scriptedRoller struct {
	t   *testing.T
	seq []int
	idx int
}

func (s *scriptedRoller) IntN(n int) int {
	if s.idx >= len(s.seq) {
		s.t.Fatalf("scriptedRoller exhausted after %d rolls", s.idx)
	}
	v := s.seq[s.idx]
	s.idx++
	if v < 0 || v >= n {
		s.t.Fatalf("scriptedRoller value %d out of range [0,%d)", v, n)
	}
	return v
}

const (
	roomA world.RoomID = "tapestry-core:room-a"
	roomB world.RoomID = "tapestry-core:room-b"
)

// autoAttackRig builds attacker + target with default stats (HitMod 0,
// AC 10, STR 10, no weapon damage = unarmed default) plus a recording
// sink, a manager engaged on the pair, and a scriptedRoller.
type autoAttackRig struct {
	mgr      *Manager
	sink     *recordingSink
	attacker *liveCombatant
	target   *liveCombatant
	locator  MapLocator
	rooms    MapRoomLocator
	roller   *scriptedRoller
}

func newAutoAttackRig(t *testing.T, atkStats, defStats Stats, atkHP, defHP int, rollSeq []int) *autoAttackRig {
	t.Helper()
	atkID := NewPlayerCombatantID("atk")
	tgtID := NewMobCombatantID("tgt")
	rig := &autoAttackRig{
		attacker: &liveCombatant{id: atkID, name: "attacker", vitals: NewVitals(atkHP), stats: atkStats},
		target:   &liveCombatant{id: tgtID, name: "target", vitals: NewVitals(defHP), stats: defStats},
		sink:     &recordingSink{},
		locator:  MapLocator{},
		rooms:    MapRoomLocator{},
		roller:   &scriptedRoller{t: t, seq: rollSeq},
	}
	rig.locator[atkID] = rig.attacker
	rig.locator[tgtID] = rig.target
	rig.rooms[atkID] = roomA
	rig.rooms[tgtID] = roomA
	rig.mgr = NewManager(rig.locator, rig.sink)
	rig.mgr.Engage(context.Background(), atkID, tgtID, roomA)
	return rig
}

func (r *autoAttackRig) phase() PhaseFunc {
	return NewAutoAttack(AutoAttackConfig{
		Locator:     r.locator,
		RoomLocator: r.rooms,
		Sink:        r.sink,
		Roller:      r.roller,
	})
}

func TestAutoAttackNaturalTwentyAlwaysHits(t *testing.T) {
	// Attacker has crushing -100 HitMod that would otherwise miss any
	// AC. Roller returns 19 → raw d20 = 20 → critical override.
	atkStats := Stats{HitMod: -100, STR: 10}
	defStats := Stats{AC: 100}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{
		19, // d20: 19+1 = 20, crit
		0,  // damage 1d3: 0+1 = 1
	})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d (misses=%d)", len(hits), len(rig.sink.snapshotMisses()))
	}
	if !hits[0].IsCritical {
		t.Error("nat-20 must set IsCritical")
	}
	if hits[0].Damage < 1 {
		t.Errorf("damage must be >= 1, got %d", hits[0].Damage)
	}
}

func TestAutoAttackNaturalOneAlwaysMisses(t *testing.T) {
	// Attacker has overwhelming +100 HitMod that would otherwise auto-
	// hit. Roller returns 0 → raw d20 = 1 → fumble override.
	atkStats := Stats{HitMod: 100, STR: 10}
	defStats := Stats{AC: 5}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{0})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr)

	misses := rig.sink.snapshotMisses()
	if len(misses) != 1 {
		t.Fatalf("want 1 miss, got %d (hits=%d)", len(misses), len(rig.sink.snapshotHits()))
	}
	if !misses[0].IsFumble {
		t.Error("nat-1 must set IsFumble")
	}
}

func TestAutoAttackHitAppliesDamageClampedToOne(t *testing.T) {
	// Roll a mid d20 that auto-hits (10+10 vs AC 10), damage 1d3 with
	// min roll (1) and a -10 STR penalty → raw = 1 + -10 = -9 → clamp 1.
	atkStats := Stats{HitMod: 10, STR: 0}
	defStats := Stats{AC: 10}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 5, []int{
		9, // d20: 10
		0, // damage 1d3: 1
	})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d", len(hits))
	}
	if hits[0].Damage != 1 {
		t.Errorf("damage clamp: got %d, want 1", hits[0].Damage)
	}
	if got := rig.target.Vitals().Current(); got != 4 {
		t.Errorf("target HP after 1 damage: got %d, want 4", got)
	}
}

func TestAutoAttackDepletesVitalAndStops(t *testing.T) {
	// Multi-swing scenario: 1 extra attack would be needed for two
	// swings, but M7.4 stubs extra-attack to 0, so we only get one
	// swing. Use STR to push damage past the target's HP and
	// observe the vital-depleted event.
	atkStats := Stats{HitMod: 10, STR: 20} // STR 20 → +5 damage
	defStats := Stats{AC: 5}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 3, []int{
		9, // d20: 10, auto-hit vs AC 5
		2, // damage 1d3: 3 → +5 STR = 8 damage on 3HP target
	})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr)

	deaths := rig.sink.snapshotDeaths()
	if len(deaths) != 1 {
		t.Fatalf("want 1 vital-depleted, got %d", len(deaths))
	}
	if deaths[0].Vital != VitalHP {
		t.Errorf("vital field = %q, want %q", deaths[0].Vital, VitalHP)
	}
	if deaths[0].VictimID != rig.target.id {
		t.Errorf("victim id mismatch")
	}
	if deaths[0].AttackerID != rig.attacker.id {
		t.Errorf("attacker attribution missing")
	}
	if !rig.target.Vitals().IsDead() {
		t.Error("target should be dead")
	}
}

func TestAutoAttackPreflightDifferentRoomDisengages(t *testing.T) {
	rig := newAutoAttackRig(t, Stats{}, Stats{AC: 10}, 10, 10, nil)
	rig.rooms[rig.target.id] = roomB // target moved

	rig.phase()(context.Background(), rig.attacker.id, rig.mgr)

	if len(rig.sink.snapshotHits())+len(rig.sink.snapshotMisses()) != 0 {
		t.Error("preflight should skip the swing entirely")
	}
	if rig.mgr.InCombat(rig.attacker.id) {
		t.Error("attacker should be disengaged after preflight room mismatch")
	}
}

func TestAutoAttackPreflightDeadTargetDisengages(t *testing.T) {
	rig := newAutoAttackRig(t, Stats{}, Stats{AC: 10}, 10, 10, nil)
	rig.target.vitals.ApplyDamage(100) // pre-kill target

	rig.phase()(context.Background(), rig.attacker.id, rig.mgr)

	if len(rig.sink.snapshotHits())+len(rig.sink.snapshotMisses()) != 0 {
		t.Error("preflight should skip the swing on dead target")
	}
	if rig.mgr.InCombat(rig.attacker.id) {
		t.Error("attacker should be disengaged after preflight dead target")
	}
}

func TestAutoAttackPreflightAttackerMissingFromLocatorDisengages(t *testing.T) {
	rig := newAutoAttackRig(t, Stats{}, Stats{AC: 10}, 10, 10, nil)
	// Simulate the attacker disappearing between the round snapshot
	// and the phase callback (e.g. mid-round logout). Manager still
	// has the attacker engaged because nothing cleaned it up yet.
	delete(rig.locator, rig.attacker.id)

	rig.phase()(context.Background(), rig.attacker.id, rig.mgr)

	if len(rig.sink.snapshotHits())+len(rig.sink.snapshotMisses()) != 0 {
		t.Error("missing attacker should produce no swing events")
	}
	if rig.mgr.InCombat(rig.attacker.id) {
		t.Error("missing attacker should be DisengageAll'd, leaving lists empty")
	}
	// The opposite side also clears since DisengageAll cleans up
	// symmetric state.
	if rig.mgr.InCombat(rig.target.id) {
		t.Error("target should be disengaged when attacker disappears")
	}
}

func TestAutoAttackPreflightAttackerRoomMissingDisengages(t *testing.T) {
	rig := newAutoAttackRig(t, Stats{}, Stats{AC: 10}, 10, 10, nil)
	// Attacker resolves via locator but has no tracked room.
	delete(rig.rooms, rig.attacker.id)

	rig.phase()(context.Background(), rig.attacker.id, rig.mgr)

	if len(rig.sink.snapshotHits())+len(rig.sink.snapshotMisses()) != 0 {
		t.Error("attacker with no room should produce no swing events")
	}
	if rig.mgr.InCombat(rig.attacker.id) {
		t.Error("attacker with no room should be DisengageAll'd")
	}
}

func TestAutoAttackPreflightNoTargetReturns(t *testing.T) {
	rig := newAutoAttackRig(t, Stats{}, Stats{AC: 10}, 10, 10, nil)
	// Force-disengage so the attacker has no primary target.
	rig.mgr.DisengageAll(context.Background(), rig.attacker.id, roomA)

	// Must not panic, must not emit anything.
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr)

	if len(rig.sink.snapshotHits())+len(rig.sink.snapshotMisses()) != 0 {
		t.Error("no swings should emit when attacker has no target")
	}
}

func TestSortPlayersFirstStable(t *testing.T) {
	cases := []struct {
		in   []CombatantID
		want []CombatantID
	}{
		{nil, nil},
		{
			[]CombatantID{NewMobCombatantID("a"), NewPlayerCombatantID("p1")},
			[]CombatantID{NewPlayerCombatantID("p1"), NewMobCombatantID("a")},
		},
		{
			// Stability: relative order of like-kinds preserved.
			[]CombatantID{
				NewMobCombatantID("m1"),
				NewPlayerCombatantID("p1"),
				NewMobCombatantID("m2"),
				NewPlayerCombatantID("p2"),
			},
			[]CombatantID{
				NewPlayerCombatantID("p1"),
				NewPlayerCombatantID("p2"),
				NewMobCombatantID("m1"),
				NewMobCombatantID("m2"),
			},
		},
	}
	for i, c := range cases {
		got := append([]CombatantID(nil), c.in...)
		SortPlayersFirst(got)
		if !equalIDs(got, c.want) {
			t.Errorf("case %d: got %v, want %v", i, got, c.want)
		}
	}
}

func equalIDs(a, b []CombatantID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
