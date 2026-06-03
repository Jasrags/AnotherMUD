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

// doorFixture is a two-room world with paired north/south exits;
// the caller supplies the (optional) doors via the with* helpers.
type doorFixture struct {
	world *world.World
	store *entities.Store
}

func newDoorFixture(t *testing.T, doorA, doorB *world.DoorState) *doorFixture {
	t.Helper()
	w := world.New()
	w.AddRoom(&world.Room{
		ID:   "a",
		Name: "Room A",
		Exits: map[world.Direction]world.Exit{
			world.DirNorth: {Target: "b", Door: doorA},
		},
	})
	w.AddRoom(&world.Room{
		ID:   "b",
		Name: "Room B",
		Exits: map[world.Direction]world.Exit{
			world.DirSouth: {Target: "a", Door: doorB},
		},
	})
	return &doorFixture{world: w, store: entities.NewStore()}
}

func (f *doorFixture) env() command.Env {
	return command.Env{World: f.world, Items: f.store}
}

func (f *doorFixture) roomA(t *testing.T) *world.Room {
	t.Helper()
	r, err := f.world.Room("a")
	if err != nil {
		t.Fatalf("Room a: %v", err)
	}
	return r
}

func ironGate(keyID string) *world.DoorState {
	return &world.DoorState{
		Name:          "iron gate",
		Keywords:      []string{"iron", "gate"},
		Closed:        true,
		Locked:        keyID != "",
		KeyID:         keyID,
		DefaultClosed: true,
		DefaultLocked: keyID != "",
	}
}

func dispatchDoor(t *testing.T, f *doorFixture, a *testActor, line string) {
	t.Helper()
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), f.env(), a, line); err != nil {
		t.Fatalf("dispatch %q: %v", line, err)
	}
}

func TestOpenVerb_OpensClosedUnlockedDoor(t *testing.T) {
	d := ironGate("")
	f := newDoorFixture(t, d, ironGate(""))
	a := newTestActor(f.roomA(t))

	dispatchDoor(t, f, a, "open north")
	if got := a.lastLine(); !strings.Contains(got, "You open iron gate") {
		t.Errorf("open: %q", got)
	}
	d2, _ := f.world.GetDoor("a", world.DirNorth)
	if d2.Closed {
		t.Error("door still closed after open verb")
	}
}

func TestOpenVerb_BlockedByLock(t *testing.T) {
	f := newDoorFixture(t, ironGate("village-key"), ironGate("village-key"))
	a := newTestActor(f.roomA(t))

	dispatchDoor(t, f, a, "open gate")
	if got := a.lastLine(); !strings.Contains(got, "is locked") {
		t.Errorf("locked open: %q", got)
	}
}

func TestOpenVerb_AlreadyOpen(t *testing.T) {
	d := ironGate("")
	d.Closed = false
	f := newDoorFixture(t, d, nil)
	a := newTestActor(f.roomA(t))

	dispatchDoor(t, f, a, "open north")
	if got := a.lastLine(); !strings.Contains(got, "already open") {
		t.Errorf("already-open: %q", got)
	}
}

func TestCloseVerb_RoundTrip(t *testing.T) {
	d := ironGate("")
	d.Closed = false
	f := newDoorFixture(t, d, nil)
	a := newTestActor(f.roomA(t))

	dispatchDoor(t, f, a, "close gate")
	if got := a.lastLine(); !strings.Contains(got, "You close iron gate") {
		t.Errorf("close: %q", got)
	}
	dispatchDoor(t, f, a, "close gate")
	if got := a.lastLine(); !strings.Contains(got, "already closed") {
		t.Errorf("re-close: %q", got)
	}
}

// shut is close's alias. Before aliases inherited their primary's
// declared args, `shut` carried none — so the door arg never resolved
// and it silently failed. It now resolves identically to `close`.
func TestShutVerb_AliasResolvesDoor(t *testing.T) {
	d := ironGate("")
	d.Closed = false
	f := newDoorFixture(t, d, nil)
	a := newTestActor(f.roomA(t))

	dispatchDoor(t, f, a, "shut gate")
	if got := a.lastLine(); !strings.Contains(got, "You close iron gate") {
		t.Errorf("shut (close alias) should resolve the door like close: %q", got)
	}
}

func TestUnlockVerb_RequiresKey(t *testing.T) {
	f := newDoorFixture(t, ironGate("village-key"), nil)
	a := newTestActor(f.roomA(t))

	dispatchDoor(t, f, a, "unlock gate")
	if got := a.lastLine(); !strings.Contains(got, "don't have a key") {
		t.Errorf("no key: %q", got)
	}
}

func TestUnlockVerb_WithKeyByTemplateID(t *testing.T) {
	f := newDoorFixture(t, ironGate("village-key"), nil)
	a := newTestActor(f.roomA(t))

	// Spawn a key item directly into the store + actor inventory.
	keyInst, err := f.store.Spawn(&item.Template{
		ID:   "village-key",
		Name: "a brass key",
		Type: "item",
	})
	if err != nil {
		t.Fatalf("Spawn key: %v", err)
	}
	a.AddToInventory(keyInst.ID())

	dispatchDoor(t, f, a, "unlock gate")
	if got := a.lastLine(); !strings.Contains(got, "You unlock") {
		t.Errorf("unlock with key: %q", got)
	}
	d2, _ := f.world.GetDoor("a", world.DirNorth)
	if d2.Locked {
		t.Error("door still locked after unlock with key")
	}
}

func TestUnlockVerb_WithKeyByProperty(t *testing.T) {
	f := newDoorFixture(t, ironGate("village-key"), nil)
	a := newTestActor(f.roomA(t))

	// Spawn an item whose template id differs from village-key
	// but which carries `key_for: village-key` — the PD-4 hook.
	keyInst, err := f.store.Spawn(&item.Template{
		ID:         "fancy-keychain",
		Name:       "a brass keychain",
		Type:       "item",
		Properties: map[string]any{"key_for": "village-key"},
	})
	if err != nil {
		t.Fatalf("Spawn keychain: %v", err)
	}
	a.AddToInventory(keyInst.ID())

	dispatchDoor(t, f, a, "unlock gate")
	if got := a.lastLine(); !strings.Contains(got, "You unlock") {
		t.Errorf("unlock with key_for property: %q", got)
	}
}

func TestLockVerb_RequiresClosed(t *testing.T) {
	d := ironGate("")
	d.Closed = false
	f := newDoorFixture(t, d, nil)
	a := newTestActor(f.roomA(t))

	dispatchDoor(t, f, a, "lock gate")
	if got := a.lastLine(); !strings.Contains(got, "close") {
		t.Errorf("lock-open: %q", got)
	}
}

func TestDoorVerb_NoArgPrompt(t *testing.T) {
	f := newDoorFixture(t, ironGate(""), nil)
	a := newTestActor(f.roomA(t))

	dispatchDoor(t, f, a, "open")
	// M17.2c/d: the dispatcher emits the §5.4 missing-arg prompt for
	// the declared `door` arg instead of the old "Open what?".
	if got := a.lastLine(); !strings.Contains(got, "What door?") {
		t.Errorf("no-arg open: %q", got)
	}
}

func TestDoorVerb_UnknownTarget(t *testing.T) {
	f := newDoorFixture(t, ironGate(""), nil)
	a := newTestActor(f.roomA(t))

	dispatchDoor(t, f, a, "open trapdoor")
	if got := a.lastLine(); !strings.Contains(got, "don't see") {
		t.Errorf("unknown target: %q", got)
	}
}

func TestDoorVerb_Ambiguous(t *testing.T) {
	// Two doors both named "gate" on different exits → ambiguous.
	w := world.New()
	w.AddRoom(&world.Room{
		ID:   "a",
		Name: "Crossroads",
		Exits: map[world.Direction]world.Exit{
			world.DirNorth: {Target: "n", Door: &world.DoorState{Name: "gate", Keywords: []string{"gate"}, Closed: true}},
			world.DirEast:  {Target: "e", Door: &world.DoorState{Name: "gate", Keywords: []string{"gate"}, Closed: true}},
		},
	})
	w.AddRoom(&world.Room{ID: "n", Name: "North"})
	w.AddRoom(&world.Room{ID: "e", Name: "East"})
	f := &doorFixture{world: w, store: entities.NewStore()}
	a := newTestActor(f.roomA(t))

	dispatchDoor(t, f, a, "open gate")
	// M17.2c/d: ambiguous door resolution surfaces the door resolver's
	// standardized ErrDoorAmbiguous copy instead of the op-specific
	// "Which one do you want to open?".
	if got := a.lastLine(); !strings.Contains(got, "Which door do you mean?") {
		t.Errorf("ambiguous: %q", got)
	}
}

func TestDoorVerb_OrdinalDisambiguates(t *testing.T) {
	w := world.New()
	w.AddRoom(&world.Room{
		ID:   "a",
		Name: "Crossroads",
		Exits: map[world.Direction]world.Exit{
			world.DirNorth: {Target: "n", Door: &world.DoorState{Name: "gate", Keywords: []string{"gate"}, Closed: true}},
			world.DirEast:  {Target: "e", Door: &world.DoorState{Name: "gate", Keywords: []string{"gate"}, Closed: true}},
		},
	})
	w.AddRoom(&world.Room{ID: "n", Name: "North"})
	w.AddRoom(&world.Room{ID: "e", Name: "East"})
	f := &doorFixture{world: w, store: entities.NewStore()}
	a := newTestActor(f.roomA(t))

	// 1.gate is the canonically-first direction (north).
	dispatchDoor(t, f, a, "open 1.gate")
	if got := a.lastLine(); !strings.Contains(got, "You open gate") {
		t.Errorf("ordinal 1: %q", got)
	}
	// 2.gate is east.
	dispatchDoor(t, f, a, "open 2.gate")
	if got := a.lastLine(); !strings.Contains(got, "You open gate") {
		t.Errorf("ordinal 2: %q", got)
	}
}

// TestRenderExits_DecoratesDoorState pins the M15.1c render hook:
// exit listing shows (closed)/(locked) decorators when the exit
// carries a door, plain direction when doorless or open.
func TestRenderExits_DecoratesDoorState(t *testing.T) {
	w := world.New()
	w.AddRoom(&world.Room{
		ID:   "x",
		Name: "Crossroads",
		Exits: map[world.Direction]world.Exit{
			world.DirNorth: {Target: "n", Door: &world.DoorState{Name: "iron gate", Closed: true, Locked: true}},
			world.DirEast:  {Target: "e", Door: &world.DoorState{Name: "oak gate", Closed: true}},
			world.DirSouth: {Target: "s", Door: &world.DoorState{Name: "open arch", Closed: false}},
			world.DirWest:  {Target: "ww"},
		},
	})
	r, _ := w.Room("x")
	out := command.RenderRoom(r, nil, nil, nil, nil)
	if !strings.Contains(out, "north (locked)") {
		t.Errorf("locked decorator missing: %q", out)
	}
	if !strings.Contains(out, "east (closed)") {
		t.Errorf("closed decorator missing: %q", out)
	}
	if !strings.Contains(out, "south,") && !strings.HasSuffix(out, "south") {
		t.Errorf("open door should render plain: %q", out)
	}
	if !strings.Contains(out, "west") {
		t.Errorf("doorless exit missing: %q", out)
	}
}
