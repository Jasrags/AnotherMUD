package condition

import "testing"

func TestResolve_Empty(t *testing.T) {
	if got := Resolve(nil, DefaultConfig()); (got != Impact{}) {
		t.Errorf("Resolve(nil) = %+v, want zero", got)
	}
}

func TestResolve_Fatigued_NoCombatHookImpact(t *testing.T) {
	// Fatigued is pure stat modifiers — it contributes nothing to the combat
	// hooks (its −Str/−Dex live on the effect's own modifier list).
	if got := Resolve([]string{FlagFatigued}, DefaultConfig()); (got != Impact{}) {
		t.Errorf("fatigued Impact = %+v, want zero", got)
	}
}

func TestResolve_Prone(t *testing.T) {
	cfg := DefaultConfig()
	got := Resolve([]string{FlagProne}, cfg)
	if got.AttackerHitPenalty != cfg.ProneAttackPenalty || got.DefenderVulnerability != cfg.ProneVulnerability {
		t.Errorf("prone = %+v", got)
	}
	if got.Incapacitated || got.ForcesFlee || got.SavePenalty != 0 {
		t.Errorf("prone set unexpected fields: %+v", got)
	}
}

func TestResolve_Stunned(t *testing.T) {
	cfg := DefaultConfig()
	got := Resolve([]string{FlagStunned}, cfg)
	if !got.Incapacitated {
		t.Error("stunned must incapacitate")
	}
	if got.DefenderVulnerability != cfg.StunnedVulnerability {
		t.Errorf("stunned vulnerability = %d, want %d", got.DefenderVulnerability, cfg.StunnedVulnerability)
	}
	if got.AttackerHitPenalty != 0 {
		t.Errorf("stunned should not carry an attacker hit penalty (it skips swings): %+v", got)
	}
}

func TestResolve_Frightened(t *testing.T) {
	cfg := DefaultConfig()
	got := Resolve([]string{FlagFrightened}, cfg)
	if got.AttackerHitPenalty != cfg.FearPenalty || got.SavePenalty != cfg.FearPenalty {
		t.Errorf("frightened should penalize attack AND saves: %+v", got)
	}
	if !got.ForcesFlee {
		t.Error("frightened must force flee")
	}
}

func TestResolve_Blinded(t *testing.T) {
	cfg := DefaultConfig()
	got := Resolve([]string{FlagBlinded}, cfg)
	if got.AttackerHitPenalty != cfg.BlindedAttackPenalty || got.DefenderVulnerability != cfg.BlindedVulnerability {
		t.Errorf("blinded = %+v", got)
	}
}

func TestResolve_Compose_ProneAndFrightened(t *testing.T) {
	cfg := DefaultConfig()
	got := Resolve([]string{FlagProne, FlagFrightened}, cfg)
	// Attacker penalty sums prone + fear; vulnerability from prone; save
	// penalty from fear; flee from fear.
	if got.AttackerHitPenalty != cfg.ProneAttackPenalty+cfg.FearPenalty {
		t.Errorf("summed attacker penalty = %d, want %d", got.AttackerHitPenalty, cfg.ProneAttackPenalty+cfg.FearPenalty)
	}
	if got.DefenderVulnerability != cfg.ProneVulnerability {
		t.Errorf("vulnerability = %d, want %d", got.DefenderVulnerability, cfg.ProneVulnerability)
	}
	if got.SavePenalty != cfg.FearPenalty || !got.ForcesFlee {
		t.Errorf("compose lost fear effects: %+v", got)
	}
}

func TestResolve_StunnedAndProne_VulnerabilityStacks(t *testing.T) {
	cfg := DefaultConfig()
	got := Resolve([]string{FlagStunned, FlagProne}, cfg)
	if !got.Incapacitated {
		t.Error("stun+prone must still incapacitate")
	}
	if got.DefenderVulnerability != cfg.StunnedVulnerability+cfg.ProneVulnerability {
		t.Errorf("vulnerability should stack: %d", got.DefenderVulnerability)
	}
}

func TestResolve_Unconscious(t *testing.T) {
	cfg := DefaultConfig()
	got := Resolve([]string{FlagUnconscious}, cfg)
	if !got.Incapacitated {
		t.Error("unconscious must incapacitate (lands no swings)")
	}
	if got.DefenderVulnerability != cfg.UnconsciousVulnerability {
		t.Errorf("unconscious vulnerability = %d, want %d", got.DefenderVulnerability, cfg.UnconsciousVulnerability)
	}
	// Helpless is the heaviest Core vulnerability — STRICTLY more exposed than a
	// stunned or prone foe (subdual-damage §3: "stronger than prone/stunned").
	if cfg.UnconsciousVulnerability <= cfg.StunnedVulnerability || cfg.UnconsciousVulnerability <= cfg.ProneVulnerability {
		t.Errorf("unconscious (%d) should be the heaviest vulnerability, > stunned (%d)/prone (%d)",
			cfg.UnconsciousVulnerability, cfg.StunnedVulnerability, cfg.ProneVulnerability)
	}
	// No shake-off / morale axis — the wake is duration-only (subdual-damage §5),
	// and an unconscious attacker skips swings rather than swinging at a penalty.
	if got.SavePenalty != 0 || got.AttackerHitPenalty != 0 || got.ForcesFlee {
		t.Errorf("unconscious set unexpected fields: %+v", got)
	}
}

func TestUnconscious_IsRecognizedConditionFlag(t *testing.T) {
	// It must be a `condition:` flag so `cure` clears it and `afflict` applies it
	// through the conditions §D inflict path (subdual-damage §3).
	if !IsConditionFlag(FlagUnconscious) {
		t.Errorf("%q is not recognized as a condition flag", FlagUnconscious)
	}
	found := false
	for _, f := range Flags() {
		if f == FlagUnconscious {
			found = true
		}
	}
	if !found {
		t.Errorf("FlagUnconscious missing from Flags() — `cure` would not clear it")
	}
}

func TestResolve_UnrecognizedFlagInert(t *testing.T) {
	if got := Resolve([]string{"blessed", "well-fed", "condition:unknown"}, DefaultConfig()); (got != Impact{}) {
		t.Errorf("non-condition flags must be inert: %+v", got)
	}
}
