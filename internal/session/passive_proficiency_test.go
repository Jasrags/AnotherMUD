package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// constRoller returns roll value v+1 (IntN+1) every call — enough for
// the single binary check the integration test exercises.
type constRoller int

func (c constRoller) IntN(n int) int {
	if n <= 0 {
		return 0
	}
	return int(c) % n
}

// secondAttackRegistry returns an ability registry holding a single
// extra-attack passive whose binary chance equals proficiency exactly
// (variance/maxChance 100), with no gain (GainBaseChance 0) so the
// resolver consumes exactly one roll per evaluation.
func secondAttackRegistry(t *testing.T) *progression.AbilityRegistry {
	t.Helper()
	reg := progression.NewAbilityRegistry()
	if err := reg.Register(&progression.Ability{
		ID: "second-attack", Type: progression.AbilityPassive,
		Category: progression.AbilitySkill, Hook: progression.HookExtraAttack,
		Variance: 100, MaxHitChance: 100,
	}); err != nil {
		t.Fatalf("register second-attack: %v", err)
	}
	return reg
}

// spawnGuardWithSecondAttack spawns a mob carrying a second-attack
// proficiency and returns the store + the mob's bare entity id.
func spawnGuardWithSecondAttack(t *testing.T, prof int) (*entities.Store, string) {
	t.Helper()
	store := entities.NewStore()
	inst, err := store.SpawnMob(&mob.Template{
		ID: "tapestry-core:village-guard", Name: "a village guard",
		Type: "npc", Behavior: "stationary",
		Stats:         map[string]int{"hp_max": 22, "str": 10},
		Proficiencies: map[string]int{"second-attack": prof},
	})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	return store, inst.EntityID()
}

// The composite resolves a player's proficiency through the manager and
// a mob's through the entity store, behind one entity-id-keyed surface.
func TestPassiveProficiency_RoutesPlayerThenMob(t *testing.T) {
	reg := secondAttackRegistry(t)
	mgr := progression.NewProficiencyManager(reg, progression.ProficiencyConfig{})
	mgr.Learn("player-hero", "second-attack", 40)
	store, mobID := spawnGuardWithSecondAttack(t, 50)

	pp := NewPassiveProficiency(mgr, store)

	if v, ok := pp.Proficiency("player-hero", "second-attack"); !ok || v != 40 {
		t.Errorf("player Proficiency = (%d, %v), want (40, true)", v, ok)
	}
	if v, ok := pp.Proficiency(mobID, "second-attack"); !ok || v != 50 {
		t.Errorf("mob Proficiency = (%d, %v), want (50, true)", v, ok)
	}
	if v, ok := pp.Proficiency("nobody", "second-attack"); ok || v != 0 {
		t.Errorf("unknown id Proficiency = (%d, %v), want (0, false)", v, ok)
	}
	if !pp.Has(mobID, "second-attack") {
		t.Error("Has(mob, second-attack) = false, want true")
	}
	if pp.Has(mobID, "parry") {
		t.Error("Has(mob, parry) = true, want false — not seeded")
	}
}

// Cap reports the player's per-ability cap, and the global ceiling for a
// mob (which has no per-ability caps).
func TestPassiveProficiency_Cap(t *testing.T) {
	reg := secondAttackRegistry(t)
	mgr := progression.NewProficiencyManager(reg, progression.ProficiencyConfig{})
	mgr.Learn("player-hero", "second-attack", 40)
	store, mobID := spawnGuardWithSecondAttack(t, 50)

	pp := NewPassiveProficiency(mgr, store)
	if got := pp.Cap(mobID, "second-attack"); got != 100 {
		t.Errorf("mob Cap = %d, want 100 (global ceiling)", got)
	}
	if got := pp.Cap("player-hero", "second-attack"); got <= 0 {
		t.Errorf("player Cap = %d, want positive", got)
	}
}

// CRITICAL: AddProficiency on a mob id must be a no-op — routing it to
// the player manager would seed (and leak) a per-mob entry there.
func TestPassiveProficiency_MobGainIsNoLeak(t *testing.T) {
	reg := secondAttackRegistry(t)
	mgr := progression.NewProficiencyManager(reg, progression.ProficiencyConfig{})
	store, mobID := spawnGuardWithSecondAttack(t, 50)

	pp := NewPassiveProficiency(mgr, store)
	pp.AddProficiency(mobID, "second-attack", 1) // would-be gain

	if mgr.Has(mobID, "second-attack") {
		t.Fatal("mob id leaked into the player ProficiencyManager via AddProficiency")
	}
	// A real player still trains.
	pp.AddProficiency("player-hero", "second-attack", 1)
	if !mgr.Has("player-hero", "second-attack") {
		t.Error("player AddProficiency did not reach the manager")
	}
}

// End-to-end: a real PassiveResolver reading through the composite earns
// a mob an extra swing from its content-defined second-attack — proving
// the combat passive path lights up for mobs (M9.5 #3).
func TestPassiveProficiency_MobEarnsExtraAttack(t *testing.T) {
	reg := secondAttackRegistry(t)
	mgr := progression.NewProficiencyManager(reg, progression.ProficiencyConfig{})
	store, mobID := spawnGuardWithSecondAttack(t, 50)

	pp := NewPassiveProficiency(mgr, store)
	// constRoller(0) → IntN+1 == 1 ≤ 50 (chance) → the passive fires.
	resolver := progression.NewPassiveResolver(reg, pp, pp, constRoller(0))

	if got := resolver.ExtraAttacks(mobID); got != 1 {
		t.Errorf("ExtraAttacks(mob) = %d, want 1 (second-attack fired)", got)
	}
	// An id the composite can't resolve (not a player, not a tracked
	// mob) reads proficiency 0 and earns no extra swing.
	if got := resolver.ExtraAttacks("not-in-store"); got != 0 {
		t.Errorf("ExtraAttacks(unknown) = %d, want 0", got)
	}
}
