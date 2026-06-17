package command_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/grade"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/stats"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// wieldThrown spawns a thrown-class weapon, gives it to the actor, and equips
// it into the wield slot. gradeKey "" leaves it ungraded. Returns its id.
func wieldThrown(t *testing.T, f *considerFixture, a *combatActor, gradeKey string) entities.EntityID {
	t.Helper()
	inst, err := f.store.Spawn(&item.Template{
		ID:            "test:throwing-knife",
		Name:          "a throwing knife",
		Type:          "weapon",
		Keywords:      []string{"knife"},
		WeaponDamage:  "1d4",
		EligibleSlots: []string{"wield"},
		RangedClass:   item.RangedThrown,
		Grade:         gradeKey,
	})
	if err != nil {
		t.Fatalf("Spawn knife: %v", err)
	}
	a.AddToInventory(inst.ID())
	if !a.Equip([]string{"wield"}, inst.ID(), []stats.Modifier{}) {
		t.Fatal("Equip knife returned false")
	}
	return inst.ID()
}

func throwManager(f *considerFixture, a *combatActor) *combat.Manager {
	return combat.NewManager(combat.MapLocator{
		f.guard.CombatantID(): f.guard,
		a.CombatantID():       a,
	}, &recordingSink{})
}

func TestThrow_NoThrownWeaponWielded(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	called := false
	env := f.env()
	env.Combat = throwManager(f, a)
	env.ResolveAttack = func(context.Context, combat.CombatantID, combat.CombatantID, world.RoomID) bool {
		called = true
		return false
	}
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "throw guard")

	if called {
		t.Error("ResolveAttack must not fire when nothing throwable is wielded")
	}
	if got := a.lastLine(); got != "You aren't wielding anything you can throw." {
		t.Errorf("line = %q, want the no-throwable refusal", got)
	}
}

func TestThrow_LandsWeaponInRoomAndResolves(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	knifeID := wieldThrown(t, f, a, "")

	called := false
	env := f.env()
	env.Combat = throwManager(f, a)
	env.ResolveAttack = func(_ context.Context, attacker, target combat.CombatantID, _ world.RoomID) bool {
		called = true
		if attacker != a.CombatantID() || target != f.guard.CombatantID() {
			t.Errorf("ResolveAttack ids = (%v,%v), want (%v,%v)",
				attacker, target, a.CombatantID(), f.guard.CombatantID())
		}
		return true
	}
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "throw guard")

	if !called {
		t.Fatal("ResolveAttack was not called on a valid throw")
	}
	if _, stillWielded := a.Equipment()["wield"]; stillWielded {
		t.Error("thrown weapon should no longer be wielded")
	}
	found := false
	for _, id := range f.place.InRoom(f.room.ID) {
		if id == knifeID {
			found = true
		}
	}
	if !found {
		t.Error("ungraded thrown weapon should land in the room (recoverable)")
	}
}

func TestThrow_GradedWeaponDestroyedOnUse(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	knifeID := wieldThrown(t, f, a, "masterwork")

	env := f.env()
	reg := grade.NewRegistry()
	reg.Register(grade.Grade{Key: "masterwork", Order: 1})
	env.Grades = reg
	env.Combat = throwManager(f, a)
	env.ResolveAttack = func(context.Context, combat.CombatantID, combat.CombatantID, world.RoomID) bool { return true }
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "throw guard")

	if _, stillWielded := a.Equipment()["wield"]; stillWielded {
		t.Error("thrown weapon should no longer be wielded")
	}
	for _, id := range f.place.InRoom(f.room.ID) {
		if id == knifeID {
			t.Error("a masterwork thrown weapon is destroyed on use, not landed")
		}
	}
}
