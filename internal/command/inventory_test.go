package command_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
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
	// M17.2d₃: the room_item resolver reports its standardized
	// not-found copy ("You don't see that here.") instead of the
	// old hand-rolled "There is nothing here to get."
	if !strings.Contains(a.lastLine(), "don't see that here") {
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
	// M17.2d₃: dispatcher emits the §5.4 missing-arg prompt.
	if a.lastLine() != "What item?" {
		t.Errorf("got %q, want 'What item?'", a.lastLine())
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

func TestGet_ConcurrentGetsOnSameItemPickOneWinner(t *testing.T) {
	// Regression for M5-H1: two sessions in the same room both
	// resolve the same placement entry and race to claim it. The
	// fix treats Placement.Remove as the atomic ownership claim —
	// exactly one goroutine's Remove returns true, the loser's
	// returns false and surfaces "you don't see that here".
	//
	// Without the guard, both actors would AddToInventory the same
	// id, leaving the item duplicated across inventories with the
	// store / Placement showing it absent from the room. Stress
	// the path with many rounds to make the bug detectable even
	// when the race window is narrow.
	const rounds = 200
	const contenders = 4

	for round := range rounds {
		f := newInvFixture(t)
		inst := f.spawnInRoom(t, sword())

		// Independent actor per goroutine — they race for the same
		// placement entry, but their inventories are separate so
		// the test can count winners post-hoc.
		actors := make([]*testActor, contenders)
		for i := range actors {
			actors[i] = newTestActor(f.room)
		}

		r := command.New()
		if err := command.RegisterBuiltins(r); err != nil {
			t.Fatalf("RegisterBuiltins: %v", err)
		}

		// Use a start-gate so every goroutine launches its dispatch
		// at roughly the same instant, maximising race-detector
		// signal and pressure on Placement.Remove.
		var start sync.WaitGroup
		var done sync.WaitGroup
		start.Add(1)
		var winners int64
		for i := range contenders {
			done.Add(1)
			actor := actors[i]
			go func() {
				defer done.Done()
				start.Wait()
				if err := r.Dispatch(context.Background(), f.env(), actor, "get sword"); err != nil {
					t.Errorf("dispatch: %v", err)
					return
				}
				if len(actor.Inventory()) == 1 {
					atomic.AddInt64(&winners, 1)
				}
			}()
		}
		start.Done()
		done.Wait()

		if got := atomic.LoadInt64(&winners); got != 1 {
			t.Fatalf("round %d: winners = %d, want exactly 1 (item duplicated)", round, got)
		}
		// The placement index must not still show the item in the
		// room — the winning goroutine's Remove cleared it, and the
		// losers' Remove calls were no-ops.
		if _, ok := f.place.RoomOf(inst.ID()); ok {
			t.Fatalf("round %d: item still in placement after winner claimed it", round)
		}
	}
}
