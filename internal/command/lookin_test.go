package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

func emptySackTpl() *item.Template {
	return &item.Template{
		ID: "tapestry-core:empty-sack", Name: "a burlap sack",
		Type: entities.ContainerType, Keywords: []string{"sack", "burlap"},
	}
}

func TestLook_InCorpseListsContentsAndCoins(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	f.placeCorpse(t, []string{"player:p-alice"}, 100, 6, ration(), sword())

	dispatchLoot(t, f, a, "look in corpse")
	out := a.lastLine()
	if !strings.Contains(out, "contains") {
		t.Fatalf("look-in output = %q, want a contents listing", out)
	}
	if !strings.Contains(out, "ration") || !strings.Contains(out, "6 gold") {
		t.Errorf("look-in should list items + coins, got %q", out)
	}
}

// Looking is gated by presence only — a non-owner can see what a corpse
// holds during the ownership window (only taking is restricted).
func TestLook_InCorpseAllowedForNonOwner(t *testing.T) {
	f := newLootFixture(t)
	eve := newNamedTestActor("Eve", "p-eve", f.room)
	f.placeCorpse(t, []string{"player:p-alice"}, 100, 6, ration())

	dispatchLoot(t, f, eve, "look corpse")
	if !strings.Contains(eve.lastLine(), "ration") {
		t.Errorf("non-owner look-in should still show contents, got %q", eve.lastLine())
	}
}

func TestLook_InEmptyContainer(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	sack, err := f.store.Spawn(emptySackTpl())
	if err != nil {
		t.Fatalf("spawn sack: %v", err)
	}
	f.place.Place(sack.ID(), f.room.ID)

	dispatchLoot(t, f, a, "look in sack")
	if !strings.Contains(a.lastLine(), "empty") {
		t.Errorf("empty container look = %q, want 'is empty'", a.lastLine())
	}
}

func TestLook_AtPlainItem(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	sw := f.spawnInRoom(t, sword())
	_ = sw
	dispatchLoot(t, f, a, "look sword")
	if !strings.Contains(a.lastLine(), "You see") {
		t.Errorf("look at item = %q, want a 'You see ...' line", a.lastLine())
	}
}

func TestLook_NoArgRendersRoom(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	dispatchLoot(t, f, a, "look")
	// Room render contains the room name (set up by the fixture's room).
	if a.lastLine() == "" {
		t.Error("bare look produced no room render")
	}
}

func TestLook_TargetNotFound(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	dispatchLoot(t, f, a, "look unicorn")
	if !strings.Contains(a.lastLine(), "don't see that") {
		t.Errorf("missing target = %q, want a not-found message", a.lastLine())
	}
}
