package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// weapon-identity §3: wielding a weapon outside your class proficiency
// applies a silent to-hit penalty, so the equip path warns the player so
// the disadvantage actually reads in-game.

func bladeWithDamage() *item.Template {
	return &item.Template{
		ID:              "wot:test-blade",
		Name:            "a test blade",
		Type:            "weapon",
		Keywords:        []string{"blade", "sword"},
		EligibleSlots:   []string{"wield"},
		WeaponDamage:    "1d8",
		ProficiencyTier: "martial",
	}
}

func TestEquip_WarnsWhenNonProficient(t *testing.T) {
	f := newEqFixture(t)
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}
	a.nonProficient = true // the actor's class doesn't grant this weapon
	inst, err := f.store.Spawn(bladeWithDamage())
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a.AddToInventory(inst.ID())

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), f.env(), a, "equip blade wield"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if last := a.lastLine(); !strings.Contains(strings.ToLower(last), "clumsil") {
		t.Errorf("non-proficient equip should warn; last line = %q", last)
	}
}

func TestEquip_NoWarnWhenProficient(t *testing.T) {
	f := newEqFixture(t)
	a := &namedActor{testActor: newTestActor(f.room), name: "Alice", playerID: "p-1"}
	// nonProficient defaults false ⇒ proficient ⇒ no warning.
	inst, err := f.store.Spawn(bladeWithDamage())
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a.AddToInventory(inst.ID())

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), f.env(), a, "equip blade wield"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if last := a.lastLine(); strings.Contains(strings.ToLower(last), "clumsil") {
		t.Errorf("proficient equip should not warn; last line = %q", last)
	}
}
