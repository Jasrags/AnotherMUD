package mob

import (
	"errors"
	"testing"
)

func TestTemplatesAddAndLookup(t *testing.T) {
	r := NewTemplates()

	tpl := &Template{
		ID:          "tapestry-core:village-guard",
		Name:        "a village guard",
		Type:        "npc",
		Disposition: 0,
		Behavior:    "stationary",
		Tags:        []string{"humanoid", "guard"},
		Keywords:    []string{"guard", "villager"},
		Properties: map[string]any{
			"level": 3,
		},
		Stats:     map[string]int{"str": 12, "hp_max": 40},
		Equipment: []string{"tapestry-core:short-sword"},
	}

	if err := r.TryAdd(tpl); err != nil {
		t.Fatalf("TryAdd: %v", err)
	}

	got, err := r.Get("tapestry-core:village-guard")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != tpl.Name {
		t.Errorf("Name = %q, want %q", got.Name, tpl.Name)
	}
	if got.Behavior != "stationary" {
		t.Errorf("Behavior = %q, want %q", got.Behavior, "stationary")
	}
	if !r.Has("tapestry-core:village-guard") {
		t.Error("Has = false, want true")
	}
	if n := r.Count(); n != 1 {
		t.Errorf("Count = %d, want 1", n)
	}
}

func TestTemplatesTryAddDuplicate(t *testing.T) {
	r := NewTemplates()
	tpl := &Template{ID: "tapestry-core:foo", Name: "foo", Type: "npc", Behavior: "idle"}
	if err := r.TryAdd(tpl); err != nil {
		t.Fatalf("first TryAdd: %v", err)
	}
	err := r.TryAdd(tpl)
	if !errors.Is(err, ErrDuplicateID) {
		t.Errorf("second TryAdd err = %v, want ErrDuplicateID", err)
	}
}

func TestTemplatesAddReplaces(t *testing.T) {
	// Per mobs-ai-spawning §2.1: later registrations replace earlier ones.
	r := NewTemplates()
	r.Add(&Template{ID: "tapestry-core:foo", Name: "old", Type: "npc", Behavior: "idle"})
	r.Add(&Template{ID: "tapestry-core:foo", Name: "new", Type: "npc", Behavior: "idle"})

	got, err := r.Get("tapestry-core:foo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "new" {
		t.Errorf("Name = %q, want %q (replacement)", got.Name, "new")
	}
	if n := r.Count(); n != 1 {
		t.Errorf("Count = %d, want 1", n)
	}
}

func TestTemplatesGetUnknown(t *testing.T) {
	r := NewTemplates()
	_, err := r.Get("tapestry-core:nope")
	if !errors.Is(err, ErrTemplateNotFound) {
		t.Errorf("Get err = %v, want ErrTemplateNotFound", err)
	}
	if r.Has("tapestry-core:nope") {
		t.Error("Has = true, want false")
	}
}

func TestTemplatesAllReturnsSnapshot(t *testing.T) {
	r := NewTemplates()
	r.Add(&Template{ID: "tapestry-core:a", Name: "a", Type: "npc", Behavior: "idle"})
	r.Add(&Template{ID: "tapestry-core:b", Name: "b", Type: "npc", Behavior: "idle"})

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("All() len = %d, want 2", len(all))
	}

	// Mutating the snapshot must not affect the registry.
	all[0] = nil
	if r.Count() != 2 {
		t.Errorf("Count after snapshot mutation = %d, want 2", r.Count())
	}
}
