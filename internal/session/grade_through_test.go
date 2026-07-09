package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// TestGradeThroughHolder proves grade-through-holder (ammo-and-reloading §8): a
// clip filled with graded rounds carries the grade into the weapon, so a shot
// fired from an inserted clip reports the rounds' grade — which the round loop
// maps to a to-hit bonus. Exercises the whole session path: FillHolder captures
// the grade, InsertHolder moves it onto the gun, ConsumeAmmo returns it.
func TestGradeThroughHolder(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)

	// A holder-fed pistol, wielded.
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

	// A 3-round clip + three GRADED rounds carried loose.
	clip, err := store.Spawn(&item.Template{
		ID: "sr:clip", Name: "a clip", Type: "item",
		Keywords: []string{"clip"}, HolderFits: "heavy-pistol", Magazine: 3, AmmoKind: "bullet",
	})
	if err != nil {
		t.Fatalf("spawn clip: %v", err)
	}
	a.AddToInventory(clip.ID())
	for range 3 {
		r, err := store.Spawn(&item.Template{ID: "sr:apds", Name: "an APDS round", Type: "item", AmmoKind: "bullet", Grade: "apds"})
		if err != nil {
			t.Fatalf("spawn round: %v", err)
		}
		a.AddToInventory(r.ID())
	}

	// Fill the clip from the graded rounds — it captures the grade.
	if _, after, _, ok := a.FillHolder(clip.ID()); !ok || after != 3 {
		t.Fatalf("FillHolder = (after %d, ok %v), want (3, true)", after, ok)
	}
	if g := clip.HolderAmmoGrade(); g != "apds" {
		t.Fatalf("filled clip grade = %q, want apds", g)
	}

	// Insert the clip into the gun; the grade moves onto the inserted-holder state.
	if outcome, _, _, _, _, _, _ := a.InsertHolder(); outcome != "ok" {
		t.Fatalf("InsertHolder outcome = %q, want ok", outcome)
	}
	if _, _, g, has := gun.InsertedHolder(); !has || g != "apds" {
		t.Fatalf("inserted holder grade = (%q, has %v), want (apds, true)", g, has)
	}

	// Firing draws from the inserted clip and reports the rounds' grade.
	grade, fired := a.ConsumeAmmo("bullet")
	if !fired || grade != "apds" {
		t.Fatalf("ConsumeAmmo = (%q, %v), want (apds, true) — the clip's grade did not ride the shot", grade, fired)
	}
}
