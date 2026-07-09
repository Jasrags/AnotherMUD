package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// spawnHolderGun wields a fresh holder-fed pistol on a and returns the actor.
func spawnHolderGun(t *testing.T, store *entities.Store, a *connActor) {
	t.Helper()
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
}

// spawnClip mints a heavy-pistol clip preloaded to `loaded` and adds it to a's
// inventory, returning it so the test can assert on it later.
func spawnClip(t *testing.T, store *entities.Store, a *connActor, id string, loaded int) *entities.ItemInstance {
	t.Helper()
	clip, err := store.Spawn(&item.Template{
		ID: item.TemplateID(id), Name: "a clip", Type: "item",
		Keywords: []string{"clip"}, HolderFits: "heavy-pistol", Magazine: 5, AmmoKind: "bullet",
	})
	if err != nil {
		t.Fatalf("spawn clip %s: %v", id, err)
	}
	clip.SetMagazineLoaded(loaded)
	a.AddToInventory(clip.ID())
	return clip
}

// TestInsertHolderDeclinesWorseSpare proves the smart-swap guard (ammo-and-
// reloading §11): with a fuller clip already seated, InsertHolder declines to
// swap in a lesser spare — it returns "no-benefit", keeps the seated clip, and
// leaves the spare carried (not consumed, not ejected).
func TestInsertHolderDeclinesWorseSpare(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	spawnHolderGun(t, store, a)

	// Seat a clip with 4 rounds.
	spawnClip(t, store, a, "sr:seated", 4)
	if outcome, _, loaded, _, _, _, _ := a.InsertHolder(); outcome != "ok" || loaded != 4 {
		t.Fatalf("initial seat = (%q, %d), want (ok, 4)", outcome, loaded)
	}

	// Carry a worse spare (2 rounds). Reload should decline.
	spare := spawnClip(t, store, a, "sr:spare", 2)
	outcome, _, loaded, _, ejTpl, ejLoaded, _ := a.InsertHolder()
	if outcome != "no-benefit" {
		t.Fatalf("outcome = %q, want no-benefit", outcome)
	}
	if loaded != 4 {
		t.Fatalf("reported seated load = %d, want 4", loaded)
	}
	if ejTpl != "" || ejLoaded != 0 {
		t.Fatalf("declined swap should eject nothing, got (%q, %d)", ejTpl, ejLoaded)
	}
	if _, ok := store.GetByID(spare.ID()); !ok {
		t.Fatal("declined spare was consumed — it should stay carried")
	}
	// The seated clip is untouched at 4.
	if _, seated, _, has := currentGun(t, a).InsertedHolder(); !has || seated != 4 {
		t.Fatalf("seated after decline = (%d, has %v), want (4, true)", seated, has)
	}
}

// TestInsertHolderDeclinesEqualSpare proves the guard uses >= (an equal-load spare
// is also declined): swapping equal clips gains no rounds but churns a good clip
// onto the ground to decay.
func TestInsertHolderDeclinesEqualSpare(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	spawnHolderGun(t, store, a)

	spawnClip(t, store, a, "sr:seated", 3)
	if outcome, _, _, _, _, _, _ := a.InsertHolder(); outcome != "ok" {
		t.Fatalf("initial seat outcome = %q, want ok", outcome)
	}
	spare := spawnClip(t, store, a, "sr:spare", 3)
	if outcome, _, _, _, _, _, _ := a.InsertHolder(); outcome != "no-benefit" {
		t.Fatalf("equal-load spare outcome = %q, want no-benefit", outcome)
	}
	if _, ok := store.GetByID(spare.ID()); !ok {
		t.Fatal("equal-load spare was consumed — it should stay carried")
	}
}

// TestInsertHolderSwapsFullerSpare proves the guard still allows a genuine upgrade:
// a fuller spare swaps in, ejecting the lesser seated clip.
func TestInsertHolderSwapsFullerSpare(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	spawnHolderGun(t, store, a)

	spawnClip(t, store, a, "sr:seated", 2)
	if outcome, _, _, _, _, _, _ := a.InsertHolder(); outcome != "ok" {
		t.Fatalf("initial seat outcome = %q, want ok", outcome)
	}
	spare := spawnClip(t, store, a, "sr:spare", 4)
	outcome, _, loaded, _, ejTpl, ejLoaded, _ := a.InsertHolder()
	if outcome != "ok" || loaded != 4 {
		t.Fatalf("swap = (%q, %d), want (ok, 4)", outcome, loaded)
	}
	if ejTpl == "" || ejLoaded != 2 {
		t.Fatalf("swap should eject the 2-round seated clip, got (%q, %d)", ejTpl, ejLoaded)
	}
	if _, ok := store.GetByID(spare.ID()); ok {
		t.Fatal("swapped-in spare should be consumed/untracked")
	}
}

// currentGun returns the wielded weapon instance for assertions.
func currentGun(t *testing.T, a *connActor) *entities.ItemInstance {
	t.Helper()
	wid, ok := a.equipment[mainHandSlot]
	if !ok {
		t.Fatal("no wielded weapon")
	}
	e, ok := a.items.GetByID(wid)
	if !ok {
		t.Fatal("wielded weapon not tracked")
	}
	gun, ok := e.(*entities.ItemInstance)
	if !ok {
		t.Fatal("wielded entity is not an item")
	}
	return gun
}
