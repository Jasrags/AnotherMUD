package session

import (
	"reflect"
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
