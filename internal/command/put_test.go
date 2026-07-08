package command_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// putFixture extends invFixture with the substrate `put` needs:
// Contents + Bus.
type putFixture struct {
	*invFixture
	contents *entities.Contents
	bus      *eventbus.Bus
}

func newPutFixture(t *testing.T) *putFixture {
	t.Helper()
	return &putFixture{
		invFixture: newInvFixture(t),
		contents:   entities.NewContents(),
		bus:        eventbus.New(),
	}
}

func (f *putFixture) env() command.Env {
	e := f.invFixture.env()
	e.Contents = f.contents
	e.Bus = f.bus
	return e
}

func sackTpl() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:canvas-sack",
		Name:     "a canvas sack",
		Type:     "container",
		Keywords: []string{"sack", "canvas"},
		Properties: map[string]any{
			propWeight:            1,
			propContainerCapacity: 2,
			propWeightLimit:       10,
		},
	}
}

func tinySackTpl() *item.Template {
	t := sackTpl()
	t.ID = "tapestry-core:tiny-sack"
	t.Properties[propContainerCapacity] = 1
	t.Properties[propWeightLimit] = 100
	return t
}

func heavySackTpl() *item.Template {
	t := sackTpl()
	t.ID = "tapestry-core:heavy-sack"
	t.Properties[propContainerCapacity] = 99
	t.Properties[propWeightLimit] = 3
	return t
}

func swordWithWeight() *item.Template {
	t := sword()
	if t.Properties == nil {
		t.Properties = make(map[string]any)
	}
	t.Properties[propWeight] = 5
	return t
}

// re-exported constants — put.go defines them as lowercase package
// locals. The test package needs symbolic access, so the test file
// can declare local mirrors. Keeping them in sync with put.go is
// trivial (compile-time pin via the property the templates use).
const (
	propWeight            = "weight"
	propContainerCapacity = "container_capacity"
	propWeightLimit       = "container_weight_limit"
)

// spawnInActorInventory adds a fresh item to the test actor's
// inventory (skipping the room → get round-trip).
func (f *putFixture) spawnInActorInventory(t *testing.T, a *testActor, tpl *item.Template) *entities.ItemInstance {
	t.Helper()
	inst, err := f.store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a.AddToInventory(inst.ID())
	return inst
}

func dispatchPut(t *testing.T, f *putFixture, a *testActor, input string) {
	t.Helper()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), f.env(), a, input); err != nil {
		t.Fatalf("dispatch %q: %v", input, err)
	}
}

func TestPut_HappyPath_CarriedContainer(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	gem := f.spawnInActorInventory(t, a, sword())
	sack := f.spawnInActorInventory(t, a, sackTpl())

	var added []eventbus.ContainerItemAdded
	f.bus.Subscribe(eventbus.EventContainerItemAdded, func(_ context.Context, e eventbus.Event) {
		added = append(added, e.(eventbus.ContainerItemAdded))
	})

	dispatchPut(t, f, a, "put sword sack")

	inv := a.Inventory()
	if len(inv) != 1 || inv[0] != sack.ID() {
		t.Errorf("inventory = %v, want only sack remaining", inv)
	}
	contents := f.contents.In(sack.ID())
	if len(contents) != 1 || contents[0] != gem.ID() {
		t.Errorf("sack contents = %v, want [%q]", contents, gem.ID())
	}
	if len(added) != 1 {
		t.Fatalf("ContainerItemAdded events = %d, want 1", len(added))
	}
	if added[0].ContainerID != sack.ID() || added[0].ItemID != gem.ID() {
		t.Errorf("event payload wrong: %+v", added[0])
	}
}

func TestPut_AcceptsInAndIntoPrepositions(t *testing.T) {
	for _, input := range []string{"put sword in sack", "put sword into sack"} {
		t.Run(input, func(t *testing.T) {
			f := newPutFixture(t)
			a := newNamedTestActor("Alice", "p-alice", f.room)
			f.spawnInActorInventory(t, a, sword())
			sack := f.spawnInActorInventory(t, a, sackTpl())
			dispatchPut(t, f, a, input)
			if got := f.contents.In(sack.ID()); len(got) != 1 {
				t.Errorf("contents after %q = %v, want one item", input, got)
			}
		})
	}
}

func TestPut_RoomPlacedContainer(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	gem := f.spawnInActorInventory(t, a, sword())
	sack := f.spawnInRoom(t, sackTpl())

	dispatchPut(t, f, a, "put sword sack")

	if got := f.contents.In(sack.ID()); len(got) != 1 || got[0] != gem.ID() {
		t.Errorf("room sack contents = %v, want [%q]", got, gem.ID())
	}
	if len(a.Inventory()) != 0 {
		t.Errorf("inventory not cleared: %v", a.Inventory())
	}
}

// A no_put container (e.g. a corpse, loot-and-corpses §2.2) refuses
// `put`: the item stays in inventory and nothing enters the container.
func TestPut_RefusesNoPutContainer(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	gem := f.spawnInActorInventory(t, a, sword())
	corpseTpl := sackTpl()
	corpseTpl.ID = "tapestry-core:a-corpse"
	corpseTpl.Name = "the corpse of a goblin"
	corpseTpl.Keywords = []string{"corpse", "goblin"}
	corpseTpl.Tags = []string{"corpse", "no_put"}
	corpse := f.spawnInRoom(t, corpseTpl)

	dispatchPut(t, f, a, "put sword corpse")

	if got := f.contents.In(corpse.ID()); len(got) != 0 {
		t.Errorf("no_put container received items: %v", got)
	}
	if inv := a.Inventory(); len(inv) != 1 || inv[0] != gem.ID() {
		t.Errorf("item should stay in inventory, got %v", inv)
	}
}

func TestPut_NotAContainer(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	sword1 := f.spawnInActorInventory(t, a, sword())
	// Second sword carried so the resolver can resolve "sword" twice
	// (`put sword sword`) — the second match is the non-container.
	sword2 := f.spawnInActorInventory(t, a, &item.Template{
		ID: "tapestry-core:sword-2", Name: "another sword", Type: "weapon",
		Keywords: []string{"sword", "another"},
	})

	// M17.2d₃: the `container` arg resolver only yields
	// ContainerCandidate items, so a plain sword is never a candidate;
	// the miss surfaces the resolver's standardized container copy.
	dispatchPut(t, f, a, "put sword another")

	if !strings.Contains(a.lastLine(), "don't see a container") {
		t.Errorf("reply = %q, want container not-found message", a.lastLine())
	}
	if len(a.Inventory()) != 2 {
		t.Errorf("inventory mutated on failure: %v", a.Inventory())
	}
	_ = sword1
	_ = sword2
}

func TestPut_Full(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	// Tiny sack: capacity 1. Pre-fill via the substrate directly so
	// the put attempt hits the cap at exactly one occupant.
	sack := f.spawnInActorInventory(t, a, tinySackTpl())
	prefill, _ := f.store.Spawn(sword())
	f.contents.Put(sack.ID(), prefill.ID())

	gem := f.spawnInActorInventory(t, a, &item.Template{
		ID: "tapestry-core:gem", Name: "a gem", Type: "treasure",
		Keywords: []string{"gem"},
	})

	dispatchPut(t, f, a, "put gem sack")

	if !strings.Contains(a.lastLine(), "is full") {
		t.Errorf("reply = %q, want 'is full' message", a.lastLine())
	}
	if got := f.contents.In(sack.ID()); len(got) != 1 || got[0] != prefill.ID() {
		t.Errorf("sack contents mutated on full: %v", got)
	}
	if !inventoryContains(a.Inventory(), gem.ID()) {
		t.Error("gem removed from inventory on full")
	}
}

func TestPut_TooHeavy(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	sack := f.spawnInActorInventory(t, a, heavySackTpl())     // weight_limit=3
	heavy := f.spawnInActorInventory(t, a, swordWithWeight()) // weight=5

	dispatchPut(t, f, a, "put sword sack")

	if !strings.Contains(a.lastLine(), "too heavy") {
		t.Errorf("reply = %q, want 'too heavy' message", a.lastLine())
	}
	if got := f.contents.In(sack.ID()); len(got) != 0 {
		t.Errorf("sack contents = %v, want empty after rejection", got)
	}
	if !inventoryContains(a.Inventory(), heavy.ID()) {
		t.Error("heavy item removed from inventory on rejection")
	}
}

func TestPut_Cancelled_ListenerVetoes(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	gem := f.spawnInActorInventory(t, a, sword())
	sack := f.spawnInActorInventory(t, a, sackTpl())

	// Subscribe a veto listener — flips the cancel flag on the
	// adding event, which the handler must observe before
	// committing the transfer.
	f.bus.Subscribe(eventbus.EventContainerItemAdding, func(_ context.Context, e eventbus.Event) {
		if ce, ok := e.(*eventbus.ContainerItemAdding); ok {
			ce.Cancel()
		}
	})
	addedFired := false
	f.bus.Subscribe(eventbus.EventContainerItemAdded, func(context.Context, eventbus.Event) { addedFired = true })

	dispatchPut(t, f, a, "put sword sack")

	if !strings.Contains(a.lastLine(), "right now") {
		t.Errorf("reply = %q, want 'right now' cancellation message", a.lastLine())
	}
	if got := f.contents.In(sack.ID()); len(got) != 0 {
		t.Errorf("sack contents = %v, want empty after veto", got)
	}
	if !inventoryContains(a.Inventory(), gem.ID()) {
		t.Error("gem removed from inventory on veto")
	}
	if addedFired {
		t.Error("ContainerItemAdded fired despite veto")
	}
}

// TestPut_RollsBackWhenContainerVanishesMidTransfer simulates the
// HIGH 2 race: container is in the room when accessibleContainers
// scans, but is removed (e.g. another player got it) before
// Contents.Put runs. The item must come back to the actor's
// inventory and no Contents entry should be recorded.
func TestPut_RollsBackWhenContainerVanishesMidTransfer(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	gem := f.spawnInActorInventory(t, a, sword())
	sack := f.spawnInRoom(t, sackTpl())

	// Subscribe a pre-event listener that yanks the container out
	// of Placement between the accessibility scan (which already
	// ran by the time the pre-event fires) and the Contents.Put.
	// The listener doesn't veto — it just races the mutation.
	f.bus.Subscribe(eventbus.EventContainerItemAdding, func(_ context.Context, e eventbus.Event) {
		_ = e
		f.place.Remove(sack.ID())
	})

	dispatchPut(t, f, a, "put sword sack")

	if !strings.Contains(a.lastLine(), "no longer here") {
		t.Errorf("reply = %q, want 'no longer here' rollback message", a.lastLine())
	}
	if !inventoryContains(a.Inventory(), gem.ID()) {
		t.Error("gem was not returned to inventory on rollback")
	}
	if got := f.contents.In(sack.ID()); len(got) != 0 {
		t.Errorf("sack contents = %v, want empty after rollback", got)
	}
}

func TestPut_RejectsSelfNesting(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	// Only a sack in inventory. `put sack sack` resolves the same
	// instance on both sides.
	sack := f.spawnInActorInventory(t, a, sackTpl())

	dispatchPut(t, f, a, "put sack sack")

	if !strings.Contains(a.lastLine(), "inside itself") {
		t.Errorf("reply = %q, want 'inside itself' rejection", a.lastLine())
	}
	if got := f.contents.In(sack.ID()); len(got) != 0 {
		t.Errorf("sack contents mutated on self-nest: %v", got)
	}
}

func TestPut_MissingArgs(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	f.spawnInActorInventory(t, a, sword())
	f.spawnInActorInventory(t, a, sackTpl())

	// M17.2d₃: the dispatcher emits the §5.4 missing-arg prompt for
	// whichever declared arg ran out of tokens — "What item?" when no
	// tokens remain, "What container?" once the item is consumed (with
	// or without the trailing "in").
	cases := []struct{ input, want string }{
		{"put", "What item?"},
		{"put sword", "What container?"},
		{"put sword in", "What container?"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			a.lines = nil
			dispatchPut(t, f, a, tc.input)
			if !strings.Contains(a.lastLine(), tc.want) {
				t.Errorf("%q reply = %q, want %q", tc.input, a.lastLine(), tc.want)
			}
		})
	}
}

// TestPut_ConcurrentPutsRaceClean drives multiple goroutines
// putting items into the same sack to exercise the
// inventory.Remove → contents.Put pair under -race.
func TestPut_ConcurrentPutsRaceClean(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	sack := f.spawnInActorInventory(t, a, &item.Template{
		ID: "tapestry-core:big-sack", Name: "a big sack", Type: "container",
		Keywords: []string{"sack"},
		Properties: map[string]any{
			propContainerCapacity: 1000,
			propWeightLimit:       0, // unlimited
		},
	})

	// Give each item a unique keyword so concurrent dispatches
	// target distinct ids. Without this, all 50 dispatches would
	// resolve the same first-match-by-"sword" id and only one
	// would win the TOCTOU Remove. The race-detector signal we
	// care about is on the Remove → Put pair, not on the resolver.
	const items = 50
	keywords := make([]string, items)
	for i := range items {
		kw := fmt.Sprintf("gem%d", i)
		keywords[i] = kw
		f.spawnInActorInventory(t, a, &item.Template{
			ID:       item.TemplateID("tapestry-core:" + kw),
			Name:     "a " + kw,
			Type:     "treasure",
			Keywords: []string{kw},
		})
	}

	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}

	var wg sync.WaitGroup
	for _, kw := range keywords {
		wg.Go(func() {
			_ = r.Dispatch(context.Background(), f.env(), a, "put "+kw+" sack")
		})
	}
	wg.Wait()

	if got := len(f.contents.In(sack.ID())); got != items {
		t.Errorf("sack contents = %d, want %d (conservation)", got, items)
	}
	if got := len(a.Inventory()); got != 1 {
		t.Errorf("inventory = %d, want 1 (sack)", got)
	}
}

func inventoryContains(inv []entities.EntityID, id entities.EntityID) bool {
	return slices.Contains(inv, id)
}
