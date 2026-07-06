package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/pool"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// The data-driven seed (SR-M3a step 3): an actor with content-declared
// player-seed pool decls builds exactly those pools, each with its declared
// Rules and its ceiling from the max_channel stat — not the hardcoded
// mana/movement pair.
func TestSeedResourcePools_DataDriven(t *testing.T) {
	a := &connActor{
		playerID: "p-1",
		statBlock: progression.NewWithBase(map[progression.StatType]int{
			progression.StatResourceMax: 20,
			"hp_stun":                   12,
		}),
		equipment: map[string]entities.EntityID{},
		poolDecls: []*pool.Decl{
			{Kind: "mana", Rules: pool.Rules{Floor: 0}, MaxChannel: "resource_max", SeedOnPlayer: true},
			{Kind: "stun", Rules: pool.Rules{Floor: 0, Nonlethal: true, DepletionEvent: true}, MaxChannel: "hp_stun", SeedOnPlayer: true},
		},
	}

	a.seedResourcePools()

	// Exactly the two declared pools — no hardcoded movement pool leaked in.
	if _, ok := a.pools.Get(poolKindMovement); ok {
		t.Error("movement pool present; the data-driven path must not seed the hardcoded pair")
	}
	mana, ok := a.pools.Get("mana")
	if !ok || mana.Max() != 20 {
		t.Fatalf("mana pool: ok=%v max=%d, want max 20 (resource_max)", ok, mana.Max())
	}
	stun, ok := a.pools.Get("stun")
	if !ok {
		t.Fatal("stun pool not seeded from the decl")
	}
	if stun.Max() != 12 {
		t.Errorf("stun max = %d, want 12 (hp_stun)", stun.Max())
	}
	// The Stun monitor's declared Rules ride onto the pool (SR-M2 KO routing
	// reads Rules.Nonlethal off exactly this pool).
	if r := stun.Rules(); !r.Nonlethal || !r.DepletionEvent {
		t.Errorf("stun rules = %+v, want Nonlethal+DepletionEvent", r)
	}
}

// A pool decl with no max_channel seeds inert (max 0) and binds no listener —
// the "declared but no ceiling yet" case.
func TestSeedResourcePools_NoChannelSeedsInert(t *testing.T) {
	a := &connActor{
		playerID:  "p-1",
		statBlock: progression.NewWithBase(map[progression.StatType]int{}),
		equipment: map[string]entities.EntityID{},
		poolDecls: []*pool.Decl{
			{Kind: "edge", Rules: pool.Rules{Floor: 0}, SeedOnPlayer: true}, // no MaxChannel
		},
	}

	a.seedResourcePools()

	p, ok := a.pools.Get("edge")
	if !ok {
		t.Fatal("edge pool not seeded")
	}
	if p.Max() != 0 {
		t.Errorf("edge max = %d, want 0 (no max_channel)", p.Max())
	}
}

// playerSeedPoolDecls filters to SeedOnPlayer and is nil-safe (nil registry →
// nil → seedResourcePools falls back to the hardcoded pair).
func TestPlayerSeedPoolDecls_FiltersAndNilSafe(t *testing.T) {
	if got := playerSeedPoolDecls(nil); got != nil {
		t.Errorf("nil registry → %v, want nil", got)
	}

	reg := pool.NewRegistry()
	_ = reg.Register(&pool.Decl{Kind: "mana", SeedOnPlayer: true})
	_ = reg.Register(&pool.Decl{Kind: "stun", SeedOnPlayer: true, SeedOnMob: true})
	_ = reg.Register(&pool.Decl{Kind: "mob-only", SeedOnMob: true}) // not player-seeded

	got := playerSeedPoolDecls(reg)
	if len(got) != 2 {
		t.Fatalf("player-seed decls = %d, want 2 (mana, stun — not mob-only)", len(got))
	}
	for _, d := range got {
		if !d.SeedOnPlayer {
			t.Errorf("non-player-seed decl %q leaked through the filter", d.Kind)
		}
	}
}
