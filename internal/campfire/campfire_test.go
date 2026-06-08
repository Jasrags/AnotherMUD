package campfire

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

func TestPlace_CreatesTaggedStationEntity(t *testing.T) {
	store := entities.NewStore()
	placement := entities.NewPlacement()
	store.SwapTagIndex() // publish the write-side index so GetByTag sees it

	id, err := Place(store, placement, world.RoomID("core:field"), 100)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}

	// It's placed in the room.
	room, ok := placement.RoomOf(id)
	if !ok || room != "core:field" {
		t.Errorf("RoomOf = %q, %v; want core:field", room, ok)
	}

	e, ok := store.GetByID(id)
	if !ok {
		t.Fatal("campfire not tracked")
	}
	it := e.(*entities.ItemInstance)
	if CreatedTick(it) != 100 {
		t.Errorf("CreatedTick = %d, want 100", CreatedTick(it))
	}
	// Carries the station map the craft path reads.
	if _, ok := it.Property(PropCraftStations); !ok {
		t.Error("campfire missing craft_stations property")
	}
}

func TestDecaySweep_RemovesExpiredKeepsFresh(t *testing.T) {
	store := entities.NewStore()
	placement := entities.NewPlacement()

	old, _ := Place(store, placement, world.RoomID("core:a"), 0)
	fresh, _ := Place(store, placement, world.RoomID("core:b"), 90)
	store.SwapTagIndex() // make both visible to GetByTag

	svc := NewService(store, placement)
	const lifetime = 50
	// now=100: old (created 0, age 100 >= 50) expires; fresh (created 90,
	// age 10 < 50) survives.
	rooms := svc.DecaySweep(context.Background(), 100, lifetime)

	if len(rooms) != 1 || rooms[0] != "core:a" {
		t.Errorf("decayed rooms = %v, want [core:a]", rooms)
	}
	if _, ok := store.GetByID(old); ok {
		t.Error("expired campfire still tracked")
	}
	if _, ok := placement.RoomOf(old); ok {
		t.Error("expired campfire still placed")
	}
	if _, ok := store.GetByID(fresh); !ok {
		t.Error("fresh campfire was wrongly swept")
	}
}

func TestDecaySweep_NoClockSkewExpiry(t *testing.T) {
	store := entities.NewStore()
	placement := entities.NewPlacement()
	id, _ := Place(store, placement, world.RoomID("core:a"), 200) // created in the "future"
	store.SwapTagIndex()

	svc := NewService(store, placement)
	if rooms := svc.DecaySweep(context.Background(), 100, 50); len(rooms) != 0 {
		t.Errorf("clock-skew (now<created) should not expire: %v", rooms)
	}
	if _, ok := store.GetByID(id); !ok {
		t.Error("campfire wrongly swept under clock skew")
	}
}
