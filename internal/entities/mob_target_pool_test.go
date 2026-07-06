package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// MobInstance.Stats threads an equipped weapon's target_pool into
// combat.Stats.TargetPool (shadowrun-mvp SR-M3b), so a stun-baton-wielding mob's
// damage routes to the target's Stun monitor. An ordinary weapon leaves it
// empty (the hp path).
func TestMobStats_TargetPool(t *testing.T) {
	spawn := func(t *testing.T) *MobInstance {
		t.Helper()
		s := NewStore()
		inst, err := s.SpawnMob(&mob.Template{
			ID: "test:enforcer", Name: "an enforcer", Type: "npc",
			Size: "medium", Stats: map[string]int{"str": 12},
		})
		if err != nil {
			t.Fatalf("SpawnMob: %v", err)
		}
		return inst
	}

	t.Run("equipped stun weapon → Stats.TargetPool", func(t *testing.T) {
		inst := spawn(t)
		inst.SetWeapon(combat.DiceExpr{Count: 1, Sides: 6}, "a stun baton", nil, "", "", "", "medium")
		inst.SetWeaponTargetPool("stun")
		if got := inst.Stats().TargetPool; got != "stun" {
			t.Errorf("Stats.TargetPool = %q, want stun", got)
		}
	})

	t.Run("ordinary weapon routes to hp (empty)", func(t *testing.T) {
		inst := spawn(t)
		inst.SetWeapon(combat.DiceExpr{Count: 1, Sides: 6}, "a club", nil, "", "", "", "medium")
		// No SetWeaponTargetPool — an ordinary weapon fills hp.
		if got := inst.Stats().TargetPool; got != "" {
			t.Errorf("Stats.TargetPool = %q, want empty (hp path)", got)
		}
	})
}
