package pack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// TestLoad_RejectsAttributeNamedLikeSyntheticInput verifies that a
// content-declared attribute whose id collides with a reserved synthetic
// combat-input name (channel.InputArmor "armor" / channel.InputDexAC "dex_ac")
// is a fatal load error (validateAttributeReservedNames). Without the guard the
// combat stat lookup silently shadows the attribute — the formula var resolves
// to the synthetic value, never the stored stat — a fail-silent trap for a
// future pack author. See sr-m3c-deferred-fixes.
func TestLoad_RejectsAttributeNamedLikeSyntheticInput(t *testing.T) {
	// The "ARMOR" row proves the normalization chain is load-bearing: an id is
	// lowercased on Register, so a mixed-case author spelling still collides.
	for _, id := range []string{"armor", "dex_ac", "ARMOR"} {
		t.Run(id, func(t *testing.T) {
			root := t.TempDir()
			pack := filepath.Join(root, "core")
			writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  attribute_sets: [attributes/*.yaml]
`)
			writeFile(t, filepath.Join(pack, "attributes/set.yaml"), `
id: colliding
name: Colliding Set
attributes:
  - { id: str, name: Strength, default: 10 }
  - { id: `+id+`, name: Bad, default: 10 }
`)

			regs := NewRegistries()
			err := Load(context.Background(), root, nil, regs, nil, nil, nil)
			if !errors.Is(err, ErrAttributeReservedName) {
				t.Fatalf("Load err = %v, want ErrAttributeReservedName", err)
			}
		})
	}
}

// TestLoad_RejectsCollisionAcrossSets proves the guard checks every registered
// set, not just one: a clean set plus a second colliding set still fails.
func TestLoad_RejectsCollisionAcrossSets(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  attribute_sets: [attributes/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "attributes/clean.yaml"), `
id: clean
name: Clean Set
attributes:
  - { id: str, name: Strength, default: 10 }
`)
	writeFile(t, filepath.Join(pack, "attributes/bad.yaml"), `
id: bad
name: Bad Set
attributes:
  - { id: dex_ac, name: Bad, default: 10 }
`)

	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrAttributeReservedName) {
		t.Fatalf("Load err = %v, want ErrAttributeReservedName", err)
	}
}

// TestLoad_AcceptsNonCollidingAttributeSet is the control: a set whose keys
// avoid the reserved vocabulary loads cleanly.
func TestLoad_AcceptsNonCollidingAttributeSet(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  attribute_sets: [attributes/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "attributes/set.yaml"), `
id: clean
name: Clean Set
attributes:
  - { id: str, name: Strength, default: 10 }
  - { id: body, name: Body, default: 10 }
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
}
