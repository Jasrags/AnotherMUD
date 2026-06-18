package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/mount"
)

// A template with no Mount block spawns an ordinary, non-rideable mob: every
// mount accessor reports the inert default.
func TestSpawnMob_NotAMount(t *testing.T) {
	s := NewStore()
	inst, err := s.SpawnMob(&mob.Template{ID: "test:boar", Name: "a boar", Type: "npc"})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if inst.IsMount() {
		t.Error("IsMount() = true for a mob with no mount block")
	}
	if inst.TravelMax() != 0 || inst.Travel() != 0 {
		t.Errorf("non-mount travel = (%d/%d), want 0/0", inst.Travel(), inst.TravelMax())
	}
	if inst.TrySpendTravel(1) {
		t.Error("TrySpendTravel succeeded on a non-mount")
	}
	if inst.OwnerID() != "" {
		t.Errorf("OwnerID() = %q, want empty", inst.OwnerID())
	}
}

// A template with a Mount block spawns a mount: the descriptor resolves the
// temperament and seeds a full travel pool from the content ceiling.
func TestSpawnMob_IsMount(t *testing.T) {
	s := NewStore()
	inst, err := s.SpawnMob(&mob.Template{
		ID: "test:horse", Name: "a horse", Type: "npc",
		Mount: &mob.MountSpec{Temperament: "war", TravelMax: 60, TravelRegen: 5},
	})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if !inst.IsMount() {
		t.Fatal("IsMount() = false for a mob with a mount block")
	}
	if inst.Temperament() != mount.War {
		t.Errorf("Temperament() = %q, want war", inst.Temperament())
	}
	if inst.TravelMax() != 60 {
		t.Errorf("TravelMax() = %d, want 60", inst.TravelMax())
	}
	if inst.Travel() != 60 {
		t.Errorf("Travel() = %d, want 60 (full at spawn)", inst.Travel())
	}
	if inst.TravelRegenAmount() != 5 {
		t.Errorf("TravelRegenAmount() = %d, want 5", inst.TravelRegenAmount())
	}
}

// A mount with an unset temperament resolves to the cautious default.
func TestSpawnMob_DefaultTemperament(t *testing.T) {
	s := NewStore()
	inst, _ := s.SpawnMob(&mob.Template{
		ID: "test:pony", Name: "a pony", Type: "npc",
		Mount: &mob.MountSpec{TravelMax: 40},
	})
	if inst.Temperament() != mount.Default {
		t.Errorf("Temperament() = %q, want default %q", inst.Temperament(), mount.Default)
	}
}

// The travel pool meters spending: a step is refused once the budget is short,
// and regen restores it (never above the ceiling).
func TestMount_TravelSpendAndRegen(t *testing.T) {
	s := NewStore()
	inst, _ := s.SpawnMob(&mob.Template{
		ID: "test:horse", Name: "a horse", Type: "npc",
		Mount: &mob.MountSpec{TravelMax: 10},
	})
	if !inst.TrySpendTravel(7) {
		t.Fatal("TrySpendTravel(7) failed with 10 in the pool")
	}
	if inst.Travel() != 3 {
		t.Fatalf("Travel() = %d after spending 7, want 3", inst.Travel())
	}
	// 3 left, a 4-cost step is refused without mutating (the exhausted mount).
	if inst.TrySpendTravel(4) {
		t.Error("TrySpendTravel(4) succeeded with only 3 left")
	}
	if inst.Travel() != 3 {
		t.Errorf("Travel() = %d after a refused spend, want 3 (unchanged)", inst.Travel())
	}
	inst.RestoreTravel(100) // over-restore clamps at max
	if inst.Travel() != 10 {
		t.Errorf("Travel() = %d after over-restore, want 10 (capped)", inst.Travel())
	}
}

// Ownership is exclusive and queryable; an empty id never matches.
func TestMount_Ownership(t *testing.T) {
	s := NewStore()
	inst, _ := s.SpawnMob(&mob.Template{
		ID: "test:horse", Name: "a horse", Type: "npc",
		Mount: &mob.MountSpec{TravelMax: 10},
	})
	if inst.IsOwnedBy("alice") {
		t.Error("a fresh mount is owned by alice")
	}
	inst.SetOwner("alice")
	if !inst.IsOwnedBy("alice") {
		t.Error("SetOwner(alice) did not take")
	}
	if inst.IsOwnedBy("bob") || inst.IsOwnedBy("") {
		t.Error("mount matched a non-owner / empty id")
	}
	inst.SetOwner("bob") // exclusive: replaces
	if inst.IsOwnedBy("alice") || !inst.IsOwnedBy("bob") {
		t.Error("ownership transfer did not replace the prior owner")
	}
}

// Per-type impassable terrain bars the named terrains only.
func TestMount_CannotEnterTerrain(t *testing.T) {
	s := NewStore()
	inst, _ := s.SpawnMob(&mob.Template{
		ID: "test:horse", Name: "a horse", Type: "npc",
		Mount: &mob.MountSpec{TravelMax: 10, Impassable: []string{"cave", "indoors"}},
	})
	if !inst.CannotEnterTerrain("cave") {
		t.Error("expected the horse barred from cave")
	}
	if inst.CannotEnterTerrain("forest") {
		t.Error("horse should be able to enter forest")
	}
	if inst.CannotEnterTerrain("") {
		t.Error("empty terrain should never be impassable")
	}
}
