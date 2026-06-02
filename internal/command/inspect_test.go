package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// allLines joins every line an actor has received, for asserting on the
// multi-line inspect dump.
func allLines(a *testActor) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return strings.Join(a.lines, "\n")
}

// adminInRoom builds an admin actor (HasRole "admin") positioned in the
// fixture room, so it passes the M19.3 gate and findCombatantInRoom can
// resolve room targets relative to it.
func adminInRoom(f *considerFixture, name, playerID string) *roleActor {
	a := newRoleActor(name, playerID, "admin")
	a.SetRoom(f.room)
	return a
}

// No argument inspects the actor: the header renders and one admin.action
// fires with the actor as its own target and "self" as the args.
func TestInspect_SelfAudits(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "inspect")

	out := allLines(admin.testActor)
	if !strings.Contains(out, "yourself") || !strings.Contains(out, "player") {
		t.Errorf("self-inspect header = %q, want 'yourself (player)'", out)
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	ev := (*got)[0].(eventbus.AdminAction)
	if ev.Verb != "inspect" || ev.Target != "p-admin" || ev.Args != "self" {
		t.Errorf("event = %+v, want verb=inspect target=p-admin args=self", ev)
	}
}

// inspect is admin-gated (§2): a non-admin gets the unknown-verb "Huh?",
// no dump and no audit.
func TestInspect_RefusedForNonAdmin(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	bob := newRoleActor("Bob", "p-bob") // no admin role
	bob.SetRoom(f.room)
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, bob, "inspect")

	if bob.lastLine() != "Huh?" {
		t.Errorf("refusal = %q, want 'Huh?'", bob.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a refused inspect must not audit, got %d", len(*got))
	}
}

// inspect resolves a mob in the room and dumps its identity, vitals, and
// stats; the audit carries the mob's entity id as the target.
func TestInspect_RoomMobDumpsRecordAndAudits(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "inspect guard")

	out := allLines(admin.testActor)
	if !strings.Contains(out, "a village guard") || !strings.Contains(out, "npc") {
		t.Errorf("mob header = %q, want 'a village guard (npc)'", out)
	}
	if !strings.Contains(out, "40/40 HP") {
		t.Errorf("mob vitals = %q, want '40/40 HP'", out)
	}
	if !strings.Contains(out, "AC 14") {
		t.Errorf("mob stats = %q, want 'AC 14'", out)
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	ev := (*got)[0].(eventbus.AdminAction)
	if ev.Verb != "inspect" || ev.Target != f.guard.EntityID() || ev.Args != "guard" {
		t.Errorf("event = %+v, want verb=inspect target=%s args=guard", ev, f.guard.EntityID())
	}
}

// An unresolved target reports the miss and audits nothing (resolution
// fails before the audit point).
func TestInspect_NotFoundNoAudit(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "inspect ghost")

	if !strings.Contains(admin.lastLine(), "don't see") {
		t.Errorf("miss = %q, want a not-found message", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a missed inspect must not audit, got %d", len(*got))
	}
}
