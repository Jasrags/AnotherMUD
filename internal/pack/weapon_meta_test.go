package pack

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

// Phase A — weapon-identity §2 metadata decode + validation. Category is
// an opaque normalized label; tier and damage types validate against the
// engine vocabularies so an authoring typo fails the pack by file name.

func TestDecodeItem_WeaponMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, `
id: longsword
name: a longsword
type: weapon
weapon_damage: "1d8"
weapon_category: Longsword
proficiency_tier: Martial
damage_types: [Slashing]
`)
	tpl, err := decodeItem(path, "wot")
	if err != nil {
		t.Fatalf("decodeItem: %v", err)
	}
	if tpl.WeaponCategory != "longsword" {
		t.Errorf("WeaponCategory = %q, want longsword (normalized)", tpl.WeaponCategory)
	}
	if tpl.ProficiencyTier != "martial" {
		t.Errorf("ProficiencyTier = %q, want martial (normalized)", tpl.ProficiencyTier)
	}
	if !slices.Equal(tpl.DamageTypes, []string{"slashing"}) {
		t.Errorf("DamageTypes = %v, want [slashing]", tpl.DamageTypes)
	}
}

func TestDecodeItem_DamageTypesNormalizeAndDedup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, `
id: warpick
name: a war pick
type: weapon
damage_types: [Piercing, piercing, Bludgeoning]
`)
	tpl, err := decodeItem(path, "wot")
	if err != nil {
		t.Fatalf("decodeItem: %v", err)
	}
	// Lowercased and de-duplicated, order preserved (first occurrence).
	if !slices.Equal(tpl.DamageTypes, []string{"piercing", "bludgeoning"}) {
		t.Errorf("DamageTypes = %v, want [piercing bludgeoning] (normalized, deduped)", tpl.DamageTypes)
	}
}

func TestDecodeItem_WeaponMetadataDefaultsToEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: token\nname: a quest token\ntype: item\n")
	tpl, err := decodeItem(path, "wot")
	if err != nil {
		t.Fatalf("decodeItem: %v", err)
	}
	if tpl.WeaponCategory != "" || tpl.ProficiencyTier != "" || tpl.DamageTypes != nil {
		t.Errorf("absent weapon metadata should be empty, got category=%q tier=%q types=%v",
			tpl.WeaponCategory, tpl.ProficiencyTier, tpl.DamageTypes)
	}
}

func TestDecodeItem_RejectsUnknownTier(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: x\nname: a thing\ntype: weapon\nproficiency_tier: legendary\n")
	_, err := decodeItem(path, "wot")
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent for unknown tier", err)
	}
}

func TestDecodeItem_RejectsUnknownDamageType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: x\nname: a thing\ntype: weapon\ndamage_types: [fire]\n")
	_, err := decodeItem(path, "wot")
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent for unknown damage type", err)
	}
}
