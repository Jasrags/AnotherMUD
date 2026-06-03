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

// look (visible scope) and consider (entity scope) carry completion args
// from the HandParsed sweep, so suggest lists their candidates too — no
// per-verb wiring; suggest is generic over the completion query.
func TestSuggest_LookAndConsider(t *testing.T) {
	f := newConsiderFixture(t) // a village guard is in the room
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "suggest look gu")
	if got := a.lastLine(); !strings.Contains(got, "look guard") {
		t.Errorf("suggest look gu = %q, want 'look guard'", got)
	}
	dispatchActor(t, r, f.env(), a, "suggest consider gu")
	if got := a.lastLine(); !strings.Contains(got, "consider guard") {
		t.Errorf("suggest consider gu = %q, want 'consider guard'", got)
	}
	dispatchActor(t, r, f.env(), a, "suggest con gu") // con = consider alias
	if got := a.lastLine(); !strings.Contains(got, "guard") {
		t.Errorf("suggest con gu (alias) = %q, want a guard suggestion", got)
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
