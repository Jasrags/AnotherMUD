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
	if _, statErr := os.Stat(filepath.Join(tmp, "index.html")); statErr != nil {
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

	// Every room appears exactly once (one entry card per room).
	if n := strings.Count(md, `<div class="entry">`); n != 3 {
		t.Errorf("got %d room entries, want 3", n)
	}
	// Headers, markers, notes, and roles (as HTML).
	wants := []string{
		"<h2>Andor</h2>",
		"<h3>Town ",
		"<h2>Unassigned region</h2>",
		"<h3>Wilds ",
		"weather: plains",
		`locked door: Iron Gate`,
		`<span class="marker cross">cross-area</span>`,
		`<span class="tag hidden">hidden</span>`,
		`<span class="tag start">start room</span>`,
		`<strong>Guard</strong> <span class="tag shop">shop</span>`,
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
	// guard placed in two rooms (sorted), shop role, faction cell populated.
	wants := []string{
		"<code>guard</code>",
		"A Guard",
		`<span class="tag shop">shop</span>`,
		`<span class="tag faction">queens-guard</span>`,
		"<code>gate</code>, <code>square</code>",
		"<code>drifter</code>",
	}
	for _, want := range wants {
		if !strings.Contains(md, want) {
			t.Errorf("mob catalog missing %q\n---\n%s", want, md)
		}
	}
	// drifter has no roles/faction/rooms → dash cells.
	if !strings.Contains(md, "<td>—</td>") {
		t.Errorf("expected dash cells for the placeless drifter:\n%s", md)
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

func TestHtmlTableAndEscaping(t *testing.T) {
	// htmlTable escapes header text and renders empty cells as a dash.
	out := htmlTable([]string{"A", "B"}, [][]string{{codeID("x"), ""}})
	for _, want := range []string{"<th>A</th>", "<th>B</th>", "<td><code>x</code></td>", "<td>—</td>"} {
		if !strings.Contains(out, want) {
			t.Errorf("htmlTable missing %q\n%s", want, out)
		}
	}
	// esc HTML-escapes dynamic content (the XSS guard for content-derived text).
	if got := esc(`<script>&"`); got != `&lt;script&gt;&amp;&#34;` {
		t.Errorf("esc = %q, want escaped", got)
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
		`<h2>Unreachable rooms <span class="count some">1</span>`,
		"<code>d</code> (D) — area <code>known</code>",
		`<h2>Orphan rooms <span class="count some">1</span>`,
		"<code>c</code> down → <code>ghost</code>",              // dangling target
		"<code>b</code> east → <code>c</code> (no return exit)", // one-way
		`<h2>Rooms missing a description <span class="count some">1</span>`,
		"<code>empty-area</code> (Empty)",
		"<code>c</code> references mob <code>unknown-mob</code>",
		"quest <code>q1</code> giver <code>ghost-giver</code> is not a known mob",
		"quest <code>q2</code> giver <code>guard</code> is not placed in any room",
		"quest <code>q1</code> reward references faction <code>nofaction</code>",
	}
	for _, want := range wants {
		if !strings.Contains(md, want) {
			t.Errorf("health report missing %q\n---\n%s", want, md)
		}
	}
}

// TestRenderGuide covers the Phase 6 acceptance: deterministic (identical on
// repeat runs), every region with content appears, and services surface.
func TestRenderGuide(t *testing.T) {
	m := &worldModel{
		Pack:  "t",
		Start: "square",
		Areas: map[string]areaYAML{
			"town": {ID: "town", Name: "Town", Description: "A tidy town.", Region: "andor"},
		},
		Mobs: map[string]mobJSON{
			"guard": {Name: "Guard", Shop: true},
			"smith": {Name: "Smith", Trainer: true},
		},
		Rooms: map[string]roomYAML{
			"square": {ID: "square", Area: "town", Name: "Square", Description: "The heart of town.",
				Mobs: []string{"guard"}, Exits: map[string]string{"north": "forge"}},
			"forge": {ID: "forge", Area: "town", Name: "Forge", Mobs: []string{"smith"}},
		},
	}
	md := renderGuide(m)

	wants := []string{
		"You begin in <strong>Square, in Town (Andor)</strong>",
		"The heart of town.",
		"Paths lead north to Forge.",
		"<h3>Andor</h3>",
		"<strong>Town</strong> — A tidy town.",
		`<li><strong>Square</strong> <span class="tag shop">shop</span></li>`,
		`<li><strong>Forge</strong> <span class="tag trainer">trainer</span></li>`,
		"<strong>Shops:</strong> Square (Town)",
		"<strong>Trainers:</strong> Forge (Town)",
	}
	for _, want := range wants {
		if !strings.Contains(md, want) {
			t.Errorf("guide missing %q\n---\n%s", want, md)
		}
	}
	// Deterministic: identical on repeat renders (no timestamp, stable ordering).
	if md2 := renderGuide(m); md2 != md {
		t.Error("renderGuide is not deterministic across calls")
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
