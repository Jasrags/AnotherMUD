package main

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// responsiblePlayer resolves a MobKilled killer id to the player who bears
// responsibility (hireable-mobs.md §6): a direct player killer, a hireling's
// owner (viaHireling), or no one for a wild/unknown killer.
func TestResponsiblePlayer(t *testing.T) {
	store := entities.NewStore()

	hire, err := store.SpawnMob(&mob.Template{ID: "sw:sellsword", Name: "a sellsword", Type: "npc"})
	if err != nil {
		t.Fatalf("SpawnMob hireling: %v", err)
	}
	hire.SetOwner("boss")
	hire.SetHireling(true)
	hireCID := string(combat.NewMobCombatantID(hire.EntityID()))

	wild, err := store.SpawnMob(&mob.Template{ID: "wolf", Name: "a wolf", Type: "npc"})
	if err != nil {
		t.Fatalf("SpawnMob wild: %v", err)
	}
	wildCID := string(combat.NewMobCombatantID(wild.EntityID()))

	// A half-initialized hireling (marked but never stamped with an owner) must
	// resolve to no one, not panic or credit an empty owner.
	orphan, err := store.SpawnMob(&mob.Template{ID: "sw:stray", Name: "a stray", Type: "npc"})
	if err != nil {
		t.Fatalf("SpawnMob orphan: %v", err)
	}
	orphan.SetHireling(true) // no SetOwner
	orphanCID := string(combat.NewMobCombatantID(orphan.EntityID()))

	for _, tc := range []struct {
		name         string
		killerID     string
		wantPID      string
		wantHireling bool
	}{
		{"direct player", string(combat.NewPlayerCombatantID("p-hex")), "p-hex", false},
		{"hireling → owner", hireCID, "boss", true},
		{"wild mob → no one", wildCID, "", false},
		{"owner-less hireling → no one", orphanCID, "", false},
		{"unknown id → no one", "mob:does-not-exist", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pid, via := responsiblePlayer(store, tc.killerID)
			if pid != tc.wantPID || via != tc.wantHireling {
				t.Errorf("responsiblePlayer(%q) = (%q, %v), want (%q, %v)",
					tc.killerID, pid, via, tc.wantPID, tc.wantHireling)
			}
		})
	}
}
