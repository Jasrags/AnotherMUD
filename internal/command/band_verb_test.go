package command_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// engageForBands builds a manager, engages the actor with the guard, points the
// env at that manager, and returns both so a test can dispatch advance/withdraw
// and inspect the resulting band.
func engageForBands(t *testing.T, f *considerFixture, a *combatActor) *combat.Manager {
	t.Helper()
	m := throwManager(f, a)
	if _, ok := m.EngageWithReason(context.Background(), a.CombatantID(), f.guard.CombatantID(), f.room.ID); !ok {
		t.Fatal("engage failed")
	}
	return m
}

func TestWithdraw_OpensTheBand(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room) // melee → opens at melee
	m := engageForBands(t, f, a)
	env := f.env()
	env.Combat = m
	r := newRegistry(t)

	dispatchActor(t, r, env, a, "withdraw")
	if got := m.BandOf(a.CombatantID(), f.guard.CombatantID()); got == 0 {
		t.Error("withdraw should open the band away from melee (got still melee)")
	}
}

func TestAdvance_AtMeleeIsRefused(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room) // melee → opens at melee
	env := f.env()
	env.Combat = engageForBands(t, f, a)
	r := newRegistry(t)

	dispatchActor(t, r, env, a, "advance")
	if got := a.lastLine(); got != "You're already in melee range." {
		t.Errorf("advance at melee = %q, want the already-in-melee refusal", got)
	}
}

func TestBandVerbs_NotFighting(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	env := f.env()
	env.Combat = throwManager(f, a) // a manager, but no engagement
	r := newRegistry(t)

	for _, verb := range []string{"advance", "withdraw"} {
		dispatchActor(t, r, env, a, verb)
		if got := a.lastLine(); got != "You aren't fighting anyone." {
			t.Errorf("%s when not fighting = %q, want the not-fighting refusal", verb, got)
		}
	}
}
