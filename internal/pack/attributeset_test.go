package pack

import (
	"context"
	"path/filepath"
	"testing"
)

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
