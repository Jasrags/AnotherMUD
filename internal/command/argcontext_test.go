package command_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// These tests cover the M17.2d production adapter end-to-end: build a
// ResolveContext from a live command.Context, then run the real
// resolver registry against it and assert the resolved shapes. This
// proves the candidate adapters (item / mob / door) satisfy the
// M17.2b/c resolver interfaces against actual runtime types, not just
// the hand-rolled fakes the argresolve_*_test files use.

func containerTpl() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:sack",
		Name:     "a leather sack",
		Type:     "container",
		Keywords: []string{"sack"},
	}
}

// invContext wires an invFixture + actor into a command.Context with
// the item/world/placement scopes populated, mirroring what Dispatch
// builds at runtime.
func invContext(f *invFixture, a *testActor) *command.Context {
	return &command.Context{
		Actor:     a,
		World:     f.world,
		Items:     f.store,
		Placement: f.place,
	}
}

func TestBuildResolveContext_InventoryScope(t *testing.T) {
	f := newInvFixture(t)
	inst, err := f.store.Spawn(sword())
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a := newNamedTestActor("Alice", "p-1", f.room)
	a.AddToInventory(inst.ID())

	rc := invContext(f, a).BuildResolveContext()
	if len(rc.Inventory) != 1 {
		t.Fatalf("Inventory len = %d, want 1", len(rc.Inventory))
	}

	r := command.NewArgResolverRegistry()
	res, _, _, err := r.ResolveArgsWithContext(
		[]command.ArgDefinition{{Name: "what", Type: command.ArgInventory}},
		[]string{"sword"},
		rc,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	ref := res["what"].(command.ItemRef)
	if ref.ID != string(inst.ID()) || ref.TemplateID != string(inst.TemplateID()) {
		t.Errorf("ref = %+v, want id %q tpl %q", ref, inst.ID(), inst.TemplateID())
	}
}

func TestBuildResolveContext_RoomItemScope(t *testing.T) {
	f := newInvFixture(t)
	inst := f.spawnInRoom(t, sword())
	a := newNamedTestActor("Alice", "p-1", f.room)

	rc := invContext(f, a).BuildResolveContext()
	if len(rc.RoomItems) != 1 {
		t.Fatalf("RoomItems len = %d, want 1", len(rc.RoomItems))
	}

	r := command.NewArgResolverRegistry()
	res, _, _, err := r.ResolveArgsWithContext(
		[]command.ArgDefinition{{Name: "it", Type: command.ArgRoomItem}},
		[]string{"sword"},
		rc,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	if res["it"].(command.ItemRef).ID != string(inst.ID()) {
		t.Errorf("got %+v", res["it"])
	}
}

func TestBuildResolveContext_RoomEntityScope_Mob(t *testing.T) {
	f := newInvFixture(t)
	guard, err := f.store.SpawnMob(guardTplForConsider())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	f.place.Place(guard.ID(), f.room.ID)
	a := newNamedTestActor("Alice", "p-1", f.room)

	rc := invContext(f, a).BuildResolveContext()
	if len(rc.RoomEntities) != 1 {
		t.Fatalf("RoomEntities len = %d, want 1", len(rc.RoomEntities))
	}
	// A mob must NOT leak into the item scope.
	if len(rc.RoomItems) != 0 {
		t.Errorf("RoomItems len = %d, want 0 (mob is not an item)", len(rc.RoomItems))
	}

	r := command.NewArgResolverRegistry()
	res, _, _, err := r.ResolveArgsWithContext(
		[]command.ArgDefinition{{Name: "tgt", Type: command.ArgNPC}},
		[]string{"guard"},
		rc,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	ref := res["tgt"].(command.EntityRef)
	if ref.ID != guard.EntityID() || ref.Type != "mob" {
		t.Errorf("ref = %+v, want id %q type mob", ref, guard.EntityID())
	}
}

func TestBuildResolveContext_ContainerDetection(t *testing.T) {
	f := newInvFixture(t)
	sack, err := f.store.Spawn(containerTpl())
	if err != nil {
		t.Fatalf("Spawn container: %v", err)
	}
	plainSword, err := f.store.Spawn(sword())
	if err != nil {
		t.Fatalf("Spawn sword: %v", err)
	}
	a := newNamedTestActor("Alice", "p-1", f.room)
	a.AddToInventory(sack.ID())
	a.AddToInventory(plainSword.ID())

	rc := invContext(f, a).BuildResolveContext()
	r := command.NewArgResolverRegistry()

	// The sack resolves as a container.
	res, _, _, err := r.ResolveArgsWithContext(
		[]command.ArgDefinition{{Name: "c", Type: command.ArgContainer}},
		[]string{"sack"},
		rc,
	)
	if err != nil {
		t.Fatalf("container resolve: %v", err)
	}
	if res["c"].(command.ItemRef).ID != string(sack.ID()) {
		t.Errorf("got %+v, want sack", res["c"])
	}

	// The sword is not a container — it must be excluded.
	_, _, _, err = r.ResolveArgsWithContext(
		[]command.ArgDefinition{{Name: "c", Type: command.ArgContainer}},
		[]string{"sword"},
		rc,
	)
	if err == nil {
		t.Error("expected sword to be rejected as a non-container")
	}
}

func TestBuildResolveContext_VisibleSelfTag(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Alice", "p-1", f.room)

	rc := invContext(f, a).BuildResolveContext()
	if rc.ActorName != "Alice" || rc.ActorID != "p-1" {
		t.Fatalf("actor identity = %q/%q, want Alice/p-1", rc.ActorName, rc.ActorID)
	}

	r := command.NewArgResolverRegistry()
	res, _, _, err := r.ResolveArgsWithContext(
		[]command.ArgDefinition{{Name: "t", Type: command.ArgVisible}},
		[]string{"alice"},
		rc,
	)
	if err != nil {
		t.Fatalf("visible resolve: %v", err)
	}
	v := res["t"].(command.VisibleRef)
	if v.Source != "self" || v.ID != "p-1" {
		t.Errorf("v = %+v, want self / p-1", v)
	}
}

func TestBuildResolveContext_DoorScope_ByDirection(t *testing.T) {
	d := newDoorFixture(t, ironGate("key.iron"), ironGate("key.iron"))
	a := newNamedTestActor("Alice", "p-1", d.roomA(t))

	rc := (&command.Context{Actor: a, World: d.world}).BuildResolveContext()
	if rc.Doors == nil {
		t.Fatal("Doors scope is nil")
	}

	r := command.NewArgResolverRegistry()
	res, _, _, err := r.ResolveArgsWithContext(
		[]command.ArgDefinition{{Name: "door", Type: command.ArgDoor}},
		[]string{"north"},
		rc,
	)
	if err != nil {
		t.Fatalf("door resolve: %v", err)
	}
	ref := res["door"].(command.DoorRef)
	if ref.Direction != "n" || !ref.Door.Locked || ref.Door.KeyID != "key.iron" {
		t.Errorf("ref = %+v", ref)
	}
}

func TestBuildResolveContext_DoorScope_ByKeyword(t *testing.T) {
	d := newDoorFixture(t, ironGate(""), ironGate(""))
	a := newNamedTestActor("Alice", "p-1", d.roomA(t))

	rc := (&command.Context{Actor: a, World: d.world}).BuildResolveContext()
	r := command.NewArgResolverRegistry()
	res, _, _, err := r.ResolveArgsWithContext(
		[]command.ArgDefinition{{Name: "door", Type: command.ArgDoor}},
		[]string{"gate"},
		rc,
	)
	if err != nil {
		t.Fatalf("door resolve by keyword: %v", err)
	}
	if res["door"].(command.DoorRef).Direction != "n" {
		t.Errorf("got %+v, want direction n", res["door"])
	}
}

func TestBuildResolveContext_NilActor_ZeroContext(t *testing.T) {
	rc := (&command.Context{}).BuildResolveContext()
	if rc.Inventory != nil || rc.RoomItems != nil || rc.RoomEntities != nil || rc.Doors != nil {
		t.Errorf("nil-actor context not zero: %+v", rc)
	}
}

func TestBuildResolveContext_NilWorld_NilDoorScope(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Alice", "p-1", f.room)
	// Context with no World — door scope must stay nil so the door
	// resolver reports ErrNoSuchDoor rather than panicking.
	rc := (&command.Context{Actor: a, Items: f.store, Placement: f.place}).BuildResolveContext()
	if rc.Doors != nil {
		t.Error("Doors scope should be nil with no World")
	}

	r := command.NewArgResolverRegistry()
	_, _, _, err := r.ResolveArgsWithContext(
		[]command.ArgDefinition{{Name: "door", Type: command.ArgDoor}},
		[]string{"gate"},
		rc,
	)
	if err == nil {
		t.Error("expected ErrNoSuchDoor with nil door scope")
	}
}
