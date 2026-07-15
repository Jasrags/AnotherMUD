package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// MobInstance carries its equipped weapon's bound skill id (skills §7 mob
// ratings) so the host's weapon-skill to-hit read can put a rated grunt on the
// weapon-skill model. A natural / non-skill-binding weapon leaves it empty (the
// binary always-proficient model).
func TestMobInstance_WieldedWeaponSkill(t *testing.T) {
	spawn := func(t *testing.T) *MobInstance {
		t.Helper()
		s := NewStore()
		inst, err := s.SpawnMob(&mob.Template{
			ID: "test:officer", Name: "an officer", Type: "npc",
			Size: "medium", Stats: map[string]int{"str": 12},
		})
		if err != nil {
			t.Fatalf("SpawnMob: %v", err)
		}
		return inst
	}

	t.Run("skill-bound weapon → the bound skill", func(t *testing.T) {
		inst := spawn(t)
		inst.SetWeapon(combat.DiceExpr{Count: 1, Sides: 6}, "an SMG", nil, "", "", "", "medium")
		inst.SetWeaponSkill("automatics")
		if got := inst.WieldedWeaponSkill(); got != "automatics" {
			t.Errorf("WieldedWeaponSkill = %q, want automatics", got)
		}
	})

	t.Run("natural / unbound weapon → empty (binary model)", func(t *testing.T) {
		inst := spawn(t)
		inst.SetWeapon(combat.DiceExpr{Count: 1, Sides: 6}, "a claw", nil, "", "", "", "medium")
		// No SetWeaponSkill — a natural weapon binds no skill.
		if got := inst.WieldedWeaponSkill(); got != "" {
			t.Errorf("WieldedWeaponSkill = %q, want empty", got)
		}
	})
}
