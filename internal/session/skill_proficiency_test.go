package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// Skill proficiency folds into the visibility checks via SkillBonus
// (floor(proficiency * scale) + the governing-stat modifier): a trained hider /
// sneak is harder to detect, and a trained observer pierces concealment more
// often. The complementary regression — proficiency 0 reproducing the bare
// stat-modifier behavior — is locked by TestPerceptionAndStealth_FoldFeatBonus.
func TestVisibility_FoldSkillProficiency(t *testing.T) {
	a := newFeatActor(t, 0)
	// Baselines computed with no proficiency manager wired (skillProficiency → 0),
	// i.e. the pre-skill values.
	baseHide, baseSneak, basePer := a.HideScore(), a.SneakDifficulty(), a.PerceptionBonus()

	mgr := progression.NewProficiencyManager(progression.NewAbilityRegistry(), progression.ProficiencyConfig{DefaultLearnCap: 100})
	a.prof = mgr
	mgr.Learn(a.playerID, skillAbilityHide, 50)
	mgr.Learn(a.playerID, skillAbilityMoveSilently, 50)
	mgr.Learn(a.playerID, skillAbilityPerception, 50)

	// floor(50 * DefaultSkillConfig scale 0.2) = +10 folded into each.
	const want = 10
	if got := a.HideScore(); got != baseHide+want {
		t.Errorf("HideScore = %d, want %d (+%d from Hide proficiency 50)", got, baseHide+want, want)
	}
	if got := a.SneakDifficulty(); got != baseSneak+want {
		t.Errorf("SneakDifficulty = %d, want %d (+%d from Move Silently 50)", got, baseSneak+want, want)
	}
	if got := a.PerceptionBonus(); got != basePer+want {
		t.Errorf("PerceptionBonus = %d, want %d (+%d from Perception 50)", got, basePer+want, want)
	}
}
