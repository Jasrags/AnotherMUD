package pack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// character-select §4.1: a kind:world pack MUST declare a connect splash; the
// loader reads it into Registries.Splashes (keyed by namespace) and rejects a
// world pack that lacks one or points at an empty/missing file. Library packs
// are exempt.

func TestLoad_WorldSplash_LoadedIntoRegistries(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "lib/pack.yaml"), "name: lib\nkind: library\n")
	writeFile(t, filepath.Join(root, "w1/pack.yaml"), "name: w1\nkind: world\nsplash: splash.txt\ndependencies:\n  lib: \"*\"\n")
	writeFile(t, filepath.Join(root, "w1/splash.txt"), "{Y}Welcome to W1{x}\n")

	regs := NewRegistries()
	if err := Load(context.Background(), root, []string{"w1"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := regs.Splashes["w1"]
	if !ok {
		t.Fatalf("Splashes missing w1; have %v", regs.Splashes)
	}
	if got != "{Y}Welcome to W1{x}" { // trailing newline trimmed, markup preserved
		t.Errorf("splash text = %q, want the file contents (newline-trimmed)", got)
	}
	// A library pack contributes no splash entry.
	if _, ok := regs.Splashes["lib"]; ok {
		t.Errorf("library pack should not have a splash entry")
	}
}

func TestLoad_WorldSplash_MissingFieldRejected(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "w1/pack.yaml"), "name: w1\nkind: world\n")
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("Load err = %v, want ErrInvalidContent for a world pack with no splash", err)
	}
}

func TestLoad_WorldSplash_EmptyFileRejected(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "w1/pack.yaml"), "name: w1\nkind: world\nsplash: splash.txt\n")
	writeFile(t, filepath.Join(root, "w1/splash.txt"), "   \n")
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("Load err = %v, want ErrInvalidContent for an empty splash file", err)
	}
}

func TestLoad_WorldSplash_MissingFileRejected(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "w1/pack.yaml"), "name: w1\nkind: world\nsplash: nope.txt\n")
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("Load err = %v, want ErrInvalidContent for an unreadable splash file", err)
	}
}

func TestLoad_WorldSplash_EscapingPathRejected(t *testing.T) {
	root := t.TempDir()
	// A secret outside the pack dir the splash must not be able to read.
	writeFile(t, filepath.Join(root, "secret.txt"), "top secret")
	writeFile(t, filepath.Join(root, "w1/pack.yaml"), "name: w1\nkind: world\nsplash: ../secret.txt\n")
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("Load err = %v, want ErrInvalidContent for a splash path escaping the pack dir", err)
	}
}
