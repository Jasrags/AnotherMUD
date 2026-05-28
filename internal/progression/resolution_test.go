package progression

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// --- test fakes -----------------------------------------------------

// fakeSource implements ResolutionSource. Only the fields the
// resolver reads are wired; the rest satisfy ValidationEntity with
// inert defaults (validation already passed before Resolve runs).
type fakeSource struct {
	id         string
	movement   int
	mana       int
	race       *Race
	statVals   map[StatType]int
	lastUsed   string
	deductedMv int
	deductedMn int
}

func (s *fakeSource) EntityID() string                     { return s.id }
func (s *fakeSource) IsResting() bool                      { return false }
func (s *fakeSource) Alignment() int                       { return 0 }
func (s *fakeSource) EquippedTags(string) ([]string, bool) { return nil, false }
func (s *fakeSource) InCombat() bool                       { return true }
func (s *fakeSource) CurrentTarget() (string, bool)        { return "", false }
func (s *fakeSource) Movement() int                        { return s.movement }
func (s *fakeSource) Mana() int                            { return s.mana }
func (s *fakeSource) Race() *Race                          { return s.race }
func (s *fakeSource) DeductMovement(n int)                 { s.movement -= n; s.deductedMv += n }
func (s *fakeSource) DeductMana(n int)                     { s.mana -= n; s.deductedMn += n }
func (s *fakeSource) SetLastAbility(id string)             { s.lastUsed = id }
func (s *fakeSource) StatValue(st StatType) int            { return s.statVals[st] }

// recordingSink captures every AbilitySink emission.
type recordingSink struct {
	used     []AbilityUsedEvent
	missed   []AbilityMissedEvent
	fizzled  []AbilityFizzledEvent
	depleted []VitalDepletedEvent
}

func (s *recordingSink) OnAbilityUsed(_ context.Context, e AbilityUsedEvent) {
	s.used = append(s.used, e)
}
func (s *recordingSink) OnAbilityMissed(_ context.Context, e AbilityMissedEvent) {
	s.missed = append(s.missed, e)
}
func (s *recordingSink) OnAbilityFizzled(_ context.Context, e AbilityFizzledEvent) {
	s.fizzled = append(s.fizzled, e)
}
func (s *recordingSink) OnVitalDepleted(_ context.Context, e VitalDepletedEvent) {
	s.depleted = append(s.depleted, e)
}

// seqRoller returns a programmed IntN sequence; fatal on exhaustion.
type seqRoller struct {
	t   *testing.T
	seq []int
	idx int
}

func (r *seqRoller) IntN(n int) int {
	if r.idx >= len(r.seq) {
		r.t.Fatalf("seqRoller exhausted after %d rolls", r.idx)
	}
	v := r.seq[r.idx]
	r.idx++
	if v < 0 || v >= n {
		r.t.Fatalf("seqRoller value %d out of range [0,%d)", v, n)
	}
	return v
}

// profStub satisfies ProficiencyReader + the richer Proficiency
// accessor + ProficiencyMutator so one fake serves all three resolver
// seams.
type profStub struct {
	vals  map[string]int // abilityID -> proficiency
	gains map[string]int // abilityID -> AddProficiency call count
}

func newProfStub() *profStub {
	return &profStub{vals: map[string]int{}, gains: map[string]int{}}
}
func (p *profStub) Has(_, abilityID string) bool { _, ok := p.vals[abilityID]; return ok }
func (p *profStub) Proficiency(_, abilityID string) (int, bool) {
	v, ok := p.vals[abilityID]
	return v, ok
}
func (p *profStub) AddProficiency(_, abilityID string, delta int) { p.gains[abilityID] += delta }

// effectSpy records Apply calls and returns a programmed result.
type effectSpy struct {
	calls  []EffectTemplate
	result bool
}

func (e *effectSpy) Apply(_ context.Context, _ string, tpl EffectTemplate, _, _ string) bool {
	e.calls = append(e.calls, tpl)
	return e.result
}

// --- tests ----------------------------------------------------------

func TestResolve_VarianceZeroAlwaysHits(t *testing.T) {
	src := &fakeSource{id: "p1", movement: 100}
	sink := &recordingSink{}
	prof := newProfStub()
	// No roller injected (nil → alwaysHitRoller). Variance 0 means the
	// hit path is taken without consuming a roll anyway.
	r := NewAbilityResolver(DefaultResolutionConfig(), prof, nil, nil, nil, nil, sink, nil)
	ab := &Ability{ID: "kick", DisplayName: "Kick", Category: AbilitySkill, Variance: 0}

	out := r.Resolve(context.Background(), src, ab, "m1", 0)

	if !out.Hit {
		t.Fatalf("variance-0 ability must always hit")
	}
	if len(sink.used) != 1 || len(sink.missed) != 0 {
		t.Fatalf("want 1 used / 0 missed, got %d/%d", len(sink.used), len(sink.missed))
	}
	if sink.used[0].Category != AbilitySkill || sink.used[0].AbilityID != "kick" {
		t.Fatalf("unexpected used event: %+v", sink.used[0])
	}
}

func TestResolve_HandlerTokenPropagatesOnUsed(t *testing.T) {
	src := &fakeSource{id: "p1"}
	sink := &recordingSink{}
	r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, nil, nil, nil, sink, nil)
	ab := &Ability{ID: "kick", DisplayName: "Kick", Category: AbilitySkill, Variance: 0, HandlerToken: "damage"}

	r.Resolve(context.Background(), src, ab, "m1", 0)

	if len(sink.used) != 1 {
		t.Fatalf("want 1 used event, got %d", len(sink.used))
	}
	if sink.used[0].HandlerToken != "damage" {
		t.Errorf("HandlerToken = %q, want %q", sink.used[0].HandlerToken, "damage")
	}
}

func TestResolve_DeductsRaceAdjustedCost(t *testing.T) {
	src := &fakeSource{id: "p1", movement: 50, race: &Race{CastCostModifier: 3}}
	r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, nil, nil, nil, nil, nil)
	ab := &Ability{ID: "kick", Category: AbilitySkill, Cost: 10} // adjusted → 13

	out := r.Resolve(context.Background(), src, ab, "m1", 0)

	if out.ResourceSpent != 13 {
		t.Fatalf("want 13 spent, got %d", out.ResourceSpent)
	}
	if src.deductedMv != 13 || src.deductedMn != 0 {
		t.Fatalf("want 13 movement deducted, got mv=%d mn=%d", src.deductedMv, src.deductedMn)
	}
	if src.movement != 37 {
		t.Fatalf("want movement 37 after deduct, got %d", src.movement)
	}
}

func TestResolve_SpellDeductsMana(t *testing.T) {
	src := &fakeSource{id: "p1", mana: 40}
	r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, nil, nil, nil, nil, nil)
	ab := &Ability{ID: "bolt", Category: AbilitySpell, Cost: 12}

	r.Resolve(context.Background(), src, ab, "m1", 0)

	if src.deductedMn != 12 || src.deductedMv != 0 {
		t.Fatalf("spell must draw mana: mv=%d mn=%d", src.deductedMv, src.deductedMn)
	}
}

func TestResolve_RecordsLastUsedLowercased(t *testing.T) {
	src := &fakeSource{id: "p1"}
	r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, nil, nil, nil, nil, nil)
	ab := &Ability{ID: "Kick", Category: AbilitySkill} // mixed-case ID

	r.Resolve(context.Background(), src, ab, "m1", 0)

	if src.lastUsed != "kick" {
		t.Fatalf("want last-used normalized to 'kick', got %q", src.lastUsed)
	}
}

func TestResolve_PulseDelayRecordedOnHitOnly(t *testing.T) {
	tracker := NewPulseDelayTracker()
	prof := newProfStub()
	// Force a miss: variance 50, prof 0 → chance floored to 1, roll
	// returns 99 → 100 > 1 → miss. The pulse delay MUST NOT record.
	roller := &seqRoller{t: t, seq: []int{99}}
	r := NewAbilityResolver(DefaultResolutionConfig(), prof, nil, tracker, nil, nil, nil, roller)
	ab := &Ability{ID: "kick", Category: AbilitySkill, Variance: 50, PulseDelay: 3}
	src := &fakeSource{id: "p1"}

	out := r.Resolve(context.Background(), src, ab, "m1", 10)
	if out.Hit {
		t.Fatalf("expected miss")
	}
	// Spec §4.5 step 3: pulse delay recorded on success, NOT on miss.
	if _, ok := tracker.ReadyAt("p1", "kick"); ok {
		t.Fatalf("pulse delay must not record on miss")
	}
}

func TestResolve_PulseDelayRecordedOnHit(t *testing.T) {
	tracker := NewPulseDelayTracker()
	r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, tracker, nil, nil, nil, nil)
	ab := &Ability{ID: "kick", Category: AbilitySkill, Variance: 0, PulseDelay: 3}
	src := &fakeSource{id: "p1"}

	r.Resolve(context.Background(), src, ab, "m1", 10)

	// next-ready = currentPulse + delay + 1 = 10 + 3 + 1 = 14.
	readyAt, ok := tracker.ReadyAt("p1", "kick")
	if !ok || readyAt != 14 {
		t.Fatalf("want readyAt 14, got %d ok=%v", readyAt, ok)
	}
}

func TestResolve_MissEmitsMissedAndNoEffect(t *testing.T) {
	sink := &recordingSink{}
	eff := &effectSpy{result: true}
	prof := newProfStub()
	roller := &seqRoller{t: t, seq: []int{99}} // hit roll → miss
	r := NewAbilityResolver(DefaultResolutionConfig(), prof, nil, nil, eff, nil, sink, roller)
	ab := &Ability{
		ID: "curse", Category: AbilitySkill, Variance: 30,
		Effect: &EffectTemplate{ID: "weakened", Duration: 5},
	}
	src := &fakeSource{id: "p1"}

	out := r.Resolve(context.Background(), src, ab, "m1", 0)

	if out.Hit {
		t.Fatalf("expected miss")
	}
	if len(sink.missed) != 1 || len(sink.used) != 0 {
		t.Fatalf("want 1 missed / 0 used, got %d/%d", len(sink.missed), len(sink.used))
	}
	if len(eff.calls) != 0 {
		t.Fatalf("miss must not apply an effect, got %d Apply calls", len(eff.calls))
	}
}

func TestResolve_HitWithEffectApplies(t *testing.T) {
	sink := &recordingSink{}
	eff := &effectSpy{result: true}
	r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, nil, eff, nil, sink, nil)
	ab := &Ability{
		ID: "bless", Category: AbilitySpell, Variance: 0,
		Effect: &EffectTemplate{ID: "blessed", Duration: 10, Modifiers: []stats.Modifier{{Stat: "str", Value: 2}}},
	}
	src := &fakeSource{id: "p1"}

	out := r.Resolve(context.Background(), src, ab, "", 0) // self-cast (empty target)

	if !out.Hit || !out.EffectApplied {
		t.Fatalf("want hit+effectApplied, got hit=%v applied=%v", out.Hit, out.EffectApplied)
	}
	if len(eff.calls) != 1 || eff.calls[0].ID != "blessed" {
		t.Fatalf("expected one Apply of 'blessed', got %+v", eff.calls)
	}
	if len(sink.used) != 1 {
		t.Fatalf("want 1 used event, got %d", len(sink.used))
	}
}

func TestResolve_VitalDepletedEmittedWhenTargetDead(t *testing.T) {
	sink := &recordingSink{}
	hp := TargetHPLookupFunc(func(id string) (int, bool) {
		if id == "m1" {
			return 0, true // target at zero HP after resolution
		}
		return 100, true
	})
	r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, nil, nil, hp, sink, nil)
	ab := &Ability{ID: "smite", Category: AbilitySkill, Variance: 0}
	src := &fakeSource{id: "p1"}

	out := r.Resolve(context.Background(), src, ab, "m1", 0)

	if !out.TargetDepleted {
		t.Fatalf("want TargetDepleted true")
	}
	if len(sink.depleted) != 1 {
		t.Fatalf("want 1 vital-depleted event, got %d", len(sink.depleted))
	}
	got := sink.depleted[0]
	if got.VictimID != "m1" || got.KillerID != "p1" || got.Vital != VitalHP {
		t.Fatalf("unexpected vital-depleted event: %+v", got)
	}
}

func TestResolve_NoVitalDepletedWhenTargetAlive(t *testing.T) {
	sink := &recordingSink{}
	hp := TargetHPLookupFunc(func(string) (int, bool) { return 50, true })
	r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, nil, nil, hp, sink, nil)
	ab := &Ability{ID: "smite", Category: AbilitySkill, Variance: 0}
	src := &fakeSource{id: "p1"}

	out := r.Resolve(context.Background(), src, ab, "m1", 0)

	if out.TargetDepleted || len(sink.depleted) != 0 {
		t.Fatalf("alive target must not emit vital-depleted")
	}
}

func TestResolve_SelfCastNeverDeathChecks(t *testing.T) {
	sink := &recordingSink{}
	called := false
	hp := TargetHPLookupFunc(func(string) (int, bool) { called = true; return 0, true })
	r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, nil, nil, hp, sink, nil)
	ab := &Ability{ID: "bless", Category: AbilitySpell, Variance: 0}
	src := &fakeSource{id: "p1"}

	r.Resolve(context.Background(), src, ab, "", 0) // empty resolved target

	if called {
		t.Fatalf("self-cast (empty target) must not probe target HP")
	}
	if len(sink.depleted) != 0 {
		t.Fatalf("self-cast must not emit vital-depleted")
	}
}

func TestResolve_ProficiencyGainRolledOnHitAndMiss(t *testing.T) {
	t.Run("hit gains", func(t *testing.T) {
		prof := newProfStub()
		prof.vals["kick"] = 1
		// Variance 0 → auto-hit (no hit roll). Gain roll: base 100,
		// taper (1-1/100)=0.99 → chance ~99; roll 0→1 ≤ 99 → gain.
		roller := &seqRoller{t: t, seq: []int{0}} // single gain roll
		r := NewAbilityResolver(DefaultResolutionConfig(), prof, prof, nil, nil, nil, nil, roller)
		ab := &Ability{ID: "kick", Category: AbilitySkill, Variance: 0, GainBaseChance: 100}
		src := &fakeSource{id: "p1"}

		r.Resolve(context.Background(), src, ab, "m1", 0)

		if prof.gains["kick"] != 1 {
			t.Fatalf("want 1 gain on hit, got %d", prof.gains["kick"])
		}
	})

	t.Run("miss still rolls gain", func(t *testing.T) {
		prof := newProfStub()
		prof.vals["kick"] = 1
		// Variance 50, prof 1 → chance = 1*50/100 = 0 → floored to 1.
		// First roll (hit): 50 → 51 > 1 → miss. Second roll (gain):
		// 0 → 1, threshold base 100 * 0.99 * failureMult 0.5 ≈ 49 → gain.
		roller := &seqRoller{t: t, seq: []int{50, 0}}
		r := NewAbilityResolver(DefaultResolutionConfig(), prof, prof, nil, nil, nil, nil, roller)
		ab := &Ability{
			ID: "kick", Category: AbilitySkill, Variance: 50,
			GainBaseChance: 100, GainFailureMultiplier: 0.5,
		}
		src := &fakeSource{id: "p1"}

		out := r.Resolve(context.Background(), src, ab, "m1", 0)
		if out.Hit {
			t.Fatalf("expected miss")
		}
		if prof.gains["kick"] != 1 {
			t.Fatalf("want gain rolled on miss, got %d", prof.gains["kick"])
		}
	})
}

func TestResolve_NoGainAtEffectiveCap(t *testing.T) {
	prof := newProfStub()
	prof.vals["kick"] = 100 // already at ceiling
	// alwaysHitRoller (nil) would otherwise fire the gain; the cap
	// guard must short-circuit before any roll.
	r := NewAbilityResolver(DefaultResolutionConfig(), prof, prof, nil, nil, nil, nil, nil)
	ab := &Ability{ID: "kick", Category: AbilitySkill, Variance: 0, GainBaseChance: 100}
	src := &fakeSource{id: "p1"}

	r.Resolve(context.Background(), src, ab, "m1", 0)

	if prof.gains["kick"] != 0 {
		t.Fatalf("no gain expected at prof 100, got %d", prof.gains["kick"])
	}
}

// capStub adds a Cap accessor so the resolver's effectiveCapOf can
// read a per-ability cap below 100.
type capStub struct {
	*profStub
	caps map[string]int
}

func (c *capStub) Cap(_, abilityID string) int {
	if v, ok := c.caps[abilityID]; ok {
		return v
	}
	return 100
}

func TestResolve_NoGainAtEffectiveCapBelow100(t *testing.T) {
	base := newProfStub()
	base.vals["kick"] = 50 // at the per-ability cap of 50
	prof := &capStub{profStub: base, caps: map[string]int{"kick": 50}}
	r := NewAbilityResolver(DefaultResolutionConfig(), prof, prof, nil, nil, nil, nil, nil)
	ab := &Ability{ID: "kick", Category: AbilitySkill, Variance: 0, GainBaseChance: 100}
	src := &fakeSource{id: "p1"}

	r.Resolve(context.Background(), src, ab, "m1", 0)

	if base.gains["kick"] != 0 {
		t.Fatalf("no gain expected at effective cap 50, got %d", base.gains["kick"])
	}
}

// hasOnlyReader implements ProficiencyReader without the richer
// Proficiency/Cap accessors, exercising proficiencyOf's fallback
// (prof 0) and effectiveCapOf's default (100).
type hasOnlyReader struct{ learned map[string]bool }

func (h hasOnlyReader) Has(_, abilityID string) bool { return h.learned[abilityID] }

func TestResolve_BareReaderUsesConservativeDefaults(t *testing.T) {
	reader := hasOnlyReader{learned: map[string]bool{"kick": true}}
	// Variance 50, prof unknown → 0 → chance floored to 1. Roll 0 → 1
	// ≤ 1 → hit. Confirms the Has-only fallback produces a usable
	// (minimum) hit chance rather than panicking on the missing
	// accessor.
	roller := &seqRoller{t: t, seq: []int{0}}
	r := NewAbilityResolver(DefaultResolutionConfig(), reader, nil, nil, nil, nil, nil, roller)
	ab := &Ability{ID: "kick", Category: AbilitySkill, Variance: 50}
	src := &fakeSource{id: "p1"}

	if out := r.Resolve(context.Background(), src, ab, "m1", 0); !out.Hit {
		t.Fatalf("bare reader: prof-0 floor-1 chance should hit on roll 1")
	}
}

func TestResolve_NilSourceOrAbilityIsNoOp(t *testing.T) {
	r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, nil, nil, nil, nil, nil)
	if out := r.Resolve(context.Background(), nil, &Ability{ID: "x"}, "", 0); out != (ResolveOutcome{}) {
		t.Fatalf("nil source must yield zero outcome, got %+v", out)
	}
	if out := r.Resolve(context.Background(), &fakeSource{id: "p1"}, nil, "", 0); out != (ResolveOutcome{}) {
		t.Fatalf("nil ability must yield zero outcome, got %+v", out)
	}
}

func TestResolve_HitChanceClampedByMaxHitChance(t *testing.T) {
	prof := newProfStub()
	prof.vals["kick"] = 100
	// prof 100 * variance 100 / 100 = 100, but MaxHitChance 80 caps it.
	// Roll 80 → 81 > 80 → miss; roll 79 → 80 ≤ 80 → hit.
	roller := &seqRoller{t: t, seq: []int{80, 79}}
	r := NewAbilityResolver(DefaultResolutionConfig(), prof, nil, nil, nil, nil, nil, roller)
	ab := &Ability{ID: "kick", Category: AbilitySkill, Variance: 100, MaxHitChance: 80}
	src := &fakeSource{id: "p1"}

	if out := r.Resolve(context.Background(), src, ab, "m1", 0); out.Hit {
		t.Fatalf("roll 81 vs cap 80 should miss")
	}
	if out := r.Resolve(context.Background(), src, ab, "m1", 0); !out.Hit {
		t.Fatalf("roll 80 vs cap 80 should hit")
	}
}
