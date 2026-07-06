package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/pool"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/size"
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

// SR-M3a step 5 — save round-trip for a NON-CORE pool. A Shadowrun Stun monitor
// (Nonlethal, not one of the hardcoded mana/movement pair) drained mid-session
// must persist its current through the v21 pair-list save and restore on the
// next login — proving the substrate needs no save-version bump for a new kind.
func TestResourcePools_StunMonitorRoundTrips(t *testing.T) {
	stunDecl := &pool.Decl{
		Kind: "stun", Rules: pool.Rules{Floor: 0, Nonlethal: true, DepletionEvent: true},
		MaxChannel: "hp_stun", SeedOnPlayer: true,
	}
	newActor := func(save *player.Save) *connActor {
		return &connActor{
			playerID:  "p-1",
			statBlock: progression.NewWithBase(map[progression.StatType]int{"hp_stun": 10}),
			equipment: map[string]entities.EntityID{},
			poolDecls: []*pool.Decl{stunDecl},
			save:      save,
		}
	}

	// Session 1: fresh character, stun seeds full at 10, then a hit drains it to 4.
	a := newActor(&player.Save{Version: player.CurrentVersion, Name: "Runner"})
	a.seedResourcePools()
	stun, ok := a.pools.Get("stun")
	if !ok {
		t.Fatal("stun monitor not seeded")
	}
	if cur, mx := stun.Snapshot(); cur != 10 || mx != 10 {
		t.Fatalf("seeded stun = %d/%d, want 10/10", cur, mx)
	}
	stun.ApplyDamage(6) // 10 → 4

	// Persist: syncPoolsToSaveLocked writes the drained stun into the save.
	func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if !a.syncPoolsToSaveLocked() {
			t.Fatal("sync: want a change after draining the stun monitor")
		}
	}()
	var savedStun *pool.Entry
	for i := range a.save.Pools {
		if a.save.Pools[i].Kind == "stun" {
			savedStun = &a.save.Pools[i]
		}
	}
	if savedStun == nil || savedStun.Current != 4 {
		t.Fatalf("save.Pools stun = %+v, want current 4", savedStun)
	}

	// Session 2: reload from the persisted snapshot (simulating a fresh login).
	reloaded := append(pool.Snapshot(nil), a.save.Pools...)
	b := newActor(&player.Save{Version: player.CurrentVersion, Name: "Runner", Pools: reloaded})
	b.seedResourcePools()
	stun2, ok := b.pools.Get("stun")
	if !ok {
		t.Fatal("stun monitor not re-seeded on reload")
	}
	if cur, mx := stun2.Snapshot(); cur != 4 || mx != 10 {
		t.Fatalf("reloaded stun = %d/%d, want 4/10 (persisted current, re-derived max)", cur, mx)
	}
}

// SR-M3b: connActor.Stats threads a wielded weapon's target_pool into
// combat.Stats.TargetPool, so a player wielding a stun baton routes damage to
// the target's Stun monitor. An ordinary weapon leaves it empty (the hp path).
func TestConnActorStats_TargetPool(t *testing.T) {
	base := map[progression.StatType]int{progression.StatSTR: 10}

	a := &connActor{statBlock: progression.NewWithBase(base)}
	a.weapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 6}, name: "a stun baton", wieldMode: size.OneHanded, targetPool: "stun"})
	if got := a.Stats().TargetPool; got != "stun" {
		t.Errorf("Stats.TargetPool = %q, want stun", got)
	}

	b := &connActor{statBlock: progression.NewWithBase(base)}
	b.weapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 8}, name: "a sword", wieldMode: size.OneHanded})
	if got := b.Stats().TargetPool; got != "" {
		t.Errorf("Stats.TargetPool = %q, want empty (hp path)", got)
	}
}
