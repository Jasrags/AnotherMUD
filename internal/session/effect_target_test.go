package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/pool"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// TestConnActor_Stats_ChannelMapRouting proves the channel mapping is LIVE
// in player combat-stat derivation: a non-baseline mapping changes the
// derived AC; a nil mapping preserves the direct stat read.
func TestConnActor_Stats_ChannelMapRouting(t *testing.T) {
	cm, err := channel.NewMapping(map[channel.Channel]string{
		channel.Attack:  "hit_mod",
		channel.Defense: "ac + 5",
	})
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	a := &connActor{
		playerID:   "p-1",
		save:       &player.Save{Version: player.CurrentVersion, Name: "Tester"},
		statBlock:  progression.NewWithBase(progression.DefaultPlayerBase()), // ac=10, hit_mod=0
		equipment:  map[string]entities.EntityID{},
		channelMap: cm,
	}
	if s := a.Stats(); s.AC != 15 || s.HitMod != 0 {
		t.Fatalf("mapped player Stats = AC %d, HitMod %d; want 15, 0 (defense=ac+5)", s.AC, s.HitMod)
	}

	// nil mapping: direct read of the ac stat (10 from DefaultPlayerBase).
	a.channelMap = nil
	if s := a.Stats(); s.AC != 10 {
		t.Fatalf("nil-mapping player AC = %d; want 10", s.AC)
	}
}

// TestConnActor_ResourcePoolDeduction exercises the alongside-route pool
// wiring: when an actor carries a seeded pool.Set, Mana/Movement read the
// live pool current and DeductMana/DeductMovement actually subtract
// (flooring at zero). This is the substrate the One Power channeling pool
// will reuse.
func TestConnActor_ResourcePoolDeduction(t *testing.T) {
	a := &connActor{
		playerID:  "p-1",
		save:      &player.Save{Version: player.CurrentVersion, Name: "Tester"},
		statBlock: progression.NewWithBase(progression.DefaultPlayerBase()),
		equipment: map[string]entities.EntityID{},
		pools:     pool.NewSet(),
	}
	a.pools.Add(pool.New(poolKindMovement, 20, pool.Rules{Floor: 0}))
	a.pools.Add(pool.New(poolKindMana, 15, pool.Rules{Floor: 0}))

	if mv := a.Movement(); mv != 20 {
		t.Fatalf("Movement = %d; want 20 (full)", mv)
	}
	a.DeductMovement(5)
	if mv := a.Movement(); mv != 15 {
		t.Fatalf("after DeductMovement(5) = %d; want 15", mv)
	}
	a.DeductMana(10)
	if mn := a.Mana(); mn != 5 {
		t.Fatalf("after DeductMana(10) = %d; want 5", mn)
	}
	// Max accessors report the ceiling, unchanged by spending — the
	// prompt needs current/max separately, not current/current.
	if mx := a.ManaMax(); mx != 15 {
		t.Fatalf("ManaMax = %d; want 15 (unchanged by spend)", mx)
	}
	if mx := a.MovementMax(); mx != 20 {
		t.Fatalf("MovementMax = %d; want 20 (unchanged by spend)", mx)
	}
	// promptVitals surfaces the drained current against the full max
	// (the old stub reported MaxMana == current).
	pv := a.promptVitals()
	if pv.Mana != 5 || pv.MaxMana != 15 {
		t.Fatalf("promptVitals mana = %d/%d; want 5/15", pv.Mana, pv.MaxMana)
	}
	if pv.MV != 15 || pv.MaxMV != 20 {
		t.Fatalf("promptVitals mv = %d/%d; want 15/20", pv.MV, pv.MaxMV)
	}
	// Over-spend floors at zero, never negative.
	a.DeductMovement(1000)
	if mv := a.Movement(); mv != 0 {
		t.Fatalf("over-spend Movement = %d; want 0 (floored)", mv)
	}
}

// seedResourcePools seeds each pool full from the stat max, then applies any
// persisted current from the save — so a channeler logged out mid-drain
// returns drained while an unseeded pool defaults full. syncPoolsToSaveLocked
// then writes back only the non-full pool, keeping a full save clean.
func TestConnActor_SeedAndSyncResourcePools(t *testing.T) {
	a := &connActor{
		playerID:  "p-1",
		statBlock: progression.NewWithBase(map[progression.StatType]int{progression.StatResourceMax: 20}),
		equipment: map[string]entities.EntityID{},
		save: &player.Save{
			Version: player.CurrentVersion, Name: "Drained",
			Pools: pool.Snapshot{{Kind: poolKindMana, Current: 7, Max: 20}},
		},
	}

	a.seedResourcePools()

	// Mana restored to the persisted current (7), capped at the stat max (20).
	if mn, mx := a.Mana(), a.ManaMax(); mn != 7 || mx != 20 {
		t.Fatalf("seeded mana = %d/%d; want 7/20 (persisted current, stat max)", mn, mx)
	}
	// Movement had no stat max and no persisted entry → full at 0 (unseeded).
	if mx := a.MovementMax(); mx != 0 {
		t.Fatalf("MovementMax = %d; want 0 (no movement_max stat)", mx)
	}

	// Live state (mana 7/20) already matches the save, so sync is a no-op.
	if a.syncPoolsToSaveLocked() {
		t.Fatalf("syncPoolsToSaveLocked: want no change when live == save")
	}

	// Spend more → the non-full pool is rewritten into the save.
	a.DeductMana(2) // 7 → 5
	if !a.syncPoolsToSaveLocked() {
		t.Fatalf("syncPoolsToSaveLocked: want change after draining to 5")
	}
	if len(a.save.Pools) != 1 || a.save.Pools[0].Current != 5 {
		t.Fatalf("save.Pools = %+v; want [{mana 5 20}]", a.save.Pools)
	}

	// Refill to full → the pool is OMITTED (current == max), keeping the save clean.
	p, _ := a.pools.Get(poolKindMana)
	p.SetCurrent(20)
	if !a.syncPoolsToSaveLocked() {
		t.Fatalf("syncPoolsToSaveLocked: want change when refilling to full")
	}
	if len(a.save.Pools) != 0 {
		t.Fatalf("save.Pools = %+v; want empty (full pool omitted)", a.save.Pools)
	}
}

// The resolver now resolves a live mob id to the MobInstance, so an
// effect cast on a mob installs its modifiers (cluster 1 payoff).
func TestEffectTargetResolver_ResolvesMob(t *testing.T) {
	store := entities.NewStore()
	m, err := store.SpawnMob(&mob.Template{
		ID: "core:goblin", Name: "a goblin", Type: "npc",
		Stats: map[string]int{"hp_max": 12, "ac": 8},
	})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	// nil manager: a player lookup always misses, so resolution must
	// fall through to the store.
	r := NewEffectTargetResolver(nil, store)

	tgt, ok := r.ResolveTarget(string(m.ID()))
	if !ok {
		t.Fatal("resolver should resolve a live mob id")
	}
	if tgt.EntityID() != string(m.ID()) {
		t.Errorf("resolved EntityID = %q, want %q", tgt.EntityID(), m.ID())
	}
	if _, ok := r.ResolveTarget("core:nope"); ok {
		t.Error("unknown id should not resolve")
	}
}

// TestConnActor_SatisfiesResolutionSource pins the M9.4b wiring: a
// connActor must satisfy progression.ResolutionSource (which embeds
// ValidationEntity) so the ability-resolution phase can validate +
// resolve a player's queued abilities. Compile-time assertion plus
// a spot-check of the thin-pool / no-rest defaults.
func TestConnActor_SatisfiesResolutionSource(t *testing.T) {
	a := &connActor{
		playerID:  "p-1",
		save:      &player.Save{Version: player.CurrentVersion, Name: "Tester"},
		statBlock: progression.NewWithBase(progression.DefaultPlayerBase()),
		equipment: map[string]entities.EntityID{},
	}
	var src progression.ResolutionSource = a // compile-time pin

	if src.IsResting() {
		t.Error("players have no rest state yet; IsResting must be false")
	}
	if src.InCombat() {
		t.Error("actor with nil combat manager must report not-in-combat")
	}
	if _, ok := src.CurrentTarget(); ok {
		t.Error("actor with nil combat manager must report no target")
	}
	// A bare actor seeds no pool.Set, so the resource accessors are
	// nil-safe and read 0; deduction is a safe no-op. Real pool deduction
	// is exercised in TestConnActor_ResourcePoolDeduction.
	if mv := src.Movement(); mv != 0 {
		t.Errorf("pool-less actor Movement = %d; want 0", mv)
	}
	src.DeductMovement(5)
	if src.Movement() != 0 {
		t.Error("DeductMovement on a pool-less actor must stay 0")
	}
	src.SetLastAbility("slash")
	if a.LastAbility() != "slash" {
		t.Errorf("SetLastAbility not recorded, got %q", a.LastAbility())
	}
}

// TestConnActor_SatisfiesEffectTarget pins the M9.2 wiring: a
// connActor must satisfy progression.EffectTarget so the
// EffectManager can write modifiers through it without an
// adapter. Compile-time check via interface assignment plus a
// runtime Apply/RemoveBySource round-trip.
func TestConnActor_SatisfiesEffectTarget(t *testing.T) {
	a := &connActor{
		playerID:  "p-1",
		save:      &player.Save{Version: player.CurrentVersion, Name: "Tester"},
		statBlock: progression.NewWithBase(progression.DefaultPlayerBase()),
	}
	var target progression.EffectTarget = a // compile-time pin

	if id := target.EntityID(); id != "p-1" {
		t.Errorf("EntityID = %q, want p-1", id)
	}

	src := progression.EffectSourceKey("bless")
	target.AddModifiers(src, []stats.Modifier{{Stat: "str", Value: 3}})
	if !a.statBlock.HasSource(src) {
		t.Errorf("AddModifiers did not install under %s", src)
	}
	if !a.dirty {
		t.Errorf("dirty not set after AddModifiers")
	}

	// Drop dirty so we can pin RemoveBySource flips it back.
	a.dirty = false
	if !target.RemoveBySource(src) {
		t.Errorf("RemoveBySource returned false")
	}
	if a.statBlock.HasSource(src) {
		t.Errorf("RemoveBySource did not clear stat block")
	}
	if !a.dirty {
		t.Errorf("dirty not set after RemoveBySource")
	}

	// Round-trip through the EffectManager to pin the resolver
	// path works end-to-end against a real connActor.
	mgr := progression.NewEffectManager(progression.TargetResolverFunc(
		func(id string) (progression.EffectTarget, bool) {
			if id == a.EntityID() {
				return a, true
			}
			return nil, false
		}), nil)
	ok := mgr.Apply(context.Background(), "p-1", progression.EffectTemplate{
		ID: "shield", Duration: 5,
		Modifiers: []stats.Modifier{{Stat: "ac", Value: 2}},
	}, "", "spell.shield")
	if !ok {
		t.Fatalf("Apply returned false")
	}
	if !a.statBlock.HasSource(progression.EffectSourceKey("shield")) {
		t.Errorf("effect modifiers not installed via EffectManager")
	}
	if !mgr.RemoveByID(context.Background(), "p-1", "shield") {
		t.Fatalf("RemoveByID returned false")
	}
	if a.statBlock.HasSource(progression.EffectSourceKey("shield")) {
		t.Errorf("effect modifiers not reversed via EffectManager")
	}
	// Ensure the type assertion compiled (silences "declared and
	// not used" if future refactors drop the runtime calls).
	_ = entities.SourceKey("")
}

// TestSyncStats_ExcludesEffectModifiers pins the m9-2 #3 fix: a buff
// active when the stat block is persisted must NOT round-trip into a
// permanent bonus. syncStatsToSaveLocked drops effect-sourced
// modifiers from save.Stats while keeping equipment-sourced ones
// (active effects are ephemeral per spec §5.5; equipment persists +
// rebinds at login).
func TestSyncStats_ExcludesEffectModifiers(t *testing.T) {
	a := &connActor{
		playerID:  "p-1",
		save:      &player.Save{Version: player.CurrentVersion, Name: "Tester"},
		statBlock: progression.NewWithBase(progression.DefaultPlayerBase()),
		equipment: map[string]entities.EntityID{},
	}
	equipSrc := entities.EquipmentSourceKey("sword-1")
	effectSrc := progression.EffectSourceKey("bless")
	a.statBlock.AddModifiers(equipSrc, []stats.Modifier{{Stat: "hit_mod", Value: 1}})
	a.statBlock.AddModifiers(effectSrc, []stats.Modifier{{Stat: "hit_mod", Value: 2}})

	a.syncStatsToSaveLocked()

	var sawEquip, sawEffect bool
	for _, e := range a.save.Stats {
		if e.Source == equipSrc {
			sawEquip = true
		}
		if progression.IsEffectSource(e.Source) {
			sawEffect = true
		}
	}
	if !sawEquip {
		t.Error("equipment modifier must persist in save.Stats")
	}
	if sawEffect {
		t.Error("effect modifier must NOT persist (ephemeral per §5.5) — buff would become permanent on reload")
	}
}
