package pack

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

// character-identity §2: a pack is a world or a library; the loader collects
// the active world set (world-flagged packs) and rejects an unknown kind.

func TestManifest_IsWorldAndValidKind(t *testing.T) {
	if !(&Manifest{Kind: "world"}).IsWorld() {
		t.Error(`Kind:"world" should be a world`)
	}
	if !(&Manifest{Kind: "WORLD"}).IsWorld() {
		t.Error("IsWorld should be case-insensitive")
	}
	for _, k := range []string{"", "library", "Library"} {
		if (&Manifest{Kind: k}).IsWorld() {
			t.Errorf("Kind:%q should NOT be a world (empty/library)", k)
		}
	}
	for _, ok := range []string{"", "world", "library", "WORLD"} {
		if !ValidKind(ok) {
			t.Errorf("ValidKind(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"setting", "leaf", "ruleset"} {
		if ValidKind(bad) {
			t.Errorf("ValidKind(%q) = true, want false", bad)
		}
	}
}

// writeWorldKindPacks writes a library pack + a world pack that depends on
// it under a fresh root, and returns the root.
func writeWorldKindPacks(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "lib/pack.yaml"), "name: lib\nkind: library\n")
	// A world pack must declare a splash (loadPackSplash); supply one.
	writeFile(t, filepath.Join(root, "w1/pack.yaml"), "name: w1\nkind: world\nsplash: splash.txt\ndependencies:\n  lib: \"*\"\n")
	writeFile(t, filepath.Join(root, "w1/splash.txt"), "{Y}W1{x}\n")
	return root
}

func TestLoad_DerivesActiveWorldSet(t *testing.T) {
	root := writeWorldKindPacks(t)
	regs := NewRegistries()
	if err := Load(context.Background(), root, []string{"w1"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	// The closure is {lib, w1}; only the world-flagged pack is in Worlds.
	if !slices.Equal(regs.Worlds, []string{"w1"}) {
		t.Errorf("Worlds = %v, want [w1] (library excluded)", regs.Worlds)
	}
}

func TestLoad_RejectsInvalidKind(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "bad/pack.yaml"), "name: bad\nkind: setting\n")
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("Load err = %v, want ErrInvalidContent for unknown kind", err)
	}
}
