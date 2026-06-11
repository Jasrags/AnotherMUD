package progression

// Skill checks — EPIC sub-epic S3, spec docs/specs/skills.md §3. A skill is a
// use-based proficiency keyed by an ability id (character-model D3); this file
// is the resolution primitive every skill consumer (lockpicking, and later
// visibility/locks/climb) calls. It mirrors the saves primitive (saves.go):
// `d20 + bonus vs DC`, with the same natural-1/20 edges, so the engine has one
// check idiom across to-hit, saves, and skills.

// SkillConfig holds the magnitudes a skill check needs (skills §7). One config
// governs every skill; per-skill difficulty lives on the thing being checked
// (a door's pick difficulty, etc.).
type SkillConfig struct {
	// ProficiencyBonusScale maps a 1–100 proficiency onto the d20 skill-bonus
	// scale (the WoT "ranks" term, sourced from use-based proficiency rather
	// than point-buy). bonus contribution = floor(proficiency * scale), so a
	// novice (low proficiency) contributes ~nothing and a master a large
	// bonus.
	ProficiencyBonusScale float64
}

// DefaultSkillConfig returns the engine-default skill magnitudes: a master
// (proficiency 100) contributes +20 to the check, a novice ~0. Packs may tune.
func DefaultSkillConfig() SkillConfig {
	return SkillConfig{ProficiencyBonusScale: 0.2}
}

// SkillBonus composes a skill-check bonus (skills §3): the proficiency-derived
// term plus the governing ability's d20 modifier. proficiency is the 1–100
// value from the proficiency manager (0 for untrained); statScore is the
// governing ability's effective score, run through the same AbilityModifier
// `(score-10)/2` the saves use, so a stat buff helps the skill for free.
func SkillBonus(proficiency, statScore int, cfg SkillConfig) int {
	return int(float64(proficiency)*cfg.ProficiencyBonusScale) + AbilityModifier(statScore)
}

// SkillOutcome is the result of one resolved skill check (skills §3). It
// carries the full roll detail — not just the boolean — so a consumer can
// render the math and a future degrees-of-success consumer can read the margin
// (Total vs DC). Mirrors SaveOutcome.
type SkillOutcome struct {
	Roll      int // the raw d20 face (1..20)
	Bonus     int // the character's skill bonus for this check
	Total     int // Roll + Bonus
	DC        int // the difficulty checked against
	Success   bool
	Natural1  bool
	Natural20 bool
}

// ResolveSkillCheck rolls one skill check: d20 + bonus vs dc (skills §3). A
// natural 1 always fails and a natural 20 always succeeds regardless of bonus
// or DC — the same edges the to-hit roll and saves use — otherwise success is
// `Total >= DC`. Pure over the injected Roller, so it is deterministic under a
// seeded roller and carries no global state.
func ResolveSkillCheck(r Roller, bonus, dc int) SkillOutcome {
	roll := r.IntN(20) + 1
	out := SkillOutcome{Roll: roll, Bonus: bonus, Total: roll + bonus, DC: dc}
	switch roll {
	case 1:
		out.Natural1 = true
		out.Success = false
	case 20:
		out.Natural20 = true
		out.Success = true
	default:
		out.Success = out.Total >= dc
	}
	return out
}
