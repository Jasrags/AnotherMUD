package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// A weapon_proficiency feat grant (Militia — feats Bucket B) makes a weapon
// whose category the feat names proficient, even when no class grants it. The
// union is additive: with no such feat the result is unchanged (the regression
// lock for the non-proficient path).
func TestIsWeaponProficient_FeatGrantedCategory(t *testing.T) {
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	// No class proficiencies — isolate the feat path.
	a.mu.Lock()
	a.classes = nil
	a.classIDs = nil
	a.mu.Unlock()

	// A martial-tier pike: not the lowest tier, so a bare actor is non-proficient.
	a.weapon.Store(&weaponInfo{category: "pike", tier: "martial"})
	if a.IsWeaponProficient() {
		t.Fatal("a martial pike should be non-proficient without a class grant or feat")
	}

	// Militia's cached weapon_proficiency category makes the pike proficient.
	a.featWeaponBonus.Store(&featWeaponBonuses{weaponProficiencyCategories: []string{"pike"}})
	if !a.IsWeaponProficient() {
		t.Error("a feat granting pike proficiency should make the pike proficient")
	}

	// A different granted category does NOT cover the pike (no over-grant).
	a.featWeaponBonus.Store(&featWeaponBonuses{weaponProficiencyCategories: []string{"light-crossbow"}})
	if a.IsWeaponProficient() {
		t.Error("a crossbow grant must not make the pike proficient")
	}
}
