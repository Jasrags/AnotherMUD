package progression

import (
	"context"
	"testing"
)

// special-weapons §4/§5 — a per-caster save-DC bonus (a trip/disarm weapon)
// raises the maneuver's entry-save DC additively, after the potency scale. nil
// hook leaves the content DC unchanged.
func TestResolve_SaveDCBonusRaisesEntryDC(t *testing.T) {
	newRig := func() (*AbilityResolver, *fakeSaveResolver) {
		sv := &fakeSaveResolver{made: false} // target fails → DC captured
		r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, nil, &effectSpy{result: true}, nil, &recordingSink{}, nil)
		r.SetSaveResolver(sv)
		return r, sv
	}
	ab := &Ability{
		ID: "trip", Category: AbilitySkill, Variance: 0,
		Effect:    &EffectTemplate{ID: "prone", Duration: 5, Flags: []string{"condition:prone"}},
		ApplySave: &ConditionSave{Axis: SaveReflex, DC: 13},
	}

	// A trip weapon adds +2 → DC 15.
	r, sv := newRig()
	r.SetSaveDCBonus(func(_, abilityID string) int {
		if abilityID == "trip" {
			return 2
		}
		return 0
	})
	r.Resolve(context.Background(), &fakeSource{id: "p1", movement: 100}, ab, "mob1", 0)
	if len(sv.dcs) != 1 || sv.dcs[0] != 15 {
		t.Errorf("trip-weapon DC = %v, want [15] (base 13 + 2)", sv.dcs)
	}

	// A bonus on a DIFFERENT ability leaves trip at the base DC.
	r, sv = newRig()
	r.SetSaveDCBonus(func(_, abilityID string) int {
		if abilityID == "disarm" {
			return 3
		}
		return 0
	})
	r.Resolve(context.Background(), &fakeSource{id: "p1", movement: 100}, ab, "mob1", 0)
	if len(sv.dcs) != 1 || sv.dcs[0] != 13 {
		t.Errorf("no trip bonus → DC = %v, want [13] (base, unchanged)", sv.dcs)
	}

	// No hook → content DC, byte-identical to pre-feature behavior.
	r, sv = newRig()
	r.Resolve(context.Background(), &fakeSource{id: "p1", movement: 100}, ab, "mob1", 0)
	if len(sv.dcs) != 1 || sv.dcs[0] != 13 {
		t.Errorf("nil DC-bonus hook → DC = %v, want [13]", sv.dcs)
	}
}
