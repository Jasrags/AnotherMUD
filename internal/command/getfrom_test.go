package command_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/corpse"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// getFromCorpse takes a fresh ration from the corpse into inventory.
func TestGetFrom_TakesItemFromCorpse(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	cor := f.placeCorpse(t, []string{"player:p-alice"}, 100, 0, ration(), sword())

	var removed *eventbus.ContainerItemRemoved
	f.bus.Subscribe(eventbus.EventContainerItemRemoved, func(_ context.Context, ev eventbus.Event) {
		e := ev.(eventbus.ContainerItemRemoved)
		removed = &e
	})

	dispatchLoot(t, f, a, "get ration from corpse")

	if len(a.Inventory()) != 1 {
		t.Fatalf("inventory = %d, want 1", len(a.Inventory()))
	}
	// One item taken; the sword remains in the corpse, so the corpse stays.
	if got := f.contents.In(cor.ID()); len(got) != 1 {
		t.Errorf("corpse should still hold 1 item, got %d", len(got))
	}
	if _, ok := f.store.GetByID(cor.ID()); !ok {
		t.Error("partly-looted corpse should remain")
	}
	if removed == nil || removed.ContainerID != cor.ID() {
		t.Errorf("container.item_removed = %+v", removed)
	}
}

func TestGetFrom_CoinsFromCorpse(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	f.placeCorpse(t, []string{"player:p-alice"}, 100, 12) // coins only

	dispatchLoot(t, f, a, "get coins from corpse")

	if a.Gold() != 12 {
		t.Errorf("gold = %d, want 12", a.Gold())
	}
	if len(a.Inventory()) != 0 {
		t.Errorf("coins must not enter inventory, got %d items", len(a.Inventory()))
	}
}

func TestGetFrom_NonOwnerRefusedDuringWindow(t *testing.T) {
	f := newLootFixture(t)
	eve := newNamedTestActor("Eve", "p-eve", f.room)
	cor := f.placeCorpse(t, []string{"player:p-alice"}, 100, 5, ration())

	dispatchLoot(t, f, eve, "get ration from corpse")
	if len(eve.Inventory()) != 0 {
		t.Errorf("non-owner took an item during the window: %d", len(eve.Inventory()))
	}
	dispatchLoot(t, f, eve, "get coins from corpse")
	if eve.Gold() != 0 {
		t.Errorf("non-owner took coins during the window: %d", eve.Gold())
	}
	if _, ok := f.store.GetByID(cor.ID()); !ok {
		t.Error("corpse should remain after refused takes")
	}
}

func TestGetFrom_LastItemRemovesCorpse(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	cor := f.placeCorpse(t, []string{"player:p-alice"}, 100, 0, ration()) // single item, no coins

	var looted bool
	f.bus.Subscribe(eventbus.EventCorpseLooted, func(_ context.Context, _ eventbus.Event) { looted = true })

	dispatchLoot(t, f, a, "get ration from corpse")

	if _, ok := f.store.GetByID(cor.ID()); ok {
		t.Error("corpse emptied by get-from should be removed")
	}
	if !looted {
		t.Error("emptying a corpse via get-from should emit corpse.looted")
	}
}

func TestGetFrom_ItemNotInContainer(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	f.placeCorpse(t, []string{"player:p-alice"}, 100, 0, ration())

	dispatchLoot(t, f, a, "get sword from corpse")
	if len(a.Inventory()) != 0 {
		t.Errorf("should not take a non-present item, got %d", len(a.Inventory()))
	}
}

// A non-corpse container (a sack) supports get-from without rights, and
// is NOT removed when emptied.
func TestGetFrom_PlainContainerNotRemovedWhenEmptied(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	sack, err := f.store.Spawn(&item.Template{
		ID: "tapestry-core:sack", Name: "a sack", Type: entities.ContainerType, Keywords: []string{"sack"},
	})
	if err != nil {
		t.Fatalf("spawn sack: %v", err)
	}
	f.place.Place(sack.ID(), f.room.ID)
	gem, _ := f.store.Spawn(&item.Template{ID: "tapestry-core:gem", Name: "a gem", Type: "misc", Keywords: []string{"gem"}})
	f.contents.Put(sack.ID(), gem.ID())

	dispatchLoot(t, f, a, "get gem from sack")

	if len(a.Inventory()) != 1 {
		t.Fatalf("inventory = %d, want 1", len(a.Inventory()))
	}
	// The sack is not a corpse, so emptying it must NOT remove it.
	if corpse.IsCorpse(sack) {
		t.Fatal("sack should not be a corpse")
	}
	if _, ok := f.store.GetByID(sack.ID()); !ok {
		t.Error("an emptied plain container must NOT be removed")
	}
}
