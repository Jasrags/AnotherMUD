package entities

import (
	"reflect"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

func TestPlacementPlaceAndRoomOf(t *testing.T) {
	p := NewPlacement()
	p.Place("a", "tapestry-core:town-square")
	got, ok := p.RoomOf("a")
	if !ok || got != "tapestry-core:town-square" {
		t.Fatalf("RoomOf(a) = %q,%v", got, ok)
	}
	if _, ok := p.RoomOf("missing"); ok {
		t.Error("RoomOf missing returned ok")
	}
}

func TestPlacementInsertionOrderPreserved(t *testing.T) {
	p := NewPlacement()
	for _, id := range []EntityID{"a", "b", "c"} {
		p.Place(id, "r")
	}
	got := p.InRoom("r")
	want := []EntityID{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("InRoom = %v, want %v", got, want)
	}
}

func TestPlacementSameRoomIsNoop(t *testing.T) {
	p := NewPlacement()
	p.Place("a", "r")
	p.Place("a", "r")
	if got := p.InRoom("r"); len(got) != 1 {
		t.Errorf("InRoom = %v, want single entry", got)
	}
}

func TestPlacementMoveBetweenRooms(t *testing.T) {
	p := NewPlacement()
	p.Place("a", "r1")
	p.Place("b", "r1")
	p.Place("a", "r2") // a leaves r1, lands in r2

	if got := p.InRoom("r1"); !reflect.DeepEqual(got, []EntityID{"b"}) {
		t.Errorf("r1 after move = %v, want [b]", got)
	}
	if got := p.InRoom("r2"); !reflect.DeepEqual(got, []EntityID{"a"}) {
		t.Errorf("r2 after move = %v, want [a]", got)
	}
	if got, _ := p.RoomOf("a"); got != "r2" {
		t.Errorf("RoomOf(a) = %q, want r2", got)
	}
}

func TestPlacementRemove(t *testing.T) {
	p := NewPlacement()
	p.Place("a", "r")
	p.Place("b", "r")

	if !p.Remove("a") {
		t.Error("Remove(a) = false, want true")
	}
	if p.Remove("a") {
		t.Error("second Remove(a) = true, want false")
	}
	if got := p.InRoom("r"); !reflect.DeepEqual(got, []EntityID{"b"}) {
		t.Errorf("InRoom after remove = %v, want [b]", got)
	}
	// Removing the last item prunes the bucket entirely.
	p.Remove("b")
	if got := p.InRoom("r"); got != nil {
		t.Errorf("InRoom of pruned room = %v, want nil", got)
	}
}

func TestPlacementInRoomReturnsSnapshot(t *testing.T) {
	p := NewPlacement()
	p.Place("a", "r")
	snap := p.InRoom("r")
	snap[0] = "MUTATED"
	if got := p.InRoom("r"); got[0] != "a" {
		t.Errorf("internal state aliased; second InRoom = %v", got)
	}
}

func TestPlacementConcurrentSafe(t *testing.T) {
	// Smoke test under -race: many goroutines placing, moving, and
	// removing should not corrupt internal indices.
	p := NewPlacement()
	const rooms = 4
	const workers = 16
	const ops = 200

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			id := EntityID(rune('a' + w))
			for i := 0; i < ops; i++ {
				r := world.RoomID(rune('0' + (i % rooms)))
				p.Place(id, r)
				_ = p.InRoom(r)
				_, _ = p.RoomOf(id)
			}
			p.Remove(id)
		}(w)
	}
	wg.Wait()
}

func TestStoreRoomScanWiresThroughPlacement(t *testing.T) {
	// Demonstrates the intended composition: Placement is the room-side
	// truth, Store falls back to it for byID lookups when the tracked
	// index misses (per spec §4.2 step 2).
	s := NewStore()
	p := NewPlacement()
	stray := &fakeEntity{id: "stray", typ: "item"}

	// Stray is "in a room" but not in the byID index — simulates a
	// scenario where placement was recorded but tracking was skipped or
	// dropped.
	p.Place(stray.ID(), "r")
	s.SetRoomScan(func(id EntityID) (Entity, bool) {
		if _, ok := p.RoomOf(id); !ok {
			return nil, false
		}
		if id == stray.ID() {
			return stray, true
		}
		return nil, false
	})

	got, ok := s.GetByID("stray")
	if !ok || got != stray {
		t.Errorf("GetByID via placement scan: ok=%v got=%v", ok, got)
	}
}
