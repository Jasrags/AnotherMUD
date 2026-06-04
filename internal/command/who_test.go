package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
)

// stubRoster is a command.Roster that returns a fixed entry set. It hands
// back a fresh copy each call because WhoHandler sorts in place.
type stubRoster []command.WhoEntry

func (s stubRoster) OnlineRoster() []command.WhoEntry {
	return append([]command.WhoEntry(nil), s...)
}

func TestWho_SortedLinesTagsAndCount(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Zed", "p-z", f.room)
	env := f.env()
	env.Roster = stubRoster{
		{Name: "Zed"},
		{Name: "alice", RoleMarker: "Admin"},
		{Name: "Bob", Idle: true},
	}
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "who")

	out := a.lastLine()
	// Case-insensitive alphabetical: alice, Bob, Zed.
	ai, bi, zi := strings.Index(out, "alice"), strings.Index(out, "Bob"), strings.Index(out, "Zed")
	if ai < 0 || bi < 0 || zi < 0 || !(ai < bi && bi < zi) {
		t.Errorf("roster not alphabetically ordered: %q", out)
	}
	if !strings.Contains(out, "[Admin]") {
		t.Errorf("admin role marker missing: %q", out)
	}
	if !strings.Contains(out, "(idle)") {
		t.Errorf("idle marker missing: %q", out)
	}
	if !strings.Contains(out, "3 players online.") {
		t.Errorf("count line wrong: %q", out)
	}
}

func TestWho_SingularSummary(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Solo", "p-s", f.room)
	env := f.env()
	env.Roster = stubRoster{{Name: "Solo"}}
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "who")
	if got := a.lastLine(); !strings.Contains(got, "1 player online.") {
		t.Errorf("singular summary = %q, want '1 player online.'", got)
	}
}

func TestWho_EmptyRoster(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Nobody", "p-n", f.room)
	env := f.env()
	env.Roster = stubRoster{}
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "who")
	if got := a.lastLine(); !strings.Contains(got, "0 players online.") {
		t.Errorf("empty summary = %q, want '0 players online.'", got)
	}
}

func TestWho_NilRosterDegrades(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Alice", "p-a", f.room)
	r := newRegistry(t)
	// f.env() carries no Roster.
	dispatchActor(t, r, f.env(), a, "who")
	if got := a.lastLine(); !strings.Contains(got, "Nobody seems to be around") {
		t.Errorf("nil-roster who = %q, want graceful message", got)
	}
}
