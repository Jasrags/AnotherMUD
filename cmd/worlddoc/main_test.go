package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		if _, err := resolveEmitters("no-such-emitter"); err == nil {
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
		render: func(m *worldModel, packDir string) ([]string, error) {
			if m.Pack == "wot" {
				return nil, fmt.Errorf("boom")
			}
			if err := os.MkdirAll(packDir, 0o755); err != nil {
				return nil, err
			}
			p := filepath.Join(packDir, "map.html")
			if err := os.WriteFile(p, []byte("ok"), 0o644); err != nil {
				return nil, err
			}
			return []string{p}, nil
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

// TestRenderGazetteer covers the acceptance criteria: every room appears
// exactly once, exits render direction + destination with door/locked/hidden/
// cross-area markers, and rooms group under region → area (unassigned region
// last).
func TestRenderGazetteer(t *testing.T) {
	w := worldJSON{
		Pack:    "testpack",
		Regions: []string{"andor"},
		Areas: []areaMeta{
			{ID: "town", Name: "Town", Region: "andor"},
			{ID: "wild", Name: "Wilds", Region: ""},
		},
		Rooms: []roomJSON{
			{ID: "square", Name: "Square", Area: "town", Region: "andor", Terrain: "road", Spawn: true,
				Exits: []exitJSON{
					{Dir: "north", To: "gate", Locked: true, Door: "Iron Gate"},
					{Dir: "east", To: "field", Cross: true},
				},
				Mobs: []mobJSON{{Name: "Guard", Shop: true}}},
			{ID: "gate", Name: "Gate", Area: "town", Region: "andor", Terrain: "road",
				Exits: []exitJSON{
					{Dir: "south", To: "square"},
					{Dir: "up", To: "secret", Hidden: true},
				}},
			{ID: "field", Name: "Field", Area: "wild", Region: "", Terrain: "field", Weather: "plains"},
		},
	}
	md := renderGazetteer(w)

	// Every room appears exactly once (backtick-quoted id).
	for _, id := range []string{"square", "gate", "field"} {
		if n := strings.Count(md, "`"+id+"`"); n != 1 {
			t.Errorf("room %q appears %d times, want exactly 1", id, n)
		}
	}
	// Headers, markers, notes, and roles.
	wants := []string{
		"## Andor",
		"### Town (`town`)",
		"## Unassigned region",
		"### Wilds (`wild` · weather: plains)",
		"north → gate (locked door: Iron Gate)",
		"east → field (cross-area)",
		"up → secret (hidden)",
		"- Notes: start room",
		"Guard (shop)",
	}
	for _, want := range wants {
		if !strings.Contains(md, want) {
			t.Errorf("gazetteer missing %q\n---\n%s", want, md)
		}
	}
}

func TestCatalogMobsPlacement(t *testing.T) {
	m := &worldModel{
		Pack: "testpack",
		Mobs: map[string]mobJSON{
			"guard":   {Name: "A Guard", Shop: true, Faction: "queens-guard"},
			"drifter": {Name: "A Drifter"},
		},
		Rooms: map[string]roomYAML{
			"gate":   {ID: "gate", Mobs: []string{"guard"}},
			"square": {ID: "square", Mobs: []string{"guard"}},
		},
	}
	md := catalogMobs(m)
	// guard placed in two rooms, sorted; shop role; faction column populated.
	if !strings.Contains(md, "| guard | A Guard | shop | queens-guard | gate, square |") {
		t.Errorf("guard row wrong:\n%s", md)
	}
	// drifter has no room placement and no roles/faction → dashes.
	if !strings.Contains(md, "| drifter | A Drifter | — | — | — |") {
		t.Errorf("drifter row wrong:\n%s", md)
	}
}

func TestQuestReward(t *testing.T) {
	q := questYAML{}
	q.Reward.XP = 150
	q.Reward.Gold = 30
	q.Reward.Reputation = 120
	q.Reward.Abilities = []string{"guards-bulwark"}
	q.Reward.Faction = []struct {
		Faction string `yaml:"faction"`
		Delta   int    `yaml:"delta"`
	}{{Faction: "queens-guard", Delta: 700}}

	got := questReward(q)
	want := "150 xp; 30 gold; +120 renown; +700 queens-guard; teaches guards-bulwark"
	if got != want {
		t.Errorf("questReward = %q, want %q", got, want)
	}
	if got := questReward(questYAML{}); got != "—" {
		t.Errorf("empty reward = %q, want —", got)
	}
}

func TestMdTableEscaping(t *testing.T) {
	var b strings.Builder
	mdTable(&b, []string{"A", "B"}, [][]string{{"x|y", "line1\nline2"}})
	out := b.String()
	if !strings.Contains(out, `x\|y`) {
		t.Errorf("pipe not escaped in cell:\n%s", out)
	}
	if strings.Contains(out, "line1\nline2") {
		t.Errorf("newline not collapsed in cell:\n%s", out)
	}
}

// TestRenderHealth feeds a deliberately broken world and asserts each class of
// gap is surfaced (the plan's Phase 5 acceptance).
func TestRenderHealth(t *testing.T) {
	q1 := questYAML{ID: "q1", Giver: "ghost-giver"}
	q1.Reward.Faction = []struct {
		Faction string `yaml:"faction"`
		Delta   int    `yaml:"delta"`
	}{{Faction: "nofaction", Delta: 100}}

	m := &worldModel{
		Pack:  "t",
		Start: "a",
		Areas: map[string]areaYAML{
			"known":      {ID: "known", Name: "Known"},
			"empty-area": {ID: "empty-area", Name: "Empty"},
		},
		Mobs: map[string]mobJSON{"guard": {Name: "Guard"}}, // exists but placed nowhere
		Rooms: map[string]roomYAML{
			"a": {ID: "a", Area: "known", Name: "A", Description: "d", Exits: map[string]string{"north": "b"}},
			"b": {ID: "b", Area: "known", Name: "B", Description: "d", Exits: map[string]string{"south": "a", "east": "c"}},
			"c": {ID: "c", Area: "known", Name: "C", Exits: map[string]string{"down": "ghost"}, Mobs: []string{"unknown-mob"}},
			"d": {ID: "d", Area: "known", Name: "D", Description: "d", Exits: map[string]string{"west": "a"}}, // orphan + unreachable
		},
		Quests: []questYAML{q1, {ID: "q2", Giver: "guard"}},
	}
	md := renderHealth(m)

	wants := []string{
		"## Unreachable rooms (1)",
		"`d` (D) — area `known`",
		"## Orphan rooms (1)",
		"`c` down → `ghost`",              // dangling target
		"`b` east → `c` (no return exit)", // one-way
		"## Rooms missing a description (1)",
		"`empty-area` (Empty)",
		"`c` references mob `unknown-mob`",
		"quest `q1` giver `ghost-giver` is not a known mob",
		"quest `q2` giver `guard` is not placed in any room",
		"quest `q1` reward references faction `nofaction`",
	}
	for _, want := range wants {
		if !strings.Contains(md, want) {
			t.Errorf("health report missing %q\n---\n%s", want, md)
		}
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
