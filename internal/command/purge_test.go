package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// An admin purges a mob: it leaves the entity store and the room placement,
// and one admin.action fires with the mob id as target.
func TestPurge_MobRemovedAndAudits(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus
	guardID := f.guard.ID()

	dispatchRole(t, env, admin, "purge guard")

	if _, ok := f.store.GetByID(guardID); ok {
		t.Error("purged mob still in the entity store")
	}
	if _, ok := f.place.RoomOf(guardID); ok {
		t.Error("purged mob still in room placement")
	}
	if !strings.Contains(admin.lastLine(), "You purge a village guard") {
		t.Errorf("confirmation = %q", admin.lastLine())
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	if ev := (*got)[0].(eventbus.AdminAction); ev.Verb != "purge" || ev.Target != string(guardID) {
		t.Errorf("event = %+v, want verb=purge target=%s", ev, guardID)
	}
}

// Purging a followed mob releases its trailing players (follow.md §3): the mob
// emits no MobKilled, so purge must tear down the follow edge itself.
func TestPurge_DropsMobFollowers(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	follower := newNamedTestActor("Trailer", "p-foll", nil)
	env.Follow = &stubFollow{lost: []string{"p-foll"}}
	env.ActorByID = func(id string) (command.Actor, bool) {
		if id == "p-foll" {
			return follower, true
		}
		return nil, false
	}

	dispatchRole(t, env, admin, "purge guard")

	if follower.lastLine() != "You lose the trail of a village guard." {
		t.Errorf("follower msg = %q, want trail-lost notice", follower.lastLine())
	}
}

// An admin purges a dropped room item: it leaves the store + placement.
func TestPurge_RoomItemRemoved(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus
	sword := f.spawnInRoom(t, swordWithMods())

	dispatchRole(t, env, admin, "purge sword")

	if _, ok := f.store.GetByID(sword.ID()); ok {
		t.Error("purged item still in the entity store")
	}
	if _, ok := f.place.RoomOf(sword.ID()); ok {
		t.Error("purged item still in room placement")
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	if ev := (*got)[0].(eventbus.AdminAction); ev.Verb != "purge" || ev.Target != string(sword.ID()) {
		t.Errorf("event = %+v, want verb=purge target=%s", ev, sword.ID())
	}
}

// purge never targets a player (§5/§9): a player match is refused, the
// player stays, and nothing is audited.
func TestPurge_RefusesPlayer(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	bob := newCombatActor("Bob", "p-bob", f.room)
	loc := &stubLocator{}
	loc.add(bob)
	env := f.env()
	env.Bus = bus
	env.Locator = loc

	dispatchRole(t, env, admin, "purge Bob")

	if !strings.Contains(admin.lastLine(), "can't purge a player") {
		t.Errorf("message = %q, want a player-refusal", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a refused purge must not audit, got %d", len(*got))
	}
}

// An unknown target reports the miss and audits nothing.
func TestPurge_NotFoundNoAudit(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "purge ghost")

	if !strings.Contains(admin.lastLine(), "don't see that here") {
		t.Errorf("message = %q, want a not-found message", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a missed purge must not audit, got %d", len(*got))
	}
}

// purge is admin-gated (§2): a non-admin gets "Huh?", the mob stays, and
// nothing is audited.
func TestPurge_RefusedForNonAdmin(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	bob := newRoleActor("Bob", "p-bob") // no admin role
	bob.SetRoom(f.room)
	env := f.env()
	env.Bus = bus
	guardID := f.guard.ID()

	dispatchRole(t, env, bob, "purge guard")

	if bob.lastLine() != "Huh?" {
		t.Errorf("refusal = %q, want 'Huh?'", bob.lastLine())
	}
	if _, ok := f.store.GetByID(guardID); !ok {
		t.Error("non-admin purge removed the mob")
	}
	if len(*got) != 0 {
		t.Errorf("a refused purge must not audit, got %d", len(*got))
	}
}
