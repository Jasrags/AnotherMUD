package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// special-weapons §8 — worn/visible gear with a `reputation` delta folds into the
// actor's effective renown (a masterwork blade +1, a Trolloc scythesword −2). The
// sum is cached at equip time and read by EffectiveRenown / RenownTier.

// Equipping reputation-bearing gear raises (and lowers) the worn-reputation sum,
// which flows into EffectiveRenown.
func TestGearReputation_FoldsIntoEffectiveRenown(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)

	if got := a.WornReputation(); got != 0 {
		t.Fatalf("bare actor WornReputation = %d, want 0", got)
	}

	famed, _ := store.Spawn(&item.Template{
		ID: "x:heron-blade", Name: "a heron-mark blade", Type: "weapon",
		Keywords: []string{"blade"}, WeaponDamage: "1d8", Reputation: 1, // masterwork: +1
	})
	shameful, _ := store.Spawn(&item.Template{
		ID: "x:trolloc-maul", Name: "a Trolloc scythesword", Type: "item",
		Keywords: []string{"scythesword"}, EligibleSlots: []string{"body"}, Reputation: -2,
	})
	a.AddToInventory(famed.ID())
	a.AddToInventory(shameful.ID())

	if !a.Equip([]string{"wield"}, famed.ID(), nil) {
		t.Fatal("equip the heron blade")
	}
	if got := a.WornReputation(); got != 1 {
		t.Fatalf("with the +1 blade, WornReputation = %d, want 1", got)
	}
	// EffectiveRenown = base renown (0) + Fame feat bonus (0) + worn gear (+1).
	if got := a.EffectiveRenown(); got != 1 {
		t.Errorf("EffectiveRenown = %d, want 1 (worn-gear fold)", got)
	}

	if !a.Equip([]string{"body"}, shameful.ID(), nil) {
		t.Fatal("equip the scythesword")
	}
	if got := a.WornReputation(); got != -1 { // +1 − 2
		t.Fatalf("with both pieces, WornReputation = %d, want -1", got)
	}

	// Removing the infamous piece restores the +1.
	if _, ok := a.Unequip("body"); !ok {
		t.Fatal("unequip the scythesword")
	}
	if got := a.WornReputation(); got != 1 {
		t.Errorf("after unequip, WornReputation = %d, want 1", got)
	}
}

// Ordinary gear (no reputation field) contributes nothing — the common case is
// unchanged.
func TestGearReputation_PlainGearZero(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	plain, _ := store.Spawn(&item.Template{
		ID: "x:sword", Name: "a sword", Type: "weapon",
		Keywords: []string{"sword"}, WeaponDamage: "1d8", // no Reputation
	})
	a.AddToInventory(plain.ID())
	if !a.Equip([]string{"wield"}, plain.ID(), nil) {
		t.Fatal("equip the plain sword")
	}
	if got := a.WornReputation(); got != 0 {
		t.Errorf("plain gear WornReputation = %d, want 0", got)
	}
}
