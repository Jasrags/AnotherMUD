package combat

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/pool"
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
	pools  *pool.Set
}

func (l *liveCombatant) CombatantID() CombatantID { return l.id }
func (l *liveCombatant) Name() string             { return l.name }
func (l *liveCombatant) Vitals() *Vitals          { return l.vitals }
func (l *liveCombatant) Stats() Stats             { return l.stats }
func (l *liveCombatant) Pools() *pool.Set         { return l.pools }

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
	mgr            *Manager
	sink           *recordingSink
	attacker       *liveCombatant
	target         *liveCombatant
	locator        MapLocator
	rooms          MapRoomLocator
	roller         *scriptedRoller
	passives       PassiveEvaluator                         // nil ⇒ pre-M9.5 behavior
	critMult       int                                      // 0 ⇒ NewAutoAttack default (DefaultCritMultiplier)
	massive        *MassiveDamageConfig                     // nil ⇒ saves §4 rule disabled
	incap          func(CombatantID) bool                   // nil ⇒ never incapacitated (conditions §3)
	defAdj         func(CombatantID) int                    // nil ⇒ no defender vulnerability (conditions §3)
	ammoFor        func(CombatantID) (bool, int)            // nil ⇒ no ammo gate (ranged-combat §3)
	takeLoadedShot func(CombatantID) bool                   // TakeLoadedShot (action-economy §7.1) — nil ⇒ always loaded
	falloff        int                                      // RangeFalloff (ranged-combat §5.3)
	pblank         int                                      // PointBlankPenalty (ranged-combat §5.3)
	magBands       int                                      // MagnificationBands (ranged-combat §5.3)
	fireModes      map[string]FireModeEffect                // FireModes (ranged-combat §5.5)
	kite           func(CombatantID, CombatantID, int) bool // KitePolicy (ranged-combat §5.4)
	secOff         int                                      // SecondaryOffHandPenalty (two-weapon-fighting §4.3)
	setBonus       int                                      // SetDamageBonus (special-weapons §4)
	whipThreshold  int                                      // WhipArmorThreshold (subdual-damage §6)
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
		Locator:                 r.locator,
		RoomLocator:             r.rooms,
		Sink:                    r.sink,
		Roller:                  r.roller,
		Passives:                r.passives,
		CritMultiplier:          r.critMult,
		MassiveDamage:           r.massive,
		Incapacitated:           r.incap,
		DefenderHitAdjust:       r.defAdj,
		AmmoFor:                 r.ammoFor,
		TakeLoadedShot:          r.takeLoadedShot,
		RangeFalloff:            r.falloff,
		PointBlankPenalty:       r.pblank,
		MagnificationBands:      r.magBands,
		FireModes:               r.fireModes,
		KitePolicy:              r.kite,
		SecondaryOffHandPenalty: r.secOff,
		SetDamageBonus:          r.setBonus,
		WhipArmorThreshold:      r.whipThreshold,
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
	// min roll (1) and a -10 DamageBonus → raw = 1 + (-10) = -9 → clamp 1.
	// Exercises the negative-bonus path of the min-1 floor.
	atkStats := Stats{HitMod: 10, DamageBonus: -10}
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
	// swing. Use a flat DamageBonus to push damage past the target's HP
	// and observe the vital-depleted event.
	atkStats := Stats{HitMod: 10, DamageBonus: 5} // flat +5 damage
	defStats := Stats{AC: 5}
	rig := newAutoAttackRig(t, atkStats, defStats, 10, 3, []int{
		9, // d20: 10, auto-hit vs AC 5
		2, // damage 1d3: 3 → +5 bonus = 8 damage on 3HP target
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

// --- saves §4 massive-damage Fortitude consumer ---------------------------

// massiveRig builds an attacker that deals a known, large per-hit amount via
// a high STR bonus (STRBonus(100) = 45) plus a deterministic 1d1 weapon
// (always 1), so raw damage = 46. The roller sequence per swing is:
//
//	[hitFace-1, 0 (the 1d1 die), saveFace-1 (only if the save fires)].
//
// TestAutoAttack_MitigationReducesDamage exercises the §6 channel-layer
// damage step: raw = dice + attacker DamageBonus − defender Mitigation.
func TestAutoAttack_MitigationReducesDamage(t *testing.T) {
	atk := Stats{HitMod: 100, DamageBonus: 5, Damage: DiceExpr{1, 1, 0}} // always hit; 1d1=1; +5
	def := Stats{AC: 10, Mitigation: 3}
	rig := newAutoAttackRig(t, atk, def, 10, 50, []int{9, 0}) // hit face 10; 1d1 die → 1
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d", len(hits))
	}
	if hits[0].Damage != 3 { // 1 + 5 − 3
		t.Errorf("damage = %d, want 3 (1 die + 5 bonus − 3 mitigation)", hits[0].Damage)
	}
}

// TestAutoAttack_MitigationFlooredAtOne confirms a landed hit still does the
// per-swing minimum of 1 even when mitigation would drive it negative.
func TestAutoAttack_MitigationFlooredAtOne(t *testing.T) {
	atk := Stats{HitMod: 100, DamageBonus: 0, Damage: DiceExpr{1, 1, 0}}
	def := Stats{AC: 10, Mitigation: 50} // dwarfs the 1 damage
	rig := newAutoAttackRig(t, atk, def, 10, 50, []int{9, 0})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || hits[0].Damage != 1 {
		t.Fatalf("want 1 hit of 1 damage (floored), got %+v", hits)
	}
}

func massiveRig(t *testing.T, defHP int, mc *MassiveDamageConfig, rollSeq []int) *autoAttackRig {
	t.Helper()
	// DamageBonus is now the flat damage add (was inline STRBonus(STR) before
	// the channel layer); STRBonus(100)=45 keeps raw damage at 46 (45 + 1d1).
	atk := Stats{HitMod: 0, STR: 100, DamageBonus: STRBonus(100), Damage: DiceExpr{1, 1, 0}}
	def := Stats{AC: 10}
	rig := newAutoAttackRig(t, atk, def, 20, defHP, rollSeq)
	rig.massive = mc
	return rig
}

func runMassive(t *testing.T, rig *autoAttackRig) {
	t.Helper()
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)
}

func TestMassiveDamage_FailedSaveKills(t *testing.T) {
	mc := &MassiveDamageConfig{Threshold: 40, DC: 15, FortBonus: func(CombatantID) int { return 0 }}
	// hit face 10 (normal hit vs AC 10); 1d1 die = 0; save face 3 → 3 < 15 fail.
	rig := massiveRig(t, 100, mc, []int{9, 0, 2})
	runMassive(t, rig)

	if got := len(rig.sink.snapshotHits()); got != 1 {
		t.Fatalf("want 1 hit, got %d", got)
	}
	saves := rig.sink.snapshotSaves()
	if len(saves) != 1 {
		t.Fatalf("want 1 save event, got %d", len(saves))
	}
	s := saves[0]
	if s.Outcome.Success {
		t.Error("save should have failed")
	}
	if s.SaveType != SaveAxisFortitude || s.Cause != SaveCauseMassiveDamage {
		t.Errorf("save axis/cause wrong: %q / %q", s.SaveType, s.Cause)
	}
	if s.CreatureID != rig.target.id || s.RoomID != roomA {
		t.Errorf("save target/room wrong: %s / %s", s.CreatureID, s.RoomID)
	}
	if got := len(rig.sink.snapshotDeaths()); got != 1 {
		t.Errorf("want 1 death, got %d", got)
	}
	if hp := rig.target.vitals.Current(); hp != 0 {
		t.Errorf("victim HP should be depleted, got %d", hp)
	}
}

func TestMassiveDamage_SuccessfulSaveSurvives(t *testing.T) {
	mc := &MassiveDamageConfig{Threshold: 40, DC: 15, FortBonus: func(CombatantID) int { return 0 }}
	// save face 18 → 18 >= 15 success. Victim keeps the post-damage HP.
	rig := massiveRig(t, 100, mc, []int{9, 0, 17})
	runMassive(t, rig)

	saves := rig.sink.snapshotSaves()
	if len(saves) != 1 || !saves[0].Outcome.Success {
		t.Fatalf("want 1 successful save, got %+v", saves)
	}
	if got := len(rig.sink.snapshotDeaths()); got != 0 {
		t.Errorf("want 0 deaths on a made save, got %d", got)
	}
	if hp := rig.target.vitals.Current(); hp != 54 { // 100 - 46
		t.Errorf("victim HP = %d, want 54 (normal damage still applied)", hp)
	}
}

func TestMassiveDamage_FortBonusRescues(t *testing.T) {
	// Same roll that failed at bonus 0 now passes — proves FortBonus is
	// consumed and added before the DC compare. save face 3 + bonus 20 = 23.
	mc := &MassiveDamageConfig{Threshold: 40, DC: 15, FortBonus: func(id CombatantID) int {
		if id != NewMobCombatantID("tgt") {
			t.Errorf("FortBonus keyed on wrong id: %s", id)
		}
		return 20
	}}
	rig := massiveRig(t, 100, mc, []int{9, 0, 2})
	runMassive(t, rig)

	saves := rig.sink.snapshotSaves()
	if len(saves) != 1 || !saves[0].Outcome.Success {
		t.Fatalf("want a rescued (successful) save, got %+v", saves)
	}
	if got := len(rig.sink.snapshotDeaths()); got != 0 {
		t.Errorf("want 0 deaths after the bonus rescued the save, got %d", got)
	}
}

func TestMassiveDamage_BelowThresholdNoSave(t *testing.T) {
	// Threshold above the 46 raw damage → no save is rolled at all.
	mc := &MassiveDamageConfig{Threshold: 100, DC: 15, FortBonus: func(CombatantID) int { return 0 }}
	rig := massiveRig(t, 100, mc, []int{9, 0}) // no save face programmed
	runMassive(t, rig)

	if got := len(rig.sink.snapshotSaves()); got != 0 {
		t.Errorf("want 0 saves below threshold, got %d", got)
	}
	if got := len(rig.sink.snapshotDeaths()); got != 0 {
		t.Errorf("want 0 deaths, got %d", got)
	}
	if hp := rig.target.vitals.Current(); hp != 54 {
		t.Errorf("victim HP = %d, want 54", hp)
	}
}

func TestMassiveDamage_AlreadyKilledNoSave(t *testing.T) {
	// The swing itself drops the victim to 0 → the kill branch returns
	// before the massive-damage save; no save is forced on a corpse.
	mc := &MassiveDamageConfig{Threshold: 40, DC: 15, FortBonus: func(CombatantID) int { return 0 }}
	rig := massiveRig(t, 10, mc, []int{9, 0}) // 46 dmg vs 10 HP → dead on the hit
	runMassive(t, rig)

	if got := len(rig.sink.snapshotSaves()); got != 0 {
		t.Errorf("want 0 saves when the hit already killed, got %d", got)
	}
	if got := len(rig.sink.snapshotDeaths()); got != 1 {
		t.Errorf("want exactly 1 death (from the killing hit), got %d", got)
	}
}

func TestMassiveDamage_DisabledWhenNil(t *testing.T) {
	// nil MassiveDamage ⇒ the rule is off; a huge hit forces no save.
	rig := massiveRig(t, 100, nil, []int{9, 0})
	runMassive(t, rig)
	if got := len(rig.sink.snapshotSaves()); got != 0 {
		t.Errorf("want 0 saves with the rule disabled, got %d", got)
	}
}

func TestNewAutoAttack_PanicsOnMassiveWithoutFortBonus(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic when MassiveDamage is set without FortBonus")
		}
	}()
	NewAutoAttack(AutoAttackConfig{
		Locator:       MapLocator{},
		RoomLocator:   MapRoomLocator{},
		Sink:          &recordingSink{},
		Roller:        &scriptedRoller{t: t},
		MassiveDamage: &MassiveDamageConfig{Threshold: 50, DC: 15}, // FortBonus nil
	})
}

// --- conditions §3: combat hooks (incapacitation + defender vulnerability) ---

func TestConditions_IncapacitatedAttackerTakesNoSwingsButStaysEngaged(t *testing.T) {
	// A roll that would clearly hit (face 20) is programmed, but the attacker
	// is incapacitated — so it must never reach the roller. No hit, no miss.
	rig := newAutoAttackRig(t, Stats{HitMod: 0, STR: 10}, Stats{AC: 10}, 20, 20, nil)
	rig.incap = func(id CombatantID) bool { return id == rig.attacker.id }
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if h, m := len(rig.sink.snapshotHits()), len(rig.sink.snapshotMisses()); h != 0 || m != 0 {
		t.Fatalf("incapacitated attacker swung: hits=%d misses=%d", h, m)
	}
	// Still engaged — incapacitation skips the swing, it does not disengage.
	if _, ok := rig.mgr.PrimaryTargetOf(rig.attacker.id); !ok {
		t.Error("incapacitated attacker was disengaged; should stay in combat")
	}
	if len(rig.sink.snapshotDeaths()) != 0 {
		t.Errorf("unexpected death events: %d", len(rig.sink.snapshotDeaths()))
	}
}

func TestConditions_DefenderVulnerabilityTurnsAMissIntoAHit(t *testing.T) {
	// AC 15, attacker hitMod 0, raw roll 14 → 14 < 15 would MISS. A +4
	// defender-vulnerability delta (prone) lifts it to 18 ≥ 15 → HIT. Proves
	// the delta is keyed on the TARGET and summed into the roll.
	atk := Stats{HitMod: 0, STR: 10}
	def := Stats{AC: 15}

	// Control: without the delta, the same roll misses.
	ctrl := newAutoAttackRig(t, atk, def, 20, 20, []int{13}) // face 14, no damage roll consumed on a miss
	ctrl.phase()(context.Background(), ctrl.attacker.id, ctrl.mgr, 0)
	if got := len(ctrl.sink.snapshotHits()); got != 0 {
		t.Fatalf("control: expected a miss without vulnerability, got %d hits", got)
	}
	if got := len(ctrl.sink.snapshotMisses()); got != 1 {
		t.Fatalf("control: expected 1 miss, got %d", got)
	}

	// With the +4 vulnerability delta keyed on the target, the same roll hits.
	rig := newAutoAttackRig(t, atk, def, 20, 20, []int{13, 0}) // face 14, then 1d3 damage
	rig.defAdj = func(id CombatantID) int {
		if id != rig.target.id {
			t.Errorf("DefenderHitAdjust keyed on %s, want target %s", id, rig.target.id)
		}
		return 4
	}
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)
	if got := len(rig.sink.snapshotHits()); got != 1 {
		t.Fatalf("expected the vulnerability delta to land the hit, got %d hits", got)
	}
}

func TestConditions_DefenderVulnerabilityComposesWithAttackerHitMod(t *testing.T) {
	// Attacker HitModAdjust −2 (e.g. blinded) and defender vulnerability +3
	// sum with the base roll: face 14, 14 + (−2) + 3 = 15 ≥ AC 15 → hit.
	atk := Stats{HitMod: 0, STR: 10}
	def := Stats{AC: 15}
	rig := newAutoAttackRig(t, atk, def, 20, 20, []int{13, 0})
	p := NewAutoAttack(AutoAttackConfig{
		Locator: rig.locator, RoomLocator: rig.rooms, Sink: rig.sink, Roller: rig.roller,
		HitModAdjust:      func(CombatantID) int { return -2 },
		DefenderHitAdjust: func(CombatantID) int { return 3 },
	})
	p(context.Background(), rig.attacker.id, rig.mgr, 0)
	if got := len(rig.sink.snapshotHits()); got != 1 {
		t.Fatalf("expected the summed adjustments to land the hit, got %d hits (misses=%d)",
			got, len(rig.sink.snapshotMisses()))
	}
}

// TestSubdualFinishingBlowMarksVitalDepleted locks subdual-damage §2/§4: a
// subdual weapon's FINISHING blow carries Subdual=true on both the Hit and the
// VitalDepleted events, so the death pipeline can knock out instead of kill.
func TestSubdualFinishingBlowMarksVitalDepleted(t *testing.T) {
	atkStats := Stats{HitMod: 10, DamageBonus: 5, Subdual: true}
	rig := newAutoAttackRig(t, atkStats, Stats{AC: 5}, 10, 3, []int{
		9, // d20: 10, auto-hit
		2, // damage 1d3: 3 → +5 = 8 on a 3HP target → finishing blow
	})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || !hits[0].Subdual {
		t.Fatalf("subdual Hit not marked: hits=%+v", hits)
	}
	deaths := rig.sink.snapshotDeaths()
	if len(deaths) != 1 || !deaths[0].Subdual {
		t.Fatalf("subdual VitalDepleted not marked: deaths=%+v", deaths)
	}
}

// TestLethalFinishingBlowIsNotSubdual is the control: an ordinary (non-subdual)
// weapon's finish carries Subdual=false, so the death pipeline kills as before.
func TestLethalFinishingBlowIsNotSubdual(t *testing.T) {
	atkStats := Stats{HitMod: 10, DamageBonus: 5} // Subdual defaults false
	rig := newAutoAttackRig(t, atkStats, Stats{AC: 5}, 10, 3, []int{9, 2})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	deaths := rig.sink.snapshotDeaths()
	if len(deaths) != 1 || deaths[0].Subdual {
		t.Fatalf("lethal finish must not be subdual: deaths=%+v", deaths)
	}
	if hits := rig.sink.snapshotHits(); len(hits) != 1 || hits[0].Subdual {
		t.Fatalf("lethal Hit must not be subdual: hits=%+v", hits)
	}
}

// TestSubdualNonFinishingBlowAboveZero: a subdual blow that leaves
// the target above zero marks the Hit subdual (renderers) but emits no
// VitalDepleted — subdual is inert above zero HP (subdual-damage §2).
func TestSubdualNonFinishingBlowAboveZero(t *testing.T) {
	atkStats := Stats{HitMod: 10, Subdual: true} // no bonus: a 1-damage scratch
	rig := newAutoAttackRig(t, atkStats, Stats{AC: 5}, 10, 20, []int{9, 0})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if hits := rig.sink.snapshotHits(); len(hits) != 1 || !hits[0].Subdual {
		t.Fatalf("subdual non-finishing Hit not marked: hits=%+v", hits)
	}
	if deaths := rig.sink.snapshotDeaths(); len(deaths) != 0 {
		t.Fatalf("a non-finishing blow must not deplete a vital: deaths=%+v", deaths)
	}
}

// TestOffHandSubdualIndependentOfMainHand locks subdual-damage §2: the off-hand
// swing's lethality is its OWN — a subdual off-hand finish is marked subdual even
// when the main hand is lethal. The main swing here misses (low roll) so the
// off-hand lands the finishing blow.
func TestOffHandSubdualIndependentOfMainHand(t *testing.T) {
	atkStats := Stats{
		HitMod: 10, Subdual: false, // main hand: lethal, but it will miss
		OffHand: &OffHandProfile{HitMod: 10, DamageBonus: 5, Subdual: true, Attacks: 1},
	}
	rig := newAutoAttackRig(t, atkStats, Stats{AC: 5}, 10, 3, []int{
		0, // main d20: 1 → fumble/miss vs AC 5 (no damage roll consumed on a miss)
		9, // off-hand d20: 10, auto-hit
		2, // off-hand damage 1d3: 3 → +5 = 8 on 3HP → subdual finish
	})
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	deaths := rig.sink.snapshotDeaths()
	if len(deaths) != 1 || !deaths[0].Subdual {
		t.Fatalf("off-hand subdual finish should mark VitalDepleted subdual: deaths=%+v", deaths)
	}
}

// TestMassiveDamage_SubdualFailedSaveKnocksOut locks the review fix: a SUBDUAL
// blow that drops the victim through the massive-damage Fortitude save still
// carries Subdual=true on the resulting VitalDepleted, so the death pipeline
// knocks out rather than kills on this path too (subdual-damage §4).
func TestMassiveDamage_SubdualFailedSaveKnocksOut(t *testing.T) {
	mc := &MassiveDamageConfig{Threshold: 40, DC: 15, FortBonus: func(CombatantID) int { return 0 }}
	rig := massiveRig(t, 100, mc, []int{9, 0, 2}) // hit; raw 46 ≥ 40; save 3 < 15 → fail
	rig.attacker.stats.Subdual = true
	runMassive(t, rig)

	deaths := rig.sink.snapshotDeaths()
	if len(deaths) != 1 || !deaths[0].Subdual {
		t.Fatalf("a subdual massive-damage death must carry Subdual=true: deaths=%+v", deaths)
	}
}

// TestWhipIneffectiveVsArmor locks subdual-damage §6: a whip that LANDS against
// a defender whose ArmorRating meets the threshold deals NO damage (an
// ineffective Hit), leaving the target's HP untouched.
func TestWhipIneffectiveVsArmor(t *testing.T) {
	atk := Stats{HitMod: 10, DamageBonus: 5, IneffectiveVsArmor: true}
	def := Stats{AC: 5, ArmorRating: 2}
	rig := newAutoAttackRig(t, atk, def, 10, 20, []int{9}) // hit; no damage roll consumed (ineffective)
	rig.whipThreshold = 1
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || !hits[0].Ineffective || hits[0].Damage != 0 {
		t.Fatalf("want one ineffective 0-damage hit, got %+v", hits)
	}
	if got := rig.target.Vitals().Current(); got != 20 {
		t.Errorf("armored target HP should be untouched by the whip, got %d/20", got)
	}
}

// A whip vs an UNARMORED foe (ArmorRating below threshold) bites normally.
func TestWhipBitesUnarmored(t *testing.T) {
	atk := Stats{HitMod: 10, DamageBonus: 5, IneffectiveVsArmor: true}
	def := Stats{AC: 5, ArmorRating: 0}
	rig := newAutoAttackRig(t, atk, def, 10, 20, []int{9, 2}) // hit + damage roll
	rig.whipThreshold = 1
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 1 || hits[0].Ineffective || hits[0].Damage == 0 {
		t.Fatalf("a whip vs an unarmored foe should deal normal damage, got %+v", hits)
	}
}

// A NON-whip weapon vs an armored foe is unaffected by the gate (bites normally).
func TestNonWhipUnaffectedByArmorGate(t *testing.T) {
	atk := Stats{HitMod: 10, DamageBonus: 5} // IneffectiveVsArmor false
	def := Stats{AC: 5, ArmorRating: 99}
	rig := newAutoAttackRig(t, atk, def, 10, 20, []int{9, 2})
	rig.whipThreshold = 1
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if hits := rig.sink.snapshotHits(); len(hits) != 1 || hits[0].Ineffective {
		t.Fatalf("a non-whip weapon must ignore the armor gate, got %+v", hits)
	}
}

// Threshold 0 (disabled / unconfigured) makes even a whip bite an armored foe.
func TestWhipGateDisabledWhenThresholdZero(t *testing.T) {
	atk := Stats{HitMod: 10, DamageBonus: 5, IneffectiveVsArmor: true}
	def := Stats{AC: 5, ArmorRating: 99}
	rig := newAutoAttackRig(t, atk, def, 10, 20, []int{9, 2})
	rig.whipThreshold = 0 // rule disabled
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if hits := rig.sink.snapshotHits(); len(hits) != 1 || hits[0].Ineffective {
		t.Fatalf("threshold 0 disables the gate — the whip should bite, got %+v", hits)
	}
}

// TestOffHandWhipIndependentOfMainHand locks the review fix (subdual-damage §6):
// the off-hand swing's anti-armor behavior is its OWN. A whip MAIN hand + a
// steel (non-whip) off-hand vs an armored foe: the main swing is ineffective
// (0 damage), but the off-hand swing bites normally — it must NOT inherit the
// main hand's IneffectiveVsArmor.
func TestOffHandWhipIndependentOfMainHand(t *testing.T) {
	atk := Stats{
		HitMod: 10, DamageBonus: 5, IneffectiveVsArmor: true, // main hand: a whip
		OffHand: &OffHandProfile{HitMod: 10, DamageBonus: 5, IneffectiveVsArmor: false, Attacks: 1}, // off hand: steel
	}
	def := Stats{AC: 5, ArmorRating: 2}
	rig := newAutoAttackRig(t, atk, def, 10, 50, []int{
		9, // main d20: hit → ineffective (no damage roll consumed)
		9, // off-hand d20: hit
		2, // off-hand damage 1d3: 3 → +5 = 8 (a real bite)
	})
	rig.whipThreshold = 1
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	hits := rig.sink.snapshotHits()
	if len(hits) != 2 {
		t.Fatalf("want 2 hits (main ineffective + off-hand bite), got %d: %+v", len(hits), hits)
	}
	if !hits[0].Ineffective || hits[0].Damage != 0 {
		t.Errorf("main whip swing should be ineffective 0-damage, got %+v", hits[0])
	}
	if hits[1].Ineffective || hits[1].Damage == 0 {
		t.Errorf("off-hand steel swing should bite (not inherit the main whip's anti-armor), got %+v", hits[1])
	}
}

// --- shadowrun-mvp SR-M2: typed damage target_pool routing ---
//
// An attack's Stats.TargetPool names which of the defender's pools it fills.
// Empty routes to the canonical Vitals/hp path (every test above); a named
// kind routes through the defender's pool.Set (Pools()), with a per-pool
// KO-vs-death meaning (Rules.Nonlethal) on each crossing. hp always lives in
// Vitals, never in the Set, so a stun swing never touches hp.

// stunPools builds a Set holding one depletion-signalling monitor of the given
// kind/max — the Shadowrun condition monitor a typed attack fills. nonlethal
// marks a Stun monitor (crossing = knock-out); false marks a Physical-style
// monitor (crossing = kill).
func stunPools(kind pool.Kind, max int, nonlethal bool) *pool.Set {
	s := pool.NewSet()
	s.Add(pool.New(kind, max, pool.Rules{Floor: 0, DepletionEvent: true, Nonlethal: nonlethal}))
	return s
}

// TestTargetPool_StunFillsMonitorLeavingHPUntouched: a stun swing below the
// monitor's max fills the Stun pool, deals no depletion, and never touches hp.
func TestTargetPool_StunFillsMonitorLeavingHPUntouched(t *testing.T) {
	atk := Stats{HitMod: 10, DamageBonus: 3, TargetPool: "stun", Subdual: true}
	rig := newAutoAttackRig(t, atk, Stats{AC: 5}, 10, 50, []int{9, 0}) // 1d3=1 +3 = 4 stun
	rig.target.pools = stunPools("stun", 10, true)                     // 4 < 10 → no crossing
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if got := rig.target.vitals.Current(); got != 50 {
		t.Fatalf("hp must be untouched by a stun hit, got %d want 50", got)
	}
	if p, _ := rig.target.pools.Get("stun"); p.Current() != 6 {
		t.Fatalf("stun monitor current = %d, want 6 (10-4)", p.Current())
	}
	if d := len(rig.sink.snapshotDeaths()); d != 0 {
		t.Fatalf("no VitalDepleted expected below the monitor max, got %d", d)
	}
	if h := len(rig.sink.snapshotHits()); h != 1 {
		t.Fatalf("want 1 hit, got %d", h)
	}
}

// TestTargetPool_StunDepletionKnocksOut: filling the Stun monitor to its floor
// emits one VitalDepleted marked Subdual (knock-out) whose Vital is the stun
// pool — the death pipeline knocks out instead of killing, and hp is untouched.
func TestTargetPool_StunDepletionKnocksOut(t *testing.T) {
	atk := Stats{HitMod: 10, DamageBonus: 9, TargetPool: "stun", Subdual: true}
	rig := newAutoAttackRig(t, atk, Stats{AC: 5}, 10, 50, []int{9, 2}) // 1d3=3 +9 = 12 stun
	rig.target.pools = stunPools("stun", 10, true)                     // 12 >= 10 → crosses
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	deaths := rig.sink.snapshotDeaths()
	if len(deaths) != 1 {
		t.Fatalf("want exactly 1 depletion, got %d: %+v", len(deaths), deaths)
	}
	if deaths[0].Vital != "stun" {
		t.Errorf("depletion Vital = %q, want \"stun\"", deaths[0].Vital)
	}
	if !deaths[0].Subdual {
		t.Error("a stun-monitor depletion must be Subdual (knock-out), not a kill")
	}
	if got := rig.target.vitals.Current(); got != 50 {
		t.Errorf("hp must be untouched by a stun KO, got %d want 50", got)
	}
}

// TestTargetPool_NonlethalMonitorKOsEvenWithLethalWeapon: the KO-vs-death
// meaning is the POOL's (Rules.Nonlethal), independent of the weapon — a
// lethal (non-subdual) attack routed into a Stun monitor still knocks out.
func TestTargetPool_NonlethalMonitorKOsEvenWithLethalWeapon(t *testing.T) {
	atk := Stats{HitMod: 10, DamageBonus: 9, TargetPool: "stun"} // Subdual defaults false
	rig := newAutoAttackRig(t, atk, Stats{AC: 5}, 10, 50, []int{9, 2})
	rig.target.pools = stunPools("stun", 10, true)
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	deaths := rig.sink.snapshotDeaths()
	if len(deaths) != 1 || !deaths[0].Subdual {
		t.Fatalf("a nonlethal monitor must KO regardless of weapon: %+v", deaths)
	}
}

// TestTargetPool_LethalNamedPoolKills: a named monitor whose Rules.Nonlethal is
// false (a Physical track) crossing to its floor is a KILL (Subdual false) even
// via the pool-routing path — the control for the stun case.
func TestTargetPool_LethalNamedPoolKills(t *testing.T) {
	atk := Stats{HitMod: 10, DamageBonus: 9, TargetPool: "physical"}
	rig := newAutoAttackRig(t, atk, Stats{AC: 5}, 10, 50, []int{9, 2})
	rig.target.pools = stunPools("physical", 10, false) // lethal monitor
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	deaths := rig.sink.snapshotDeaths()
	if len(deaths) != 1 || deaths[0].Subdual {
		t.Fatalf("a lethal named-pool crossing must kill (Subdual false): %+v", deaths)
	}
	if deaths[0].Vital != "physical" {
		t.Errorf("Vital = %q, want \"physical\"", deaths[0].Vital)
	}
}

// TestTargetPool_NilPoolsLandsHarmlessly: a stun attack against a target with
// no pool.Set (Pools() nil — a fantasy/WoT mob) still LANDS (a Hit) but moves
// no vital; the destination monitor does not exist.
func TestTargetPool_NilPoolsLandsHarmlessly(t *testing.T) {
	atk := Stats{HitMod: 10, DamageBonus: 3, TargetPool: "stun", Subdual: true}
	rig := newAutoAttackRig(t, atk, Stats{AC: 5}, 10, 50, []int{9, 0})
	// rig.target.pools stays nil (the mob default)
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	if h := len(rig.sink.snapshotHits()); h != 1 {
		t.Fatalf("want the swing to land, got %d hits", h)
	}
	if d := len(rig.sink.snapshotDeaths()); d != 0 {
		t.Fatalf("nil Pools ⇒ no depletion, got %d", d)
	}
	if got := rig.target.vitals.Current(); got != 50 {
		t.Errorf("hp must be untouched, got %d", got)
	}
}

// TestTargetPool_EmptyRoutesToHPAndKills: the default (empty TargetPool) is the
// canonical hp path — a finishing blow depletes hp with Vital="hp", byte-
// identical to pre-slice behavior. Locks the equivalence explicitly.
func TestTargetPool_EmptyRoutesToHPAndKills(t *testing.T) {
	atk := Stats{HitMod: 10, DamageBonus: 5} // TargetPool empty
	rig := newAutoAttackRig(t, atk, Stats{AC: 5}, 10, 3, []int{9, 2})
	rig.target.pools = stunPools("stun", 10, true) // present but never touched
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	deaths := rig.sink.snapshotDeaths()
	if len(deaths) != 1 || deaths[0].Vital != VitalHP {
		t.Fatalf("empty TargetPool must deplete hp, got %+v", deaths)
	}
	if p, _ := rig.target.pools.Get("stun"); p.Current() != 10 {
		t.Errorf("stun monitor must be untouched by an hp swing, got %d want 10", p.Current())
	}
}

// stunPoolsOverflow is stunPools with an OverflowTo target — the Shadowrun Stun
// monitor spilling its excess into hp (SR-M3c stun→Physical overflow).
func stunPoolsOverflow(kind pool.Kind, max int, overflowTo pool.Kind) *pool.Set {
	s := pool.NewSet()
	s.Add(pool.New(kind, max, pool.Rules{Floor: 0, DepletionEvent: true, Nonlethal: true, OverflowTo: overflowTo}))
	return s
}

// TestTargetPool_StunOverflowSpillsToHP: a stun swing exceeding the monitor's
// max knocks the target out (stun crossing) AND the excess lands on hp as
// Physical damage — but a NON-lethal overflow leaves the target alive, so the
// stun knock-out still stands (SR5: stun overflows into Physical).
func TestTargetPool_StunOverflowSpillsToHP(t *testing.T) {
	atk := Stats{HitMod: 10, DamageBonus: 9, TargetPool: "stun", Subdual: true}
	rig := newAutoAttackRig(t, atk, Stats{AC: 5}, 10, 50, []int{9, 2}) // 1d3=2 +9 = 11 stun
	rig.target.pools = stunPoolsOverflow("stun", 10, "hp")             // 11 > 10 → 1 overflow to hp
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	deaths := rig.sink.snapshotDeaths()
	if len(deaths) != 1 || deaths[0].Vital != "stun" || !deaths[0].Subdual {
		t.Fatalf("want one nonlethal stun KO (overflow didn't kill): %+v", deaths)
	}
	if got := rig.target.vitals.Current(); got >= 50 {
		t.Errorf("hp = %d, want < 50 — the stun overflow should have spilled onto Vitals", got)
	}
	if rig.target.vitals.IsDead() {
		t.Error("a non-lethal overflow must leave the target alive (knocked out, not killed)")
	}
}

// TestTargetPool_StunOverflowLethalSupersedes: when the stun overflow drives hp
// to zero, the death supersedes the knock-out — exactly one VitalDepleted, on
// hp (Physical), NOT subdual, and no separate stun-KO event.
func TestTargetPool_StunOverflowLethalSupersedes(t *testing.T) {
	atk := Stats{HitMod: 10, DamageBonus: 9, TargetPool: "stun", Subdual: true}
	rig := newAutoAttackRig(t, atk, Stats{AC: 5}, 10, 3, []int{9, 2}) // ~12 stun
	rig.target.pools = stunPoolsOverflow("stun", 5, "hp")             // 5 crosses, 7 overflow → hp 3-7 dead
	rig.phase()(context.Background(), rig.attacker.id, rig.mgr, 0)

	deaths := rig.sink.snapshotDeaths()
	if len(deaths) != 1 {
		t.Fatalf("a lethal overflow must emit exactly one depletion (death supersedes KO), got %d: %+v", len(deaths), deaths)
	}
	if deaths[0].Vital != VitalHP || deaths[0].Subdual {
		t.Errorf("depletion = {Vital:%q Subdual:%v}, want {hp, false} (lethal Physical overflow)", deaths[0].Vital, deaths[0].Subdual)
	}
	if !rig.target.vitals.IsDead() {
		t.Error("the target should be dead after a lethal stun overflow into Physical")
	}
}
