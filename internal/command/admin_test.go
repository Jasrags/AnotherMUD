package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// fakeAnnouncer captures every SendToAll broadcast so tests can assert on
// the lines that reached the (mocked) all-sessions fan-out.
type fakeAnnouncer struct {
	lines   []string
	exclude [][]string
}

func (f *fakeAnnouncer) SendToAll(_ context.Context, text string, excludePlayerIDs ...string) {
	f.lines = append(f.lines, text)
	f.exclude = append(f.exclude, excludePlayerIDs)
}

// An admin announces: the message reaches the all-sessions broadcast
// attributed as an administrative announcement, and exactly one
// admin.action audit fact fires carrying actor/verb/args (no target).
func TestAnnounce_BroadcastsToAllAndAudits(t *testing.T) {
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	ann := &fakeAnnouncer{}
	admin := newRoleActor("Maerys", "p-admin", "admin")
	env := command.Env{Bus: bus, Announcer: ann}

	dispatchRole(t, env, admin, "announce the realm trembles")

	if len(ann.lines) != 1 {
		t.Fatalf("broadcast count = %d, want 1", len(ann.lines))
	}
	if !strings.Contains(ann.lines[0], "the realm trembles") {
		t.Errorf("broadcast %q missing the message", ann.lines[0])
	}
	if !strings.Contains(ann.lines[0], "Announcement") {
		t.Errorf("broadcast %q not attributed as an announcement", ann.lines[0])
	}
	// No self-exclusion — an announcement is a fact the whole server sees.
	if len(ann.exclude[0]) != 0 {
		t.Errorf("announce should exclude no one, got %v", ann.exclude[0])
	}

	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	ev := (*got)[0].(eventbus.AdminAction)
	if ev.Actor != "p-admin" || ev.Verb != "announce" || ev.Target != "" {
		t.Errorf("event = %+v, want actor=p-admin verb=announce target=''", ev)
	}
	if ev.Args != "the realm trembles" {
		t.Errorf("event args = %q, want the message", ev.Args)
	}
}

// A bare `announce` renders usage and neither broadcasts nor audits.
func TestAnnounce_EmptyMessageUsage(t *testing.T) {
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	ann := &fakeAnnouncer{}
	admin := newRoleActor("Maerys", "p-admin", "admin")
	env := command.Env{Bus: bus, Announcer: ann}

	dispatchRole(t, env, admin, "announce")

	if !strings.Contains(admin.lastLine(), "Usage") {
		t.Errorf("message = %q, want usage", admin.lastLine())
	}
	if len(ann.lines) != 0 {
		t.Errorf("empty announce should not broadcast, got %v", ann.lines)
	}
	if len(*got) != 0 {
		t.Errorf("empty announce should not audit, got %d", len(*got))
	}
}

// With no Announcer wired, announce reports it's not enabled and audits
// nothing (the broadcast never happened, so there's nothing to record).
func TestAnnounce_NilAnnouncerDisabled(t *testing.T) {
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := newRoleActor("Maerys", "p-admin", "admin")
	env := command.Env{Bus: bus} // no Announcer

	dispatchRole(t, env, admin, "announce hello")

	if !strings.Contains(admin.lastLine(), "not enabled") {
		t.Errorf("message = %q, want 'not enabled'", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("disabled announce should not audit, got %d", len(*got))
	}
}

// announce is admin-gated (§2): a non-admin gets the same "Huh?" an unknown
// verb produces — no broadcast, no audit, no disclosure the verb exists.
func TestAnnounce_RefusedForNonAdmin(t *testing.T) {
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	ann := &fakeAnnouncer{}
	bob := newRoleActor("Bob", "p-bob") // no admin role
	env := command.Env{Bus: bus, Announcer: ann}

	dispatchRole(t, env, bob, "announce surprise")

	if bob.lastLine() != "Huh?" {
		t.Errorf("refusal = %q, want the unknown-verb 'Huh?'", bob.lastLine())
	}
	if len(ann.lines) != 0 {
		t.Errorf("a refused announce must not broadcast, got %v", ann.lines)
	}
	if len(*got) != 0 {
		t.Errorf("a refused announce must not audit, got %d", len(*got))
	}
}
