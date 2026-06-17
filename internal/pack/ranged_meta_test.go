package pack

import (
	"errors"
	"path/filepath"
	"testing"
)

// ranged-combat §2 — ranged weapon metadata decode + validation (Slice A
// data slice; recorded only, no combat consumer yet). ranged_class
// validates against the engine vocabulary; a projectile must name the ammo
// kind it fires; range_increment + str_rating are non-negative.

func TestDecodeItem_RangedProjectileMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, `
id: short-bow
name: a short bow
type: weapon
weapon_damage: "1d6"
ranged_class: Projectile
ammo_kind: Arrow
range_increment: 6
`)
	tpl, err := decodeItem(path, "wot")
	if err != nil {
		t.Fatalf("decodeItem: %v", err)
	}
	if tpl.RangedClass != "projectile" {
		t.Errorf("RangedClass = %q, want projectile (normalized)", tpl.RangedClass)
	}
	if tpl.AmmoKind != "arrow" {
		t.Errorf("AmmoKind = %q, want arrow (normalized)", tpl.AmmoKind)
	}
	if tpl.RangeIncrement != 6 {
		t.Errorf("RangeIncrement = %d, want 6", tpl.RangeIncrement)
	}
}

func TestDecodeItem_ThrownAndStrRating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, `
id: throwing-knife
name: a throwing knife
type: weapon
weapon_damage: "1d4"
ranged_class: thrown
`)
	tpl, err := decodeItem(path, "wot")
	if err != nil {
		t.Fatalf("decodeItem: %v", err)
	}
	if tpl.RangedClass != "thrown" {
		t.Errorf("RangedClass = %q, want thrown", tpl.RangedClass)
	}
	// A thrown weapon needs no ammo_kind.
	if tpl.AmmoKind != "" {
		t.Errorf("AmmoKind = %q, want empty for thrown", tpl.AmmoKind)
	}

	// A Strength-rated bow caps the positive Strength bonus.
	path2 := filepath.Join(t.TempDir(), "bow.yaml")
	writeFile(t, path2, `
id: composite-bow
name: a composite bow
type: weapon
weapon_damage: "1d8"
ranged_class: projectile
ammo_kind: arrow
str_rating: 2
`)
	tpl2, err := decodeItem(path2, "wot")
	if err != nil {
		t.Fatalf("decodeItem (str-rated): %v", err)
	}
	if tpl2.StrRating == nil || *tpl2.StrRating != 2 {
		t.Errorf("StrRating = %v, want 2", tpl2.StrRating)
	}
}

// Ammo items carry ammo_kind too (what they supply), matched verbatim
// against a projectile weapon's ammo_kind.
func TestDecodeItem_AmmoItemKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: arrow\nname: an arrow\ntype: item\nammo_kind: Arrow\n")
	tpl, err := decodeItem(path, "wot")
	if err != nil {
		t.Fatalf("decodeItem: %v", err)
	}
	if tpl.AmmoKind != "arrow" {
		t.Errorf("AmmoKind = %q, want arrow (normalized)", tpl.AmmoKind)
	}
}

func TestDecodeItem_RangedDefaultsToMelee(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: sword\nname: a sword\ntype: weapon\nweapon_damage: \"1d8\"\n")
	tpl, err := decodeItem(path, "wot")
	if err != nil {
		t.Fatalf("decodeItem: %v", err)
	}
	if tpl.RangedClass != "" || tpl.AmmoKind != "" || tpl.RangeIncrement != 0 || tpl.StrRating != nil {
		t.Errorf("absent ranged metadata should be zero, got class=%q kind=%q incr=%d rating=%v",
			tpl.RangedClass, tpl.AmmoKind, tpl.RangeIncrement, tpl.StrRating)
	}
}

func TestDecodeItem_RejectsUnknownRangedClass(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: x\nname: a thing\ntype: weapon\nranged_class: hurled\n")
	if _, err := decodeItem(path, "wot"); !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent for unknown ranged_class", err)
	}
}

func TestDecodeItem_RejectsProjectileWithoutAmmoKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: x\nname: a bow\ntype: weapon\nranged_class: projectile\n")
	if _, err := decodeItem(path, "wot"); !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent for projectile without ammo_kind", err)
	}
}

func TestDecodeItem_RejectsNegativeRangeIncrement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: x\nname: a thing\ntype: weapon\nrange_increment: -1\n")
	if _, err := decodeItem(path, "wot"); !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent for negative range_increment", err)
	}
}

func TestDecodeItem_RejectsNegativeStrRating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: x\nname: a bow\ntype: weapon\nranged_class: projectile\nammo_kind: arrow\nstr_rating: -1\n")
	if _, err := decodeItem(path, "wot"); !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent for negative str_rating", err)
	}
}
