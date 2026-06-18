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

// The recorded-only equipment-depth metadata round-trips to the instance
// accessors (special-weapons §2 — inert, but carried for the authoring pass).
func TestItemInstance_EquipmentDepthMetadata(t *testing.T) {
	s := NewStore()
	staff, err := s.Spawn(&item.Template{
		ID: "core:quarterstaff", Name: "a quarterstaff", Type: "weapon",
		WeaponDamage: "1d6", DoubleDamage: "1d8",
		Subdual: true, ArmorSpeed: 20, Reputation: -2,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !staff.Subdual() {
		t.Error("Subdual() = false, want true")
	}
	if dd, ok := staff.DoubleDamage(); !ok || dd.Sides != 8 {
		t.Errorf("DoubleDamage() = (%v,%v), want a 1d8", dd, ok)
	}
	if staff.ArmorSpeed() != 20 {
		t.Errorf("ArmorSpeed() = %d, want 20", staff.ArmorSpeed())
	}
	if staff.Reputation() != -2 {
		t.Errorf("Reputation() = %d, want -2", staff.Reputation())
	}

	plain, _ := s.Spawn(&item.Template{ID: "core:rock", Name: "a rock", Type: "item"})
	if _, ok := plain.DoubleDamage(); ok {
		t.Error("a non-double item should report DoubleDamage ok=false")
	}
	if plain.Subdual() || plain.ArmorSpeed() != 0 || plain.Reputation() != 0 {
		t.Error("an ordinary item carries no equipment-depth metadata")
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
