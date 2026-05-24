package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// invFixture wires up the minimum (room + store + placement +
// templates) needed to exercise get/drop end-to-end.
type invFixture struct {
	world *world.World
	room  *world.Room
	store *entities.Store
	place *entities.Placement
}

func newInvFixture(t *testing.T) *invFixture {
	t.Helper()
	w := world.New()
	r := &world.Room{ID: "tapestry-core:town-square", Name: "Square", Description: "x"}
	w.AddRoom(r)
	return &invFixture{
		world: w,
		room:  r,
		store: entities.NewStore(),
		place: entities.NewPlacement(),
	}
}

func (f *invFixture) spawnInRoom(t *testing.T, tpl *item.Template) *entities.ItemInstance {
	t.Helper()
	inst, err := f.store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	f.place.Place(inst.ID(), f.room.ID)
	return inst
}

func (f *invFixture) env() command.Env {
	return command.Env{World: f.world, Items: f.store, Placement: f.place}
}

func sword() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:short-sword",
		Name:     "a short sword",
		Type:     "weapon",
		Keywords: []string{"sword", "short"},
	}
}

func fixtureStatue() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:statue",
		Name:     "a marble statue",
		Type:     "decoration",
		Tags:     []string{"fixture"},
		Keywords: []string{"statue"},
	}
}

func TestGet_MovesItemRoomToInventory(t *testing.T) {
	f := newInvFixture(t)
	inst := f.spawnInRoom(t, sword())
	a := newTestActor(f.room)

	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), f.env(), a, "get sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !strings.Contains(a.lastLine(), "pick up") {
		t.Fatalf("unexpected reply: %q", a.lastLine())
	}
	if got := a.Inventory(); len(got) != 1 || got[0] != inst.ID() {
		t.Errorf("inventory = %v, want [%q]", got, inst.ID())
	}
	if _, ok := f.place.RoomOf(inst.ID()); ok {
		t.Error("item still placed in room after get")
	}
}

func TestGet_FixtureTagRejected(t *testing.T) {
	f := newInvFixture(t)
	inst := f.spawnInRoom(t, fixtureStatue())
	a := newTestActor(f.room)

	r := command.New()
	_ = command.RegisterBuiltins(r)
	if err := r.Dispatch(context.Background(), f.env(), a, "get statue"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !strings.Contains(a.lastLine(), "can't take") {
		t.Errorf("expected rejection, got: %q", a.lastLine())
	}
	if got := a.Inventory(); len(got) != 0 {
		t.Errorf("inventory = %v, want empty", got)
	}
	if got, _ := f.place.RoomOf(inst.ID()); got != f.room.ID {
		t.Errorf("placement = %q, want %q (item stayed in room)", got, f.room.ID)
	}
}

func TestGet_NoMatchDoesNotMutate(t *testing.T) {
	f := newInvFixture(t)
	a := newTestActor(f.room)

	r := command.New()
	_ = command.RegisterBuiltins(r)
	if err := r.Dispatch(context.Background(), f.env(), a, "get unicorn"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !strings.Contains(a.lastLine(), "nothing here to get") {
		t.Errorf("got %q", a.lastLine())
	}
}

func TestGet_ArgumentRequired(t *testing.T) {
	f := newInvFixture(t)
	a := newTestActor(f.room)

	r := command.New()
	_ = command.RegisterBuiltins(r)
	if err := r.Dispatch(context.Background(), f.env(), a, "get"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if a.lastLine() != "Get what?" {
		t.Errorf("got %q, want 'Get what?'", a.lastLine())
	}
}

func TestDrop_MovesItemInventoryToRoom(t *testing.T) {
	f := newInvFixture(t)
	inst, err := f.store.Spawn(sword())
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	a := newTestActor(f.room)
	a.AddToInventory(inst.ID())

	r := command.New()
	_ = command.RegisterBuiltins(r)
	if err := r.Dispatch(context.Background(), f.env(), a, "drop sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !strings.Contains(a.lastLine(), "drop") {
		t.Errorf("got %q", a.lastLine())
	}
	if len(a.Inventory()) != 0 {
		t.Errorf("inventory not emptied: %v", a.Inventory())
	}
	got, ok := f.place.RoomOf(inst.ID())
	if !ok || got != f.room.ID {
		t.Errorf("placement = %q,%v, want %q,true", got, ok, f.room.ID)
	}
}

func TestDrop_NotCarryingThat(t *testing.T) {
	f := newInvFixture(t)
	a := newTestActor(f.room)
	a.AddToInventory("some-other-id")
	// Track the entity so collectItems can resolve at least something.
	_, _ = f.store.Spawn(sword())

	r := command.New()
	_ = command.RegisterBuiltins(r)
	if err := r.Dispatch(context.Background(), f.env(), a, "drop sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !strings.Contains(a.lastLine(), "aren't carrying") {
		t.Errorf("got %q", a.lastLine())
	}
}

func TestGet_OrdinalSelection(t *testing.T) {
	// Two swords in the room; "get 2.sword" must take the second.
	f := newInvFixture(t)
	first := f.spawnInRoom(t, sword())
	second := f.spawnInRoom(t, sword())
	a := newTestActor(f.room)

	r := command.New()
	_ = command.RegisterBuiltins(r)
	if err := r.Dispatch(context.Background(), f.env(), a, "get 2.sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	inv := a.Inventory()
	if len(inv) != 1 || inv[0] != second.ID() {
		t.Errorf("inventory = %v, want [%q] (second sword)", inv, second.ID())
	}
	if _, ok := f.place.RoomOf(first.ID()); !ok {
		t.Error("first sword should still be in room")
	}
}
