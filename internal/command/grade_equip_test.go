package command_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/grade"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// gradedBladeTpl is a clean weapon (no hand-written modifiers) so a grade's
// to-hit bonus is the ONLY modifier applied — the bonus is the grade's.
func gradedBladeTpl(g string) *item.Template {
	return &item.Template{
		ID:            "x:blade",
		Name:          "a blade",
		Type:          "weapon",
		Keywords:      []string{"blade"},
		EligibleSlots: []string{"wield"},
		WeaponDamage:  "1d8",
		Grade:         g,
	}
}

func gradeLadder() *grade.Registry {
	r := grade.NewRegistry()
	r.Register(grade.Grade{Key: "masterwork", Order: 1, WeaponToHit: 1})
	r.Register(grade.Grade{Key: "power-wrought", Order: 3, WeaponToHit: 3, WeaponDamage: 3})
	return r
}

// A masterwork weapon adds its grade's to-hit bonus as a hit_mod modifier
// when wielded (masterwork §3).
func TestEquip_MasterworkWeaponAddsToHit(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, gradedBladeTpl("masterwork"), a)
	env := f.env()
	env.Grades = gradeLadder()

	dispatch(t, newRegistry(t), env, a, "equip blade wield")

	mods := a.mods[entities.EquipmentSourceKey(inst.ID())]
	if len(mods) != 1 || mods[0].Stat != "hit_mod" || mods[0].Value != 1 {
		t.Fatalf("masterwork weapon mods = %+v; want one {hit_mod 1}", mods)
	}
}

// An ungraded weapon (same template, no grade) gets no grade bonus.
func TestEquip_UngradedWeaponNoGradeBonus(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, gradedBladeTpl(""), a)
	env := f.env()
	env.Grades = gradeLadder()

	dispatch(t, newRegistry(t), env, a, "equip blade wield")

	if mods := a.mods[entities.EquipmentSourceKey(inst.ID())]; len(mods) != 0 {
		t.Fatalf("ungraded weapon should apply no grade modifier, got %+v", mods)
	}
}

// A power-wrought weapon adds BOTH to-hit (hit_mod) and flat damage
// (damage_mod) — masterwork §3. masterwork adds to-hit only.
func TestEquip_PowerWroughtWeaponAddsToHitAndDamage(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, gradedBladeTpl("power-wrought"), a)
	env := f.env()
	env.Grades = gradeLadder()

	dispatch(t, newRegistry(t), env, a, "equip blade wield")

	mods := a.mods[entities.EquipmentSourceKey(inst.ID())]
	hit, dmg := 0, 0
	for _, m := range mods {
		switch m.Stat {
		case "hit_mod":
			hit = m.Value
		case "damage_mod":
			dmg = m.Value
		default:
			t.Errorf("unexpected grade modifier %+v", m)
		}
	}
	if hit != 3 || dmg != 3 {
		t.Fatalf("power-wrought mods = %+v; want hit_mod 3 + damage_mod 3", mods)
	}
}

// A masterwork weapon (no weapon_damage grade step) adds to-hit but NOT
// damage — only power-wrought buffs damage.
func TestEquip_MasterworkWeaponNoDamageMod(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, gradedBladeTpl("masterwork"), a)
	env := f.env()
	env.Grades = gradeLadder()

	dispatch(t, newRegistry(t), env, a, "equip blade wield")

	for _, m := range a.mods[entities.EquipmentSourceKey(inst.ID())] {
		if m.Stat == "damage_mod" {
			t.Fatalf("masterwork weapon must not add damage_mod, got %+v", m)
		}
	}
}

// A graded item that is NOT a weapon gets no weapon to-hit bonus (this slice
// only wires the weapon kind).
func TestEquip_GradedNonWeaponNoToHit(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	// A graded "armor" item (no weapon_damage) — armor grade bonuses are a
	// later increment, so equipping it applies no hit_mod.
	tpl := &item.Template{
		ID: "x:cap", Name: "a cap", Type: "armor",
		Keywords: []string{"cap"}, EligibleSlots: []string{"head"}, Grade: "masterwork",
	}
	inst := f.spawnInInventory(t, tpl, a)
	env := f.env()
	env.Grades = gradeLadder()

	dispatch(t, newRegistry(t), env, a, "equip cap head")

	if mods := a.mods[entities.EquipmentSourceKey(inst.ID())]; len(mods) != 0 {
		t.Fatalf("graded non-weapon should get no to-hit bonus, got %+v", mods)
	}
}
