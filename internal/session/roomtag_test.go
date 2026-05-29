package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// connActor.HasRoomTag now reads the current room's Tags (cluster 2) —
// the training safe-room gate (progression §7.4, tag "safe") consumes
// it. Previously a hard-coded false.
func TestConnActorHasRoomTag(t *testing.T) {
	safe := &world.Room{ID: "x:1", Tags: []string{"safe"}}
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", safe)
	if !a.HasRoomTag("safe") {
		t.Error("HasRoomTag(safe) = false in a safe room, want true")
	}
	if a.HasRoomTag("indoors") {
		t.Error("HasRoomTag(indoors) = true, want false")
	}

	// A room without the tag reports false.
	plain := &world.Room{ID: "x:2"}
	b, _ := newFakeActor("c2", "p2", "acc1", "Bob", plain)
	if b.HasRoomTag("safe") {
		t.Error("HasRoomTag(safe) = true in an untagged room, want false")
	}

	// nil room (pre-spawn / detached) is safe and false. Construct a
	// bare actor — newFakeActor dereferences the room for the save.
	if (&connActor{}).HasRoomTag("safe") {
		t.Error("HasRoomTag on a nil room should be false")
	}
}
