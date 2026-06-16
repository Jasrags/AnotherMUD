package command_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/grade"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// gradeLadderWithArmor is the grade ladder with armor-check-improve set
// (masterwork reduces a penalty by 1, power-wrought by 3).
func gradeLadderWithArmor() *grade.Registry {
	r := grade.NewRegistry()
	r.Register(grade.Grade{Key: "masterwork", Order: 1, WeaponToHit: 1, ArmorCheckImprove: 1})
	r.Register(grade.Grade{Key: "power-wrought", Order: 3, WeaponToHit: 3, WeaponDamage: 3, ArmorCheckImprove: 3})
	return r
}

// capTpl is a head armor with a check penalty (armor-depth §6), optionally
// graded.
func armorCapTpl(penalty int, g string) *item.Template {
	return &item.Template{
		ID: "x:cap", Name: "a cap", Type: "armor",
		Keywords:          []string{"cap"},
		EligibleSlots:     []string{"head"},
		ArmorCheckPenalty: penalty,
		Grade:             g,
	}
}

// armorCheckMod returns the armor_check modifier value applied for inst, or
// -1 if none was applied.
func armorCheckMod(a *testActor, id entities.EntityID) int {
	for _, m := range a.mods[entities.EquipmentSourceKey(id)] {
		if m.Stat == "armor_check" {
			return m.Value
		}
	}
	return -1
}

// A worn armor's check penalty is applied as an armor_check stat modifier
// (armor-depth §6) — the value a Str/Dex skill check subtracts.
func TestEquip_ArmorCheckPenaltyAppliesStat(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, armorCapTpl(3, ""), a)

	dispatch(t, newRegistry(t), f.env(), a, "equip cap head")

	if got := armorCheckMod(a, inst.ID()); got != 3 {
		t.Fatalf("armor_check modifier = %d; want 3", got)
	}
}

// A quality grade improves (reduces) the armor check penalty (masterwork §3):
// masterwork (improve 1) turns a penalty of 3 into 2.
func TestEquip_GradedArmorReducesCheckPenalty(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, armorCapTpl(3, "masterwork"), a)
	env := f.env()
	env.Grades = gradeLadderWithArmor()

	dispatch(t, newRegistry(t), env, a, "equip cap head")

	if got := armorCheckMod(a, inst.ID()); got != 2 {
		t.Fatalf("masterwork armor_check = %d; want 2 (3 - 1 improve)", got)
	}
}

// The improved penalty floors at zero: a grade improvement larger than the
// penalty applies no armor_check modifier at all.
func TestEquip_GradedArmorCheckFloorsAtZero(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, armorCapTpl(1, "power-wrought"), a) // improve 3 > penalty 1
	env := f.env()
	env.Grades = gradeLadderWithArmor()

	dispatch(t, newRegistry(t), env, a, "equip cap head")

	if got := armorCheckMod(a, inst.ID()); got != -1 {
		t.Fatalf("over-improved armor should apply no armor_check modifier, got %d", got)
	}
}

// An armor with no check penalty applies no armor_check modifier.
func TestEquip_NoCheckPenaltyNoStat(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, armorCapTpl(0, ""), a)

	dispatch(t, newRegistry(t), f.env(), a, "equip cap head")

	if got := armorCheckMod(a, inst.ID()); got != -1 {
		t.Fatalf("penalty-free armor should apply no armor_check modifier, got %d", got)
	}
}
