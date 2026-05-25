package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/player"
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
		stats:     stats.New(),
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
		stats:     stats.New(),
	}
	a2.stats.Restore(saved.Stats)

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
	if !a2.stats.Has(entities.EquipmentSourceKey(id2)) {
		t.Errorf("modifiers not rebound to new source key %q", entities.EquipmentSourceKey(id2))
	}
	if a2.stats.Has(entities.EquipmentSourceKey(id1)) {
		t.Errorf("modifiers still present under stale source key %q", entities.EquipmentSourceKey(id1))
	}

	// And unequip cleanly clears them, which only works because the
	// rebind succeeded.
	if _, ok := a2.Unequip("wield"); !ok {
		t.Fatal("post-restart Unequip failed")
	}
	if a2.stats.Has(entities.EquipmentSourceKey(id2)) {
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

// newTestTemplates returns a templates registry holding one entry.
func newTestTemplates(t *testing.T, tpl *item.Template) *item.Templates {
	t.Helper()
	reg := item.NewTemplates()
	if err := reg.TryAdd(tpl); err != nil {
		t.Fatalf("TryAdd: %v", err)
	}
	return reg
}
