package session

import (
	"reflect"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// SetRoom is the room-entry chokepoint: every arrival marks the
// destination visited, deduped, and dirties the save (player-maps §3).
func TestSetRoom_MarksVisited(t *testing.T) {
	a := &connActor{save: &player.Save{ID: "p-1", Name: "Mapper"}}

	a.SetRoom(&world.Room{ID: "core:a"})
	a.SetRoom(&world.Room{ID: "core:b"})
	a.SetRoom(&world.Room{ID: "core:a"}) // re-entry must not duplicate

	if got, want := a.VisitedRooms(), []string{"core:a", "core:b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("VisitedRooms = %v, want %v", got, want)
	}
	if !a.HasVisited("core:a") || !a.HasVisited("core:b") {
		t.Error("HasVisited should be true for entered rooms")
	}
	if a.HasVisited("core:z") {
		t.Error("HasVisited(core:z) = true, want false for an unentered room")
	}
	if !a.dirty {
		t.Error("save should be dirty after a new room is visited")
	}
}

// The in-memory set seeds from the persisted save, so a returning
// character's prior exploration is honored and not re-appended.
func TestVisited_SeededFromSave(t *testing.T) {
	a := &connActor{save: &player.Save{VisitedRooms: []string{"core:x", "core:y"}}}

	if !a.HasVisited("core:x") || !a.HasVisited("core:y") {
		t.Error("visited set should seed from the persisted save")
	}
	a.SetRoom(&world.Room{ID: "core:x"}) // already visited
	if got := a.VisitedRooms(); len(got) != 2 {
		t.Errorf("VisitedRooms = %v, want 2 (no duplicate of a seeded room)", got)
	}
}

// An ephemeral actor with no save tracks nothing and never panics.
func TestVisited_NilSaveNoop(t *testing.T) {
	a := &connActor{}
	a.SetRoom(&world.Room{ID: "core:a"})
	if a.HasVisited("core:a") {
		t.Error("an actor with no save should not track visits")
	}
	if a.VisitedRooms() != nil {
		t.Error("VisitedRooms should be nil with no save")
	}
}

// MarkAreaSeen is the once-ever first-entry gate: a new area dirties the
// save and persists; a repeat is a no-op and leaves the save clean.
func TestSeenAreas_MarkAndQuery(t *testing.T) {
	a := &connActor{save: &player.Save{ID: "p-a", Name: "Walker"}}

	if a.HasSeenArea("village") {
		t.Error("fresh character should not have seen any area")
	}
	a.MarkAreaSeen("village")
	if !a.HasSeenArea("village") {
		t.Error("village should be seen after MarkAreaSeen")
	}
	if !a.dirty {
		t.Error("first MarkAreaSeen should dirty the save")
	}
	if want := []string{"village"}; !reflect.DeepEqual(a.save.SeenAreas, want) {
		t.Errorf("save.SeenAreas = %v, want %v", a.save.SeenAreas, want)
	}

	a.dirty = false
	a.MarkAreaSeen("village") // repeat
	if a.dirty {
		t.Error("re-marking a seen area should not dirty the save")
	}
	if len(a.save.SeenAreas) != 1 {
		t.Errorf("save.SeenAreas grew on repeat: %v", a.save.SeenAreas)
	}
}

// AreaTransition folds the LastAreaSeen/SetLastAreaSeen/HasSeenArea/
// MarkAreaSeen check-then-act into one critical section. It must report
// the same crossing/first-entry decisions the four separate accessors
// did, and mutate atomically.
func TestAreaTransition_Decisions(t *testing.T) {
	a := &connActor{save: &player.Save{ID: "p-t", Name: "Crosser"}}

	// First render: no prior area, entering "village" for the first time.
	prev, changed, first := a.AreaTransition("village")
	if prev != "" || !changed || !first {
		t.Errorf("first render = (%q, %v, %v), want (\"\", true, true)", prev, changed, first)
	}
	if !a.dirty || !reflect.DeepEqual(a.save.SeenAreas, []string{"village"}) {
		t.Errorf("first entry should persist+dirty; SeenAreas=%v dirty=%v", a.save.SeenAreas, a.dirty)
	}

	// Intra-area render (same area): no change, no mutation.
	a.dirty = false
	prev, changed, first = a.AreaTransition("village")
	if prev != "village" || changed || first {
		t.Errorf("same-area = (%q, %v, %v), want (\"village\", false, false)", prev, changed, first)
	}
	if a.dirty {
		t.Error("an intra-area render must not dirty the save")
	}

	// Cross to a new area: changed, first-entry, from = village.
	prev, changed, first = a.AreaTransition("wild")
	if prev != "village" || !changed || !first {
		t.Errorf("cross to new area = (%q, %v, %v), want (\"village\", true, true)", prev, changed, first)
	}

	// Cross back to an already-seen area: changed but NOT first-entry, and
	// no duplicate append.
	a.dirty = false
	prev, changed, first = a.AreaTransition("village")
	if prev != "wild" || !changed || first {
		t.Errorf("cross to seen area = (%q, %v, %v), want (\"wild\", true, false)", prev, changed, first)
	}
	if a.dirty {
		t.Error("re-entering a seen area must not dirty the save")
	}
	if want := []string{"village", "wild"}; !reflect.DeepEqual(a.save.SeenAreas, want) {
		t.Errorf("SeenAreas = %v, want %v (no duplicate)", a.save.SeenAreas, want)
	}
}

// An ephemeral actor (no save) still reports crossings and first-entry
// (nothing persists), matching the old HasSeenArea==false path, and never
// panics.
func TestAreaTransition_NilSave(t *testing.T) {
	a := &connActor{}
	prev, changed, first := a.AreaTransition("wild")
	if prev != "" || !changed || !first {
		t.Errorf("nil-save cross = (%q, %v, %v), want (\"\", true, true)", prev, changed, first)
	}
}

// AreaTransition is the atomic replacement for the four-accessor
// check-then-act: concurrent crossings into the same new area must yield
// exactly one first-entry and append the area exactly once (no
// double-fire). Run under -race.
func TestAreaTransition_ConcurrentFirstEntryIsOnce(t *testing.T) {
	a := &connActor{save: &player.Save{ID: "p-c", Name: "Racer"}}

	const n = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	changedCount, firstCount := 0, 0
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, changed, first := a.AreaTransition("wild")
			mu.Lock()
			if changed {
				changedCount++
			}
			if first {
				firstCount++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if changedCount != 1 {
		t.Errorf("changed fired %d times, want exactly 1 (the winning crossing)", changedCount)
	}
	if firstCount != 1 {
		t.Errorf("first-entry fired %d times, want exactly 1", firstCount)
	}
	if want := []string{"wild"}; !reflect.DeepEqual(a.save.SeenAreas, want) {
		t.Errorf("SeenAreas = %v, want %v (no double append)", a.save.SeenAreas, want)
	}
}

// An unset size preset reads as "auto", and storing "auto" on a fresh
// character must NOT dirty the save — the empty stored value and "auto"
// are equivalent (both mean "scale to terminal"), unlike the bool toggle
// whose zero value already equals its default.
func TestMinimapSize_AutoOnFreshCharIsNoWrite(t *testing.T) {
	a := &connActor{save: &player.Save{ID: "p-2", Name: "Sizer"}}

	if got := a.MinimapSize(); got != "auto" {
		t.Errorf("unset MinimapSize = %q, want auto", got)
	}
	a.SetMinimapSize("auto")
	if a.dirty {
		t.Error("setting auto on a character with no stored size should not dirty the save")
	}
	if a.save.MinimapSize != "" {
		t.Errorf("save MinimapSize = %q, want \"\" (auto stays unstored)", a.save.MinimapSize)
	}

	// A real change dirties and persists.
	a.SetMinimapSize("large")
	if !a.dirty {
		t.Error("changing the size should dirty the save")
	}
	if a.MinimapSize() != "large" {
		t.Errorf("MinimapSize = %q, want large", a.MinimapSize())
	}

	// Returning to auto from a stored value IS a change and persists.
	a.dirty = false
	a.SetMinimapSize("auto")
	if !a.dirty {
		t.Error("changing large->auto should dirty the save")
	}
	if a.MinimapSize() != "auto" {
		t.Errorf("MinimapSize = %q, want auto after reset", a.MinimapSize())
	}
}
