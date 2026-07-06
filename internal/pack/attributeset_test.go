package pack

import (
	"context"
	"path/filepath"
	"testing"
)

// A kind:world pack's `attribute_set:` selection is recorded into
// WorldAttributeSets keyed by namespace (SR-M1 step 3); a world declaring none
// is absent from the map (→ the classic fallback at seed time).
func TestLoad_RecordsWorldAttributeSetSelection(t *testing.T) {
	root := t.TempDir()
	// A world that selects a set.
	writeFile(t, filepath.Join(root, "sr/pack.yaml"), "name: sr\nkind: world\nsplash: splash.txt\nattribute_set: Shadowrun5\n")
	writeFile(t, filepath.Join(root, "sr/splash.txt"), "{Y}SR{x}\n")
	// A world that selects nothing.
	writeFile(t, filepath.Join(root, "plain/pack.yaml"), "name: plain\nkind: world\nsplash: splash.txt\n")
	writeFile(t, filepath.Join(root, "plain/splash.txt"), "{Y}P{x}\n")

	regs := NewRegistries()
	if err := Load(context.Background(), root, []string{"sr", "plain"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Selection recorded, lowercased.
	if got := regs.WorldAttributeSets["sr"]; got != "shadowrun5" {
		t.Errorf("WorldAttributeSets[sr] = %q, want shadowrun5 (lowercased)", got)
	}
	// A world that selects nothing is absent (→ classic fallback).
	if _, ok := regs.WorldAttributeSets["plain"]; ok {
		t.Error("WorldAttributeSets[plain] present; a world selecting no set should be absent")
	}
}

// A pack declaring an `attribute_sets:` glob must register the set end-to-end
// (SR-M1 — shadowrun-mvp.md Appendix A). Mirrors the languages-glob trap: the
// loader enumerates by manifest, so without the glob the files are silently
// ignored.
func TestLoad_RegistersAttributeSet(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  attribute_sets: [attributes/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "attributes/classic.yaml"), `
id: classic
name: Classic Six
attributes:
  - { id: str, name: Strength, abbrev: STR, default: 10, cap: 22, trainable: true, category: physical }
  - { id: luck, name: Luck, abbrev: LCK, default: 10, category: special }
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}

	set, ok := regs.AttributeSets.Get("classic")
	if !ok {
		t.Fatal("attribute set 'classic' not registered")
	}
	if len(set.Attributes) != 2 {
		t.Fatalf("attributes len = %d, want 2", len(set.Attributes))
	}
	str, ok := set.Get("str")
	if !ok {
		t.Fatal("attribute 'str' missing from set")
	}
	if str.Name != "Strength" || str.Abbrev != "STR" || str.Default != 10 || str.Cap != 22 || !str.Trainable || str.Category != "physical" {
		t.Errorf("str decoded wrong: %+v", str)
	}
	if set.Pack != "tapestry-core" {
		t.Errorf("Pack = %q, want tapestry-core", set.Pack)
	}
}

// A malformed set (duplicate attribute id) must fail at load with attribution,
// not silently seed a broken character later.
func TestLoad_MalformedAttributeSetFails(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  attribute_sets: [attributes/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "attributes/bad.yaml"), `
id: bad
attributes:
  - { id: str }
  - { id: str }
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err == nil {
		t.Fatal("Load: expected error for duplicate attribute id, got nil")
	}
}
