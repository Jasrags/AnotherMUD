package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// TestConsumeAmmoUntracksSpentRound proves the loose-round branch of ConsumeAmmo
// removes the fired round from the entity store, not just from inventory — closing
// the store-leak family (ammo-and-reloading review). With no wielded weapon, a
// ConsumeAmmo draws a loose round; after firing it must be gone from both the
// actor's inventory AND the store's id index.
func TestConsumeAmmoUntracksSpentRound(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)

	round, err := store.Spawn(&item.Template{
		ID: "sr:round", Name: "a round", Type: "item", AmmoKind: "bullet",
	})
	if err != nil {
		t.Fatalf("spawn round: %v", err)
	}
	a.AddToInventory(round.ID())

	if _, fired := a.ConsumeAmmo("bullet"); !fired {
		t.Fatal("ConsumeAmmo did not fire the loose round")
	}
	if _, ok := store.GetByID(round.ID()); ok {
		t.Fatal("spent round still tracked in the store — leak not closed")
	}
	if got := len(a.Inventory()); got != 0 {
		t.Fatalf("inventory has %d items after firing, want 0", got)
	}
}

// TestFillHolderUntracksPulledRounds proves pullAmmoLocked (reached via FillHolder)
// untracks each loose round it loads into a holder. The clip itself stays tracked;
// only the consumed rounds leave the store.
func TestFillHolderUntracksPulledRounds(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)

	clip, err := store.Spawn(&item.Template{
		ID: "sr:clip", Name: "a clip", Type: "item",
		Keywords: []string{"clip"}, HolderFits: "heavy-pistol", Magazine: 3, AmmoKind: "bullet",
	})
	if err != nil {
		t.Fatalf("spawn clip: %v", err)
	}
	a.AddToInventory(clip.ID())

	var roundIDs []entities.EntityID
	for range 3 {
		r, err := store.Spawn(&item.Template{ID: "sr:round", Name: "a round", Type: "item", AmmoKind: "bullet"})
		if err != nil {
			t.Fatalf("spawn round: %v", err)
		}
		a.AddToInventory(r.ID())
		roundIDs = append(roundIDs, r.ID())
	}

	if _, after, _, ok := a.FillHolder(clip.ID()); !ok || after != 3 {
		t.Fatalf("FillHolder = (after %d, ok %v), want (3, true)", after, ok)
	}
	for _, id := range roundIDs {
		if _, ok := store.GetByID(id); ok {
			t.Fatalf("round %s still tracked after fill — leak not closed", id)
		}
	}
	if _, ok := store.GetByID(clip.ID()); !ok {
		t.Fatal("clip should remain tracked after FillHolder")
	}
}

// TestInsertHolderUntracksConsumedClip proves InsertHolder untracks the clip it
// loads into the wielded weapon — the clip becomes the gun's abstract inserted-
// holder state, not a carried entity, so it must leave the store.
func TestInsertHolderUntracksConsumedClip(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)

	gun, err := store.Spawn(&item.Template{
		ID: "sr:predator", Name: "a pistol", Type: "weapon",
		Keywords: []string{"pistol"}, WeaponDamage: "2d6",
		RangedClass: "projectile", AmmoKind: "bullet", AcceptsHolder: "heavy-pistol",
	})
	if err != nil {
		t.Fatalf("spawn gun: %v", err)
	}
	a.AddToInventory(gun.ID())
	if !a.Equip([]string{"wield"}, gun.ID(), nil) {
		t.Fatal("could not wield the pistol")
	}

	clip, err := store.Spawn(&item.Template{
		ID: "sr:clip", Name: "a clip", Type: "item",
		Keywords: []string{"clip"}, HolderFits: "heavy-pistol", Magazine: 3, AmmoKind: "bullet",
	})
	if err != nil {
		t.Fatalf("spawn clip: %v", err)
	}
	a.AddToInventory(clip.ID())
	for range 3 {
		r, err := store.Spawn(&item.Template{ID: "sr:round", Name: "a round", Type: "item", AmmoKind: "bullet"})
		if err != nil {
			t.Fatalf("spawn round: %v", err)
		}
		a.AddToInventory(r.ID())
	}
	if _, after, _, ok := a.FillHolder(clip.ID()); !ok || after != 3 {
		t.Fatalf("FillHolder = (after %d, ok %v), want (3, true)", after, ok)
	}

	if outcome, _, _, _, _, _, _ := a.InsertHolder(); outcome != "ok" {
		t.Fatalf("InsertHolder outcome = %q, want ok", outcome)
	}
	if _, ok := store.GetByID(clip.ID()); ok {
		t.Fatal("inserted clip still tracked in the store — leak not closed")
	}
}
