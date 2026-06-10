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
	passives PassiveEvaluator // nil ⇒ pre-M9.5 behavior
	critMult int              // 0 ⇒ NewAutoAttack default (DefaultCritMultiplier)
}

// fakePassives is a deterministic PassiveEvaluator for the §4.2/§4.3
// auto-attack hook tests — fixed extra-swing count + a canned evade.
type fakePassives struct {
	extra     int
	evadeName string
	evade     bool
}

func (f fakePassives) ExtraAttacks(string) int              { return f.extra }
func (f fakePassives) DefensiveEvade(string) (string, bool) { return f.evadeName, f.evade }

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
		Locator:        r.locator,
		RoomLocator:    r.rooms,
		Sink:           r.sink,
		Roller:         r.roller,
		Passives:       r.passives,
		CritMultiplier: r.critMult,
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
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

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

// TestAutoAttackCritDoublesDice — a crit multiplies the rolled dice by
// the default (2). STR 10 → 0 bonus isolates the dice doubling.
// TestRollHit_ThreatRange — a weapon's threat-low widens which rolls crit.
// rollHit computes raw = IntN(20)+1, so program IntN = raw-1. AC is set
// unreachable so only an auto-hit crit lands, isolating the threat test.
func TestRollHit_ThreatRange(t *testing.T) {
	roll := func(raw int) *fixedRoller { return &fixedRoller{t: t, values: []int{raw - 1}} }
	const unreachableAC = 100

	cases := []struct {
		raw, threatLow int
		wantCrit       bool
	}{
		{20, 20, true},  // natural max always crits at the default threat
		{19, 20, false}, // 19 does not crit when only 20 threatens
		{19, 19, true},  // widened threat range: 19 now crits
		{18, 19, false}, // 18 is below a 19 threat
		{18, 18, true},  // wider still
	}
	for _, c := range cases {
		out := rollHit(roll(c.raw), 0, unreachableAC, c.threatLow)
		if out.critical != c.wantCrit {
			t.Errorf("raw=%d threatLow=%d: critical=%v, want %v", c.raw, c.threatLow, out.critical, c.wantCrit)
		}
	}

	// A natural 1 is a fumble, never a threat, even with a widened range.
	if out := rollHit(roll(1), 0, unreachableAC, 18); !out.fumble || out.hit {
		t.Errorf("natural 1 should be a fumble miss, got %+v", out)
	}
}

// TestAutoAttack_WeaponThreatRangeCrits — a weapon whose Stats widen the
// threat range crits on a sub-maximum roll (19) it otherwise would not.
func TestAutoAttack_WeaponThreatRangeCrits(t *testing.T) {
	atk := Stats{HitMod: 0, STR: 10, CritThreatLow: 19} // threatens on 19-20
	rig := newAutoAttackRig(t, atk, Stats{AC: 100}, 10, 50, []int{
		18, // d20: 19 → crit only because the weapon threatens at 19
		2,  // 1d3: 3
	})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || !hits[0].IsCritical {
		t.Fatalf("want one critical hit from the widened threat range, got %v", hits)
	}
}

// TestAutoAttack_WeaponCritMultiplierOverridesConfig — the wielded
// weapon's multiplier (3×) wins over the configured global default (2×).
func TestAutoAttack_WeaponCritMultiplierOverridesConfig(t *testing.T) {
	atk := Stats{HitMod: 0, STR: 10, CritMultiplier: 3} // weapon triples
	rig := newAutoAttackRig(t, atk, Stats{AC: 100}, 10, 50, []int{
		19, // d20: 20 → crit
		2,  // 1d3: 3
	})
	// rig.critMult left 0 ⇒ cfg defaults to 2; the weapon's 3 must win.
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || hits[0].Damage != 9 { // 3 dice × 3 weapon mult
		t.Fatalf("crit damage = %v, want one hit of 9 (3×3 from the weapon)", hits)
	}
}

// TestAutoAttack_UnsetCritDefaultsToNatTwenty — the existing-content path:
// with no weapon crit fields, a 19 is an ordinary hit (only the natural
// maximum crits) and no multiplier is applied. Locks behavior preservation
// from the weaponInfo zero-value through Stats() and the default resolution.
func TestAutoAttack_UnsetCritDefaultsToNatTwenty(t *testing.T) {
	atk := Stats{HitMod: 10, STR: 10} // CritThreatLow/CritMultiplier unset (0)
	rig := newAutoAttackRig(t, atk, Stats{AC: 10}, 10, 50, []int{
		18, // d20: 19 → hits (19+10 ≥ 10) but must NOT crit at the default threat
		2,  // 1d3: 3
	})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || hits[0].IsCritical {
		t.Fatalf("a 19 must be an ordinary hit when the threat range is unset, got %v", hits)
	}
	if hits[0].Damage != 3 { // no crit multiplier applied
		t.Errorf("damage = %d, want 3 (no crit)", hits[0].Damage)
	}
}

func TestAutoAttackCritDoublesDice(t *testing.T) {
	atkStats := Stats{HitMod: 0, STR: 10} // STRBonus(10) = 0
	defStats := Stats{AC: 100}            // unreachable but nat-20 crits anyway
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 50, []int{
		19, // d20: 20 → crit
		2,  // 1d3: 3
	})
	// critMult left 0 ⇒ NewAutoAttack defaults to DefaultCritMultiplier (2).
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d", len(hits))
	}
	if !hits[0].IsCritical {
		t.Fatal("expected a critical hit")
	}
	if hits[0].Damage != 6 { // 3 dice × 2 crit + 0 STR
		t.Errorf("crit damage = %d, want 6 (3×2)", hits[0].Damage)
	}
}

// TestAutoAttackCritMultiplierConfigurable — a non-default multiplier
// (3×) scales the dice accordingly.
func TestAutoAttackCritMultiplierConfigurable(t *testing.T) {
	rig := newAutoAttackRig(t, Stats{HitMod: 0, STR: 10}, Stats{AC: 100}, 10, 50, []int{
		19, // crit
		2,  // 1d3: 3
	})
	rig.critMult = 3
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || hits[0].Damage != 9 { // 3 × 3
		t.Fatalf("crit damage = %v, want one hit of 9 (3×3)", hits)
	}
}

// TestAutoAttackCritMultiplierOneDisablesBonus — multiplier 1 restores
// the original "crit = normal damage" policy; the flag still flows.
func TestAutoAttackCritMultiplierOneDisablesBonus(t *testing.T) {
	rig := newAutoAttackRig(t, Stats{HitMod: 0, STR: 10}, Stats{AC: 100}, 10, 50, []int{
		19, // crit
		2,  // 1d3: 3
	})
	rig.critMult = 1
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d", len(hits))
	}
	if !hits[0].IsCritical {
		t.Error("crit flag must still be set with multiplier 1")
	}
	if hits[0].Damage != 3 { // no doubling
		t.Errorf("damage = %d, want 3 (multiplier 1 = no bonus)", hits[0].Damage)
	}
}

// TestAutoAttackNonCritUnaffectedByMultiplier — an ordinary hit (not a
// nat-20) is never scaled, even with a crit multiplier configured.
func TestAutoAttackNonCritUnaffectedByMultiplier(t *testing.T) {
	rig := newAutoAttackRig(t, Stats{HitMod: 10, STR: 10}, Stats{AC: 10}, 10, 50, []int{
		9, // d20: 10 → hits (10+10 ≥ 10), not a crit
		2, // 1d3: 3
	})
	rig.critMult = 5
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d", len(hits))
	}
	if hits[0].IsCritical {
		t.Fatal("d20=10 is not a crit")
	}
	if hits[0].Damage != 3 { // unscaled
		t.Errorf("non-crit damage = %d, want 3 (multiplier must not apply)", hits[0].Damage)
	}
}

func TestAutoAttackNaturalOneAlwaysMisses(t *testing.T) {
	// Attacker has overwhelming +100 HitMod that would otherwise auto-
	// hit. Roller returns 0 → raw d20 = 1 → fumble override.
	atkStats := Stats{HitMod: 100, STR: 10}
	defStats := Stats{AC: 5}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{0})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

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
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

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
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

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

	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

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

	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

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

	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

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

	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

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
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

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

// M9.5b: the PassiveEvaluator's ExtraAttacks raises the per-round
// swing count (§4.2). extra=1 ⇒ two swings ⇒ two hits on a target
// that survives both.
func TestAutoAttackExtraAttackGrantsSwing(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10} // auto-hit vs low AC; STR 10 → +0 dmg
	defStats := Stats{AC: 5}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, []int{
		5, 0, // swing 1: d20 6 hit, 1d3 → 1 damage
		5, 0, // swing 2: d20 6 hit, 1d3 → 1 damage
	})
	rig.passives = fakePassives{extra: 1}
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if hits := rig.sink.snapshotHits(); len(hits) != 2 {
		t.Fatalf("extra=1 must yield 2 swings/hits, got %d (misses=%d)",
			len(hits), len(rig.sink.snapshotMisses()))
	}
}

// M9.5b: a defensive passive that fires pre-empts the swing (§4.3
// step 2) — an evade event, no hit/miss, and (critically) no hit-roll
// consumed. The nil roller seq would fatal if a roll were attempted.
func TestAutoAttackDefensiveEvadeSkipsSwing(t *testing.T) {
	atkStats := Stats{HitMod: 100, STR: 10}
	defStats := Stats{AC: 5}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 20, nil) // no rolls expected
	rig.passives = fakePassives{evade: true, evadeName: "Parry"}
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	evades := rig.sink.snapshotEvades()
	if len(evades) != 1 {
		t.Fatalf("want 1 evade, got %d", len(evades))
	}
	if evades[0].AbilityName != "Parry" {
		t.Errorf("evade ability name = %q, want Parry", evades[0].AbilityName)
	}
	if n := len(rig.sink.snapshotHits()) + len(rig.sink.snapshotMisses()); n != 0 {
		t.Errorf("evaded swing must produce no hit/miss, got %d", n)
	}
}

// TestAutoAttack_DarknessPenaltyCausesMiss — a swing that would land at
// full accuracy misses once the §5.3 darkness HitModAdjust lowers the
// attacker's hit roll below the target's AC. Same roll, opposite outcome.
func TestAutoAttack_DarknessPenaltyCausesMiss(t *testing.T) {
	// d20 roll 14 → raw 15; AC 15 → 15+0 >= 15 hits at full accuracy.
	rig := newAutoAttackRig(t, Stats{HitMod: 0, STR: 10}, Stats{AC: 15}, 10, 50, []int{14})
	phase := NewAutoAttack(AutoAttackConfig{
		Locator:      rig.locator,
		RoomLocator:  rig.rooms,
		Sink:         rig.sink,
		Roller:       rig.roller,
		HitModAdjust: func(CombatantID) int { return -4 },
	})
	phase(context.Background(), rig.attacker.id, rig.mgr, 0)

	if got := len(rig.sink.snapshotHits()); got != 0 {
		t.Fatalf("darkness penalty should have caused a miss, got %d hits", got)
	}
	if got := len(rig.sink.snapshotMisses()); got != 1 {
		t.Fatalf("want 1 miss under darkness penalty, got %d", got)
	}
}

// TestAutoAttack_NoPenaltyHitsControl — the same roll hits with no
// HitModAdjust, proving the miss above is the penalty's doing.
func TestAutoAttack_NoPenaltyHitsControl(t *testing.T) {
	rig := newAutoAttackRig(t, Stats{HitMod: 0, STR: 10}, Stats{AC: 15}, 10, 50, []int{14, 0})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)
	if got := len(rig.sink.snapshotHits()); got != 1 {
		t.Fatalf("control swing should hit, got %d hits", got)
	}
}

// TestAutoAttack_PenaltyNeverBlocks — a nat-20 still lands even with a
// crushing darkness penalty (§5.3: accuracy degrades, combat is never
// blocked).
func TestAutoAttack_PenaltyNeverBlocks(t *testing.T) {
	rig := newAutoAttackRig(t, Stats{HitMod: 0, STR: 10}, Stats{AC: 50}, 10, 50, []int{19, 0})
	phase := NewAutoAttack(AutoAttackConfig{
		Locator:      rig.locator,
		RoomLocator:  rig.rooms,
		Sink:         rig.sink,
		Roller:       rig.roller,
		HitModAdjust: func(CombatantID) int { return -100 },
	})
	phase(context.Background(), rig.attacker.id, rig.mgr, 0)
	if got := len(rig.sink.snapshotHits()); got != 1 {
		t.Fatalf("nat-20 must land despite the penalty, got %d hits", got)
	}
}
