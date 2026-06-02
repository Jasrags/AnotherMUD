package loot

import "testing"

func TestRegistry_RegisterAndGet_CaseInsensitive(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&Table{ID: "Goblin-Loot", PoolRolls: 2}); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, ok := r.Get("goblin-loot")
	if !ok {
		t.Fatal("get: want hit, got miss")
	}
	if got.ID != "goblin-loot" {
		t.Fatalf("id normalized: want goblin-loot, got %q", got.ID)
	}
	if got.PoolRolls != 2 {
		t.Fatalf("pool rolls: want 2, got %d", got.PoolRolls)
	}
	if !r.Has("GOBLIN-LOOT") {
		t.Fatal("has: want true")
	}
}

func TestRegistry_NilAndEmptyIDRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Fatal("nil table: want error")
	}
	if err := r.Register(&Table{ID: "   "}); err == nil {
		t.Fatal("blank id: want error")
	}
}

func TestRegistry_PriorityOverride(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Table{ID: "x", Priority: 1, PoolRolls: 1})
	// Lower priority: ignored.
	_ = r.Register(&Table{ID: "x", Priority: 0, PoolRolls: 99})
	got, _ := r.Get("x")
	if got.PoolRolls != 1 {
		t.Fatalf("lower priority should not override: got PoolRolls=%d", got.PoolRolls)
	}
	// Higher priority: wins.
	_ = r.Register(&Table{ID: "x", Priority: 5, PoolRolls: 7})
	got, _ = r.Get("x")
	if got.PoolRolls != 7 {
		t.Fatalf("higher priority should override: got PoolRolls=%d", got.PoolRolls)
	}
}

func TestRegistry_DeepCopyIsolatesCaller(t *testing.T) {
	orig := &Table{
		ID:         "y",
		Guaranteed: []GuaranteedEntry{{ItemID: "a", Count: 1}},
		Weighted:   []WeightedEntry{{ItemID: "b", Weight: 1}},
		RareBonus:  &RareBonus{Chance: 10, Entries: []WeightedEntry{{ItemID: "c", Weight: 1}}},
	}
	r := NewRegistry()
	_ = r.Register(orig)
	// Mutate the caller's table after registration.
	orig.Guaranteed[0].ItemID = "MUTATED"
	orig.Weighted[0].ItemID = "MUTATED"
	orig.RareBonus.Entries[0].ItemID = "MUTATED"

	got, _ := r.Get("y")
	if got.Guaranteed[0].ItemID != "a" || got.Weighted[0].ItemID != "b" || got.RareBonus.Entries[0].ItemID != "c" {
		t.Fatalf("registry not isolated from caller mutation: %+v", got)
	}
}

func TestRegistry_MissReturnsFalse(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("nope"); ok {
		t.Fatal("miss: want false")
	}
	if _, ok := r.Get("  "); ok {
		t.Fatal("blank: want false")
	}
}
