package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/condition"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// downedMob spawns + places a guard and optionally flags it unconscious (the
// helpless state the rob/finish verbs gate on).
func downedMob(t *testing.T, f *lootFixture, unconscious bool) (*entities.MobInstance, *progression.EffectManager) {
	t.Helper()
	m, err := f.store.SpawnMob(guardTplForConsider())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	f.place.Place(m.ID(), f.room.ID)
	em := progression.NewEffectManager(nil, nil)
	if unconscious {
		em.Apply(context.Background(), string(m.ID()),
			progression.EffectTemplate{ID: "unconscious", Duration: 10, Flags: []string{condition.FlagUnconscious}}, "", "")
	}
	return m, em
}

func dispatchDowned(t *testing.T, env command.Env, a command.Actor, line string) {
	t.Helper()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), env, a, line); err != nil {
		t.Fatalf("dispatch %q: %v", line, err)
	}
}

func TestRob_HelplessMobTransfersItemsLeavesAlive(t *testing.T) {
	f := newLootFixture(t)
	m, em := downedMob(t, f, true)
	it, err := f.store.Spawn(ration())
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	f.contents.Put(m.ID(), it.ID())

	env := f.env()
	env.Effects = em
	env.RobCoins = func(string) int { return 250 }
	a := newCombatActor("Runner", "p-1", f.room)

	dispatchDowned(t, env, a, "rob guard")

	if !containsID(a.Inventory(), it.ID()) {
		t.Errorf("robbed item not in looter inventory: %v", a.Inventory())
	}
	if !m.IsLooted() {
		t.Error("robbed mob not marked looted (re-rob / corpse-coin guard)")
	}
	// Non-lethal: the mob is still in the room (not killed).
	if !containsID(f.place.InRoom(f.room.ID), m.ID()) {
		t.Error("robbed mob should remain alive in the room")
	}
	if got := a.lastLine(); !strings.Contains(got, "ration") {
		t.Errorf("rob message = %q, want the looted item named", got)
	}
}

func TestRob_ConsciousMobRefused(t *testing.T) {
	f := newLootFixture(t)
	_, em := downedMob(t, f, false) // awake
	env := f.env()
	env.Effects = em
	a := newCombatActor("Runner", "p-1", f.room)
	dispatchDowned(t, env, a, "rob guard")
	if got := a.lastLine(); !strings.Contains(got, "isn't helpless") {
		t.Errorf("rob of a conscious foe = %q, want a helpless refusal", got)
	}
}

func TestRob_AlreadyLootedRefused(t *testing.T) {
	f := newLootFixture(t)
	m, em := downedMob(t, f, true)
	m.ClaimLooted()
	env := f.env()
	env.Effects = em
	a := newCombatActor("Runner", "p-1", f.room)
	dispatchDowned(t, env, a, "rob guard")
	if got := a.lastLine(); !strings.Contains(got, "picked clean") {
		t.Errorf("re-rob = %q, want an already-looted refusal", got)
	}
}

func TestFinish_HelplessInvokesSeam(t *testing.T) {
	f := newLootFixture(t)
	_, em := downedMob(t, f, true)
	called := false
	env := f.env()
	env.Effects = em
	env.Finish = func(context.Context, combat.CombatantID, combat.CombatantID, world.RoomID) bool {
		called = true
		return true
	}
	a := newCombatActor("Runner", "p-1", f.room)
	dispatchDowned(t, env, a, "finish guard")
	if !called {
		t.Error("Finish seam not invoked for a helpless target")
	}
	if got := a.lastLine(); !strings.Contains(got, "killing blow") {
		t.Errorf("finish message = %q", got)
	}
}

func TestFinish_ConsciousRefused(t *testing.T) {
	f := newLootFixture(t)
	_, em := downedMob(t, f, false) // awake
	called := false
	env := f.env()
	env.Effects = em
	env.Finish = func(context.Context, combat.CombatantID, combat.CombatantID, world.RoomID) bool {
		called = true
		return true
	}
	a := newCombatActor("Runner", "p-1", f.room)
	dispatchDowned(t, env, a, "finish guard")
	if called {
		t.Error("Finish must not fire on a conscious foe (would be a free instakill)")
	}
	if got := a.lastLine(); !strings.Contains(got, "isn't helpless") {
		t.Errorf("finish of a conscious foe = %q, want a helpless refusal", got)
	}
}
