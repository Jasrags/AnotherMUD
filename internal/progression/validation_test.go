package progression_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

type fakeEntity struct {
	id        string
	resting   bool
	align     int
	standings map[string]int // faction id → standing; nil ⇒ MeetsFactionStanding fails open
	tags      map[string][]string
	equipped  map[string]bool
	inCombat  bool
	target    string
	hasTarget bool
	movement  int
	mana      int
	race      *progression.Race
}

func (f *fakeEntity) EntityID() string { return f.id }
func (f *fakeEntity) IsResting() bool  { return f.resting }
func (f *fakeEntity) Alignment() int   { return f.align }
func (f *fakeEntity) MeetsFactionStanding(faction string, min int) bool {
	if f.standings == nil {
		return true
	}
	return f.standings[faction] >= min
}
func (f *fakeEntity) EquippedTags(slot string) ([]string, bool) {
	if !f.equipped[slot] {
		return nil, false
	}
	return f.tags[slot], true
}
func (f *fakeEntity) InCombat() bool                { return f.inCombat }
func (f *fakeEntity) CurrentTarget() (string, bool) { return f.target, f.hasTarget }
func (f *fakeEntity) Movement() int                 { return f.movement }
func (f *fakeEntity) Mana() int                     { return f.mana }
func (f *fakeEntity) Race() *progression.Race       { return f.race }

type profSet map[string]map[string]bool

func (p profSet) Has(eid, aid string) bool { return p[eid][aid] }

type effSet map[string]map[string]bool

func (e effSet) Has(eid, eff string) bool { return e[eid][eff] }

type pdSet map[string]map[string]int64

func (p pdSet) IsCoolingDown(eid, aid string, current int64) bool {
	return p[eid][aid] > current
}

type tgtSet map[string]bool

func (t tgtSet) ResolveID(id string) bool { return t[id] }

func buildPipeline(t *testing.T, abilities []*progression.Ability) (*progression.ValidationPipeline, *progression.AbilityRegistry, profSet, effSet, pdSet, tgtSet) {
	t.Helper()
	reg := progression.NewAbilityRegistry()
	for _, a := range abilities {
		if err := reg.Register(a); err != nil {
			t.Fatalf("register %q: %v", a.ID, err)
		}
	}
	prof := profSet{"ent-1": {}}
	eff := effSet{"ent-1": {}}
	pd := pdSet{"ent-1": {}}
	tgt := tgtSet{"mob-1": true}
	p := progression.NewValidationPipeline(reg, prof, eff, pd, tgt)
	return p, reg, prof, eff, pd, tgt
}

func TestValidate_UnknownAbility(t *testing.T) {
	p, _, _, _, _, _ := buildPipeline(t, nil)
	res := p.Validate(&fakeEntity{id: "ent-1"}, progression.QueuedAction{AbilityID: "missing"}, 1)
	if res.Reason != progression.FizzleUnknownAbility {
		t.Errorf("Reason = %q, want unknown_ability", res.Reason)
	}
}

func TestValidate_EmptyAbilityID(t *testing.T) {
	p, _, _, _, _, _ := buildPipeline(t, nil)
	res := p.Validate(&fakeEntity{id: "ent-1"}, progression.QueuedAction{}, 1)
	if res.Reason != progression.FizzleUnknownAbility {
		t.Errorf("Reason = %q, want unknown_ability", res.Reason)
	}
}

func TestValidate_OrderingFirstFailureWins(t *testing.T) {
	// Build an ability that would fail multiple checks; rest comes
	// first (§4.3 step 1) so we expect FizzleAsleep even though
	// proficiency, equipment, alignment, etc. would also fail.
	ability := &progression.Ability{
		ID:                "slash",
		DisplayName:       "Slash",
		Type:              progression.AbilityActive,
		Category:          progression.AbilitySkill,
		HasAlignmentRange: true,
		AlignmentMin:      100, AlignmentMax: 200,
		EquipmentSlot: "wield",
		Cost:          50,
	}
	p, _, _, _, _, _ := buildPipeline(t, []*progression.Ability{ability})
	res := p.Validate(&fakeEntity{id: "ent-1", resting: true, align: 0}, progression.QueuedAction{AbilityID: "slash"}, 1)
	if res.Reason != progression.FizzleAsleep {
		t.Errorf("Reason = %q, want asleep", res.Reason)
	}
}

func TestValidate_AlignmentRestricted(t *testing.T) {
	ability := &progression.Ability{
		ID: "smite", DisplayName: "Smite",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
		HasAlignmentRange: true, AlignmentMin: 100, AlignmentMax: 1000,
	}
	p, _, prof, _, _, _ := buildPipeline(t, []*progression.Ability{ability})
	prof["ent-1"]["smite"] = true
	res := p.Validate(&fakeEntity{id: "ent-1", align: 0, inCombat: true, hasTarget: true, target: "mob-1"},
		progression.QueuedAction{AbilityID: "smite"}, 1)
	if res.Reason != progression.FizzleAlignmentRestricted {
		t.Errorf("Reason = %q, want alignment_restricted", res.Reason)
	}
}

func TestValidate_FactionRestricted(t *testing.T) {
	ability := &progression.Ability{
		ID: "rally", DisplayName: "Rally the Guard",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
		FactionRequirements: []progression.AbilityFactionRequirement{
			{Faction: "wot:queens-guard", MinStanding: 100},
		},
	}
	p, _, prof, _, _, _ := buildPipeline(t, []*progression.Ability{ability})
	prof["ent-1"]["rally"] = true

	// Below the threshold → faction_restricted.
	res := p.Validate(&fakeEntity{id: "ent-1", standings: map[string]int{"wot:queens-guard": 50},
		inCombat: true, hasTarget: true, target: "mob-1"},
		progression.QueuedAction{AbilityID: "rally"}, 1)
	if res.Reason != progression.FizzleFactionRestricted {
		t.Errorf("below standing Reason = %q, want faction_restricted", res.Reason)
	}

	// At/above the threshold → passes the faction gate (FizzleOK).
	res = p.Validate(&fakeEntity{id: "ent-1", standings: map[string]int{"wot:queens-guard": 250},
		inCombat: true, hasTarget: true, target: "mob-1"},
		progression.QueuedAction{AbilityID: "rally"}, 1)
	if res.Reason != progression.FizzleOK {
		t.Errorf("met standing Reason = %q, want OK", res.Reason)
	}
}

func TestValidate_FactionGateFailsOpenWhenUnresolved(t *testing.T) {
	ability := &progression.Ability{
		ID: "rally", DisplayName: "Rally the Guard",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
		FactionRequirements: []progression.AbilityFactionRequirement{
			{Faction: "wot:queens-guard", MinStanding: 100},
		},
	}
	p, _, prof, _, _, _ := buildPipeline(t, []*progression.Ability{ability})
	prof["ent-1"]["rally"] = true
	// nil standings (no faction wired) → MeetsFactionStanding returns true; the
	// gate does not refuse, so the ability passes validation.
	res := p.Validate(&fakeEntity{id: "ent-1", inCombat: true, hasTarget: true, target: "mob-1"},
		progression.QueuedAction{AbilityID: "rally"}, 1)
	if res.Reason != progression.FizzleOK {
		t.Errorf("unwired faction Reason = %q, want OK (fail open)", res.Reason)
	}
}

func TestValidate_NoProficiency(t *testing.T) {
	ability := &progression.Ability{
		ID: "kick", DisplayName: "Kick",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
	}
	p, _, _, _, _, _ := buildPipeline(t, []*progression.Ability{ability})
	res := p.Validate(&fakeEntity{id: "ent-1", inCombat: true, hasTarget: true, target: "mob-1"},
		progression.QueuedAction{AbilityID: "kick"}, 1)
	if res.Reason != progression.FizzleNoProficiency {
		t.Errorf("Reason = %q, want no_proficiency", res.Reason)
	}
}

func TestValidate_EquipmentRequired(t *testing.T) {
	ability := &progression.Ability{
		ID: "slash", DisplayName: "Slash",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
		EquipmentSlot: "wield", EquipmentTag: "blade",
	}
	p, _, prof, _, _, _ := buildPipeline(t, []*progression.Ability{ability})
	prof["ent-1"]["slash"] = true

	// No item equipped
	res := p.Validate(&fakeEntity{id: "ent-1", inCombat: true, hasTarget: true, target: "mob-1"},
		progression.QueuedAction{AbilityID: "slash"}, 1)
	if res.Reason != progression.FizzleEquipmentRequired {
		t.Errorf("Reason (empty slot) = %q, want equipment_required", res.Reason)
	}

	// Wrong tag
	ent := &fakeEntity{
		id: "ent-1", inCombat: true, hasTarget: true, target: "mob-1",
		equipped: map[string]bool{"wield": true},
		tags:     map[string][]string{"wield": {"mace"}},
	}
	res = p.Validate(ent, progression.QueuedAction{AbilityID: "slash"}, 1)
	if res.Reason != progression.FizzleEquipmentRequired {
		t.Errorf("Reason (wrong tag) = %q, want equipment_required", res.Reason)
	}

	// Correct tag → passes step 4
	ent.tags["wield"] = []string{"BLADE"} // case-insensitive
	res = p.Validate(ent, progression.QueuedAction{AbilityID: "slash"}, 1)
	if res.Reason != progression.FizzleOK {
		t.Errorf("Reason (good equip) = %q, want ok", res.Reason)
	}
}

func TestValidate_InitiateOnly(t *testing.T) {
	ability := &progression.Ability{
		ID: "ambush", DisplayName: "Ambush",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
		InitiateOnly: true,
	}
	p, _, prof, _, _, _ := buildPipeline(t, []*progression.Ability{ability})
	prof["ent-1"]["ambush"] = true
	res := p.Validate(&fakeEntity{id: "ent-1", inCombat: true},
		progression.QueuedAction{AbilityID: "ambush", TargetEntityID: "mob-1"}, 1)
	if res.Reason != progression.FizzleInitiateOnly {
		t.Errorf("Reason = %q, want initiate_only", res.Reason)
	}
}

func TestValidate_InvalidTarget(t *testing.T) {
	ability := &progression.Ability{
		ID: "slash", DisplayName: "Slash",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
	}
	p, _, prof, _, _, _ := buildPipeline(t, []*progression.Ability{ability})
	prof["ent-1"]["slash"] = true
	res := p.Validate(&fakeEntity{id: "ent-1", inCombat: true},
		progression.QueuedAction{AbilityID: "slash", TargetEntityID: "ghost"}, 1)
	if res.Reason != progression.FizzleInvalidTarget {
		t.Errorf("Reason = %q, want invalid_target", res.Reason)
	}
}

func TestValidate_NotInCombat(t *testing.T) {
	ability := &progression.Ability{
		ID: "slash", DisplayName: "Slash",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
	}
	p, _, prof, _, _, _ := buildPipeline(t, []*progression.Ability{ability})
	prof["ent-1"]["slash"] = true
	// Explicit resolvable target + not in combat ⇒ not_in_combat
	// (in-combat gate fires before target resolution).
	res := p.Validate(&fakeEntity{id: "ent-1", inCombat: false},
		progression.QueuedAction{AbilityID: "slash", TargetEntityID: "mob-1"}, 1)
	if res.Reason != progression.FizzleNotInCombat {
		t.Errorf("explicit-target Reason = %q, want not_in_combat", res.Reason)
	}
	// No explicit target + not in combat ⇒ STILL not_in_combat
	// (regression guard for the M9.3 review High: previously this
	// path returned invalid_target because resolveTarget hit the
	// empty-CurrentTarget fallback before the in-combat gate).
	res = p.Validate(&fakeEntity{id: "ent-1", inCombat: false},
		progression.QueuedAction{AbilityID: "slash"}, 1)
	if res.Reason != progression.FizzleNotInCombat {
		t.Errorf("no-target Reason = %q, want not_in_combat", res.Reason)
	}
}

func TestValidate_EffectPresent(t *testing.T) {
	ability := &progression.Ability{
		ID: "bless", DisplayName: "Bless",
		Type: progression.AbilityActive, Category: progression.AbilitySpell,
		Effect: &progression.EffectTemplate{ID: "blessed", Duration: 30},
	}
	p, _, prof, eff, _, _ := buildPipeline(t, []*progression.Ability{ability})
	prof["ent-1"]["bless"] = true
	eff["ent-1"]["blessed"] = true
	// Self-cast (no explicit target, not offensive ⇒ resolves to self).
	res := p.Validate(&fakeEntity{id: "ent-1"},
		progression.QueuedAction{AbilityID: "bless"}, 1)
	if res.Reason != progression.FizzleEffectPresent {
		t.Errorf("Reason = %q, want effect_present", res.Reason)
	}
}

func TestValidate_PulseDelay(t *testing.T) {
	ability := &progression.Ability{
		ID: "blast", DisplayName: "Blast",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
		PulseDelay: 5,
	}
	p, _, prof, _, pd, _ := buildPipeline(t, []*progression.Ability{ability})
	prof["ent-1"]["blast"] = true
	pd["ent-1"]["blast"] = 50
	res := p.Validate(&fakeEntity{id: "ent-1", inCombat: true},
		progression.QueuedAction{AbilityID: "blast", TargetEntityID: "mob-1"}, 10)
	if res.Reason != progression.FizzlePulseDelay {
		t.Errorf("Reason = %q, want pulse_delay", res.Reason)
	}
}

func TestValidate_InsufficientResources(t *testing.T) {
	ability := &progression.Ability{
		ID: "kick", DisplayName: "Kick",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
		Cost: 25,
	}
	p, _, prof, _, _, _ := buildPipeline(t, []*progression.Ability{ability})
	prof["ent-1"]["kick"] = true
	res := p.Validate(&fakeEntity{id: "ent-1", inCombat: true, movement: 10},
		progression.QueuedAction{AbilityID: "kick", TargetEntityID: "mob-1"}, 1)
	if res.Reason != progression.FizzleInsufficientResources {
		t.Errorf("Reason = %q, want insufficient_resources", res.Reason)
	}
	// Mana-cost spell with no effect (non-offensive per M9.3
	// classifier) drawing from mana pool.
	spell := &progression.Ability{
		ID: "heal", DisplayName: "Heal",
		Type: progression.AbilityActive, Category: progression.AbilitySpell,
		Cost: 20,
	}
	p2, _, prof2, _, _, _ := buildPipeline(t, []*progression.Ability{spell})
	prof2["ent-1"]["heal"] = true
	res = p2.Validate(&fakeEntity{id: "ent-1", mana: 5},
		progression.QueuedAction{AbilityID: "heal"}, 1)
	if res.Reason != progression.FizzleInsufficientResources {
		t.Errorf("spell mana check Reason = %q", res.Reason)
	}
}

// The reserve-to-begin gate (WoT S2): a spell requires the caller to HOLD
// reserveMultiple × cost to begin, though only cost is spent. Default
// multiple 1 = the plain cost check; the gate applies to mana only.
func TestValidate_ReserveToBeginGate(t *testing.T) {
	spell := &progression.Ability{
		ID: "weave", DisplayName: "Weave",
		Type: progression.AbilityActive, Category: progression.AbilitySpell,
		Cost: 20,
	}

	// Default multiple (1): exactly cost worth of mana passes.
	p1, _, prof1, _, _, _ := buildPipeline(t, []*progression.Ability{spell})
	prof1["ent-1"]["weave"] = true
	if res := p1.Validate(&fakeEntity{id: "ent-1", mana: 20},
		progression.QueuedAction{AbilityID: "weave"}, 1); res.Reason != progression.FizzleOK {
		t.Fatalf("default multiple: 20 mana / cost 20 should pass, got %q", res.Reason)
	}

	// Reserve multiple 2: needs 40 to BEGIN even though only 20 is spent.
	p2, _, prof2, _, _, _ := buildPipeline(t, []*progression.Ability{spell})
	prof2["ent-1"]["weave"] = true
	p2.SetReserveMultiple(2)
	// 30 mana — can pay, but lacks the headroom → fizzle.
	if res := p2.Validate(&fakeEntity{id: "ent-1", mana: 30},
		progression.QueuedAction{AbilityID: "weave"}, 1); res.Reason != progression.FizzleInsufficientResources {
		t.Fatalf("reserve 2×: 30 mana / cost 20 should fizzle (need 40), got %q", res.Reason)
	}
	// 40 mana — meets the reserve.
	if res := p2.Validate(&fakeEntity{id: "ent-1", mana: 40},
		progression.QueuedAction{AbilityID: "weave"}, 1); res.Reason != progression.FizzleOK {
		t.Fatalf("reserve 2×: 40 mana / cost 20 should pass, got %q", res.Reason)
	}

	// The reserve multiple does NOT gate movement abilities — cost worth of
	// movement still passes under a 2× reserve.
	skill := &progression.Ability{
		ID: "kick", DisplayName: "Kick",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
		Cost: 20,
	}
	p3, _, prof3, _, _, _ := buildPipeline(t, []*progression.Ability{skill})
	prof3["ent-1"]["kick"] = true
	p3.SetReserveMultiple(2)
	if res := p3.Validate(&fakeEntity{id: "ent-1", inCombat: true, hasTarget: true, target: "mob-1", movement: 20},
		progression.QueuedAction{AbilityID: "kick"}, 1); res.Reason != progression.FizzleOK {
		t.Fatalf("movement ability ignores reserve: 20 mv / cost 20 should pass, got %q", res.Reason)
	}
}

// Overchannel (WoT S2): a flagged action that lacks the reserve does NOT
// fizzle — it validates as an overchannel and reports the deficit (how far
// below the reserve threshold the caster was). An unflagged short cast still
// fizzles; a flagged cast that holds the reserve is an ordinary cast.
// Channel-block (WoT S2): a stilled caster cannot weave (spell-category
// fizzles FizzleStilled) but can still use a skill; the gate is inert until a
// ruleset wires SetChannelBlockEffect.
func TestValidate_StilledBlocksChanneling(t *testing.T) {
	weave := &progression.Ability{ID: "weave", DisplayName: "Weave", Type: progression.AbilityActive, Category: progression.AbilitySpell}
	kick := &progression.Ability{ID: "kick", DisplayName: "Kick", Type: progression.AbilityActive, Category: progression.AbilitySkill}

	p, _, prof, eff, _, _ := buildPipeline(t, []*progression.Ability{weave, kick})
	prof["ent-1"]["weave"] = true
	prof["ent-1"]["kick"] = true
	eff["ent-1"]["stilled"] = true // the caster carries the block effect

	// Gate inert until wired: the weave validates fine.
	if res := p.Validate(&fakeEntity{id: "ent-1"},
		progression.QueuedAction{AbilityID: "weave"}, 1); res.Reason != progression.FizzleOK {
		t.Fatalf("gate not wired: weave should pass, got %q", res.Reason)
	}

	p.SetChannelBlockEffect("stilled")
	// Spell fizzles stilled.
	if res := p.Validate(&fakeEntity{id: "ent-1"},
		progression.QueuedAction{AbilityID: "weave"}, 1); res.Reason != progression.FizzleStilled {
		t.Fatalf("stilled caster: weave should fizzle stilled, got %q", res.Reason)
	}
	// Skill still works (a stilled channeler can swing a sword) — needs combat+target.
	if res := p.Validate(&fakeEntity{id: "ent-1", inCombat: true, hasTarget: true, target: "mob-1"},
		progression.QueuedAction{AbilityID: "kick"}, 1); res.Reason != progression.FizzleOK {
		t.Fatalf("stilled caster: kick should still pass, got %q", res.Reason)
	}
}

func TestValidate_Overchannel(t *testing.T) {
	spell := &progression.Ability{
		ID: "weave", DisplayName: "Weave",
		Type: progression.AbilityActive, Category: progression.AbilitySpell,
		Cost: 20,
	}
	build := func() (*progression.ValidationPipeline, profSet) {
		p, _, prof, _, _, _ := buildPipeline(t, []*progression.Ability{spell})
		prof["ent-1"]["weave"] = true
		p.SetReserveMultiple(2) // threshold = 40
		return p, prof
	}

	// Unflagged + below reserve → fizzle (the safe default).
	p1, _ := build()
	if res := p1.Validate(&fakeEntity{id: "ent-1", mana: 25},
		progression.QueuedAction{AbilityID: "weave"}, 1); res.Reason != progression.FizzleInsufficientResources {
		t.Fatalf("unflagged short cast: want fizzle, got %q", res.Reason)
	}

	// Flagged + below reserve → allowed as overchannel, deficit = 40 − 25 = 15.
	p2, _ := build()
	res := p2.Validate(&fakeEntity{id: "ent-1", mana: 25},
		progression.QueuedAction{AbilityID: "weave", Overchannel: true}, 1)
	if res.Reason != progression.FizzleOK {
		t.Fatalf("flagged overchannel: want FizzleOK, got %q", res.Reason)
	}
	if !res.Overchannel || res.OverchannelDeficit != 15 {
		t.Fatalf("overchannel=%v deficit=%d; want true/15", res.Overchannel, res.OverchannelDeficit)
	}

	// Flagged but the caster HOLDS the reserve → ordinary cast, no risk.
	p3, _ := build()
	res = p3.Validate(&fakeEntity{id: "ent-1", mana: 40},
		progression.QueuedAction{AbilityID: "weave", Overchannel: true}, 1)
	if res.Reason != progression.FizzleOK || res.Overchannel {
		t.Fatalf("flagged-but-sufficient: want ordinary cast, got reason=%q overchannel=%v", res.Reason, res.Overchannel)
	}
}

func TestValidate_HappyPath(t *testing.T) {
	ability := &progression.Ability{
		ID: "kick", DisplayName: "Kick",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
		Cost: 5, PulseDelay: 2,
	}
	p, _, prof, _, _, _ := buildPipeline(t, []*progression.Ability{ability})
	prof["ent-1"]["kick"] = true
	res := p.Validate(&fakeEntity{id: "ent-1", inCombat: true, hasTarget: true, target: "mob-1", movement: 10},
		progression.QueuedAction{AbilityID: "kick"}, 1)
	if res.Reason != progression.FizzleOK {
		t.Fatalf("Reason = %q, want ok", res.Reason)
	}
	if res.ResolvedTarget != "mob-1" {
		t.Errorf("ResolvedTarget = %q, want mob-1 (fallback to current target)", res.ResolvedTarget)
	}
	if res.Ability == nil || res.Ability.ID != "kick" {
		t.Errorf("Ability = %+v", res.Ability)
	}
}

func TestValidate_SelfTargetForBuff(t *testing.T) {
	ability := &progression.Ability{
		ID: "guard", DisplayName: "Guard",
		Type: progression.AbilityActive, Category: progression.AbilitySpell,
		Effect: &progression.EffectTemplate{ID: "guarded", Duration: 10},
	}
	p, _, prof, _, _, _ := buildPipeline(t, []*progression.Ability{ability})
	prof["ent-1"]["guard"] = true
	res := p.Validate(&fakeEntity{id: "ent-1"},
		progression.QueuedAction{AbilityID: "guard"}, 1)
	if res.Reason != progression.FizzleOK {
		t.Fatalf("Reason = %q, want ok", res.Reason)
	}
	if res.ResolvedTarget != "ent-1" {
		t.Errorf("ResolvedTarget = %q, want self (ent-1)", res.ResolvedTarget)
	}
}

func TestIsOffensive(t *testing.T) {
	cases := []struct {
		name string
		ab   *progression.Ability
		want bool
	}{
		{"nil", nil, false},
		{"skill", &progression.Ability{Category: progression.AbilitySkill}, true},
		{"spell-no-effect-no-damage", &progression.Ability{Category: progression.AbilitySpell}, false},
		{"spell-with-effect", &progression.Ability{Category: progression.AbilitySpell, Effect: &progression.EffectTemplate{ID: "x"}}, false},
		// M9.6b: a damage spell with no effect is offensive (§4.6).
		{"spell-damage-no-effect", &progression.Ability{Category: progression.AbilitySpell, DamageDice: "1d6"}, true},
		// Effect present overrides damage dice → non-offensive.
		{"spell-damage-with-effect", &progression.Ability{Category: progression.AbilitySpell, DamageDice: "1d6", Effect: &progression.EffectTemplate{ID: "x"}}, false},
		// A heal spell (heal dice, no damage) is never offensive.
		{"spell-heal", &progression.Ability{Category: progression.AbilitySpell, HealDice: "2d4"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := progression.IsOffensive(tc.ab); got != tc.want {
				t.Errorf("IsOffensive = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResourceFor(t *testing.T) {
	if progression.ResourceFor(&progression.Ability{Category: progression.AbilitySkill}) != progression.ResourceMovement {
		t.Error("skill → movement")
	}
	if progression.ResourceFor(&progression.Ability{Category: progression.AbilitySpell}) != progression.ResourceMana {
		t.Error("spell → mana")
	}
	if progression.ResourceFor(nil) != progression.ResourceMovement {
		t.Error("nil ability → movement (safe default)")
	}
}
