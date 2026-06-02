package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// An admin sets a mob's HP: the live vital changes, the confirmation
// reports the new fraction, and one admin.action fires with the kind/type/
// value in its args.
func TestSet_VitalHPOnMobLivesAndAudits(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "set vital hp guard 10")

	if cur, _ := f.guard.Vitals().Snapshot(); cur != 10 {
		t.Errorf("guard HP = %d, want 10", cur)
	}
	if !strings.Contains(admin.lastLine(), "HP set to 10/40") {
		t.Errorf("confirmation = %q, want 'HP set to 10/40'", admin.lastLine())
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	ev := (*got)[0].(eventbus.AdminAction)
	if ev.Verb != "set" || ev.Target != f.guard.EntityID() || ev.Args != "vital hp=10" {
		t.Errorf("event = %+v, want verb=set target=%s args='vital hp=10'", ev, f.guard.EntityID())
	}
}

// A value above the target's maximum clamps to max (§4).
func TestSet_VitalClampsToMax(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()

	dispatchRole(t, env, admin, "set vital hp guard 999")

	if cur, max := f.guard.Vitals().Snapshot(); cur != max {
		t.Errorf("guard HP = %d/%d, want clamped to max", cur, max)
	}
}

// A non-numeric vital value is a usage error that writes nothing and
// audits nothing (§4).
func TestSet_VitalNonNumericRefused(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "set vital hp guard abc")

	if cur, _ := f.guard.Vitals().Snapshot(); cur != 40 {
		t.Errorf("guard HP = %d, want 40 (unchanged on bad value)", cur)
	}
	if !strings.Contains(admin.lastLine(), "whole number") {
		t.Errorf("message = %q, want a numeric usage error", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a refused set must not audit, got %d", len(*got))
	}
}

// An unknown vital type is refused without writing.
func TestSet_UnknownVitalRefused(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()

	dispatchRole(t, env, admin, "set vital mana guard 5")

	if !strings.Contains(admin.lastLine(), "Unknown vital") {
		t.Errorf("message = %q, want 'Unknown vital'", admin.lastLine())
	}
}

// An unknown kind reports it and falls through to the usage panel.
func TestSet_UnknownKindShowsUsage(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()

	dispatchRole(t, env, admin, "set wibble x guard 5")

	out := allLines(admin.testActor)
	if !strings.Contains(out, "Unknown set kind") {
		t.Errorf("output = %q, want 'Unknown set kind'", out)
	}
	if !strings.Contains(out, "vital") || !strings.Contains(out, "hp") {
		t.Errorf("usage panel = %q, want it to list vital(hp)", out)
	}
}

// A bare set renders the self-documenting usage panel and audits nothing.
func TestSet_BareRendersUsagePanel(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "set")

	out := allLines(admin.testActor)
	if !strings.Contains(out, "Usage: set") || !strings.Contains(out, "vital") {
		t.Errorf("usage panel = %q, want grammar + vital kind", out)
	}
	if len(*got) != 0 {
		t.Errorf("bare set must not audit, got %d", len(*got))
	}
}

// set is admin-gated (§2): a non-admin gets the unknown-verb "Huh?", with
// no write and no audit — and no disclosure that the verb exists.
func TestSet_RefusedForNonAdmin(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	bob := newRoleActor("Bob", "p-bob") // no admin role
	bob.SetRoom(f.room)
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, bob, "set vital hp guard 10")

	if bob.lastLine() != "Huh?" {
		t.Errorf("refusal = %q, want 'Huh?'", bob.lastLine())
	}
	if cur, _ := f.guard.Vitals().Snapshot(); cur != 40 {
		t.Errorf("guard HP = %d, want 40 (non-admin must not write)", cur)
	}
	if len(*got) != 0 {
		t.Errorf("a refused set must not audit, got %d", len(*got))
	}
}
