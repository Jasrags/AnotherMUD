package pack

import (
	"errors"
	"path/filepath"
	"testing"
)

// armor-depth §2 metadata decode + validation. Recorded-only this slice;
// the loader validates so an authoring typo fails the pack by file name.

func TestDecodeItem_ArmorMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, `
id: full-plate
name: a suit of full plate
type: armor
armor_bonus: 8
armor_max_dex: 1
armor_check_penalty: 6
armor_tier: Heavy
resistances:
  Slashing: 2
  piercing: 1
`)
	tpl, err := decodeItem(path, "wot")
	if err != nil {
		t.Fatalf("decodeItem: %v", err)
	}
	if tpl.ArmorBonus != 8 {
		t.Errorf("ArmorBonus = %d, want 8", tpl.ArmorBonus)
	}
	if tpl.ArmorMaxDex == nil || *tpl.ArmorMaxDex != 1 {
		t.Errorf("ArmorMaxDex = %v, want ptr to 1", tpl.ArmorMaxDex)
	}
	if tpl.ArmorCheckPenalty != 6 {
		t.Errorf("ArmorCheckPenalty = %d, want 6", tpl.ArmorCheckPenalty)
	}
	if tpl.ArmorTier != "heavy" {
		t.Errorf("ArmorTier = %q, want heavy (normalized)", tpl.ArmorTier)
	}
	// Resistance keys normalized lowercase; values preserved.
	if tpl.Resistances["slashing"] != 2 || tpl.Resistances["piercing"] != 1 {
		t.Errorf("Resistances = %v, want slashing:2 piercing:1 (keys lowercased)", tpl.Resistances)
	}
}

func TestDecodeItem_ArmorMaxDexZeroIsAValidCap(t *testing.T) {
	// 0 is a meaningful cap (heavy armor): the pointer must be non-nil and
	// point to 0, distinct from "absent" (nil = no cap).
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: plate\nname: rigid plate\ntype: armor\narmor_max_dex: 0\n")
	tpl, err := decodeItem(path, "wot")
	if err != nil {
		t.Fatalf("decodeItem: %v", err)
	}
	if tpl.ArmorMaxDex == nil {
		t.Fatal("ArmorMaxDex = nil, want non-nil ptr to 0 (0 is a valid cap, not absent)")
	}
	if *tpl.ArmorMaxDex != 0 {
		t.Errorf("*ArmorMaxDex = %d, want 0", *tpl.ArmorMaxDex)
	}
}

func TestDecodeItem_ArmorMetadataDefaultsToZeroAndNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: token\nname: a quest token\ntype: item\n")
	tpl, err := decodeItem(path, "wot")
	if err != nil {
		t.Fatalf("decodeItem: %v", err)
	}
	if tpl.ArmorBonus != 0 || tpl.ArmorCheckPenalty != 0 || tpl.ArmorTier != "" {
		t.Errorf("absent armor metadata should be zero/empty, got bonus=%d check=%d tier=%q",
			tpl.ArmorBonus, tpl.ArmorCheckPenalty, tpl.ArmorTier)
	}
	if tpl.ArmorMaxDex != nil {
		t.Errorf("absent armor_max_dex should be nil (no cap), got %v", *tpl.ArmorMaxDex)
	}
	if tpl.Resistances != nil {
		t.Errorf("absent resistances should be nil, got %v", tpl.Resistances)
	}
}

func TestDecodeItem_RejectsBadArmorTier(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: x\nname: x\ntype: armor\narmor_tier: plated\n")
	_, err := decodeItem(path, "wot")
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("decodeItem err = %v, want ErrInvalidContent for unknown armor tier", err)
	}
}

func TestDecodeItem_RejectsNegativeArmorBonus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: x\nname: x\ntype: armor\narmor_bonus: -3\n")
	_, err := decodeItem(path, "wot")
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("decodeItem err = %v, want ErrInvalidContent for negative armor_bonus", err)
	}
}

func TestDecodeItem_RejectsNegativeArmorMaxDex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: x\nname: x\ntype: armor\narmor_max_dex: -1\n")
	_, err := decodeItem(path, "wot")
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("decodeItem err = %v, want ErrInvalidContent for negative armor_max_dex", err)
	}
}

func TestDecodeItem_RejectsNegativeArmorCheckPenalty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: x\nname: x\ntype: armor\narmor_check_penalty: -2\n")
	_, err := decodeItem(path, "wot")
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("decodeItem err = %v, want ErrInvalidContent for negative armor_check_penalty", err)
	}
}

func TestDecodeItem_RejectsBadResistanceType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: x\nname: x\ntype: armor\nresistances:\n  fire: 3\n")
	// `fire` is not in the current fixed damage-type vocabulary (B/P/S);
	// it becomes valid when a ruleset extends the vocabulary.
	_, err := decodeItem(path, "wot")
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("decodeItem err = %v, want ErrInvalidContent for unknown resistance type", err)
	}
}

func TestDecodeItem_RejectsNegativeResistanceValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.yaml")
	writeFile(t, path, "id: x\nname: x\ntype: armor\nresistances:\n  slashing: -1\n")
	_, err := decodeItem(path, "wot")
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("decodeItem err = %v, want ErrInvalidContent for negative resistance value", err)
	}
}
