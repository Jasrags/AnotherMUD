package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// An ItemInstance surfaces its special-maneuver tags, maneuver bonuses, and the
// numeric reach rating to the combat path (special-weapons.md §2/§3, increment J).
func TestItemInstance_SpecialWeapon(t *testing.T) {
	s := NewStore()
	bill, err := s.Spawn(&item.Template{
		ID: "core:bill", Name: "a bill", Type: "weapon",
		WeaponDamage: "2d4",
		Special:      []string{item.SpecialTrip},
		TripBonus:    2,
		Reach:        1,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	it := bill

	if !it.HasSpecial(item.SpecialTrip) {
		t.Error("bill should carry the trip maneuver tag")
	}
	if it.HasSpecial(item.SpecialDisarm) {
		t.Error("bill should NOT carry disarm")
	}
	if it.TripBonus() != 2 {
		t.Errorf("TripBonus() = %d, want 2", it.TripBonus())
	}
	if it.DisarmBonus() != 0 {
		t.Errorf("DisarmBonus() = %d, want 0 (no disarm tag/bonus)", it.DisarmBonus())
	}
	if it.Reach() != 1 {
		t.Errorf("Reach() = %d, want 1 (a reach polearm)", it.Reach())
	}
}

func TestItemInstance_OrdinaryWeaponHasNoSpecial(t *testing.T) {
	s := NewStore()
	sword, _ := s.Spawn(&item.Template{
		ID: "core:sword", Name: "a sword", Type: "weapon", WeaponDamage: "1d8",
	})
	it := sword
	if it.HasSpecial(item.SpecialTrip) || it.HasSpecial(item.SpecialDisarm) {
		t.Error("a plain sword should carry no special-maneuver tags")
	}
	if it.Reach() != 0 {
		t.Errorf("Reach() = %d, want 0 (an ordinary close weapon)", it.Reach())
	}
}
