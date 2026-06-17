package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/size"
)

// MobInstance.Stats builds an off-hand profile (two-weapon-fighting §2.3) when a
// melee main weapon is paired with an off-hand weapon that is LIGHT for the
// mob's own size — the mirror of the player producer, minus feats: the mob takes
// the full un-feated two-weapon penalty, the ½× off-hand Strength, and exactly
// one off-hand strike.
func TestMobStats_OffHandProfile(t *testing.T) {
	const str = 16 // STRBonus(16) = 3; ½× ⇒ floor(1.5)=1, delta = 1-3 = -2
	strBonus := combat.STRBonus(str)
	offDelta := size.StrBonusDelta(strBonus, size.DefaultOffHandStrFactor)

	spawn := func(t *testing.T) *MobInstance {
		t.Helper()
		s := NewStore()
		inst, err := s.SpawnMob(&mob.Template{
			ID: "test:duelist", Name: "a duelist", Type: "npc",
			Size: "medium", Stats: map[string]int{"str": str},
		})
		if err != nil {
			t.Fatalf("SpawnMob: %v", err)
		}
		return inst
	}

	t.Run("light off-hand grants the attack", func(t *testing.T) {
		inst := spawn(t)
		// Main weapon is MEDIUM (one-handed for a medium mob) so it earns no
		// two-handed Strength bonus — base.DamageBonus is the clean 1× Strength,
		// keeping the main-unchanged and ½×-off-hand assertions below honest.
		inst.SetWeapon(combat.DiceExpr{Count: 1, Sides: 8}, "a sword", nil, "", "", "medium")
		base := inst.Stats() // no off-hand yet — the un-penalized baseline
		inst.SetOffWeapon(combat.DiceExpr{Count: 1, Sides: 4}, "a dirk", []string{"piercing"}, "small")

		s := inst.Stats()
		if s.OffHand == nil {
			t.Fatal("a light off-hand weapon should grant the off-hand attack")
		}
		if want := base.HitMod - combat.DefaultTwoWeaponMainPenalty; s.HitMod != want {
			t.Errorf("main HitMod = %d, want %d (two-weapon main penalty)", s.HitMod, want)
		}
		if s.DamageBonus != base.DamageBonus {
			t.Errorf("main DamageBonus = %d, want %d (unchanged — full Strength)", s.DamageBonus, base.DamageBonus)
		}
		off := s.OffHand
		if want := base.HitMod - combat.DefaultTwoWeaponOffHandPenalty; off.HitMod != want {
			t.Errorf("off-hand HitMod = %d, want %d (larger off-hand penalty)", off.HitMod, want)
		}
		if off.WeaponName != "a dirk" || off.Damage.Sides != 4 {
			t.Errorf("off-hand weapon = %q %v, want a dirk 1d4", off.WeaponName, off.Damage)
		}
		if want := base.DamageBonus + offDelta; off.DamageBonus != want {
			t.Errorf("off-hand DamageBonus = %d, want %d (½× Strength)", off.DamageBonus, want)
		}
		if off.Attacks != 1 {
			t.Errorf("off-hand Attacks = %d, want 1 (mobs never take Improved Two-Weapon Fighting)", off.Attacks)
		}
		if len(off.WeaponDamageTypes) != 1 || off.WeaponDamageTypes[0] != "piercing" {
			t.Errorf("off-hand damage types = %v, want [piercing]", off.WeaponDamageTypes)
		}
	})

	t.Run("non-light off-hand grants nothing", func(t *testing.T) {
		inst := spawn(t)
		inst.SetWeapon(combat.DiceExpr{Count: 1, Sides: 8}, "a sword", nil, "", "", "medium")
		base := inst.Stats()
		// A medium weapon is one-handed (not light) for a medium mob.
		inst.SetOffWeapon(combat.DiceExpr{Count: 1, Sides: 6}, "a mace", nil, "medium")
		s := inst.Stats()
		if s.OffHand != nil {
			t.Error("a non-light off-hand weapon should grant no off-hand attack")
		}
		if s.HitMod != base.HitMod {
			t.Errorf("main HitMod = %d, want %d (no two-weapon penalty)", s.HitMod, base.HitMod)
		}
	})

	t.Run("ranged main suppresses off-hand", func(t *testing.T) {
		inst := spawn(t)
		inst.SetWeapon(combat.DiceExpr{Count: 1, Sides: 6}, "a bow", nil, "projectile", "arrow", "large")
		inst.SetOffWeapon(combat.DiceExpr{Count: 1, Sides: 4}, "a dirk", nil, "small")
		if s := inst.Stats(); s.OffHand != nil {
			t.Error("a ranged main weapon should suppress the off-hand attack")
		}
	})

	t.Run("no off-hand weapon ⇒ no profile", func(t *testing.T) {
		inst := spawn(t)
		inst.SetWeapon(combat.DiceExpr{Count: 1, Sides: 8}, "a sword", nil, "", "", "medium")
		if s := inst.Stats(); s.OffHand != nil {
			t.Error("a single-weapon mob should have no off-hand profile")
		}
	})
}

// EquipMobAtSpawn detects the off-hand weapon (two-weapon-fighting §2.3): a mob
// equipped with a main weapon (wield slot) and a light second weapon eligible
// for the off-hand slot dual-wields. The main weapon comes from the wield slot,
// the off-hand from the off-hand slot.
func TestEquipMobAtSpawn_DualWield(t *testing.T) {
	templates := func() *item.Templates {
		r := item.NewTemplates()
		r.Add(&item.Template{
			ID: "x:sword", Name: "a sword", Type: "item",
			EligibleSlots: []string{"wield"}, WeaponDamage: "1d8", // no size ⇒ medium ⇒ one-handed
		})
		r.Add(&item.Template{
			ID: "x:dagger", Name: "a dagger", Type: "item",
			EligibleSlots: []string{"wield", "offhand"}, WeaponDamage: "1d4", Size: "small", // light for a medium mob
		})
		r.Add(&item.Template{
			ID: "x:club", Name: "a club", Type: "item",
			EligibleSlots: []string{"wield", "offhand"}, WeaponDamage: "1d6", Size: "medium", // one-handed for a medium mob
		})
		return r
	}

	spawnRogue := func(t *testing.T) (*Store, *MobInstance) {
		t.Helper()
		s := NewStore()
		inst, err := s.SpawnMob(&mob.Template{
			ID: "test:rogue", Name: "a rogue", Type: "npc",
			Size: "medium", Stats: map[string]int{"str": 12},
		})
		if err != nil {
			t.Fatalf("SpawnMob: %v", err)
		}
		return s, inst
	}

	t.Run("light off-hand weapon arms the off hand", func(t *testing.T) {
		s, inst := spawnRogue(t)
		res, err := s.EquipMobAtSpawn(inst, []string{"x:sword", "x:dagger"}, templates(), NewContents(), mobEqSlots())
		if err != nil {
			t.Fatalf("EquipMobAtSpawn: %v", err)
		}
		if res.Equipped != 2 {
			t.Errorf("Equipped = %d, want 2", res.Equipped)
		}
		st := inst.Stats()
		if st.Damage.Sides != 8 {
			t.Errorf("main weapon = %v, want 1d8 (the wield-slot sword)", st.Damage)
		}
		if st.OffHand == nil || st.OffHand.WeaponName != "a dagger" || st.OffHand.Damage.Sides != 4 {
			t.Fatalf("off-hand = %v, want the off-hand-slot dagger 1d4", st.OffHand)
		}
	})

	t.Run("non-light second weapon is equipped but grants no off-hand attack", func(t *testing.T) {
		s, inst := spawnRogue(t)
		// The club lands in the off-hand slot but is one-handed for the mob, so
		// Stats grants no off-hand attack (it is still carried/equipped).
		if _, err := s.EquipMobAtSpawn(inst, []string{"x:sword", "x:club"}, templates(), NewContents(), mobEqSlots()); err != nil {
			t.Fatalf("EquipMobAtSpawn: %v", err)
		}
		if st := inst.Stats(); st.OffHand != nil {
			t.Error("a non-light off-hand weapon should grant no off-hand attack")
		}
	})
}
