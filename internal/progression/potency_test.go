package progression

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/stats"
)

func TestScaleMagnitude(t *testing.T) {
	tests := []struct {
		name    string
		v       int
		potency float64
		want    int
	}{
		{"full potency passes through", 5, 1.0, 5},
		{"above one is inert", 5, 1.5, 5},
		{"half rounds half up", 5, 0.5, 3},   // 2.5 → 3
		{"quarter rounds", 7, 0.25, 2},        // 1.75 → 2
		{"small rounds to zero", 1, 0.25, 0},  // 0.25 → 0
		{"zero stays zero", 0, 0.5, 0},        // 0
		{"negative preserves sign", -4, 0.5, -2},
		{"negative rounds magnitude", -7, 0.25, -2}, // -(1.75→2)
		{"zero potency zeroes magnitude", 10, 0, 0},
		{"negative potency does not flip sign", 10, -0.5, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := scaleMagnitude(tc.v, tc.potency); got != tc.want {
				t.Errorf("scaleMagnitude(%d, %v) = %d, want %d", tc.v, tc.potency, got, tc.want)
			}
		})
	}
}

func TestScaleDC(t *testing.T) {
	tests := []struct {
		name    string
		dc      int
		potency float64
		want    int
	}{
		{"full potency unchanged", 14, 1.0, 14},
		{"half", 14, 0.5, 7},
		{"quarter floors above one", 14, 0.25, 4}, // 3.5 → 4
		{"never below one", 2, 0.25, 1},           // 0.5 → 1 (floored)
		{"already one stays one", 1, 0.5, 1},      // 0.5 → 1 (floored)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := scaleDC(tc.dc, tc.potency); got != tc.want {
				t.Errorf("scaleDC(%d, %v) = %d, want %d", tc.dc, tc.potency, got, tc.want)
			}
		})
	}
}

func TestEffectTemplate_ScaledBy(t *testing.T) {
	base := EffectTemplate{
		ID:          "warding",
		Duration:    12,
		Refreshable: true,
		Flags:       []string{"condition:warded"},
		Modifiers: []stats.Modifier{
			{Stat: "ac", Value: 2},
			{Stat: "hit_mod", Value: 1},
		},
		RecurringSave: &ConditionSave{Axis: SaveReflex, DC: 14},
	}

	t.Run("full potency returns equivalent, original untouched", func(t *testing.T) {
		got := base.scaledBy(1.0)
		if got.Modifiers[0].Value != 2 || got.Modifiers[1].Value != 1 {
			t.Errorf("full potency must not scale modifiers: %+v", got.Modifiers)
		}
		if got.RecurringSave.DC != 14 {
			t.Errorf("full potency must not scale recurring DC: %d", got.RecurringSave.DC)
		}
	})

	t.Run("weak potency scales modifiers and recurring DC", func(t *testing.T) {
		got := base.scaledBy(0.5)
		if got.Modifiers[0].Value != 1 { // 2 * 0.5 = 1
			t.Errorf("ac modifier: got %d, want 1", got.Modifiers[0].Value)
		}
		if got.Modifiers[1].Value != 1 { // 1 * 0.5 = 0.5 → 1 (round half up)
			t.Errorf("hit_mod modifier: got %d, want 1", got.Modifiers[1].Value)
		}
		if got.RecurringSave.DC != 7 { // 14 * 0.5
			t.Errorf("recurring DC: got %d, want 7", got.RecurringSave.DC)
		}
		// Duration, flags, id, refresh semantics pass through unchanged.
		if got.Duration != 12 || got.ID != "warding" || len(got.Flags) != 1 || !got.Refreshable {
			t.Errorf("non-magnitude fields must pass through: %+v", got)
		}
	})

	t.Run("scaling does not mutate the shared template", func(t *testing.T) {
		_ = base.scaledBy(0.25)
		if base.Modifiers[0].Value != 2 || base.Modifiers[1].Value != 1 {
			t.Errorf("original modifiers mutated: %+v", base.Modifiers)
		}
		if base.RecurringSave.DC != 14 {
			t.Errorf("original recurring DC mutated: %d", base.RecurringSave.DC)
		}
	})

	t.Run("nil recurring save is safe", func(t *testing.T) {
		noSave := EffectTemplate{ID: "x", Modifiers: []stats.Modifier{{Stat: "ac", Value: 4}}}
		got := noSave.scaledBy(0.5)
		if got.RecurringSave != nil {
			t.Error("nil recurring save must stay nil")
		}
		if got.Modifiers[0].Value != 2 {
			t.Errorf("modifier scaling broken: %d", got.Modifiers[0].Value)
		}
	})
}

// A weave woven below affinity lands a weaker effect: the entry-save DC the
// target rolls against is scaled down, and the installed modifier magnitudes
// shrink. Mirrors the damage/heal scaling the host applies in the ability.used
// handler — the resolver owns the effect/DC half.
func TestResolve_PotencyScalesEntrySaveDCAndModifiers(t *testing.T) {
	eff := &effectSpy{result: true}
	sv := &fakeSaveResolver{made: false} // target fails → effect lands, DC captured
	r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, nil, eff, nil, &recordingSink{}, nil)
	r.SetSaveResolver(sv)
	r.SetPotencyProvider(func(_, _ string) float64 { return 0.5 })
	src := &fakeSource{id: "p1", movement: 100}

	ab := &Ability{
		ID: "bind", Category: AbilitySkill, Variance: 0,
		Effect: &EffectTemplate{
			ID: "bound", Duration: 3,
			Modifiers:     []stats.Modifier{{Stat: "ac", Value: 4}},
			RecurringSave: &ConditionSave{Axis: SaveReflex, DC: 14},
		},
		ApplySave: &ConditionSave{Axis: SaveReflex, DC: 14},
	}

	out := r.Resolve(context.Background(), src, ab, "mob1", 0)

	if !out.EffectApplied {
		t.Fatal("failed save → effect should apply")
	}
	if len(sv.dcs) != 1 || sv.dcs[0] != 7 { // 14 * 0.5
		t.Errorf("entry-save DC = %v, want [7] (scaled by 0.5)", sv.dcs)
	}
	if len(eff.calls) != 1 {
		t.Fatalf("want one Apply, got %d", len(eff.calls))
	}
	applied := eff.calls[0]
	if applied.Modifiers[0].Value != 2 { // 4 * 0.5
		t.Errorf("applied ac modifier = %d, want 2", applied.Modifiers[0].Value)
	}
	if applied.RecurringSave.DC != 7 { // 14 * 0.5
		t.Errorf("applied recurring DC = %d, want 7", applied.RecurringSave.DC)
	}
}

// With no potency provider (the fantasy default) the entry-save DC and the
// effect land at full strength — byte-identical to pre-affinity behavior.
func TestResolve_NoPotencyProviderFullStrength(t *testing.T) {
	eff := &effectSpy{result: true}
	sv := &fakeSaveResolver{made: false}
	r := NewAbilityResolver(DefaultResolutionConfig(), newProfStub(), nil, nil, eff, nil, &recordingSink{}, nil)
	r.SetSaveResolver(sv)
	src := &fakeSource{id: "p1", movement: 100}

	ab := &Ability{
		ID: "bind", Category: AbilitySkill, Variance: 0,
		Effect:    &EffectTemplate{ID: "bound", Duration: 3, Modifiers: []stats.Modifier{{Stat: "ac", Value: 4}}},
		ApplySave: &ConditionSave{Axis: SaveReflex, DC: 14},
	}

	if out := r.Resolve(context.Background(), src, ab, "mob1", 0); !out.EffectApplied {
		t.Fatal("effect should apply")
	}
	if len(sv.dcs) != 1 || sv.dcs[0] != 14 {
		t.Errorf("entry-save DC = %v, want [14] (unscaled)", sv.dcs)
	}
	if eff.calls[0].Modifiers[0].Value != 4 {
		t.Errorf("modifier = %d, want 4 (unscaled)", eff.calls[0].Modifiers[0].Value)
	}
}
