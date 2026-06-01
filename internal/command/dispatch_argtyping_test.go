package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// These tests cover the M17.2d₂ dispatch integration (Option A): a
// command that declares Args gets its arguments resolved by the
// dispatcher before the handler runs; on a resolution failure the
// dispatcher writes the resolver's message and the handler never
// executes; a command with no declared Args reads raw c.Args as before.

func TestDispatch_DeclaredArgs_PopulatesResolved(t *testing.T) {
	f := newInvFixture(t)
	inst, err := f.store.Spawn(sword())
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a := newNamedTestActor("Alice", "p-1", f.room)
	a.AddToInventory(inst.ID())

	var gotID string
	var handlerRan bool
	r := command.New()
	if err := r.RegisterCommand(command.Command{
		Keyword: "poke",
		Handler: func(ctx context.Context, c *command.Context) error {
			handlerRan = true
			if ref, ok := c.Resolved["item"].(command.ItemRef); ok {
				gotID = ref.ID
			}
			return nil
		},
		Args: []command.ArgDefinition{{Name: "item", Type: command.ArgInventory}},
	}); err != nil {
		t.Fatalf("RegisterCommand: %v", err)
	}

	if err := r.Dispatch(context.Background(), f.env(), a, "poke sword"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !handlerRan {
		t.Fatal("handler did not run")
	}
	if gotID != string(inst.ID()) {
		t.Errorf("Resolved[item].ID = %q, want %q", gotID, inst.ID())
	}
}

func TestDispatch_ResolutionFailure_ShortCircuitsHandler(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Alice", "p-1", f.room) // empty inventory

	var handlerRan bool
	r := command.New()
	_ = r.RegisterCommand(command.Command{
		Keyword: "poke",
		Handler: func(ctx context.Context, c *command.Context) error {
			handlerRan = true
			return nil
		},
		Args: []command.ArgDefinition{{Name: "item", Type: command.ArgInventory}},
	})

	if err := r.Dispatch(context.Background(), f.env(), a, "poke sword"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if handlerRan {
		t.Error("handler ran despite resolution failure")
	}
	if !strings.Contains(a.lastLine(), "carrying that") {
		t.Errorf("expected resolver error written, got %q", a.lastLine())
	}
}

func TestDispatch_MissingRequiredArg_WritesWhatPrompt(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Alice", "p-1", f.room)

	var handlerRan bool
	r := command.New()
	_ = r.RegisterCommand(command.Command{
		Keyword: "poke",
		Handler: func(ctx context.Context, c *command.Context) error {
			handlerRan = true
			return nil
		},
		Args: []command.ArgDefinition{{Name: "item", Type: command.ArgInventory}},
	})

	if err := r.Dispatch(context.Background(), f.env(), a, "poke"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if handlerRan {
		t.Error("handler ran with no argument")
	}
	// §5.4: missing required arg → "What <name>?".
	if !strings.Contains(a.lastLine(), "What item?") {
		t.Errorf("got %q, want the §5.4 What-item prompt", a.lastLine())
	}
}

func TestDispatch_NoDeclaredArgs_LegacyPassthrough(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Alice", "p-1", f.room)

	var sawArgs []string
	var sawResolved map[string]any
	r := command.New()
	_ = r.RegisterCommand(command.Command{
		Keyword: "shout",
		Handler: func(ctx context.Context, c *command.Context) error {
			sawArgs = c.Args
			sawResolved = c.Resolved
			return nil
		},
		// No Args declared.
	})

	if err := r.Dispatch(context.Background(), f.env(), a, "shout hello there"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(sawArgs) != 2 || sawArgs[0] != "hello" {
		t.Errorf("c.Args = %v, want [hello there]", sawArgs)
	}
	if sawResolved != nil {
		t.Errorf("c.Resolved = %v, want nil for an un-migrated handler", sawResolved)
	}
}

// --- the migrated drop verb, end-to-end through RegisterBuiltins ---

func TestDrop_Migrated_Success(t *testing.T) {
	f := newInvFixture(t)
	inst, err := f.store.Spawn(sword())
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a := newNamedTestActor("Alice", "p-1", f.room)
	a.AddToInventory(inst.ID())

	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), f.env(), a, "drop sword"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !strings.Contains(a.lastLine(), "You drop") {
		t.Errorf("got %q, want a drop confirmation", a.lastLine())
	}
	if got := a.Inventory(); len(got) != 0 {
		t.Errorf("inventory = %v, want empty after drop", got)
	}
	if _, ok := f.place.RoomOf(inst.ID()); !ok {
		t.Error("dropped item not placed in room")
	}
}

func TestDrop_Migrated_MissingArg_WhatItem(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Alice", "p-1", f.room)

	r := command.New()
	_ = command.RegisterBuiltins(r)
	if err := r.Dispatch(context.Background(), f.env(), a, "drop"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// Behavior change vs. the hand-parsed version ("Drop what?"): the
	// dispatcher now emits the §5.4-standard missing-arg prompt.
	if !strings.Contains(a.lastLine(), "What item?") {
		t.Errorf("got %q, want What-item prompt", a.lastLine())
	}
}

func TestGet_Migrated_OrdinalThroughDispatch(t *testing.T) {
	// Proves §5.5 ordinal selection works end-to-end through a
	// migrated verb in live dispatch, not just at the resolver layer.
	f := newInvFixture(t)
	first := f.spawnInRoom(t, sword())
	second := f.spawnInRoom(t, &item.Template{
		ID: "tapestry-core:sword-2", Name: "a rusty sword", Type: "weapon",
		Keywords: []string{"sword"},
	})
	a := newNamedTestActor("Alice", "p-1", f.room)

	r := command.New()
	_ = command.RegisterBuiltins(r)
	if err := r.Dispatch(context.Background(), f.env(), a, "get 2.sword"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	inv := a.Inventory()
	if len(inv) != 1 || inv[0] != second.ID() {
		t.Errorf("inventory = %v, want the 2nd sword %q", inv, second.ID())
	}
	if _, ok := f.place.RoomOf(first.ID()); !ok {
		t.Error("first sword should remain in the room")
	}
}

func TestDrop_Migrated_NotCarried(t *testing.T) {
	f := newInvFixture(t)
	other, err := f.store.Spawn(sword())
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a := newNamedTestActor("Alice", "p-1", f.room)
	a.AddToInventory(other.ID())

	r := command.New()
	_ = command.RegisterBuiltins(r)
	if err := r.Dispatch(context.Background(), f.env(), a, "drop lantern"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !strings.Contains(a.lastLine(), "aren't carrying that") {
		t.Errorf("got %q, want not-carried message", a.lastLine())
	}
	// The non-matching item stays in inventory.
	if got := a.Inventory(); len(got) != 1 {
		t.Errorf("inventory = %v, want the unmatched sword retained", got)
	}
}
