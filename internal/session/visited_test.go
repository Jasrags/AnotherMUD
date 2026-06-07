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
