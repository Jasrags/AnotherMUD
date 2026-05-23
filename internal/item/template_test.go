package item

import (
	"errors"
	"testing"
)

func TestTemplatesAddAndLookup(t *testing.T) {
	r := NewTemplates()

	tpl := &Template{
		ID:       "tapestry-core:short-sword",
		Name:     "a short sword",
		Type:     "item",
		Tags:     []string{"weapon", "metal"},
		Keywords: []string{"sword", "short"},
		Properties: map[string]any{
			"damage": 4,
		},
		Modifiers: []Modifier{
			{Stat: "str", Value: 1},
		},
	}

	if err := r.TryAdd(tpl); err != nil {
		t.Fatalf("TryAdd: %v", err)
	}

	got, err := r.Get("tapestry-core:short-sword")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != tpl.Name {
		t.Errorf("Name = %q, want %q", got.Name, tpl.Name)
	}
	if !r.Has("tapestry-core:short-sword") {
		t.Error("Has = false, want true")
	}
	if n := r.Count(); n != 1 {
		t.Errorf("Count = %d, want 1", n)
	}
}

func TestTemplatesTryAddDuplicate(t *testing.T) {
	r := NewTemplates()
	tpl := &Template{ID: "tapestry-core:foo", Name: "foo", Type: "item"}
	if err := r.TryAdd(tpl); err != nil {
		t.Fatalf("first TryAdd: %v", err)
	}
	err := r.TryAdd(tpl)
	if !errors.Is(err, ErrDuplicateID) {
		t.Errorf("second TryAdd err = %v, want ErrDuplicateID", err)
	}
}

func TestTemplatesAddReplaces(t *testing.T) {
	// Per inventory-equipment-items §2.1: Add (not TryAdd) MUST replace.
	r := NewTemplates()
	r.Add(&Template{ID: "tapestry-core:foo", Name: "old", Type: "item"})
	r.Add(&Template{ID: "tapestry-core:foo", Name: "new", Type: "item"})

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
	r.Add(&Template{ID: "tapestry-core:a", Name: "a", Type: "item"})
	r.Add(&Template{ID: "tapestry-core:b", Name: "b", Type: "item"})

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
