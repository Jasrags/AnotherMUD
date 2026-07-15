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

func TestResolveStealthSkills(t *testing.T) {
	// Unset / unlisted world → the two-axis engine default.
	if h, s := resolveStealthSkills(nil, "wot"); h != skillAbilityHide || s != skillAbilityMoveSilently {
		t.Errorf("nil selection = (%q, %q), want the hide/move-silently default", h, s)
	}
	sel := map[string]string{"shadowrun": "sneaking"}
	if h, s := resolveStealthSkills(sel, "starter-world"); h != skillAbilityHide || s != skillAbilityMoveSilently {
		t.Errorf("unlisted world = (%q, %q), want the default", h, s)
	}
	// A world that declares a merged stealth skill → BOTH axes read it.
	if h, s := resolveStealthSkills(sel, "shadowrun"); h != "sneaking" || s != "sneaking" {
		t.Errorf("shadowrun = (%q, %q), want both = sneaking", h, s)
	}
}

// TestVisibility_MergedStealthSkillFolds — skills §2 Slice C: when the character's
// world binds both concealment axes to one skill (SR `sneaking`), that single
// proficiency folds into BOTH HideScore and SneakDifficulty, and the core
// hide/move-silently skills are NOT read (they'd be inert redundant entries).
func TestVisibility_MergedStealthSkillFolds(t *testing.T) {
	a := newFeatActor(t, 0)
	a.hideSkill, a.sneakSkill = "sneaking", "sneaking" // as resolveStealthSkills would set for an SR world
	baseHide, baseSneak := a.HideScore(), a.SneakDifficulty()

	mgr := progression.NewProficiencyManager(progression.NewAbilityRegistry(), progression.ProficiencyConfig{DefaultLearnCap: 100})
	a.prof = mgr
	// Train the MERGED skill only — not the core axes.
	mgr.Learn(a.playerID, "sneaking", 50)

	const want = 10 // floor(50 * 0.2)
	if got := a.HideScore(); got != baseHide+want {
		t.Errorf("HideScore = %d, want %d (+%d from merged sneaking 50)", got, baseHide+want, want)
	}
	if got := a.SneakDifficulty(); got != baseSneak+want {
		t.Errorf("SneakDifficulty = %d, want %d (+%d from merged sneaking 50)", got, baseSneak+want, want)
	}

	// Training the core axes must NOT move an SR character's stealth (they're unread).
	mgr.Learn(a.playerID, skillAbilityHide, 90)
	mgr.Learn(a.playerID, skillAbilityMoveSilently, 90)
	if got := a.HideScore(); got != baseHide+want {
		t.Errorf("HideScore moved on core hide training = %d, want unchanged %d", got, baseHide+want)
	}
	if got := a.SneakDifficulty(); got != baseSneak+want {
		t.Errorf("SneakDifficulty moved on core move-silently training = %d, want unchanged %d", got, baseSneak+want)
	}
}
