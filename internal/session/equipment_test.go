package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// White-box tests for the equipment glue on connActor + the session
// helper respawnEquipment. The critical invariant: after a "restart"
// (untrack all entities, recreate actor, respawn from save), an
// unequip still removes exactly the right modifier set — proving
// EquipmentSourceKey rebinding survives the entity-id reset.

func newEqActor(t *testing.T, store *entities.Store) *connActor {
	t.Helper()
	return &connActor{
		save:      &player.Save{Version: player.CurrentVersion, Name: "Tester"},
		items:     store,
		equipment: make(map[string]entities.EntityID),
		statBlock: progression.New(),
	}
}

func swordTplWithMods() *item.Template {
	return &item.Template{
		ID:        "tapestry-core:short-sword",
		Name:      "a short sword",
		Type:      "weapon",
		Keywords:  []string{"sword"},
		Modifiers: []item.Modifier{{Stat: "str", Value: 1}},
	}
}

func TestEquip_SyncsEverythingToSave(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	tpl := swordTplWithMods()
	inst, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a.AddToInventory(inst.ID())
	a.dirty = false

	mods := []stats.Modifier{{Stat: "str", Value: 1}}
	if !a.Equip("wield", inst.ID(), mods) {
		t.Fatal("Equip returned false")
	}
	if !a.dirty {
		t.Error("dirty not set after Equip")
	}
	if len(a.save.Inventory) != 0 {
		t.Errorf("save.Inventory = %v, want empty", a.save.Inventory)
	}
	eq := a.save.Equipment["wield"]
	if eq.Template != string(tpl.ID) || eq.Entity != string(inst.ID()) {
		t.Errorf("save.Equipment[wield] = %+v", eq)
	}
	if len(a.save.Stats) != 1 || a.save.Stats[0].Source != entities.EquipmentSourceKey(inst.ID()) {
		t.Errorf("save.Stats = %+v", a.save.Stats)
	}
}

func TestUnequip_ReversesSyncs(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	inst, _ := store.Spawn(swordTplWithMods())
	a.AddToInventory(inst.ID())
	a.Equip("wield", inst.ID(), []stats.Modifier{{Stat: "str", Value: 1}})

	id, ok := a.Unequip("wield")
	if !ok || id != inst.ID() {
		t.Fatalf("Unequip = (%q,%v), want (%q,true)", id, ok, inst.ID())
	}
	if _, ok := a.save.Equipment["wield"]; ok {
		t.Error("save.Equipment[wield] still present after Unequip")
	}
	if len(a.save.Stats) != 0 {
		t.Errorf("save.Stats not cleared: %+v", a.save.Stats)
	}
	if tpls := a.save.Inventory; len(tpls) != 1 || tpls[0].Template != "tapestry-core:short-sword" {
		t.Errorf("save.Inventory after unequip = %+v", tpls)
	}
}

// The headline test for risk #2: round-trip equipment through save
// and restore, then unequip — modifier set must clear.
func TestRoundTrip_UnequipAfterRestartReversesMods(t *testing.T) {
	ctx := context.Background()

	// SESSION 1: spawn sword, equip it, "save" by snapshotting the save.
	store1 := entities.NewStore()
	a1 := newEqActor(t, store1)
	tpl := swordTplWithMods()
	inst1, _ := store1.Spawn(tpl)
	a1.AddToInventory(inst1.ID())
	a1.Equip("wield", inst1.ID(), []stats.Modifier{{Stat: "str", Value: 1}})

	saved := *a1.save // shallow copy is fine; Stats is rebuilt from a fresh Snapshot per mutation
	id1 := inst1.ID()

	// "Server restart": new store gives fresh ids. The persisted
	// EquippedItem still references id1, but the freshly-spawned
	// instance gets a new id. respawnEquipment must rebind the
	// stat-block source key from old to new.
	store2 := entities.NewStore()
	a2 := &connActor{
		save:      &saved,
		items:     store2,
		equipment: make(map[string]entities.EntityID),
		statBlock: progression.New(),
	}
	a2.statBlock.RestoreModifiers(saved.Stats)

	// Build a minimal templates registry holding the sword.
	tpls := newTestTemplates(t, tpl)
	respawnEquipment(ctx, a2, store2, tpls, saved.Equipment)

	// The new instance has a different id from the saved one.
	id2 := a2.Equipment()["wield"]
	if id2 == "" {
		t.Fatal("respawnEquipment did not install wield")
	}
	if id2 == id1 {
		t.Skipf("entity ids happened to match (%q == %q); rebind not exercised", id1, id2)
	}

	// Critical: modifiers should now be keyed under the NEW id.
	if !a2.statBlock.HasSource(entities.EquipmentSourceKey(id2)) {
		t.Errorf("modifiers not rebound to new source key %q", entities.EquipmentSourceKey(id2))
	}
	if a2.statBlock.HasSource(entities.EquipmentSourceKey(id1)) {
		t.Errorf("modifiers still present under stale source key %q", entities.EquipmentSourceKey(id1))
	}

	// And unequip cleanly clears them, which only works because the
	// rebind succeeded.
	if _, ok := a2.Unequip("wield"); !ok {
		t.Fatal("post-restart Unequip failed")
	}
	if a2.statBlock.HasSource(entities.EquipmentSourceKey(id2)) {
		t.Error("modifiers leaked after post-restart unequip")
	}
}

// Regression test for H2 from M5.6 review: a mutation that happens
// while a Persist is in flight must NOT be lost when the persist
// completes. Previously Persist compared only Location to decide
// whether to clear dirty, so equipment/inventory/stats changes during
// a save window were silently dropped from the next autosave.
//
// This is a unit test of the saveGen contract, not a true concurrency
// test (which would require a slow Persist hook). It captures the
// generation under lock, simulates a mutation, then attempts the
// "should I clear dirty" check the way Persist does.
func TestDirtyBit_SurvivesMutationDuringInFlightPersist(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	inst, _ := store.Spawn(swordTplWithMods())
	a.AddToInventory(inst.ID())

	// Snapshot generation as Persist would.
	a.mu.Lock()
	genAtSnapshot := a.saveGen
	a.mu.Unlock()

	// Simulate a concurrent equip while the save is in flight.
	a.Equip("wield", inst.ID(), []stats.Modifier{{Stat: "str", Value: 1}})

	// Persist completion path: clear dirty only if no later mutation.
	a.mu.Lock()
	cleared := false
	if a.saveGen == genAtSnapshot {
		a.dirty = false
		cleared = true
	}
	a.mu.Unlock()

	if cleared {
		t.Fatal("dirty cleared despite a mutation during the simulated Persist window")
	}
	if !a.dirty {
		t.Fatal("dirty flipped off; equipment mutation would be lost on next persist")
	}
}

// TestEquipModifiers_FlowIntoCombatStats verifies the M8.1
// integration: equipment-sourced modifiers added under the
// progression.StatBlock are reflected in connActor.Stats() (the
// combat-side derivation surface). Pre-M8.1 the combat block was a
// frozen hardcoded default that ignored equipment entirely; this test
// is the regression guard for the rewired derivation.
func TestEquipModifiers_FlowIntoCombatStats(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	// Seed the engine-default base so Stats() reads the expected
	// pre-equip baseline (STR=10, AC=10, HitMod=0).
	a.statBlock.RestoreBase(toBaseSnapshot(progression.DefaultPlayerBase()))

	if got := a.Stats(); got.STR != 10 || got.AC != 10 || got.HitMod != 0 {
		t.Fatalf("pre-equip Stats = %+v, want STR=10 AC=10 HitMod=0", got)
	}

	inst, _ := store.Spawn(swordTplWithMods())
	a.AddToInventory(inst.ID())
	a.Equip("wield", inst.ID(), []stats.Modifier{
		{Stat: "str", Value: 2},
		{Stat: "hit_mod", Value: 1},
	})

	got := a.Stats()
	if got.STR != 12 {
		t.Errorf("post-equip STR = %d, want 12 (10 base + 2 sword)", got.STR)
	}
	if got.HitMod != 1 {
		t.Errorf("post-equip HitMod = %d, want 1 (0 base + 1 sword)", got.HitMod)
	}
	if got.AC != 10 {
		t.Errorf("post-equip AC = %d, want 10 (unchanged)", got.AC)
	}

	if _, ok := a.Unequip("wield"); !ok {
		t.Fatal("Unequip returned false")
	}
	got = a.Stats()
	if got.STR != 10 || got.HitMod != 0 || got.AC != 10 {
		t.Errorf("post-unequip Stats = %+v, want baseline restored", got)
	}
}

// toBaseSnapshot lifts a base map into the persisted snapshot shape
// (deterministically ordered). Test helper only — the production
// path uses NewWithBase at construction and BaseSnapshot/RestoreBase
// for round-trip persistence.
func toBaseSnapshot(base map[progression.StatType]int) progression.BaseSnapshot {
	b := progression.NewWithBase(base)
	return b.BaseSnapshot()
}

// newTestTemplates returns a templates registry holding one entry.
func newTestTemplates(t *testing.T, tpl *item.Template) *item.Templates {
	t.Helper()
	reg := item.NewTemplates()
	if err := reg.TryAdd(tpl); err != nil {
		t.Fatalf("TryAdd: %v", err)
	}
	return reg
}
