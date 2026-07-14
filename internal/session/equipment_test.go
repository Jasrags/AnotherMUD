package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/slot"
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
		save:       &player.Save{Version: player.CurrentVersion, Name: "Tester"},
		items:      store,
		equipment:  make(map[string]entities.EntityID),
		footprints: make(map[entities.EntityID][]string),
		statBlock:  progression.New(),
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
	if !a.Equip([]string{"wield"}, inst.ID(), mods) {
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
	a.Equip([]string{"wield"}, inst.ID(), []stats.Modifier{{Stat: "str", Value: 1}})

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
	a1.Equip([]string{"wield"}, inst1.ID(), []stats.Modifier{{Stat: "str", Value: 1}})

	saved := *a1.save // shallow copy is fine; Stats is rebuilt from a fresh Snapshot per mutation
	id1 := inst1.ID()

	// "Server restart": new store gives fresh ids. The persisted
	// EquippedItem still references id1, but the freshly-spawned
	// instance gets a new id. respawnEquipment must rebind the
	// stat-block source key from old to new.
	store2 := entities.NewStore()
	a2 := &connActor{
		save:       &saved,
		items:      store2,
		equipment:  make(map[string]entities.EntityID),
		footprints: make(map[entities.EntityID][]string),
		statBlock:  progression.New(),
	}
	a2.statBlock.RestoreModifiers(saved.Stats)

	// Build a minimal templates registry holding the sword.
	tpls := newTestTemplates(t, tpl)
	slots := slot.NewRegistry()
	if err := slot.RegisterEngineBaseline(slots); err != nil {
		t.Fatalf("RegisterEngineBaseline: %v", err)
	}
	respawnEquipment(ctx, a2, store2, tpls, slots, nil, saved.Equipment)

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
	a.Equip([]string{"wield"}, inst.ID(), []stats.Modifier{{Stat: "str", Value: 1}})

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
	a.Equip([]string{"wield"}, inst.ID(), []stats.Modifier{
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

// swordTplWithDice is a weapon template carrying both modifiers and a
// damage-dice expression — the full weapon shape combat §4.5 expects.
func swordTplWithDice() *item.Template {
	return &item.Template{
		ID:           "tapestry-core:short-sword",
		Name:         "a short sword",
		Type:         "weapon",
		Keywords:     []string{"sword"},
		Modifiers:    []item.Modifier{{Stat: "str", Value: 1}},
		WeaponDamage: "1d6",
	}
}

func TestEquipWeapon_FlowsDamageDiceIntoCombatStats(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)

	// Unarmed: no weapon snapshot → Stats.Damage is zero (combat falls
	// back to the unarmed default via EffectiveDamage).
	if d := a.Stats().Damage; !d.IsZero() {
		t.Fatalf("pre-equip Damage = %+v, want zero", d)
	}

	inst, _ := store.Spawn(swordTplWithDice())
	a.AddToInventory(inst.ID())
	if !a.Equip([]string{"wield"}, inst.ID(), []stats.Modifier{{Stat: "str", Value: 1}}) {
		t.Fatal("Equip returned false")
	}

	got := a.Stats()
	want, _ := combat.ParseDice("1d6")
	if got.Damage != want {
		t.Errorf("post-equip Damage = %+v, want %+v", got.Damage, want)
	}
	if got.WeaponName != "a short sword" {
		t.Errorf("post-equip WeaponName = %q, want %q", got.WeaponName, "a short sword")
	}

	// Disarming reverts to the unarmed default.
	if _, ok := a.Unequip("wield"); !ok {
		t.Fatal("Unequip returned false")
	}
	if d := a.Stats().Damage; !d.IsZero() {
		t.Errorf("post-unequip Damage = %+v, want zero", d)
	}
	if n := a.Stats().WeaponName; n != "" {
		t.Errorf("post-unequip WeaponName = %q, want empty", n)
	}
}

func TestEquipNonWeapon_LeavesUnarmed(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	// A cloak: modifiers but no weapon dice.
	inst, _ := store.Spawn(&item.Template{
		ID:        "tapestry-core:cloak",
		Name:      "a cloak",
		Type:      "armor",
		Modifiers: []item.Modifier{{Stat: "ac", Value: 1}},
	})
	a.AddToInventory(inst.ID())
	a.Equip([]string{"cloak"}, inst.ID(), []stats.Modifier{{Stat: "ac", Value: 1}})
	if d := a.Stats().Damage; !d.IsZero() {
		t.Errorf("Damage = %+v, want zero (cloak is not a weapon)", d)
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

// TestEquipSpanning_SaveOneEntryAndRespawnReexpands verifies the §3.8
// persistence rule for a two-handed weapon: the save records exactly one
// entry keyed by the target slot (not one per footprint key), and on
// reload respawnEquipment re-derives the companion slots from the template
// so the full footprint is restored.
func TestEquipSpanning_SaveOneEntryAndRespawnReexpands(t *testing.T) {
	ctx := context.Background()
	store := entities.NewStore()
	tpl := &item.Template{
		ID:             "tapestry-core:greatsword",
		Name:           "a greatsword",
		Type:           "weapon",
		EligibleSlots:  []string{"wield"},
		CompanionSlots: []string{"offhand"},
		Modifiers:      []item.Modifier{{Stat: "str", Value: 2}},
	}
	inst, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	var saved player.Save
	a := &connActor{
		save:       &saved,
		items:      store,
		equipment:  make(map[string]entities.EntityID),
		footprints: make(map[entities.EntityID][]string),
		statBlock:  progression.New(),
	}
	a.AddToInventory(inst.ID())
	if !a.Equip([]string{"wield", "offhand"}, inst.ID(), []stats.Modifier{{Stat: "str", Value: 2}}) {
		t.Fatal("Equip(spanning) failed")
	}

	// §3.8: one save entry, keyed by the target slot only.
	if len(saved.Equipment) != 1 {
		t.Fatalf("save.Equipment = %v, want exactly 1 entry (target only)", saved.Equipment)
	}
	if _, ok := saved.Equipment["wield"]; !ok {
		t.Errorf("save.Equipment not keyed by target 'wield': %v", saved.Equipment)
	}

	// "Restart": fresh store; respawn re-expands the footprint from the
	// template's companion slots.
	store2 := entities.NewStore()
	a2 := &connActor{
		save:       &saved,
		items:      store2,
		equipment:  make(map[string]entities.EntityID),
		footprints: make(map[entities.EntityID][]string),
		statBlock:  progression.New(),
	}
	a2.statBlock.RestoreModifiers(saved.Stats)
	tpls := newTestTemplates(t, tpl)
	slots := slot.NewRegistry()
	if err := slot.RegisterEngineBaseline(slots); err != nil {
		t.Fatalf("RegisterEngineBaseline: %v", err)
	}
	respawnEquipment(ctx, a2, store2, tpls, slots, nil, saved.Equipment)

	eq := a2.Equipment()
	id2 := eq["wield"]
	if id2 == "" || eq["offhand"] != id2 {
		t.Errorf("respawn did not re-expand spanning footprint: %v", eq)
	}
}

// TestEquip_RejectsOccupiedFootprintKey locks the vacancy guard: Equip
// returns false (no mutation) when a footprint key is already occupied,
// so a caller that skipped the displacement step can't silently overwrite
// an occupant and orphan its footprint.
func TestEquip_RejectsOccupiedFootprintKey(t *testing.T) {
	store := entities.NewStore()
	tpl := &item.Template{
		ID: "tapestry-core:sword", Name: "a sword", Type: "weapon",
		EligibleSlots: []string{"wield"},
	}
	first, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	second, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a := &connActor{
		items:      store,
		equipment:  make(map[string]entities.EntityID),
		footprints: make(map[entities.EntityID][]string),
		statBlock:  progression.New(),
	}
	a.inventory = append(a.inventory, first.ID(), second.ID())

	if !a.Equip([]string{"wield"}, first.ID(), nil) {
		t.Fatal("first Equip failed")
	}
	// wield is now occupied; equipping the second without displacing must fail.
	if a.Equip([]string{"wield"}, second.ID(), nil) {
		t.Error("Equip succeeded on an occupied footprint key")
	}
	if a.Equipment()["wield"] != first.ID() {
		t.Error("occupant was overwritten")
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

// TestRespawnEquipment_RecomputesFromItem_DropsDeletedModContribution proves the
// item-modification §7 robustness fix: on respawn the equip modifier group is
// REBUILT from the current item, so a mod deleted from content between save and
// load no longer leaves its stale contribution in the stat block.
func TestRespawnEquipment_RecomputesFromItem_DropsDeletedModContribution(t *testing.T) {
	ctx := context.Background()

	// SESSION 1: a modifiable armor host with a +2 armor mod installed, equipped
	// with the mod's contribution baked into its equip group (AC 5 = host 3 + 2),
	// then "saved".
	store1 := entities.NewStore()
	a1 := newEqActor(t, store1)
	hostTpl := &item.Template{
		ID: "test:vest", Name: "a vest", Type: "item",
		Tags: []string{"armor"}, Keywords: []string{"vest"},
		EligibleSlots: []string{"head"}, Capacity: 9, ArmorBonus: 3,
	}
	modTpl := &item.Template{
		ID: "test:plate", Name: "a trauma plate", Type: "item",
		ModHost: "armor", ModCapacityCost: 3, ArmorBonus: 2,
	}
	host, _ := store1.Spawn(hostTpl)
	mod, _ := store1.Spawn(modTpl)
	if err := host.InstallMod(mod); err != nil {
		t.Fatalf("install mod: %v", err)
	}
	a1.AddToInventory(host.ID())
	a1.Equip([]string{"head"}, host.ID(), []stats.Modifier{{Stat: string(progression.StatAC), Value: 5}})
	saved := *a1.save

	// SESSION 2: content DELETED the mod — the registry holds only the host.
	store2 := entities.NewStore()
	a2 := &connActor{
		save:       &saved,
		items:      store2,
		equipment:  make(map[string]entities.EntityID),
		footprints: make(map[entities.EntityID][]string),
		statBlock:  progression.New(),
	}
	a2.statBlock.RestoreModifiers(saved.Stats)
	stale := a2.statBlock.Effective(progression.StatAC) // restored stale save: +5

	tpls := newTestTemplates(t, hostTpl) // mod NOT registered → deleted
	slots := slot.NewRegistry()
	if err := slot.RegisterEngineBaseline(slots); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	respawnEquipment(ctx, a2, store2, tpls, slots, nil, saved.Equipment)

	fresh := a2.statBlock.Effective(progression.StatAC)
	// The recompute rebuilds AC from the current item (host 3; deleted mod gone),
	// so the stale +2 is dropped: AC falls by exactly 2 versus the restored save.
	if stale-fresh != 2 {
		t.Fatalf("deleted mod's +2 not dropped on respawn: stale AC %d, fresh AC %d (want a drop of 2)", stale, fresh)
	}
	if newID := a2.Equipment()["head"]; newID == "" || !a2.statBlock.HasSource(entities.EquipmentSourceKey(newID)) {
		t.Fatal("recomputed equip group not applied under the new source id")
	}
}

// TestEquippedCapability_SmartlinkAndSmartgun covers the pairing helpers
// (item-modification §6): a smartlink installed in a worn host is found by
// HasEquippedCapability, and a smartgun accessory on the WIELDED weapon by
// WieldedWeaponHasCapability.
func TestEquippedCapability_SmartlinkAndSmartgun(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)

	// A worn cybereye host carrying a smartlink capability (any equipped slot).
	eye, _ := store.Spawn(&item.Template{
		ID: "sr:eye", Name: "cybereyes", Type: "item",
		Tags: []string{"cybereye"}, EligibleSlots: []string{"head"}, Capacity: 4,
	})
	link, _ := store.Spawn(&item.Template{
		ID: "sr:smartlink", Name: "a smartlink", Type: "item",
		ModHost: "cybereye", ModCapacityCost: 3, Grants: []string{"smartlink"},
	})
	if err := eye.InstallMod(link); err != nil {
		t.Fatalf("install smartlink: %v", err)
	}
	a.AddToInventory(eye.ID())
	a.Equip([]string{"head"}, eye.ID(), nil)

	// A wielded weapon carrying a smartgun accessory.
	gun, _ := store.Spawn(&item.Template{
		ID: "sr:gun", Name: "a pistol", Type: "weapon", WeaponDamage: "2d6",
		EligibleSlots: []string{slot.WieldSlot}, Mounts: []string{"top"},
	})
	smartgun, _ := store.Spawn(&item.Template{
		ID: "sr:smartgun", Name: "a smartgun system", Type: "item",
		ModHost: "weapon", AccessoryMounts: []string{"top"}, Grants: []string{"smartgun"},
	})
	if _, err := gun.AttachAccessory(smartgun); err != nil {
		t.Fatalf("attach smartgun: %v", err)
	}
	a.AddToInventory(gun.ID())
	a.Equip([]string{slot.WieldSlot}, gun.ID(), nil)

	if !a.HasEquippedCapability("smartlink") {
		t.Error("HasEquippedCapability(smartlink) = false with a worn smartlink eye")
	}
	if !a.WieldedWeaponHasCapability("smartgun") {
		t.Error("WieldedWeaponHasCapability(smartgun) = false with a smartgun on the wielded weapon")
	}
	if a.HasEquippedCapability("nope") {
		t.Error("HasEquippedCapability(nope) = true for an unheld capability")
	}
}
