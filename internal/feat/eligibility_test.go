package feat

import "testing"

// fakeView is a hand-built CharacterView for the evaluator tests.
type fakeView struct {
	scores  map[string]int
	skills  map[string]int
	feats   map[string]bool
	level   int
	classes []string
}

func (v fakeView) AbilityScore(stat string) int          { return v.scores[stat] }
func (v fakeView) SkillProficiency(abilityID string) int { return v.skills[abilityID] }
func (v fakeView) HasFeat(featID string) bool            { return v.feats[featID] }
func (v fakeView) CharacterLevel() int                   { return v.level }
func (v fakeView) ClassIDs() []string                    { return v.classes }

func TestEligible_NoPrereqsAlwaysOK(t *testing.T) {
	f := &Feat{ID: "toughness"}
	got := Eligible(f, fakeView{})
	if !got.OK || len(got.UnmetPrereqs) != 0 || got.ClassExcluded {
		t.Errorf("Eligible = %+v, want OK with nothing unmet", got)
	}
}

func TestEligible_AllPrereqKinds(t *testing.T) {
	f := &Feat{
		ID: "whirlwind",
		Prerequisites: []Prerequisite{
			{Kind: PrereqAbilityScore, Target: "str", Min: 13},
			{Kind: PrereqSkill, Target: "open-lock", Min: 5},
			{Kind: PrereqFeat, Target: "power-attack"},
			{Kind: PrereqLevel, Min: 4},
		},
	}

	// All satisfied exactly at the thresholds.
	meets := fakeView{
		scores: map[string]int{"str": 13},
		skills: map[string]int{"open-lock": 5},
		feats:  map[string]bool{"power-attack": true},
		level:  4,
	}
	if got := Eligible(f, meets); !got.OK {
		t.Errorf("at-threshold should be eligible, got %+v", got)
	}

	// None satisfied: all four unmet, in declaration order.
	none := fakeView{level: 3}
	got := Eligible(f, none)
	if got.OK || len(got.UnmetPrereqs) != 4 {
		t.Fatalf("none-met = %+v, want 4 unmet, not OK", got)
	}
	wantOrder := []PrereqKind{PrereqAbilityScore, PrereqSkill, PrereqFeat, PrereqLevel}
	for i, p := range got.UnmetPrereqs {
		if p.Kind != wantOrder[i] {
			t.Errorf("unmet[%d].Kind = %q, want %q", i, p.Kind, wantOrder[i])
		}
	}
}

func TestEligible_OneShortIsUnmet(t *testing.T) {
	f := &Feat{ID: "x", Prerequisites: []Prerequisite{{Kind: PrereqAbilityScore, Target: "str", Min: 13}}}
	// 12 < 13 → unmet.
	got := Eligible(f, fakeView{scores: map[string]int{"str": 12}})
	if got.OK || len(got.UnmetPrereqs) != 1 || got.UnmetPrereqs[0].Kind != PrereqAbilityScore {
		t.Errorf("str 12 vs 13 = %+v, want one unmet ability_score", got)
	}
}

func TestEligible_ClassGate(t *testing.T) {
	f := &Feat{ID: "eliminate-block", AllowedClasses: []string{"wilder", "initiate"}}

	// Held class is allowed → OK.
	if got := Eligible(f, fakeView{classes: []string{"Wilder"}}); !got.OK || got.ClassExcluded {
		t.Errorf("allowed class = %+v, want OK", got)
	}
	// Held class not in the list → excluded, not OK.
	got := Eligible(f, fakeView{classes: []string{"fighter"}})
	if got.OK || !got.ClassExcluded {
		t.Errorf("disallowed class = %+v, want excluded", got)
	}
	// No allowed list = unrestricted, even classless.
	open := &Feat{ID: "toughness"}
	if got := Eligible(open, fakeView{}); !got.OK {
		t.Errorf("empty AllowedClasses should be unrestricted, got %+v", got)
	}
}

func TestEligible_NilFeatOrViewNotEligible(t *testing.T) {
	if Eligible(nil, fakeView{}).OK {
		t.Error("nil feat should not be eligible")
	}
	if Eligible(&Feat{ID: "x"}, nil).OK {
		t.Error("nil view should not be eligible")
	}
}
