package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// AngrealPower (wot-the-one-power.md S2) reports the strongest gender-matching
// angreal the actor has EQUIPPED — the seam the composition root turns into a
// weave-potency multiplier. These tests pin the gating: gender match, the
// equipped-not-carried rule, strongest-wins (no stacking), and the inert cases.

func angrealTpl(id, gender string, power int) *item.Template {
	return &item.Template{
		ID:            item.TemplateID(id),
		Name:          "a figurine",
		Type:          "item",
		Keywords:      []string{"figurine"},
		EligibleSlots: []string{"wield", "offhand"},
		AngrealPower:  power,
		AngrealGender: gender,
	}
}

func equipAngreal(t *testing.T, a *connActor, store *entities.Store, slot, id, gender string, power int) entities.EntityID {
	t.Helper()
	inst, err := store.Spawn(angrealTpl(id, gender, power))
	if err != nil {
		t.Fatalf("Spawn %s: %v", id, err)
	}
	a.AddToInventory(inst.ID())
	if !a.Equip([]string{slot}, inst.ID(), nil) {
		t.Fatalf("Equip %s into %s returned false", id, slot)
	}
	return inst.ID()
}

func TestAngrealPower_MatchingGenderHeld(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	equipAngreal(t, a, store, "wield", "wot:saidin-angreal", "male", 2)

	if got := a.AngrealPower("male"); got != 2 {
		t.Errorf("AngrealPower(male) = %d, want 2 (matching device held)", got)
	}
}

func TestAngrealPower_CrossGenderInert(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	equipAngreal(t, a, store, "wield", "wot:saidar-angreal", "female", 3)

	if got := a.AngrealPower("male"); got != 0 {
		t.Errorf("AngrealPower(male) = %d, want 0 (a saidar device is inert to a man)", got)
	}
}

func TestAngrealPower_CarriedNotEquippedDoesNotCount(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	inst, err := store.Spawn(angrealTpl("wot:saidin-angreal", "male", 2))
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a.AddToInventory(inst.ID()) // in the pack, never equipped

	if got := a.AngrealPower("male"); got != 0 {
		t.Errorf("AngrealPower(male) = %d, want 0 (a device must be held, not merely carried)", got)
	}
}

func TestAngrealPower_StrongestWinsNoStacking(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	equipAngreal(t, a, store, "wield", "wot:weak-angreal", "male", 2)
	equipAngreal(t, a, store, "offhand", "wot:strong-angreal", "male", 5)

	// v1 takes the single strongest matching device, not the sum (2+5).
	if got := a.AngrealPower("male"); got != 5 {
		t.Errorf("AngrealPower(male) = %d, want 5 (strongest device, no stacking)", got)
	}
}

func TestAngrealPower_EmptyGenderIsInert(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	equipAngreal(t, a, store, "wield", "wot:saidin-angreal", "male", 2)

	if got := a.AngrealPower(""); got != 0 {
		t.Errorf("AngrealPower(\"\") = %d, want 0 (no gender ⇒ no affinity, no device)", got)
	}
}

func TestAngrealPower_NonAngrealEquipmentIgnored(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	// An ordinary equipped weapon is not an angreal — must not register.
	inst, _ := store.Spawn(swordTplWithMods())
	a.AddToInventory(inst.ID())
	a.Equip([]string{"wield"}, inst.ID(), nil)

	if got := a.AngrealPower("male"); got != 0 {
		t.Errorf("AngrealPower(male) = %d, want 0 (a plain sword is no angreal)", got)
	}
}
