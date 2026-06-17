package command_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// sizedWeaponTpl is a wield-slot weapon declaring a size (size-and-wielding §2)
// and no static companion slots — so its footprint is purely size-derived.
func sizedWeaponTpl(weaponSize string) *item.Template {
	return &item.Template{
		ID: "x:blade", Name: "a blade", Type: "weapon",
		Keywords:      []string{"blade"},
		EligibleSlots: []string{"wield"},
		WeaponDamage:  "1d6",
		Size:          weaponSize,
	}
}

// A size-derived two-handed weapon (one step larger than the baseline-size
// wielder) ties up BOTH hands; a one-handed/light one leaves the off hand free
// (size-and-wielding §4.1). The test actor exposes no size, so it is baseline
// (medium): a "large" blade is two-handed, "medium" is one-handed, "small" is
// light.
func TestEquip_SizeDerivedFootprint(t *testing.T) {
	tests := []struct {
		name        string
		weaponSize  string
		wantOffhand bool
	}{
		{"large weapon is two-handed (off hand tied up)", "large", true},
		{"medium weapon is one-handed (off hand free)", "medium", false},
		{"small weapon is light (off hand free)", "small", false},
	}
	for _, tt := range tests {
		f := newEqFixture(t)
		a := newTestActor(f.room)
		f.spawnInInventory(t, sizedWeaponTpl(tt.weaponSize), a)

		dispatch(t, newRegistry(t), f.env(), a, "equip blade")

		eq := a.Equipment()
		if _, wielded := eq["wield"]; !wielded {
			t.Fatalf("%s: weapon not in wield slot; equipment=%v", tt.name, eq)
		}
		_, offhandOccupied := eq["offhand"]
		if offhandOccupied != tt.wantOffhand {
			t.Errorf("%s: off-hand occupied = %v, want %v (equipment=%v)",
				tt.name, offhandOccupied, tt.wantOffhand, eq)
		}
	}
}

// A weapon two or more sizes larger than the wielder is refused at equip
// (size-and-wielding §3, §4.1) — it never reaches a wield slot.
func TestEquip_TooLargeRefused(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, sizedWeaponTpl("huge"), a) // huge vs baseline medium = +2 ⇒ too large

	dispatch(t, newRegistry(t), f.env(), a, "equip blade")

	if eq := a.Equipment(); len(eq) != 0 {
		t.Errorf("too-large weapon should not be equipped, got equipment=%v", eq)
	}
}

// A weapon that declares NO size keeps its static companion-slot footprint —
// legacy two-handed weapons are unchanged (size-and-wielding §4.1 fallback).
func TestEquip_NoSizeUsesStaticFootprint(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	// No Size; a static off-hand companion (the legacy two-handed shape).
	tpl := &item.Template{
		ID: "x:greatsword", Name: "a greatsword", Type: "weapon",
		Keywords:       []string{"greatsword"},
		EligibleSlots:  []string{"wield"},
		CompanionSlots: []string{"offhand"},
		WeaponDamage:   "2d6",
	}
	f.spawnInInventory(t, tpl, a)

	dispatch(t, newRegistry(t), f.env(), a, "equip greatsword")

	eq := a.Equipment()
	if _, ok := eq["wield"]; !ok {
		t.Fatalf("legacy weapon not wielded; equipment=%v", eq)
	}
	if _, ok := eq["offhand"]; !ok {
		t.Errorf("legacy static two-handed weapon should still tie up the off hand; equipment=%v", eq)
	}
}
