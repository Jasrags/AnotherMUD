package command_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// Wiring tests for the M5 event emissions. Asserts that successful
// get/drop/equip/unequip publish the right concrete payload. The
// bus itself is tested in internal/eventbus; here we only verify
// that handlers reach into c.Bus.Publish at the right moments with
// the right values.

func captureEvents(t *testing.T, bus *eventbus.Bus, name string) *[]eventbus.Event {
	t.Helper()
	got := make([]eventbus.Event, 0)
	bus.Subscribe(name, func(ctx context.Context, e eventbus.Event) {
		got = append(got, e)
	})
	return &got
}

func TestGet_PublishesItemPickedUp(t *testing.T) {
	f := newInvFixture(t)
	inst := f.spawnInRoom(t, sword())
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}

	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventItemPickedUp)
	env := f.env()
	env.Bus = bus

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "get sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(*got) != 1 {
		t.Fatalf("event count = %d, want 1", len(*got))
	}
	ev := (*got)[0].(eventbus.ItemPickedUp)
	if ev.HolderID != entities.EntityID("p-1") {
		t.Errorf("HolderID = %q, want p-1", ev.HolderID)
	}
	if ev.RoomID != f.room.ID {
		t.Errorf("RoomID = %q, want %q", ev.RoomID, f.room.ID)
	}
	if ev.ItemID != inst.ID() {
		t.Errorf("ItemID = %q, want %q", ev.ItemID, inst.ID())
	}
}

func TestGet_DoesNotPublishOnFixture(t *testing.T) {
	// Tag-gate rejection happens before the placement mutation, so
	// no event should fire.
	f := newInvFixture(t)
	f.spawnInRoom(t, fixtureStatue())
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}

	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventItemPickedUp)
	env := f.env()
	env.Bus = bus

	r := newRegistry(t)
	_ = r.Dispatch(context.Background(), env, a, "get statue")

	if len(*got) != 0 {
		t.Errorf("event fired despite fixture rejection: %+v", *got)
	}
}

func TestDrop_PublishesItemDropped(t *testing.T) {
	f := newInvFixture(t)
	inst := f.spawnInRoom(t, sword())
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}

	bus := eventbus.New()
	getEvents := captureEvents(t, bus, eventbus.EventItemPickedUp)
	dropEvents := captureEvents(t, bus, eventbus.EventItemDropped)
	env := f.env()
	env.Bus = bus

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "get sword"); err != nil {
		t.Fatalf("get dispatch: %v", err)
	}
	if err := r.Dispatch(context.Background(), env, a, "drop sword"); err != nil {
		t.Fatalf("drop dispatch: %v", err)
	}

	if len(*getEvents) != 1 || len(*dropEvents) != 1 {
		t.Fatalf("event counts get=%d drop=%d, want 1 each", len(*getEvents), len(*dropEvents))
	}
	ev := (*dropEvents)[0].(eventbus.ItemDropped)
	if ev.ItemID != inst.ID() || ev.RoomID != f.room.ID {
		t.Errorf("drop payload = %+v", ev)
	}
}

func TestEquip_PublishesEntityEquipped(t *testing.T) {
	f := newEqFixture(t)
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}
	inst, err := f.store.Spawn(swordWithMods())
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a.AddToInventory(inst.ID())

	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventEntityEquipped)
	env := f.env()
	env.Bus = bus

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "equip sword wield"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(*got) != 1 {
		t.Fatalf("event count = %d, want 1", len(*got))
	}
	ev := (*got)[0].(eventbus.EntityEquipped)
	if ev.ItemID != inst.ID() {
		t.Errorf("ItemID = %q, want %q", ev.ItemID, inst.ID())
	}
	if ev.SlotName != "wield" {
		t.Errorf("SlotName = %q, want wield", ev.SlotName)
	}
}

func TestEquip_AutoSwapPublishesUnequipBeforeEquip(t *testing.T) {
	// §3.3 step 3: auto-swap displaces the occupant. Observers must
	// see the displacement (unequip) BEFORE the new placement
	// (equip) so a downstream event-replay reconstructs state
	// in the same order it happened.
	f := newEqFixture(t)
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}
	first, _ := f.store.Spawn(swordWithMods())
	second, _ := f.store.Spawn(swordWithMods())
	a.AddToInventory(first.ID())
	a.AddToInventory(second.ID())

	bus := eventbus.New()
	var order []string
	bus.Subscribe(eventbus.EventEntityEquipped, func(ctx context.Context, e eventbus.Event) {
		order = append(order, "equip:"+string(e.(eventbus.EntityEquipped).ItemID))
	})
	bus.Subscribe(eventbus.EventEntityUnequipped, func(ctx context.Context, e eventbus.Event) {
		order = append(order, "unequip:"+string(e.(eventbus.EntityUnequipped).ItemID))
	})
	env := f.env()
	env.Bus = bus

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "equip 1.sword wield"); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	if err := r.Dispatch(context.Background(), env, a, "equip sword wield"); err != nil {
		t.Fatalf("second dispatch: %v", err)
	}

	// Expected order:
	//   equip:first   (no swap on initial equip)
	//   unequip:first (swap displacement)
	//   equip:second  (new placement)
	if len(order) != 3 {
		t.Fatalf("event count = %d, want 3; got %v", len(order), order)
	}
	want := []string{
		"equip:" + string(first.ID()),
		"unequip:" + string(first.ID()),
		"equip:" + string(second.ID()),
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("order[%d] = %q, want %q (full: %v)", i, order[i], w, order)
		}
	}
}

func TestUnequip_PublishesBaseSlotName(t *testing.T) {
	// §3.4 step 4: event carries base name, never the index suffix.
	// Multi-cap finger:0 → SlotName "finger".
	f := newEqFixture(t)
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}
	inst, _ := f.store.Spawn(ringTpl("tapestry-core:ring-a"))
	a.AddToInventory(inst.ID())

	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventEntityUnequipped)
	env := f.env()
	env.Bus = bus

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "equip ring finger"); err != nil {
		t.Fatalf("equip: %v", err)
	}
	if err := r.Dispatch(context.Background(), env, a, "unequip ring"); err != nil {
		t.Fatalf("unequip: %v", err)
	}

	if len(*got) != 1 {
		t.Fatalf("event count = %d, want 1", len(*got))
	}
	ev := (*got)[0].(eventbus.EntityUnequipped)
	if ev.SlotName != "finger" {
		t.Errorf("SlotName = %q, want base name 'finger' (no :index suffix)", ev.SlotName)
	}
}

func TestEquip_DoesNotPublishOnEmptyInventory(t *testing.T) {
	// Failure path for EquipHandler: empty inventory, resolver
	// returns "carrying nothing" before Equip is called. Pins that
	// the emit stays below the success guard — a future refactor
	// that hoists Publish above the Equip() check would silently
	// fire events for failed operations.
	f := newEqFixture(t)
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventEntityEquipped)
	env := f.env()
	env.Bus = bus

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "equip sword wield"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(*got) != 0 {
		t.Errorf("event fired despite empty inventory: %+v", *got)
	}
}

func TestGet_DoesNotPublishOnEmptyRoom(t *testing.T) {
	// Failure path for GetHandler: room is empty, resolver fails,
	// GetHandler returns before emit. Pins that the emit stays
	// below the success guard.
	f := newInvFixture(t) // empty room
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}

	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventItemPickedUp)
	env := f.env()
	env.Bus = bus

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "get sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(*got) != 0 {
		t.Errorf("event fired despite empty room: %+v", *got)
	}
}

func TestDrop_DoesNotPublishOnEmptyInventory(t *testing.T) {
	// Failure path for DropHandler: inventory empty, resolver
	// fails, no emit.
	f := newInvFixture(t)
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}

	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventItemDropped)
	env := f.env()
	env.Bus = bus

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "drop sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(*got) != 0 {
		t.Errorf("event fired despite empty inventory: %+v", *got)
	}
}

func TestUnequip_DoesNotPublishWhenNothingWorn(t *testing.T) {
	// Failure path for UnequipHandler: empty equipment, resolver
	// returns early before emit.
	f := newEqFixture(t)
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}

	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventEntityUnequipped)
	env := f.env()
	env.Bus = bus

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "unequip sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(*got) != 0 {
		t.Errorf("event fired with nothing equipped: %+v", *got)
	}
}

func TestHandlers_TolerateNilBus(t *testing.T) {
	// Every handler must nil-guard c.Bus per the Env doc — a test
	// fixture that doesn't subscribe to anything passes a zero-value
	// Env and the handler should still succeed.
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst, _ := f.store.Spawn(swordWithMods())
	a.AddToInventory(inst.ID())

	env := f.env() // Bus is nil

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "equip sword wield"); err != nil {
		t.Fatalf("equip with nil bus: %v", err)
	}
	if err := r.Dispatch(context.Background(), env, a, "unequip sword"); err != nil {
		t.Fatalf("unequip with nil bus: %v", err)
	}
	if len(a.Equipment()) != 0 {
		t.Errorf("equip+unequip with nil bus left equipment populated: %v", a.Equipment())
	}

	_ = command.Env{} // referenced so the package import isn't dropped if the test shrinks
}
