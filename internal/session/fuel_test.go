package session

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

func litTorch(t *testing.T, store *entities.Store, fuel int) *entities.ItemInstance {
	t.Helper()
	inst, err := store.Spawn(&item.Template{
		ID: "core:torch", Name: "a torch", Type: "light",
		Properties: map[string]any{"light": "gloom", "fuel": fuel},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	inst.SetProperty(light.PropItemLit, true)
	return inst
}

func TestBurnFuel_PartialBurnNoGutter(t *testing.T) {
	mgr := NewManager()
	store := entities.NewStore()
	r := &world.Room{ID: "x:1", Name: "X"}
	a, fc := newFakeActor("c1", "p1", "acc1", "Alice", r)
	torch := litTorch(t, store, 5)
	a.AddToInventory(torch.ID())
	mgr.Add(a)

	mgr.BurnFuel(context.Background(), light.FuelConfig{BurnAmount: 1}, store, nil)

	if fuel, _ := torch.Property(light.PropItemFuel); fuel.(int) != 4 {
		t.Fatalf("fuel after burn = %v, want 4", fuel)
	}
	if !light.IsLit(torch) {
		t.Fatal("torch should still be lit after a partial burn")
	}
	if got := fc.writes(); len(got) != 0 {
		t.Fatalf("partial burn should be silent, got %v", got)
	}
}

func TestBurnFuel_GuttersAndNotifiesAndPublishes(t *testing.T) {
	mgr := NewManager()
	store := entities.NewStore()
	bus := eventbus.New()
	var fired *eventbus.LightSourceExtinguished
	bus.Subscribe(eventbus.EventLightSourceExtinguished, func(_ context.Context, ev eventbus.Event) {
		if e, ok := ev.(eventbus.LightSourceExtinguished); ok {
			fired = &e
		}
	})

	r := &world.Room{ID: "x:1", Name: "X"}
	a, fc := newFakeActor("c1", "p1", "acc1", "Alice", r)
	torch := litTorch(t, store, 1)
	a.AddToInventory(torch.ID())
	mgr.Add(a)

	mgr.BurnFuel(context.Background(), light.FuelConfig{BurnAmount: 1}, store, bus)

	if light.IsLit(torch) {
		t.Fatal("torch should have guttered (unlit) at zero fuel")
	}
	got := fc.writes()
	if len(got) != 1 || !strings.Contains(got[0], "gutters out") {
		t.Fatalf("expected one gutter notice, got %v", got)
	}
	if fired == nil {
		t.Fatal("light.source.extinguished was not published")
	}
	if fired.SourceID != torch.ID() {
		t.Fatalf("event SourceID = %v, want %v", fired.SourceID, torch.ID())
	}
	if fired.RoomID != r.ID {
		t.Fatalf("event RoomID = %v, want %v", fired.RoomID, r.ID)
	}
}

func TestBurnFuel_NilStoreNoop(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:1", Name: "X"}
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", r)
	mgr.Add(a)
	// Must not panic with a nil store.
	mgr.BurnFuel(context.Background(), light.DefaultFuelConfig(), nil, nil)
}
