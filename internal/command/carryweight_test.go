package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// Carry-weight ceiling on pickup (inventory-equipment-items §4.2 step 2).
// A positive max-carry-weight stat caps the summed weight of carried
// items; a heavy item that would push the actor over is refused before
// the room placement is mutated. A zero/absent ceiling means no limit, so
// existing weightless content is unaffected.

// heavyItemInRoom spawns the sword in the room with a weight property.
func heavyItemInRoom(t *testing.T, f *invFixture, weight int) {
	t.Helper()
	inst := f.spawnInRoom(t, sword())
	inst.SetProperty("weight", weight)
}

func TestGet_RefusesWhenOverCarryWeight(t *testing.T) {
	f := newInvFixture(t)
	heavyItemInRoom(t, f, 10)
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}
	a.carryMax = 5 // ceiling below the item's weight

	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventItemPickedUp)
	env := f.env()
	env.Bus = bus

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "get sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(*got) != 0 {
		t.Errorf("ItemPickedUp fired despite over-weight refusal: %+v", *got)
	}
	if len(a.Inventory()) != 0 {
		t.Errorf("item entered inventory despite over-weight refusal: %v", a.Inventory())
	}
	if last := a.lastLine(); !strings.Contains(strings.ToLower(last), "heavy") {
		t.Errorf("refusal message = %q, want it to mention the weight", last)
	}
}

func TestGet_AllowsWhenUnderCarryWeight(t *testing.T) {
	f := newInvFixture(t)
	heavyItemInRoom(t, f, 4)
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}
	a.carryMax = 5 // ceiling above the item's weight

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), f.env(), a, "get sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(a.Inventory()) != 1 {
		t.Errorf("item not picked up under the ceiling: inventory=%v", a.Inventory())
	}
}

func TestGet_ZeroCarryMaxMeansNoLimit(t *testing.T) {
	f := newInvFixture(t)
	heavyItemInRoom(t, f, 1000)
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}
	// carryMax AND str both default to 0 → no derived capacity, no limit.

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), f.env(), a, "get sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(a.Inventory()) != 1 {
		t.Errorf("zero ceiling should impose no limit; inventory=%v", a.Inventory())
	}
}

// A negative carry_max is the explicit "unlimited" sentinel: it overrides
// the STR-derived cap, so even a very heavy item is picked up.
func TestGet_NegativeCarryMaxMeansUnlimited(t *testing.T) {
	f := newInvFixture(t)
	heavyItemInRoom(t, f, 1000)
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}
	a.carryMax = -1 // explicit unlimited
	a.str = 10      // would otherwise derive a cap of 80

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), f.env(), a, "get sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(a.Inventory()) != 1 {
		t.Errorf("unlimited sentinel should impose no limit; inventory=%v", a.Inventory())
	}
}

// With no explicit carry_max, capacity is derived from Strength
// (carryPerStrength × STR = 8 × 5 = 40 here), and a heavier item is refused
// — proving the STR-derived cap activates the pickup gate.
func TestGet_StrengthDerivedCapacityGatesPickup(t *testing.T) {
	f := newInvFixture(t)
	heavyItemInRoom(t, f, 50) // 50 > 8×5 = 40
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}
	a.str = 5 // derived capacity 40; no explicit carry_max

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), f.env(), a, "get sword"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(a.Inventory()) != 0 {
		t.Errorf("item over STR-derived capacity should be refused; inventory=%v", a.Inventory())
	}
}
