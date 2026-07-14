package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// modVestTpl is a modifiable armor host (capacity 9, "armor" tag) that a mod
// matches against; modWeaveTpl is an armor modification (cost 3, piercing soak).
func modVestTpl() *item.Template {
	return &item.Template{
		ID: "sr:armor-vest", Name: "an armored vest", Type: "item",
		Tags: []string{"armor"}, Keywords: []string{"vest", "armor"},
		EligibleSlots: []string{"body"}, ArmorBonus: 4, Capacity: 9,
	}
}

func modWeaveTpl() *item.Template {
	return &item.Template{
		ID: "sr:ballistic-weave", Name: "a ballistic weave insert", Type: "item",
		Keywords: []string{"weave", "ballistic"},
		ModHost:  "armor", ModCapacityCost: 3, Resistances: map[string]int{"piercing": 2},
	}
}

// modEnvWithSpawn wires a fake SpawnService that can re-mint the weave, so
// `unmodify` fully round-trips the mod back to inventory.
func modEnvWithSpawn(f *eqFixture) command.Env {
	env := f.env()
	env.Spawn = &fakeSpawnService{
		store: f.store,
		items: map[string]*item.Template{"sr:ballistic-weave": modWeaveTpl()},
	}
	return env
}

func TestModify_InstallConsumesModAndReportsCapacity(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)
	a := newTestActor(f.room)
	host := f.spawnInInventory(t, modVestTpl(), a)
	mod := f.spawnInInventory(t, modWeaveTpl(), a)

	dispatch(t, r, f.env(), a, "modify vest weave")

	// The mod is consumed from inventory; the host records it.
	if containsID(a.Inventory(), mod.ID()) {
		t.Error("the weave should be consumed from inventory on install")
	}
	if mods := host.InstalledMods(); len(mods) != 1 {
		t.Fatalf("host installed mods = %d, want 1", len(mods))
	}
	if got := host.FreeCapacity(); got != 6 {
		t.Fatalf("free capacity = %d, want 6", got)
	}
	// The effective resistance now includes the mod (§6 typed-field path).
	if got := host.Resistances()["piercing"]; got != 2 {
		t.Fatalf("effective piercing resistance = %d, want 2", got)
	}
	if out := a.lastLine(); !strings.Contains(out, "install") || !strings.Contains(out, "6 capacity free") {
		t.Errorf("install cue = %q", out)
	}
}

func TestModify_InfoFormShowsCapacityAndMods(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)
	a := newTestActor(f.room)
	_ = f.spawnInInventory(t, modVestTpl(), a)
	_ = f.spawnInInventory(t, modWeaveTpl(), a)

	dispatch(t, r, f.env(), a, "modify vest weave")
	dispatch(t, r, f.env(), a, "modify vest")

	out := a.lastLine()
	if !strings.Contains(out, "capacity 9") || !strings.Contains(out, "6 free") || !strings.Contains(out, "weave") {
		t.Errorf("info line = %q", out)
	}
}

func TestModify_RefusesOverCapacityAndIncompatible(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)
	a := newTestActor(f.room)
	_ = f.spawnInInventory(t, modVestTpl(), a)

	// A too-costly mod is refused, naming the shortfall.
	bigMod := modWeaveTpl()
	bigMod.ID = "sr:big"
	bigMod.Keywords = []string{"big"}
	bigMod.ModCapacityCost = 10
	_ = f.spawnInInventory(t, bigMod, a)
	dispatch(t, r, f.env(), a, "modify vest big")
	if out := a.lastLine(); !strings.Contains(out, "10 capacity") || !strings.Contains(out, "9 free") {
		t.Errorf("over-capacity cue = %q", out)
	}

	// A plain item is not a modification.
	rock := &item.Template{ID: "sr:rock", Name: "a rock", Type: "item", Keywords: []string{"rock"}}
	_ = f.spawnInInventory(t, rock, a)
	dispatch(t, r, f.env(), a, "modify vest rock")
	if out := a.lastLine(); !strings.Contains(out, "isn't a modification") {
		t.Errorf("non-mod cue = %q", out)
	}
}

func TestUnmodify_RemovesAndReturnsMod(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)
	a := newTestActor(f.room)
	host := f.spawnInInventory(t, modVestTpl(), a)
	_ = f.spawnInInventory(t, modWeaveTpl(), a)
	env := modEnvWithSpawn(f)

	dispatch(t, r, env, a, "modify vest weave")
	if len(host.InstalledMods()) != 1 {
		t.Fatal("precondition: mod should be installed")
	}
	dispatch(t, r, env, a, "unmodify vest weave")

	if got := len(host.InstalledMods()); got != 0 {
		t.Fatalf("installed mods after unmodify = %d, want 0", got)
	}
	if got := host.FreeCapacity(); got != 9 {
		t.Fatalf("free capacity after unmodify = %d, want 9 (fully restored)", got)
	}
	// The weave is back in inventory (a fresh entity minted from its template).
	var weaveBack bool
	for _, it := range collectInv(f.store, a.Inventory()) {
		if it.IsModification() && it.ModHost() == "armor" {
			weaveBack = true
		}
	}
	if !weaveBack {
		t.Error("the removed weave should be returned to inventory")
	}
	if out := a.lastLine(); !strings.Contains(out, "pocket") || !strings.Contains(out, "9 capacity free") {
		t.Errorf("unmodify cue = %q", out)
	}
}

func TestModify_WornHostReappliesModifiersLive(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)
	a := newTestActor(f.room)
	// A modifiable helm — the `head` slot exists in the test baseline (`body`
	// does not), so it can actually be equipped in the harness.
	host := f.spawnInInventory(t, &item.Template{
		ID: "sr:helm", Name: "an armored helm", Type: "item",
		Tags: []string{"armor"}, Keywords: []string{"helm"},
		EligibleSlots: []string{"head"}, Capacity: 9,
	}, a)
	// A mod granting a generic stat modifier, so the worn re-apply is observable
	// in the host's equipment modifier group.
	_ = f.spawnInInventory(t, &item.Template{
		ID: "sr:plate", Name: "a trauma plate", Type: "item", Keywords: []string{"plate"},
		ModHost: "armor", ModCapacityCost: 3, Modifiers: []item.Modifier{{Stat: "ac", Value: 2}},
	}, a)

	// Equip the host, THEN modify it while worn — the effect must land immediately.
	dispatch(t, r, f.env(), a, "equip helm")
	dispatch(t, r, f.env(), a, "modify helm plate")

	if len(host.InstalledMods()) != 1 {
		t.Fatalf("mod not installed on the worn host: %d installed", len(host.InstalledMods()))
	}
	var ac int
	for _, m := range a.mods[entities.EquipmentSourceKey(host.ID())] {
		if m.Stat == "ac" {
			ac += m.Value
		}
	}
	if ac != 2 {
		t.Fatalf("worn re-apply did not push the mod's +2 ac into the equipment group: got ac=%d", ac)
	}
	if out := a.lastLine(); !strings.Contains(out, "install") {
		t.Errorf("install cue = %q", out)
	}
}

func TestModify_WornHostBarredInCombat(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)
	a := newTestActor(f.room)
	_ = f.spawnInInventory(t, &item.Template{
		ID: "sr:helm", Name: "an armored helm", Type: "item",
		Tags: []string{"armor"}, Keywords: []string{"helm"},
		EligibleSlots: []string{"head"}, Capacity: 9,
	}, a)
	_ = f.spawnInInventory(t, modWeaveTpl(), a)

	dispatch(t, r, f.env(), a, "equip helm")
	a.inCombat = true
	dispatch(t, r, f.env(), a, "modify helm weave")

	if out := a.lastLine(); !strings.Contains(out, "firefight") {
		t.Errorf("expected a combat-gate refusal, got %q", out)
	}
}

// collectInv resolves inventory ids to item instances for assertions.
func collectInv(store *entities.Store, ids []entities.EntityID) []*entities.ItemInstance {
	out := make([]*entities.ItemInstance, 0, len(ids))
	for _, id := range ids {
		if e, ok := store.GetByID(id); ok {
			if it, ok := e.(*entities.ItemInstance); ok {
				out = append(out, it)
			}
		}
	}
	return out
}
