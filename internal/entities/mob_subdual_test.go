package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// MobInstance.Stats threads an equipped weapon's nonlethal flag into
// combat.Stats.Subdual (subdual-damage §2), so a sap/whip-wielding mob's
// finishing blow knocks a player out. A natural weapon stays lethal (the
// player/mob-natural distinction) — the default.
func TestMobStats_Subdual(t *testing.T) {
	spawn := func(t *testing.T) *MobInstance {
		t.Helper()
		s := NewStore()
		inst, err := s.SpawnMob(&mob.Template{
			ID: "test:thug", Name: "a thug", Type: "npc",
			Size: "medium", Stats: map[string]int{"str": 12},
		})
		if err != nil {
			t.Fatalf("SpawnMob: %v", err)
		}
		return inst
	}

	t.Run("equipped subdual weapon → Stats.Subdual", func(t *testing.T) {
		inst := spawn(t)
		inst.SetWeapon(combat.DiceExpr{Count: 1, Sides: 6}, "a sap", nil, "", "", "", "small")
		inst.SetWeaponSubdual(true)
		if !inst.Stats().Subdual {
			t.Error("a mob wielding a subdual weapon should report Stats.Subdual=true")
		}
	})

	t.Run("natural weapon stays lethal", func(t *testing.T) {
		inst := spawn(t)
		inst.SetWeapon(combat.DiceExpr{Count: 1, Sides: 4}, "claws", nil, "", "", "", "")
		// No SetWeaponSubdual — a bite/claw is lethal.
		if inst.Stats().Subdual {
			t.Error("a natural weapon must stay lethal (Subdual=false)")
		}
	})
}
