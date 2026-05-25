package command_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// stubLocator is a test fixture that resolves names against a fixed
// pool of actors. Pairs with newNamedTestActor in command_test.go.
type stubLocator struct {
	mu     sync.Mutex
	actors []command.Actor
}

func (s *stubLocator) add(a command.Actor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actors = append(s.actors, a)
}

func (s *stubLocator) FindInRoom(roomID world.RoomID, name string) command.Actor {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return nil
	}
	for _, a := range s.actors {
		room := a.Room()
		if room == nil || room.ID != roomID {
			continue
		}
		if strings.ToLower(a.Name()) == want {
			return a
		}
	}
	return nil
}

// giveFixture wires up the substrate needed for two-actor transfer
// tests: world + store + placement + locator + bus.
type giveFixture struct {
	*invFixture
	locator *stubLocator
	bus     *eventbus.Bus
}

func newGiveFixture(t *testing.T) *giveFixture {
	t.Helper()
	return &giveFixture{
		invFixture: newInvFixture(t),
		locator:    &stubLocator{},
		bus:        eventbus.New(),
	}
}

func (f *giveFixture) env() command.Env {
	e := f.invFixture.env()
	e.Locator = f.locator
	e.Bus = f.bus
	return e
}

// spawnInInventory makes a fresh item and places it directly in the
// actor's inventory (skipping the room → get round-trip that the
// inventory tests use).
func (f *giveFixture) spawnInInventory(t *testing.T, a *testActor) *entities.ItemInstance {
	t.Helper()
	inst, err := f.store.Spawn(sword())
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a.AddToInventory(inst.ID())
	return inst
}

func dispatchGive(t *testing.T, f *giveFixture, a *testActor, input string) {
	t.Helper()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), f.env(), a, input); err != nil {
		t.Fatalf("dispatch %q: %v", input, err)
	}
}

func TestGive_HappyPath_MovesItemAndEmitsEvent(t *testing.T) {
	f := newGiveFixture(t)
	alice := newNamedTestActor("Alice", "p-alice", f.room)
	bob := newNamedTestActor("Bob", "p-bob", f.room)
	f.locator.add(alice)
	f.locator.add(bob)
	inst := f.spawnInInventory(t, alice)

	var got []eventbus.ItemGiven
	f.bus.Subscribe(eventbus.EventItemGiven, func(_ context.Context, e eventbus.Event) {
		got = append(got, e.(eventbus.ItemGiven))
	})

	dispatchGive(t, f, alice, "give sword bob")

	if n := len(alice.Inventory()); n != 0 {
		t.Errorf("alice still has %d item(s) after give", n)
	}
	bobInv := bob.Inventory()
	if len(bobInv) != 1 || bobInv[0] != inst.ID() {
		t.Errorf("bob inventory = %v, want [%q]", bobInv, inst.ID())
	}
	if last := alice.lastLine(); !strings.Contains(last, "You give") {
		t.Errorf("alice reply = %q, want 'You give ...'", last)
	}
	if last := bob.lastLine(); !strings.Contains(last, "Alice gives you") {
		t.Errorf("bob reply = %q, want 'Alice gives you ...'", last)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 ItemGiven event, got %d", len(got))
	}
	ev := got[0]
	if ev.ItemID != inst.ID() {
		t.Errorf("event ItemID = %q, want %q", ev.ItemID, inst.ID())
	}
	if ev.ItemName != inst.Name() {
		t.Errorf("event ItemName = %q, want %q", ev.ItemName, inst.Name())
	}
	if ev.RoomID != f.room.ID {
		t.Errorf("event RoomID = %q, want %q", ev.RoomID, f.room.ID)
	}
	if ev.TemplateID == "" {
		t.Error("event TemplateID empty")
	}
}

func TestGive_AcceptsToPreposition(t *testing.T) {
	f := newGiveFixture(t)
	alice := newNamedTestActor("Alice", "p-alice", f.room)
	bob := newNamedTestActor("Bob", "p-bob", f.room)
	f.locator.add(alice)
	f.locator.add(bob)
	f.spawnInInventory(t, alice)

	dispatchGive(t, f, alice, "give sword to bob")
	if len(bob.Inventory()) != 1 {
		t.Errorf("bob did not receive item via 'to' form: %v", bob.Inventory())
	}
}

func TestGive_TargetNotInRoom(t *testing.T) {
	f := newGiveFixture(t)
	alice := newNamedTestActor("Alice", "p-alice", f.room)
	f.locator.add(alice)
	inst := f.spawnInInventory(t, alice)

	emitted := false
	f.bus.Subscribe(eventbus.EventItemGiven, func(context.Context, eventbus.Event) { emitted = true })

	dispatchGive(t, f, alice, "give sword ghost")

	if len(alice.Inventory()) != 1 || alice.Inventory()[0] != inst.ID() {
		t.Errorf("inventory mutated on failed give: %v", alice.Inventory())
	}
	if !strings.Contains(alice.lastLine(), "ghost") {
		t.Errorf("reply did not name missing target: %q", alice.lastLine())
	}
	if emitted {
		t.Error("ItemGiven emitted on failure")
	}
}

func TestGive_TargetInDifferentRoom(t *testing.T) {
	f := newGiveFixture(t)
	otherRoom := &world.Room{ID: "tapestry-core:forge", Name: "Forge", Description: "x"}
	f.world.AddRoom(otherRoom)
	alice := newNamedTestActor("Alice", "p-alice", f.room)
	bob := newNamedTestActor("Bob", "p-bob", otherRoom)
	f.locator.add(alice)
	f.locator.add(bob)
	f.spawnInInventory(t, alice)

	dispatchGive(t, f, alice, "give sword bob")
	if len(bob.Inventory()) != 0 {
		t.Errorf("bob received item from across the world: %v", bob.Inventory())
	}
}

func TestGive_RejectsSelfGive(t *testing.T) {
	f := newGiveFixture(t)
	alice := newNamedTestActor("Alice", "p-alice", f.room)
	f.locator.add(alice)
	inst := f.spawnInInventory(t, alice)

	dispatchGive(t, f, alice, "give sword alice")
	if len(alice.Inventory()) != 1 || alice.Inventory()[0] != inst.ID() {
		t.Errorf("self-give moved item: %v", alice.Inventory())
	}
	if !strings.Contains(alice.lastLine(), "yourself") {
		t.Errorf("reply = %q, want 'yourself' message", alice.lastLine())
	}
}

func TestGive_NotInInventory(t *testing.T) {
	f := newGiveFixture(t)
	alice := newNamedTestActor("Alice", "p-alice", f.room)
	bob := newNamedTestActor("Bob", "p-bob", f.room)
	f.locator.add(alice)
	f.locator.add(bob)
	// Item exists in the world but in the room, not in alice's bag.
	f.spawnInRoom(t, sword())

	dispatchGive(t, f, alice, "give sword bob")
	if len(bob.Inventory()) != 0 {
		t.Errorf("bob received an item alice didn't have: %v", bob.Inventory())
	}
	if !strings.Contains(alice.lastLine(), "aren't carrying") {
		t.Errorf("reply = %q, want 'aren't carrying' message", alice.lastLine())
	}
}

func TestGive_MissingArgs(t *testing.T) {
	f := newGiveFixture(t)
	alice := newNamedTestActor("Alice", "p-alice", f.room)
	f.locator.add(alice)

	for _, input := range []string{"give", "give sword", "give sword to"} {
		input := input
		t.Run(input, func(t *testing.T) {
			alice.lines = nil
			dispatchGive(t, f, alice, input)
			last := alice.lastLine()
			if !strings.Contains(last, "Give what") {
				t.Errorf("%q reply = %q, want usage message", input, last)
			}
		})
	}
}

// TestGive_ConcurrentExchange exercises two senders giving items to
// each other simultaneously. The handler does not hold both actor
// locks at once, but the race detector should still come back clean:
// each connActor mutex serializes its own mutations, and the
// transfer is just remove-then-add on disjoint actors.
func TestGive_ConcurrentExchange(t *testing.T) {
	f := newGiveFixture(t)
	alice := newNamedTestActor("Alice", "p-alice", f.room)
	bob := newNamedTestActor("Bob", "p-bob", f.room)
	f.locator.add(alice)
	f.locator.add(bob)

	const itemsEach = 25
	for i := 0; i < itemsEach; i++ {
		f.spawnInInventory(t, alice)
		f.spawnInInventory(t, bob)
	}

	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < itemsEach; i++ {
			_ = r.Dispatch(context.Background(), f.env(), alice, "give sword bob")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < itemsEach; i++ {
			_ = r.Dispatch(context.Background(), f.env(), bob, "give sword alice")
		}
	}()
	wg.Wait()

	// Conservation: total items across both inventories stays at
	// 2 * itemsEach, no matter how the interleaving played out.
	total := len(alice.Inventory()) + len(bob.Inventory())
	if total != 2*itemsEach {
		t.Errorf("item count drifted to %d, want %d", total, 2*itemsEach)
	}
}
