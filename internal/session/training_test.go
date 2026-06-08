package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// Two trainers share a room: a combat master (slash/parry) and a craft
// trainer (smithing). selectTrainer must pick the one that can teach the
// requested ability regardless of order — the bug live-testing surfaced
// (Brandr was shadowed by Maerys in the forge).
func TestSelectTrainer_PrefersCanTeach(t *testing.T) {
	combat := trainerCandidate{
		cfg:  &progression.TrainerConfig{Tier: progression.CapNovice, Teach: []string{"slash", "parry"}},
		name: "Maerys",
	}
	smith := trainerCandidate{
		cfg:  &progression.TrainerConfig{Tier: progression.CapApprentice, Teach: []string{"smithing"}},
		name: "Brandr",
	}

	// Combat master listed FIRST — smithing must still resolve to Brandr.
	cfg, name, ok := selectTrainer([]trainerCandidate{combat, smith}, "smithing")
	if !ok || name != "Brandr" {
		t.Errorf("smithing → (%q, %v), want Brandr", name, ok)
	}
	if cfg == nil || !cfg.CanTeach("smithing") {
		t.Error("returned trainer can't teach smithing")
	}

	// And the other way: slash resolves to Maerys even though she's first.
	if _, name, ok := selectTrainer([]trainerCandidate{combat, smith}, "slash"); !ok || name != "Maerys" {
		t.Errorf("slash → (%q, %v), want Maerys", name, ok)
	}
}

func TestSelectTrainer_FallbackAndEmpty(t *testing.T) {
	combat := trainerCandidate{
		cfg:  &progression.TrainerConfig{Tier: progression.CapNovice, Teach: []string{"slash"}},
		name: "Maerys",
	}
	// A trainer is present but none teach "cooking": return one present (ok)
	// so the caller's CanTeach check renders "cannot teach", not "no one here".
	cfg, name, ok := selectTrainer([]trainerCandidate{combat}, "cooking")
	if !ok || name != "Maerys" || cfg.CanTeach("cooking") {
		t.Errorf("present-but-cant-teach → (%q, %v, canTeach=%v), want (Maerys, true, false)", name, ok, cfg.CanTeach("cooking"))
	}
	// No trainer at all → not found.
	if _, _, ok := selectTrainer(nil, "smithing"); ok {
		t.Error("empty room should report no trainer")
	}
}
