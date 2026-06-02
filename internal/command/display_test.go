package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/stacking"
)

// capTpl is a head-slot item used by the registration-ordering test.
func capTpl() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:leather-cap",
		Name:     "a leather cap",
		Type:     "armor",
		Keywords: []string{"cap"},
	}
}

// Tests for `inventory` / `i` and `equipment` / `eq`.
//
// These are read-only verbs against the actor's existing contents +
// the slot registry; no Env additions, no state changes. The fixture
// reuses the equipment fixture from equipment_test.go so the slot
// registry comes pre-populated with wield/head/finger.

func TestInventory_EmptyMessage(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	r := newRegistry(t)
	dispatch(t, r, f.env(), a, "inventory")
	if got := a.lastLine(); !strings.Contains(got, "carrying nothing") {
		t.Errorf("inventory empty = %q, want carrying-nothing message", got)
	}
}

func TestInventory_ListsItemsInPickupOrder(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	first := f.spawnInInventory(t, swordWithMods(), a)
	_ = first
	second := f.spawnInInventory(t, ringTpl("tapestry-core:ring-a"), a)
	_ = second
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "inventory")
	out := a.lastLine()

	if !strings.HasPrefix(out, "You are carrying:") {
		t.Errorf("missing header: %q", out)
	}
	sword := strings.Index(out, "a short sword")
	ring := strings.Index(out, "a plain ring")
	if sword < 0 || ring < 0 {
		t.Fatalf("items missing from output: %q", out)
	}
	if sword > ring {
		t.Errorf("pickup order not preserved: sword at %d, ring at %d", sword, ring)
	}
}

func TestInventory_RendersContainerContents(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	gem := f.spawnInActorInventory(t, a, &item.Template{
		ID: "tapestry-core:gem", Name: "a gem", Type: "treasure",
		Keywords: []string{"gem"},
	})
	sack := f.spawnInActorInventory(t, a, sackTpl())
	f.contents.Put(sack.ID(), gem.ID())
	a.RemoveFromInventory(gem.ID()) // mirror what put does

	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), f.env(), a, "inventory"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	out := a.lastLine()
	if !strings.Contains(out, "a canvas sack") {
		t.Errorf("sack not rendered: %q", out)
	}
	if !strings.Contains(out, "a gem") {
		t.Errorf("gem inside sack not rendered: %q", out)
	}
	// Indentation: gem line should have more leading space than sack
	// line, so the visual nesting is obvious. The exact widths are
	// pinned by the renderer (2-space per level).
	lines := strings.Split(out, "\n")
	var sackIdx, gemIdx int = -1, -1
	for i, l := range lines {
		if strings.Contains(l, "a canvas sack") {
			sackIdx = i
		}
		if strings.Contains(l, "a gem") {
			gemIdx = i
		}
	}
	if sackIdx < 0 || gemIdx <= sackIdx {
		t.Fatalf("ordering wrong: sack at %d, gem at %d", sackIdx, gemIdx)
	}
	if leadSpaces(lines[gemIdx]) <= leadSpaces(lines[sackIdx]) {
		t.Errorf("gem not indented deeper than sack:\nsack=%q\ngem=%q", lines[sackIdx], lines[gemIdx])
	}
}

func leadSpaces(s string) int {
	n := 0
	for _, r := range s {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}

// With a stacking service wired, identical items collapse to one line
// with a trailing "(xN)" count (M21.2).
func TestInventory_StacksIdenticalItems(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	for i := 0; i < 3; i++ {
		f.spawnInInventory(t, swordWithMods(), a)
	}
	env := f.env()
	env.Stacking = stacking.NewService()
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "inventory"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	out := a.lastLine()
	if !strings.Contains(out, "a short sword (x3)") {
		t.Errorf("want one stacked 'a short sword (x3)' line; got %q", out)
	}
	if n := strings.Count(out, "a short sword"); n != 1 {
		t.Errorf("a short sword appears %d times, want 1 stacked line: %q", n, out)
	}
}

// A singleton stack carries no count suffix.
func TestInventory_SingletonNoCount(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, swordWithMods(), a)
	env := f.env()
	env.Stacking = stacking.NewService()
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "inventory"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	out := a.lastLine()
	if !strings.Contains(out, "a short sword") || strings.Contains(out, "(x") {
		t.Errorf("singleton should show no count: %q", out)
	}
}

func TestInventory_AliasI(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)
	dispatch(t, r, f.env(), a, "i")
	if got := a.lastLine(); !strings.Contains(got, "a short sword") {
		t.Errorf("i alias did not resolve to inventory: %q", got)
	}
}

func TestEquipment_EmptyMessage(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	r := newRegistry(t)
	dispatch(t, r, f.env(), a, "equipment")
	if got := a.lastLine(); !strings.Contains(got, "wearing nothing") {
		t.Errorf("equipment empty = %q, want wearing-nothing message", got)
	}
}

func TestEquipment_ListsInSlotRegistrationOrder(t *testing.T) {
	// Engine baseline registers wield, head, finger in that order.
	// Equipping head before wield should NOT reorder the listing —
	// registration order wins.
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, swordWithMods(), a)
	f.spawnInInventory(t, capTpl(), a)
	r := newRegistry(t)

	// Equip head first to confirm display order doesn't follow equip
	// order.
	dispatch(t, r, f.env(), a, "equip cap head")
	dispatch(t, r, f.env(), a, "equip sword wield")

	dispatch(t, r, f.env(), a, "equipment")
	out := a.lastLine()

	wield := strings.Index(out, "wielded")
	head := strings.Index(out, "worn on head")
	if wield < 0 || head < 0 {
		t.Fatalf("labels missing from output: %q", out)
	}
	if wield > head {
		t.Errorf("registration order not preserved: wield at %d, head at %d", wield, head)
	}
}

func TestEquipment_MultiCapEmitsLinePerFilledSubSlot(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	first := f.spawnInInventory(t, ringTpl("tapestry-core:ring-a"), a)
	second := f.spawnInInventory(t, ringTpl("tapestry-core:ring-b"), a)
	_, _ = first, second
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip 1.ring finger")
	dispatch(t, r, f.env(), a, "equip ring finger")
	dispatch(t, r, f.env(), a, "equipment")

	out := a.lastLine()
	// Two rings → two "worn on finger" lines.
	if got := strings.Count(out, "worn on finger"); got != 2 {
		t.Errorf("worn-on-finger count = %d, want 2; full output: %q", got, out)
	}
	// And no slot-key colons leaked into the user-facing render.
	if strings.Contains(out, "finger:") {
		t.Errorf("slot-key suffix leaked: %q", out)
	}
}

func TestEquipment_OnlyOccupiedSubSlotShown(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, ringTpl("tapestry-core:ring-a"), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip ring finger")
	dispatch(t, r, f.env(), a, "equipment")

	out := a.lastLine()
	if got := strings.Count(out, "worn on finger"); got != 1 {
		t.Errorf("worn-on-finger count = %d, want 1; %q", got, out)
	}
}

func TestEquipment_AliasEq(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip sword wield")
	dispatch(t, r, f.env(), a, "eq")

	if got := a.lastLine(); !strings.Contains(got, "a short sword") {
		t.Errorf("eq alias did not resolve to equipment: %q", got)
	}
}

func TestEquipment_AliasEqDoesNotShadowEquip(t *testing.T) {
	// Regression: prefix-match would have resolved `eq` to `equip`
	// (registered earlier in builtins). Explicit alias registration
	// is the fix. This test pins that decision by asserting the
	// POSITIVE signal — EquipmentHandler's empty-state message —
	// rather than the absence of EquipHandler's usage banner, so the
	// test stays load-bearing if either handler's copy changes.
	f := newEqFixture(t)
	a := newTestActor(f.room)
	r := newRegistry(t)
	dispatch(t, r, f.env(), a, "eq")
	if got := a.lastLine(); !strings.Contains(got, "wearing nothing") {
		t.Errorf("eq did not route to equipment handler: %q", got)
	}
}

func TestInventory_SkipsUntrackedEntities(t *testing.T) {
	// If an inventory id no longer resolves in the store (e.g. an
	// Untrack happened between mutation and render), the renderer
	// should skip it silently rather than panic or print "(unknown)".
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, swordWithMods(), a)
	if err := f.store.Untrack(inst.ID()); err != nil {
		t.Fatalf("Untrack: %v", err)
	}
	r := newRegistry(t)
	dispatch(t, r, f.env(), a, "inventory")

	out := a.lastLine()
	if !strings.Contains(out, "carrying nothing") {
		t.Errorf("untracked item leaked: %q", out)
	}
}
