package scrap

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// place mints an item, drops it in a room, marks it as scrap at droppedTick, and
// surfaces the scrap tag to the read index the sweep queries.
func place(t *testing.T, store *entities.Store, placement *entities.Placement, room world.RoomID, droppedTick uint64) *entities.ItemInstance {
	t.Helper()
	it, err := store.Spawn(&item.Template{ID: "sr:predator-clip", Name: "a clip", Type: "item"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	placement.Place(it.ID(), room)
	Mark(store, it, droppedTick)
	store.SwapTagIndex()
	return it
}

func TestSweep_RemovesExpiredScrap(t *testing.T) {
	store, placement := entities.NewStore(), entities.NewPlacement()
	it := place(t, store, placement, "sr:corner", 100)

	// now=150, lifetime=100 → 50 < 100: not yet expired.
	if n := Sweep(store, placement, 150, 100); n != 0 {
		t.Fatalf("Sweep removed %d before expiry, want 0", n)
	}
	if _, ok := placement.RoomOf(it.ID()); !ok {
		t.Fatal("scrap was removed from the room before expiry")
	}

	// now=200, lifetime=100 → 100 >= 100: expired.
	if n := Sweep(store, placement, 200, 100); n != 1 {
		t.Fatalf("Sweep removed %d at expiry, want 1", n)
	}
	if _, ok := placement.RoomOf(it.ID()); ok {
		t.Fatal("expired scrap was not removed from the room")
	}
	if _, ok := store.GetByID(it.ID()); ok {
		t.Fatal("expired scrap was not untracked from the store")
	}
}

func TestSweep_SkipsPickedUpScrap(t *testing.T) {
	store, placement := entities.NewStore(), entities.NewPlacement()
	it := place(t, store, placement, "sr:corner", 100)
	// Simulate a player picking it up: it leaves the room (into inventory).
	placement.Remove(it.ID())

	// Long past its lifetime, but it's not on the ground — the sweep skips it,
	// so a recovered clip survives in the finder's hands.
	if n := Sweep(store, placement, 10_000, 100); n != 0 {
		t.Fatalf("Sweep removed %d picked-up scrap, want 0", n)
	}
	if _, ok := store.GetByID(it.ID()); !ok {
		t.Fatal("a picked-up clip was wrongly untracked by the sweep")
	}
}

func TestSweep_ClockSkewNotExpired(t *testing.T) {
	store, placement := entities.NewStore(), entities.NewPlacement()
	place(t, store, placement, "sr:corner", 500) // dropped in the "future"
	if n := Sweep(store, placement, 100, 50); n != 0 {
		t.Fatalf("Sweep removed %d under clock skew, want 0", n)
	}
}

func TestSweep_NoScrap(t *testing.T) {
	store, placement := entities.NewStore(), entities.NewPlacement()
	if n := Sweep(store, placement, 1000, 100); n != 0 {
		t.Fatalf("Sweep removed %d with no scrap, want 0", n)
	}
}
