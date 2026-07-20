package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// medkitSuppliesTpl is a single-use box of refill supplies (medkit_supplies).
func medkitSuppliesTpl() *item.Template {
	return &item.Template{
		ID: "shadowrun:medkit-supplies", Name: "a box of medkit supplies", Type: "item",
		Keywords:   []string{"supplies", "restock"},
		Properties: map[string]any{"medkit_supplies": true},
	}
}

// giveSupplies spawns a box of medkit supplies into the actor's inventory.
func giveSupplies(t *testing.T, f *considerFixture, a *combatActor) entities.EntityID {
	t.Helper()
	inst, err := f.store.Spawn(medkitSuppliesTpl())
	if err != nil {
		t.Fatalf("spawn supplies: %v", err)
	}
	a.AddToInventory(inst.ID())
	return inst.ID()
}

// refill tops a spent kit back to its max and consumes one box of supplies.
func TestRefill_RestocksAndConsumesSupplies(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Medic", "p-medic", f.room)
	kit := giveMedkit(t, f, a, 3) // 3 / max 10
	suppliesID := giveSupplies(t, f, a)

	dispatchRole(t, f.env(), a, "refill")

	if got := kitCharges(t, kit); got != 10 {
		t.Errorf("charges = %d, want 10 (restocked to max)", got)
	}
	if inInventory(a, suppliesID) {
		t.Error("supplies should be consumed by a refill")
	}
	if !strings.Contains(a.lastLine(), "restock") {
		t.Errorf("line = %q, want a restock confirmation", a.lastLine())
	}
}

// No supplies on hand → refused, and the kit is untouched.
func TestRefill_NoSuppliesRefused(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Medic", "p-medic", f.room)
	kit := giveMedkit(t, f, a, 3)

	dispatchRole(t, f.env(), a, "refill")

	if got := kitCharges(t, kit); got != 3 {
		t.Errorf("charges = %d, want 3 (unchanged without supplies)", got)
	}
	if !strings.Contains(a.lastLine(), "no medkit supplies") {
		t.Errorf("line = %q, want a no-supplies refusal", a.lastLine())
	}
}

// A full kit is left alone and the supplies are NOT wasted.
func TestRefill_AlreadyFullKeepsSupplies(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Medic", "p-medic", f.room)
	giveMedkit(t, f, a, 10) // 10 / max 10 — full
	suppliesID := giveSupplies(t, f, a)

	dispatchRole(t, f.env(), a, "refill")

	if !strings.Contains(a.lastLine(), "already fully stocked") {
		t.Errorf("line = %q, want an already-full message", a.lastLine())
	}
	if !inInventory(a, suppliesID) {
		t.Error("supplies must not be consumed refilling a full kit")
	}
}

// No medkit at all → refused.
func TestRefill_NoMedkitRefused(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Medic", "p-medic", f.room)
	giveSupplies(t, f, a) // supplies but no kit

	dispatchRole(t, f.env(), a, "refill")

	if !strings.Contains(a.lastLine(), "no medkit to refill") {
		t.Errorf("line = %q, want a no-medkit refusal", a.lastLine())
	}
}

// Naming a non-medkit item is refused.
func TestRefill_NamedNonKitRefused(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Medic", "p-medic", f.room)
	inst, err := f.store.Spawn(sword())
	if err != nil {
		t.Fatalf("spawn sword: %v", err)
	}
	a.AddToInventory(inst.ID())
	giveSupplies(t, f, a)

	dispatchRole(t, f.env(), a, "refill sword")

	if !strings.Contains(a.lastLine(), "isn't a medkit") {
		t.Errorf("line = %q, want a not-a-medkit refusal", a.lastLine())
	}
}

// inInventory reports whether the actor still holds id.
func inInventory(a *combatActor, id entities.EntityID) bool {
	for _, held := range a.Inventory() {
		if held == id {
			return true
		}
	}
	return false
}
