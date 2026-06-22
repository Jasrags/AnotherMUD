package command_test

import (
	"context"
	"strings"
	"testing"
)

// assist makes the caller join the fight an ally is already in.
func TestAssist_JoinsAllyFight(t *testing.T) {
	f := newKillFixture(t)
	alice := newCombatActor("Alice", "p-1", f.room)
	bob := newCombatActor("Bob", "p-2", f.room)
	f.registerCombatant(alice)
	f.registerCombatant(bob)
	loc := &stubLocator{}
	loc.add(alice)
	loc.add(bob)
	env := f.env()
	env.Locator = loc
	r := newRegistry(t)

	// Alice engages the guard mob; Bob assists her into that fight.
	dispatchActor(t, r, env, alice, "kill guard")
	if !f.mgr.InCombat(alice.CombatantID()) {
		t.Fatal("alice should be fighting the guard")
	}
	dispatchActor(t, r, env, bob, "assist Alice")

	tgt, fighting := f.mgr.PrimaryTargetOf(bob.CombatantID())
	if !fighting || tgt != f.guard.CombatantID() {
		t.Fatalf("bob should be fighting the guard after assisting; fighting=%v tgt=%v", fighting, tgt)
	}
	if got := bob.lastLine(); !strings.Contains(got, "assist Alice") || !strings.Contains(got, f.guard.Name()) {
		t.Errorf("assist line = %q, want it to name Alice + the guard", got)
	}
}

func TestAssist_AllyNotFighting(t *testing.T) {
	f := newKillFixture(t)
	alice := newCombatActor("Alice", "p-1", f.room)
	bob := newCombatActor("Bob", "p-2", f.room)
	f.registerCombatant(alice)
	f.registerCombatant(bob)
	loc := &stubLocator{}
	loc.add(alice)
	loc.add(bob)
	env := f.env()
	env.Locator = loc
	r := newRegistry(t)

	dispatchActor(t, r, env, bob, "assist Alice")
	if got := bob.lastLine(); !strings.Contains(got, "isn't fighting anyone") {
		t.Errorf("assist of an idle ally = %q, want 'isn't fighting anyone'", got)
	}
	if f.mgr.InCombat(bob.CombatantID()) {
		t.Error("bob should not be in combat after assisting an idle ally")
	}
}

func TestAssist_Self(t *testing.T) {
	f := newKillFixture(t)
	bob := newCombatActor("Bob", "p-2", f.room)
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), f.env(), bob, "assist Bob"); err != nil {
		t.Fatal(err)
	}
	if got := bob.lastLine(); !strings.Contains(got, "can't assist yourself") {
		t.Errorf("self-assist = %q, want refusal", got)
	}
}
