package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// A weapon's bound skill (skills §7) flows from the template onto the instance,
// so the to-hit hook can read it; a weapon that binds none reports "".
func TestItemInstance_WeaponSkill(t *testing.T) {
	s := NewStore()
	bound, err := s.Spawn(&item.Template{
		ID: "sr:predator", Name: "a pistol", Type: "weapon",
		WeaponCategory: "pistol", ProficiencyTier: "martial", WeaponSkill: "pistols",
	})
	if err != nil {
		t.Fatalf("spawn bound weapon: %v", err)
	}
	if got := bound.WeaponSkill(); got != "pistols" {
		t.Errorf("WeaponSkill() = %q, want pistols", got)
	}

	unbound, err := s.Spawn(&item.Template{
		ID: "sr:club", Name: "a club", Type: "weapon", WeaponCategory: "club",
	})
	if err != nil {
		t.Fatalf("spawn unbound weapon: %v", err)
	}
	if got := unbound.WeaponSkill(); got != "" {
		t.Errorf("WeaponSkill() = %q, want empty (binary model)", got)
	}
}
