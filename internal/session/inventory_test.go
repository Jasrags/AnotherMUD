package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

// White-box tests for the inventory glue on connActor + the session
// helpers respawnInventory / untrackInventory. End-to-end coverage
// (login → drop → restart → verify) lives in session_test.go's
// dialer-driven suite once the integration harness is extended.

func newInvActor(t *testing.T, store *entities.Store) *connActor {
	t.Helper()
	return &connActor{
		save:  &player.Save{Version: player.CurrentVersion, Name: "Tester"},
		items: store,
	}
}

func swordTpl() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:short-sword",
		Name:     "a short sword",
		Type:     "weapon",
		Keywords: []string{"sword"},
	}
}

// medkitTplPersist is a charged item (a medkit) for the charge-persistence
// round-trip.
func medkitTplPersist() *item.Template {
	return &item.Template{
		ID: "tapestry-core:field-medkit", Name: "a medkit", Type: "item",
		Keywords: []string{"medkit"},
		Properties: map[string]any{
			"first_aid_kit": true,
			"charges":       10,
			"max_charges":   10,
		},
	}
}

// A spent medkit keeps its remaining charges across a save → respawn cycle
// (InventoryEntry.Charges), rather than resetting to the template's 10 —
// the persistence that makes "refillable kit" mean anything across relog.
func TestRespawnInventory_PersistsMedkitCharges(t *testing.T) {
	store := entities.NewStore()
	tpls := item.NewTemplates()
	tpls.Add(medkitTplPersist())

	// Spawn the kit into inventory, then spend it down to 3 uses.
	a := newInvActor(t, store)
	respawnInventory(context.Background(), a, store, nil, tpls,
		[]player.InventoryEntry{{Template: "tapestry-core:field-medkit"}})
	id := a.Inventory()[0]
	inst := mustItem(t, store, id)
	inst.SetProperty("charges", 3)

	// Sync to the save tree: the spent count is captured (not nil, not 10).
	a.mu.Lock()
	a.syncInventoryToSaveLocked()
	a.mu.Unlock()
	out := a.save.Inventory
	if len(out) != 1 || out[0].Charges == nil {
		t.Fatalf("save Charges = nil, want *3 (spent count not captured)")
	}
	if *out[0].Charges != 3 {
		t.Fatalf("save Charges = %d, want 3", *out[0].Charges)
	}

	// Respawn from the saved entry into a fresh store: the kit keeps 3, not
	// the template's 10.
	store2 := entities.NewStore()
	a2 := newInvActor(t, store2)
	respawnInventory(context.Background(), a2, store2, nil, tpls, out)
	inst2 := mustItem(t, store2, a2.Inventory()[0])
	if got, _ := inst2.Property("charges"); got != 3 {
		t.Errorf("respawned charges = %v, want 3 (persisted, not template 10)", got)
	}
}

func mustItem(t *testing.T, store *entities.Store, id entities.EntityID) *entities.ItemInstance {
	t.Helper()
	e, ok := store.GetByID(id)
	if !ok {
		t.Fatalf("item %s not in store", id)
	}
	it, ok := e.(*entities.ItemInstance)
	if !ok {
		t.Fatalf("entity %s is not an item", id)
	}
	return it
}

func TestActor_AddInventory_SyncsSaveTemplateIDs(t *testing.T) {
	store := entities.NewStore()
	a := newInvActor(t, store)
	tpl := swordTpl()
	inst, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	a.AddToInventory(inst.ID())

	if !a.dirty {
		t.Error("dirty not set after AddToInventory")
	}
	if got := a.save.Inventory; len(got) != 1 || got[0].Template != string(tpl.ID) {
		t.Errorf("save.Inventory = %+v, want one entry templated %q", got, tpl.ID)
	}
}

func TestActor_RemoveInventory_ClearsSaveEntry(t *testing.T) {
	store := entities.NewStore()
	a := newInvActor(t, store)
	tpl := swordTpl()
	inst, _ := store.Spawn(tpl)

	a.AddToInventory(inst.ID())
	a.dirty = false // reset so we can confirm Remove flips it

	if !a.RemoveFromInventory(inst.ID()) {
		t.Error("RemoveFromInventory returned false")
	}
	if !a.dirty {
		t.Error("dirty not set after RemoveFromInventory")
	}
	if got := a.save.Inventory; len(got) != 0 {
		t.Errorf("save.Inventory = %v, want empty", got)
	}
}

func TestRespawnInventory_RehydratesFromTemplates(t *testing.T) {
	store := entities.NewStore()
	tpls := item.NewTemplates()
	tpls.Add(swordTpl())

	a := newInvActor(t, store)
	respawnInventory(context.Background(), a, store, nil, tpls,
		[]player.InventoryEntry{{Template: "tapestry-core:short-sword"}})

	if got := a.Inventory(); len(got) != 1 {
		t.Fatalf("inventory = %v, want one entry", got)
	}
	// Save shouldn't be marked dirty when nothing was dropped.
	if a.dirty {
		t.Error("dirty=true after clean respawn; should only flip on template drops")
	}
}

func TestRespawnInventory_DropsUnknownTemplate(t *testing.T) {
	store := entities.NewStore()
	tpls := item.NewTemplates()
	tpls.Add(swordTpl())

	a := newInvActor(t, store)
	respawnInventory(context.Background(), a, store, nil, tpls,
		[]player.InventoryEntry{
			{Template: "tapestry-core:short-sword"},
			{Template: "tapestry-core:does-not-exist"},
		})

	if got := a.Inventory(); len(got) != 1 {
		t.Errorf("inventory = %v, want only the survivor", got)
	}
	if got := a.save.Inventory; len(got) != 1 || got[0].Template != "tapestry-core:short-sword" {
		t.Errorf("save.Inventory = %+v, want trimmed to survivor", got)
	}
	if !a.dirty {
		t.Error("dirty not set after trimming dead reference")
	}
}

// TestRespawnInventory_RoundTripContainerContents verifies the v4
// save tree survives a save → respawn cycle: items inside a
// container at save time end up back inside it (via Contents.Put)
// after respawn, and the regenerated save list mirrors the input.
func TestRespawnInventory_RoundTripContainerContents(t *testing.T) {
	store := entities.NewStore()
	contents := entities.NewContents()
	tpls := item.NewTemplates()
	tpls.Add(swordTpl())
	tpls.Add(sackTplPersist())

	a := newInvActor(t, store)
	a.contents = contents

	saved := []player.InventoryEntry{
		{
			Template: "tapestry-core:canvas-sack",
			Contents: []player.InventoryEntry{
				{Template: "tapestry-core:short-sword"},
			},
		},
	}
	respawnInventory(context.Background(), a, store, contents, tpls, saved)

	inv := a.Inventory()
	if len(inv) != 1 {
		t.Fatalf("inventory = %v, want one (sack)", inv)
	}
	sackID := inv[0]
	if got := contents.In(sackID); len(got) != 1 {
		t.Fatalf("sack contents after respawn = %v, want one item", got)
	}

	// Now drive syncInventoryToSaveLocked indirectly and verify
	// the save tree mirrors the input.
	a.mu.Lock()
	a.syncInventoryToSaveLocked()
	a.mu.Unlock()
	out := a.save.Inventory
	if len(out) != 1 || out[0].Template != "tapestry-core:canvas-sack" {
		t.Fatalf("save.Inventory top-level = %+v", out)
	}
	if len(out[0].Contents) != 1 || out[0].Contents[0].Template != "tapestry-core:short-sword" {
		t.Errorf("save.Inventory nested = %+v", out[0].Contents)
	}
}

// TestUntrackInventory_NoContentsIndexLeak pins the post-teardown
// invariant that the Contents index is empty after sweeping a
// container the actor was carrying. A naive recursion that called
// Take on the parent instead of the children would leave phantom
// byItem/byContainer entries that accumulate across reconnects.
//
// The current implementation walks children first (their recursive
// frame calls Take on themselves, which removes byItem[child] and
// prunes byContainer[parent] when the bucket empties), then Take
// on the parent — which is correctly a no-op for top-level
// containers and a real remove for nested ones.
func TestUntrackInventory_NoContentsIndexLeak(t *testing.T) {
	store := entities.NewStore()
	contents := entities.NewContents()

	a := newInvActor(t, store)
	a.contents = contents

	sword, _ := store.Spawn(swordTpl())
	sack, _ := store.Spawn(sackTplPersist())
	a.AddToInventory(sack.ID())
	contents.Put(sack.ID(), sword.ID())

	untrackInventory(context.Background(), store, contents, a)

	if _, ok := contents.ContainerOf(sword.ID()); ok {
		t.Error("byItem leak: sword still maps to a container")
	}
	if got := contents.In(sack.ID()); len(got) > 0 {
		t.Errorf("byContainer leak: sack still has children %v", got)
	}
}

// TestMarkContentsDirty_RebuildsSaveTreeAfterContentsPut is the
// regression test for the put-handler persistence bug: the handler
// does RemoveFromInventory (which re-syncs the save tree at a
// moment when the item is in neither inventory nor the container)
// then Contents.Put, then MarkContentsDirty. Without the
// MarkContentsDirty call the save tree would persist the container
// as empty.
func TestMarkContentsDirty_RebuildsSaveTreeAfterContentsPut(t *testing.T) {
	store := entities.NewStore()
	contents := entities.NewContents()

	a := newInvActor(t, store)
	a.contents = contents

	sword, err := store.Spawn(swordTpl())
	if err != nil {
		t.Fatalf("Spawn sword: %v", err)
	}
	sack, err := store.Spawn(sackTplPersist())
	if err != nil {
		t.Fatalf("Spawn sack: %v", err)
	}
	a.AddToInventory(sword.ID())
	a.AddToInventory(sack.ID())

	// Mirror the put handler's exact ordering.
	if !a.RemoveFromInventory(sword.ID()) {
		t.Fatal("RemoveFromInventory returned false")
	}
	contents.Put(sack.ID(), sword.ID())
	a.MarkContentsDirty()

	if len(a.save.Inventory) != 1 || a.save.Inventory[0].Template != "tapestry-core:canvas-sack" {
		t.Fatalf("save top-level = %+v, want one sack", a.save.Inventory)
	}
	child := a.save.Inventory[0].Contents
	if len(child) != 1 || child[0].Template != "tapestry-core:short-sword" {
		t.Errorf("save sack contents = %+v, want one sword", child)
	}
}

// sackTplPersist is the persistence test's container template.
// Lives here rather than in the put tests so the session package
// has no cross-package fixture dependency.
func sackTplPersist() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:canvas-sack",
		Name:     "a canvas sack",
		Type:     "container",
		Keywords: []string{"sack"},
	}
}

func TestUntrackInventory_RemovesEntitiesFromStore(t *testing.T) {
	store := entities.NewStore()
	a := newInvActor(t, store)
	inst, _ := store.Spawn(swordTpl())
	a.AddToInventory(inst.ID())

	untrackInventory(context.Background(), store, nil, a)

	if _, ok := store.GetByID(inst.ID()); ok {
		t.Error("entity still tracked after untrackInventory")
	}
}
