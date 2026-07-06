package pool

import (
	"reflect"
	"testing"
)

func TestRegistry_RegisterGetHas(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&Decl{Kind: "stun", Rules: Rules{Floor: 0, Nonlethal: true, DepletionEvent: true}, MaxChannel: "hp_stun", SeedOnPlayer: true, SeedOnMob: true}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	d, ok := r.Get("stun")
	if !ok {
		t.Fatal("stun should be present after Register")
	}
	if !d.Rules.Nonlethal || d.MaxChannel != "hp_stun" || !d.SeedOnMob {
		t.Fatalf("decl round-trip wrong: %+v", d)
	}
	if !r.Has("STUN") { // case-insensitive
		t.Error("Has should be case-insensitive")
	}
	if _, ok := r.Get("mana"); ok {
		t.Error("mana should be absent")
	}
}

func TestRegistry_RegisterErrors(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Error("nil Decl should error")
	}
	if err := r.Register(&Decl{Kind: "  "}); err == nil {
		t.Error("empty kind should error")
	}
}

func TestRegistry_LowercasesKindAndOverflow(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&Decl{Kind: "STUN", Rules: Rules{OverflowTo: "PHYSICAL"}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	d, ok := r.Get("stun")
	if !ok {
		t.Fatal("kind should be looked up lowercased")
	}
	if d.Kind != "stun" || d.Rules.OverflowTo != "physical" {
		t.Fatalf("kind/overflow not lowercased: %+v", d)
	}
}

func TestRegistry_PriorityOverride(t *testing.T) {
	r := NewRegistry()
	// Core declares mana floored at 0.
	_ = r.Register(&Decl{Kind: "mana", Rules: Rules{Floor: 0}, Pack: "core", Priority: 0})
	// Equal-or-lower priority no-ops (core retained).
	_ = r.Register(&Decl{Kind: "mana", Rules: Rules{Floor: -5}, Pack: "world", Priority: 0})
	if d, _ := r.Get("mana"); d.Rules.Floor != 0 || d.Pack != "core" {
		t.Fatalf("equal priority must not override: %+v", d)
	}
	// Higher priority replaces.
	_ = r.Register(&Decl{Kind: "mana", Rules: Rules{Floor: -6}, Pack: "world", Priority: 1})
	if d, _ := r.Get("mana"); d.Rules.Floor != -6 || d.Pack != "world" {
		t.Fatalf("higher priority must override: %+v", d)
	}
}

func TestRegistry_AllSortedByKind(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Decl{Kind: "stun"})
	_ = r.Register(&Decl{Kind: "mana"})
	_ = r.Register(&Decl{Kind: "physical"})
	got := make([]Kind, 0, 3)
	for _, d := range r.All() {
		got = append(got, d.Kind)
	}
	want := []Kind{"mana", "physical", "stun"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("All() order = %v, want %v (kind-sorted)", got, want)
	}
}
