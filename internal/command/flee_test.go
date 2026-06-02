package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// fakeFlee returns a closure that records the call and returns the
// configured outcome. Tests assert the verb's message-mapping per
// outcome without spinning up a full FleeConfig.
func fakeFlee(outcome combat.FleeOutcome, called *bool) func(context.Context, combat.CombatantID) combat.FleeOutcome {
	return func(_ context.Context, _ combat.CombatantID) combat.FleeOutcome {
		if called != nil {
			*called = true
		}
		return outcome
	}
}

func TestFlee_SuccessMessage(t *testing.T) {
	f := newKillFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)
	env := f.env()
	called := false
	env.Flee = fakeFlee(combat.FleeOutcomeSuccess, &called)

	dispatchActor(t, r, env, a, "flee")
	if !called {
		t.Error("Flee closure not invoked")
	}
	// Success writes the panic line AND renders the destination room
	// (the actor is already in the new room by the time Flee returns).
	out := allLines(a.testActor)
	if !strings.Contains(out, "panic") {
		t.Errorf("flee success should announce panic: %q", out)
	}
	if !strings.Contains(out, "Square") {
		t.Errorf("flee should render the destination room: %q", out)
	}
}

func TestFlee_OutcomeMessages(t *testing.T) {
	cases := []struct {
		name    string
		outcome combat.FleeOutcome
		want    string
	}{
		{"prevented", combat.FleeOutcomePrevented, "stops you"},
		{"no exits", combat.FleeOutcomeFailedNoExits, "nowhere to run"},
		{"unknown room", combat.FleeOutcomeFailedUnknownRoom, "stumble"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newKillFixture(t)
			a := newCombatActor("Alice", "p-1", f.room)
			r := newRegistry(t)
			env := f.env()
			env.Flee = fakeFlee(tc.outcome, nil)

			dispatchActor(t, r, env, a, "flee")
			if got := a.lastLine(); !strings.Contains(got, tc.want) {
				t.Errorf("outcome %v = %q, want substring %q", tc.outcome, got, tc.want)
			}
		})
	}
}

func TestFlee_NoFleeClosureRefuses(t *testing.T) {
	f := newKillFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)
	// env.Flee unset (nil) — verb should refuse cleanly.
	dispatchActor(t, r, f.env(), a, "flee")
	if got := a.lastLine(); !strings.Contains(got, "can't flee") {
		t.Errorf("nil-flee env = %q, want refusal", got)
	}
}

func TestFlee_NonCombatantRefuses(t *testing.T) {
	// A plain testActor doesn't satisfy combat.Combatant — verb hits
	// the early refusal before consulting env.Flee.
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newNamedTestActor("Plain", "p-1", f.room)
	env := f.env()
	called := false
	env.Flee = fakeFlee(combat.FleeOutcomeSuccess, &called)
	dispatchActor(t, r, env, a, "flee")
	if called {
		t.Error("Flee closure invoked on non-combatant actor")
	}
	if got := a.lastLine(); !strings.Contains(got, "can't flee from anything") {
		t.Errorf("non-combatant flee = %q", got)
	}
}
