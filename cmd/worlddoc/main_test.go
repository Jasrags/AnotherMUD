package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// contentDir is the repo's real content tree, relative to this package.
const contentDir = "../../content"

func TestResolveEmitters(t *testing.T) {
	t.Run("all returns every registered emitter", func(t *testing.T) {
		got, err := resolveEmitters("all")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(emitters) {
			t.Fatalf("got %d emitters, want %d", len(got), len(emitters))
		}
	})

	t.Run("named returns just that emitter", func(t *testing.T) {
		got, err := resolveEmitters("map")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].name != "map" {
			t.Fatalf("got %+v, want single map emitter", got)
		}
	})

	t.Run("unknown errors", func(t *testing.T) {
		if _, err := resolveEmitters("gazetteer"); err == nil {
			t.Fatal("expected error for unregistered emitter, got nil")
		}
	})
}

func TestResolvePacksNamed(t *testing.T) {
	packs, starts, err := resolvePacks(contentDir, "wot", "the-green")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(packs) != 1 || packs[0] != "wot" {
		t.Fatalf("got packs %v, want [wot]", packs)
	}
	if starts["wot"] != "the-green" {
		t.Fatalf("got start %q, want the-green (the -start flag)", starts["wot"])
	}
}

func TestResolvePacksAll(t *testing.T) {
	packs, starts, err := resolvePacks(contentDir, "all", "ignored")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(packs, "wot") || !contains(packs, "starter-world") {
		t.Fatalf("got packs %v, want to include wot and starter-world", packs)
	}
	if contains(packs, "core") {
		t.Fatalf("got packs %v, library pack 'core' must be excluded", packs)
	}
	// -pack all seeds from defaultStarts, not the -start flag.
	if starts["wot"] != "the-green" || starts["starter-world"] != "town-square" {
		t.Fatalf("got starts %v, want per-pack defaults", starts)
	}
}

func TestDiscoverWorldPacksSorted(t *testing.T) {
	got, err := discoverWorldPacks(contentDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sort.StringsAreSorted(got) {
		t.Fatalf("discoverWorldPacks not sorted: %v", got)
	}
	if contains(got, "core") {
		t.Fatalf("library pack 'core' leaked into world packs: %v", got)
	}
}

// TestComputeFeaturesOrder locks the canonical feature-key order. The HTML
// template drives badge display order and search off exactly this sequence, so
// a reordering here silently changes the map UI — this test makes it loud.
func TestComputeFeaturesOrder(t *testing.T) {
	allOn := []mobJSON{{
		Shop: true, Trainer: true, Stable: true,
		Hireling: true, Recruiter: true, Quest: true,
		Faction: "children-of-the-light", Hostile: true,
	}}
	got := computeFeatures(true, true, true, "black", true, true, allOn)
	want := []string{
		"spawn", "shop", "trainer", "craft", "stable", "hire",
		"quest", "faction", "hostile", "locked", "hidden", "dark", "items",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d features %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("feature order mismatch at %d: got %q, want %q\nfull: %v", i, got[i], want[i], got)
		}
	}
}

// TestRunPackAllIsolatesEmitterFailure locks the per-pack isolation invariant:
// in -pack all mode, one pack's emitter failure must not abort the others. It
// swaps in an emitter that fails only for wot and asserts the good pack still
// rendered, the index was still written, and run reports the failure.
func TestRunPackAllIsolatesEmitterFailure(t *testing.T) {
	saved := emitters
	defer func() { emitters = saved }()
	emitters = []emitter{{
		name: "map",
		render: func(m *worldModel, packDir string) (string, error) {
			if m.Pack == "wot" {
				return "", fmt.Errorf("boom")
			}
			if err := os.MkdirAll(packDir, 0o755); err != nil {
				return "", err
			}
			p := filepath.Join(packDir, "map.html")
			if err := os.WriteFile(p, []byte("ok"), 0o644); err != nil {
				return "", err
			}
			return p, nil
		},
	}}

	tmp := t.TempDir()
	err := run(contentDir, "all", "", "all", tmp)
	if err == nil {
		t.Fatal("expected a failure to be reported when one pack's emitter errors, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "starter-world", "map.html")); statErr != nil {
		t.Fatalf("good pack was not rendered after another pack failed: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "index.md")); statErr != nil {
		t.Fatalf("cross-pack index was not written: %v", statErr)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
