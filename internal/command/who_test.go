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

// A wizinvis admin is excluded from a non-staff viewer's roster and count
// (who §4 / visibility §3.4).
func TestWho_AdminInvisibleHiddenFromNonAdmin(t *testing.T) {
	f := newInvFixture(t)
	viewer := newNamedTestActor("Pleb", "p-pleb", f.room) // not a RoleHolder → non-admin
	env := f.env()
	env.Roster = stubRoster{
		{Name: "Pleb", PlayerID: "p-pleb"},
		{Name: "Ghost", PlayerID: "p-ghost", AdminInvisible: true},
		{Name: "Bob", PlayerID: "p-bob"},
	}
	dispatchActor(t, newRegistry(t), env, viewer, "who")

	out := viewer.lastLine()
	if strings.Contains(out, "Ghost") {
		t.Errorf("a wizinvis admin must be hidden from a non-admin who: %q", out)
	}
	if !strings.Contains(out, "2 players online.") {
		t.Errorf("count must exclude the hidden admin: %q", out)
	}
}

// An admin viewer sees a wizinvis peer (equal rank pierces, §3.4).
func TestWho_AdminInvisibleVisibleToAdmin(t *testing.T) {
	f := newInvFixture(t)
	viewer := newRoleActor("Watcher", "p-watch", "admin")
	viewer.testActor.room = f.room
	env := f.env()
	env.AdminRole = "admin"
	env.Roster = stubRoster{
		{Name: "Watcher", PlayerID: "p-watch", RoleMarker: "Admin"},
		{Name: "Ghost", PlayerID: "p-ghost", AdminInvisible: true},
	}
	dispatchActor(t, newRegistry(t), env, viewer, "who")

	out := viewer.lastLine()
	if !strings.Contains(out, "Ghost") {
		t.Errorf("an admin must see a wizinvis peer in who: %q", out)
	}
	if !strings.Contains(out, "2 players online.") {
		t.Errorf("admin count must include the wizinvis peer: %q", out)
	}
}

// The viewer always sees their own row, even when admin-invisible and somehow
// not classed as staff (self is always visible, §2.1 — defensive self-guard).
func TestWho_SelfVisibleWhenWizinvis(t *testing.T) {
	f := newInvFixture(t)
	viewer := newNamedTestActor("Me", "p-me", f.room) // non-admin viewer
	env := f.env()
	env.Roster = stubRoster{
		{Name: "Me", PlayerID: "p-me", AdminInvisible: true},
		{Name: "Other", PlayerID: "p-other", AdminInvisible: true},
	}
	dispatchActor(t, newRegistry(t), env, viewer, "who")

	out := viewer.lastLine()
	if !strings.Contains(out, "Me") {
		t.Errorf("viewer must always see their own row: %q", out)
	}
	if strings.Contains(out, "Other") {
		t.Errorf("another wizinvis character must stay hidden: %q", out)
	}
	if !strings.Contains(out, "1 player online.") {
		t.Errorf("count should be self only: %q", out)
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
