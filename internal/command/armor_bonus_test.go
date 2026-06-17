package command_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// armorBonusTpl is a head armor declaring a structured armor bonus
// (armor-depth §3), optionally with a check penalty.
func armorBonusTpl(bonus, penalty int) *item.Template {
	return &item.Template{
		ID: "x:helm", Name: "a helm", Type: "armor",
		Keywords:          []string{"helm"},
		EligibleSlots:     []string{"head"},
		ArmorBonus:        bonus,
		ArmorCheckPenalty: penalty,
	}
}

// acMod returns the `ac` stat modifier value applied for inst under its
// equipment source, or -1 if none was applied.
func acMod(a *testActor, id entities.EntityID) int {
	for _, m := range a.mods[entities.EquipmentSourceKey(id)] {
		if m.Stat == "ac" {
			return m.Value
		}
	}
	return -1
}

// A worn armor's structured armor bonus is applied as an `ac` stat modifier
// (armor-depth §3) — the term the defense channel reads to make armor harder
// to hit, in every ruleset.
func TestEquip_ArmorBonusAppliesACStat(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, armorBonusTpl(4, 0), a)

	dispatch(t, newRegistry(t), f.env(), a, "equip helm head")

	if got := acMod(a, inst.ID()); got != 4 {
		t.Fatalf("ac modifier from armor_bonus = %d; want 4", got)
	}
}

// Armor declaring no armor bonus applies no `ac` modifier (so a pure to-hit
// armor or a resistance-only piece does not silently bump AC).
func TestEquip_NoArmorBonusNoACStat(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, armorBonusTpl(0, 2), a) // has a check penalty, no AC bonus

	dispatch(t, newRegistry(t), f.env(), a, "equip helm head")

	if got := acMod(a, inst.ID()); got != -1 {
		t.Fatalf("bonus-free armor should apply no ac modifier, got %d", got)
	}
}
