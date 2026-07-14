package command

import (
	"context"
	"testing"
)

// TestCatalog_GroupingAndOrder verifies the catalog groups the real builtins by
// category in canonical order, and that each category is non-empty and carries
// commands with a keyword.
func TestCatalog_GroupingAndOrder(t *testing.T) {
	r := New()
	if err := RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	cats := r.Catalog(false)
	if len(cats) == 0 {
		t.Fatal("player catalog is empty")
	}
	// Canonical order is preserved: the categories that appear must be a
	// subsequence of categoryOrder (leftovers, if any, come after).
	orderIdx := make(map[string]int, len(categoryOrder))
	for i, m := range categoryOrder {
		orderIdx[m.Key] = i
	}
	last := -1
	for _, c := range cats {
		if len(c.Commands) == 0 {
			t.Errorf("category %q emitted with no commands", c.Key)
		}
		if idx, ok := orderIdx[c.Key]; ok {
			if idx < last {
				t.Errorf("category %q out of canonical order", c.Key)
			}
			last = idx
		}
		for _, cc := range c.Commands {
			if cc.Keyword == "" {
				t.Errorf("empty keyword in category %q", c.Key)
			}
		}
	}
}

// TestCatalog_AdminGate confirms the admin group and admin commands appear only
// in the admin tier, matching the bare-help index's role gate.
func TestCatalog_AdminGate(t *testing.T) {
	r := New()
	if err := RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}

	hasAdminGroup := func(cats []CatalogCategory) bool {
		for _, c := range cats {
			if c.Key == catAdmin {
				return true
			}
		}
		return false
	}
	hasCommand := func(cats []CatalogCategory, kw string) bool {
		for _, c := range cats {
			for _, cc := range c.Commands {
				if cc.Keyword == kw {
					return true
				}
			}
		}
		return false
	}

	player := r.Catalog(false)
	if hasAdminGroup(player) {
		t.Error("player catalog leaked the admin group")
	}
	if hasCommand(player, "spawn") {
		t.Error("player catalog leaked admin command 'spawn'")
	}
	if !hasCommand(player, "kill") {
		t.Error("player catalog missing a normal command 'kill'")
	}

	admin := r.Catalog(true)
	if !hasAdminGroup(admin) {
		t.Error("admin catalog missing the admin group")
	}
	if !hasCommand(admin, "spawn") {
		t.Error("admin catalog missing admin command 'spawn'")
	}
}

// TestCatalog_Syntax confirms the primary syntax line is synthesized from typed
// args when present, else falls back to the first hand-authored syntax line.
func TestCatalog_Syntax(t *testing.T) {
	r := New()
	noop := func(_ context.Context, _ *Context) error { return nil }
	if err := r.RegisterCommand(Command{
		Keyword: "stow", Brief: "Stow.", Handler: noop,
		Args: []ArgDefinition{{Name: "item", Type: ArgInventory}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterCommand(Command{
		Keyword: "ponder", Brief: "Ponder.", Syntax: []string{"ponder deeply"}, Handler: noop,
	}); err != nil {
		t.Fatal(err)
	}
	cats := r.Catalog(false)
	find := func(kw string) CatalogCommand {
		for _, c := range cats {
			for _, cc := range c.Commands {
				if cc.Keyword == kw {
					return cc
				}
			}
		}
		return CatalogCommand{}
	}
	if got := find("stow").Syntax; got != "stow [item]" {
		t.Errorf("typed syntax = %q, want 'stow [item]'", got)
	}
	if got := find("ponder").Syntax; got != "ponder deeply" {
		t.Errorf("hand-authored syntax = %q, want 'ponder deeply'", got)
	}
}
