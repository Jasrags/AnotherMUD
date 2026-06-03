package command_test

import (
	"strings"
	"testing"
)

// The player-facing `suggest` verb runs the completion query through
// dispatch (which wires the registry back-ref) and lists candidates.
func TestSuggest_ListsArgCandidates(t *testing.T) {
	f := newConsiderFixture(t) // a village guard mob is placed in the room
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "suggest kill gu")
	if got := a.lastLine(); !strings.Contains(got, "kill guard") {
		t.Errorf("suggest kill gu = %q, want it to suggest 'kill guard'", got)
	}
}

func TestSuggest_NoArgGuidance(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "suggest")
	if got := a.lastLine(); !strings.Contains(got, "Suggest what?") {
		t.Errorf("bare suggest = %q, want guidance", got)
	}
}

func TestSuggest_UnknownReportsNone(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "suggest frobnicate x")
	if got := a.lastLine(); !strings.Contains(got, "No suggestions") {
		t.Errorf("suggest unknown = %q, want 'No suggestions'", got)
	}
}
