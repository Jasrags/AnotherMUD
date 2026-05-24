package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// eqFixture extends invFixture with a slot registry holding the M5.3
// engine baseline (wield cap 1, head cap 1, finger cap 2).
type eqFixture struct {
	*invFixture
	slots *slot.Registry
}

func newEqFixture(t *testing.T) *eqFixture {
	t.Helper()
	f := newInvFixture(t)
	reg := slot.NewRegistry()
	if err := slot.RegisterEngineBaseline(reg); err != nil {
		t.Fatalf("RegisterEngineBaseline: %v", err)
	}
	return &eqFixture{invFixture: f, slots: reg}
}

func (f *eqFixture) env() command.Env {
	e := f.invFixture.env()
	e.Slots = f.slots
	return e
}

// spawnInInventory bypasses Placement so the item is owned but not in
// a room — equivalent to the actor having picked it up off the floor.
func (f *eqFixture) spawnInInventory(t *testing.T, tpl *item.Template, a *testActor) *entities.ItemInstance {
	t.Helper()
	inst, err := f.store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a.AddToInventory(inst.ID())
	return inst
}

func swordWithMods() *item.Template {
	return &item.Template{
		ID:        "tapestry-core:short-sword",
		Name:      "a short sword",
		Type:      "weapon",
		Keywords:  []string{"sword", "short"},
		Modifiers: []item.Modifier{{Stat: "str", Value: 1}},
	}
}

func ringTpl(id string) *item.Template {
	return &item.Template{
		ID:       item.TemplateID(id),
		Name:     "a plain ring",
		Type:     "ring",
		Keywords: []string{"ring"},
	}
}

func dispatch(t *testing.T, r *command.Registry, env command.Env, a *testActor, line string) {
	t.Helper()
	if err := r.Dispatch(context.Background(), env, a, line); err != nil {
		t.Fatalf("dispatch %q: %v", line, err)
	}
}

func newRegistry(t *testing.T) *command.Registry {
	t.Helper()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	return r
}

func TestEquip_MovesFromInventoryToSlotAndAppliesMods(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip sword wield")

	if len(a.Inventory()) != 0 {
		t.Errorf("inventory after equip = %v, want empty", a.Inventory())
	}
	eq := a.Equipment()
	if got := eq["wield"]; got != inst.ID() {
		t.Errorf("equipment[wield] = %q, want %q", got, inst.ID())
	}
	mods := a.mods[entities.EquipmentSourceKey(inst.ID())]
	if len(mods) != 1 || mods[0].Stat != "str" || mods[0].Value != 1 {
		t.Errorf("modifiers applied = %+v", mods)
	}
}

func TestEquip_AutoSwapDisplacesOccupant(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	first := f.spawnInInventory(t, swordWithMods(), a)
	second := f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip 1.sword wield")
	dispatch(t, r, f.env(), a, "equip sword wield")

	// Second equip should have displaced the first.
	if got := a.Equipment()["wield"]; got != second.ID() {
		t.Errorf("wield = %q, want second %q", got, second.ID())
	}
	// First sword returned to inventory.
	inv := a.Inventory()
	found := false
	for _, id := range inv {
		if id == first.ID() {
			found = true
		}
	}
	if !found {
		t.Errorf("displaced item not returned to inventory; inv = %v", inv)
	}
	// Mods from first item gone; from second present.
	if _, ok := a.mods[entities.EquipmentSourceKey(first.ID())]; ok {
		t.Error("first item's mods still applied after displacement")
	}
	if _, ok := a.mods[entities.EquipmentSourceKey(second.ID())]; !ok {
		t.Error("second item's mods not applied")
	}
}

func TestEquip_MultiCapFingerPacksFromZero(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	first := f.spawnInInventory(t, ringTpl("tapestry-core:ring-a"), a)
	second := f.spawnInInventory(t, ringTpl("tapestry-core:ring-b"), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip 1.ring finger")
	dispatch(t, r, f.env(), a, "equip ring finger")

	eq := a.Equipment()
	if eq["finger:0"] != first.ID() {
		t.Errorf("finger:0 = %q, want %q", eq["finger:0"], first.ID())
	}
	if eq["finger:1"] != second.ID() {
		t.Errorf("finger:1 = %q, want %q", eq["finger:1"], second.ID())
	}
}

func TestEquip_UnknownSlotFails(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip sword nostril")

	last := a.lastLine()
	if !strings.Contains(last, "No such slot") {
		t.Errorf("expected slot error, got %q", last)
	}
	// Item stayed in inventory.
	if len(a.Inventory()) != 1 {
		t.Errorf("inventory disturbed by failed equip: %v", a.Inventory())
	}
}

func TestEquip_ItemNotInInventoryFails(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip sword wield")

	last := a.lastLine()
	if !strings.Contains(last, "carrying") {
		t.Errorf("expected carrying-error, got %q", last)
	}
}

func TestUnequip_ReturnsItemAndReversesMods(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip sword wield")
	dispatch(t, r, f.env(), a, "unequip sword")

	if _, ok := a.Equipment()["wield"]; ok {
		t.Error("wield slot still occupied after unequip")
	}
	if inv := a.Inventory(); len(inv) != 1 || inv[0] != inst.ID() {
		t.Errorf("inventory after unequip = %v, want [%q]", inv, inst.ID())
	}
	if _, ok := a.mods[entities.EquipmentSourceKey(inst.ID())]; ok {
		t.Error("modifiers still present after unequip")
	}
}

func TestUnequip_NotEquippedFails(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	r := newRegistry(t)
	dispatch(t, r, f.env(), a, "unequip sword")
	last := a.lastLine()
	if !strings.Contains(last, "wearing") {
		t.Errorf("expected wearing-error, got %q", last)
	}
}

// recordingBroadcaster captures SendToRoom calls for assertion.
type recordingBroadcaster struct {
	calls []recordedCall
}

type recordedCall struct {
	roomID world.RoomID
	text   string
}

func (b *recordingBroadcaster) SendToRoom(ctx context.Context, roomID world.RoomID, text string, exclude ...string) {
	b.calls = append(b.calls, recordedCall{roomID: roomID, text: text})
}

// namedActor is a testActor variant with a fixed name + player id so
// broadcast guards fire.
type namedActor struct {
	*testActor
	name     string
	playerID string
}

func (n *namedActor) Name() string     { return n.name }
func (n *namedActor) PlayerID() string { return n.playerID }

func TestEquip_BroadcastFiresWithItemName(t *testing.T) {
	// Verifies the §3.3 step 7 broadcast: when a named actor with a
	// non-empty player id equips, the room receives a notification
	// referencing the item name (the base slot name is implicit in the
	// message, not the slot key — slot keys never reach players).
	f := newEqFixture(t)
	rec := &recordingBroadcaster{}
	env := f.env()
	env.Broadcaster = rec

	inner := newTestActor(f.room)
	a := &namedActor{testActor: inner, name: "Alice", playerID: "p-1"}

	inst, err := f.store.Spawn(swordWithMods())
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a.AddToInventory(inst.ID())

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "equip sword wield"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(rec.calls) != 1 {
		t.Fatalf("broadcast count = %d, want 1", len(rec.calls))
	}
	if got := rec.calls[0].text; !strings.Contains(got, "Alice") || !strings.Contains(got, "short sword") {
		t.Errorf("broadcast text = %q, expected Alice + short sword", got)
	}
	if strings.Contains(rec.calls[0].text, ":") {
		t.Errorf("broadcast leaked a slot-key colon: %q", rec.calls[0].text)
	}
}
