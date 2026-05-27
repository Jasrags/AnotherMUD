package progression

import (
	"context"
	"testing"
)

// staticTrainerSource hands back a fixed TrainerConfig for any
// entity id — used to drive TryPractice from the test side without
// dragging in the session-layer adapter.
type staticTrainerSource struct {
	cfg  *TrainerConfig
	name string
}

func (s *staticTrainerSource) TrainerInRoom(_ string) (*TrainerConfig, string, bool) {
	if s.cfg == nil {
		return nil, "", false
	}
	return s.cfg, s.name, true
}

// TestPracticePipeline_LearnThenPracticeRaisesCap is the M9.1
// acceptance criterion paired with the M8.6 deferred-debt fix:
// once the ProficiencyManager is wired as the AbilityProficiency
// seam, a learned ability + in-room trainer raises the entity's
// cap on practice (spec progression.md §7.3 / §7.5).
//
// Before M9.1 the seam was a nop and TryPractice short-circuited
// at NotLearned. This test pins that the wired path now reaches
// PracticeSuccess and that the new cap round-trips through the
// manager's GetCap.
func TestPracticePipeline_LearnThenPracticeRaisesCap(t *testing.T) {
	ctx := context.Background()

	abilities := NewAbilityRegistry()
	if err := abilities.Register(&Ability{
		ID:          "slash",
		DisplayName: "Slash",
		Type:        AbilityActive,
		Category:    AbilitySkill,
		DefaultCap:  10, // start lower than Novice so practice has room to lift
	}); err != nil {
		t.Fatal(err)
	}
	prof := NewProficiencyManager(abilities, DefaultProficiencyConfig())
	prof.Learn("p-1", "slash", 3)

	trainer := &staticTrainerSource{
		cfg:  &TrainerConfig{Tier: CapNovice, Teach: []string{"slash"}},
		name: "Maerys",
	}

	mgr := NewTrainingManager(DefaultTrainingConfig(), NewRaceRegistry(), trainer, prof)

	res := mgr.TryPractice(ctx, &trainsEntity{}, "p-1", "slash")
	if res.Outcome != PracticeSuccess {
		t.Fatalf("Outcome = %v, want PracticeSuccess; result=%+v", res.Outcome, res)
	}
	if res.NewCap != int(CapNovice) {
		t.Errorf("NewCap = %d, want %d", res.NewCap, int(CapNovice))
	}
	if !res.Boosted {
		t.Errorf("Boosted = false, want true (prof=3 < prior cap=10)")
	}

	// Cap actually moved on the manager.
	capV, profV, learned := prof.GetCap("p-1", "slash")
	if capV != int(CapNovice) {
		t.Errorf("post-practice cap = %d, want %d", capV, int(CapNovice))
	}
	if !learned {
		t.Errorf("learned = false after practice")
	}
	// Catch-up boost lifted proficiency from 3 toward prior cap (10),
	// capped at 5 by DefaultTrainingConfig.CatchUpBoost.
	if profV != 8 {
		t.Errorf("post-practice prof = %d, want 8 (3 + 5 boost)", profV)
	}
}

// TestPracticePipeline_UnknownAbilityReportsNotLearned pins that
// even with the seam wired, asking to practice an unknown ability
// reports NotLearned (the same outcome the M8.6 nop produced for
// every ability) — protecting renderers + scripts from a regression
// once content-side packs start shipping abilities the trainer
// doesn't teach.
func TestPracticePipeline_UnknownAbilityReportsNotLearned(t *testing.T) {
	ctx := context.Background()
	prof := NewProficiencyManager(NewAbilityRegistry(), DefaultProficiencyConfig())
	mgr := NewTrainingManager(DefaultTrainingConfig(), NewRaceRegistry(),
		&staticTrainerSource{}, prof)
	res := mgr.TryPractice(ctx, &trainsEntity{}, "p-1", "ghost-ability")
	if res.Outcome != PracticeNotLearned {
		t.Errorf("unknown ability outcome = %v, want PracticeNotLearned", res.Outcome)
	}
}

// TestProficiencyManager_TeachWiresGrant pins the
// ClassPathProcessor → AbilityGranter seam: a class path entry
// that points at a registered ability lands as a real
// proficiency entry after Teach.
func TestProficiencyManager_TeachWiresGrant(t *testing.T) {
	abilities := NewAbilityRegistry()
	abilities.Register(&Ability{ID: "basic-strike", DisplayName: "Basic Strike", Type: AbilityActive, Category: AbilitySkill})
	prof := NewProficiencyManager(abilities, DefaultProficiencyConfig())

	name, ok := prof.Teach(context.Background(), "p-1", "basic-strike")
	if !ok || name != "Basic Strike" {
		t.Fatalf("Teach = (%q,%v), want (Basic Strike,true)", name, ok)
	}
	if !prof.Has("p-1", "basic-strike") {
		t.Errorf("Has(basic-strike) = false after Teach")
	}
	if v, _ := prof.Proficiency("p-1", "basic-strike"); v != 1 {
		t.Errorf("Proficiency after Teach = %d, want 1", v)
	}

	// Idempotent: a second Teach with existing proficiency must not
	// reset accumulated training progress.
	prof.AddProficiency("p-1", "basic-strike", 20)
	prof.Teach(context.Background(), "p-1", "basic-strike")
	if v, _ := prof.Proficiency("p-1", "basic-strike"); v != 21 {
		t.Errorf("Teach reset accumulated prof to %d, want 21", v)
	}

	// Unknown ability misses cleanly.
	if _, ok := prof.Teach(context.Background(), "p-1", "ghost"); ok {
		t.Errorf("Teach(unknown) ok=true")
	}
}
