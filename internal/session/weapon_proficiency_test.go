package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// IsWeaponProficient resolves the actor's class live by id (so a SetClass
// change is reflected without touching the lock-free a.class pointer) and
// runs the weapon-identity §3 check against the wielded-weapon snapshot.
func TestIsWeaponProficient(t *testing.T) {
	reg := progression.NewClassRegistry()
	if err := reg.Register(&progression.Class{ID: "armsman", ProficiencyTiers: []string{"simple", "martial"}}); err != nil {
		t.Fatalf("register armsman: %v", err)
	}
	if err := reg.Register(&progression.Class{ID: "woodsman", ProficiencyCategories: []string{"two-rivers-longbow"}}); err != nil {
		t.Fatalf("register woodsman: %v", err)
	}

	tests := []struct {
		name    string
		classID string
		weapon  *weaponInfo
		want    bool
	}{
		{"unarmed is always proficient", "armsman", nil, true},
		{"granted tier is proficient", "armsman", &weaponInfo{tier: "martial", category: "battleaxe"}, true},
		{"untiered weapon is proficient", "armsman", &weaponInfo{tier: "", category: "x"}, true},
		{"lowest tier is proficient", "armsman", &weaponInfo{tier: "simple", category: "club"}, true},
		{"ungranted tier is not proficient", "armsman", &weaponInfo{tier: "exotic", category: "ashandarei"}, false},
		{"granted category out of tier is proficient", "woodsman", &weaponInfo{tier: "exotic", category: "two-rivers-longbow"}, true},
		{"no class: martial weapon not proficient", "", &weaponInfo{tier: "martial", category: "x"}, false},
		{"no class: simple weapon still proficient", "", &weaponInfo{tier: "simple", category: "x"}, true},
	}
	for _, tt := range tests {
		var classIDs []string
		if tt.classID != "" {
			classIDs = []string{tt.classID}
		}
		a := &connActor{classes: reg, classIDs: classIDs}
		if tt.weapon != nil {
			a.weapon.Store(tt.weapon)
		}
		if got := a.IsWeaponProficient(); got != tt.want {
			t.Errorf("%s: IsWeaponProficient() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
