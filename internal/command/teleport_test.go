package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// fakePlayerRoom maps lowercased player names to their current room,
// standing in for the world-scoped Manager lookup teleport-to-player uses.
type fakePlayerRoom map[string]world.RoomID

func (f fakePlayerRoom) ResolvePlayerRoom(name string) (world.RoomID, bool) {
	r, ok := f[strings.ToLower(strings.TrimSpace(name))]
	return r, ok
}

// adminAt builds an admin actor (HasRole "admin") standing in room.
func adminAt(name, playerID string, room *world.Room) *roleActor {
	a := newRoleActor(name, playerID, "admin")
	a.SetRoom(room)
	return a
}

// An admin teleports to a room by id: the actor moves and one admin.action
// fires with the destination room id as target.
func TestTeleport_ToRoomMovesAndAudits(t *testing.T) {
	w, home, field := twoRoomWorld(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminAt("Maerys", "p-admin", field)
	env := command.Env{World: w, Bus: bus}

	dispatchRole(t, env, admin, "teleport home")

	if admin.Room().ID != home.ID {
		t.Errorf("actor room = %q, want %q", admin.Room().ID, home.ID)
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	if ev := (*got)[0].(eventbus.AdminAction); ev.Verb != "teleport" || ev.Target != "home" {
		t.Errorf("event = %+v, want verb=teleport target=home", ev)
	}
}

// teleport emits player.moved (§5 — the normal room-change event) so
// room-entry subscribers (questwatch, AI disposition) react to the arrival.
func TestTeleport_EmitsPlayerMoved(t *testing.T) {
	w, home, field := twoRoomWorld(t)
	bus := eventbus.New()
	moved := captureEvents(t, bus, eventbus.EventPlayerMoved)
	admin := adminAt("Maerys", "p-admin", field)
	env := command.Env{World: w, Bus: bus}

	dispatchRole(t, env, admin, "teleport home")

	if len(*moved) != 1 {
		t.Fatalf("player.moved count = %d, want 1", len(*moved))
	}
	ev := (*moved)[0].(eventbus.PlayerMoved)
	if ev.PlayerID != "p-admin" || ev.From != field.ID || ev.To != home.ID {
		t.Errorf("player.moved = %+v, want p-admin field→home", ev)
	}
}

// `goto` is an alias for teleport.
func TestTeleport_GotoAlias(t *testing.T) {
	w, home, field := twoRoomWorld(t)
	admin := adminAt("Maerys", "p-admin", field)
	env := command.Env{World: w}

	dispatchRole(t, env, admin, "goto home")

	if admin.Room().ID != home.ID {
		t.Errorf("`goto home` did not move actor: room = %q", admin.Room().ID)
	}
}

// Teleport-to-player resolves the named online player's room (§3) and moves
// the actor there.
func TestTeleport_ToPlayerMovesToTheirRoom(t *testing.T) {
	w, home, field := twoRoomWorld(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminAt("Maerys", "p-admin", field)
	env := command.Env{World: w, Bus: bus, PlayerRoom: fakePlayerRoom{"bob": home.ID}}

	dispatchRole(t, env, admin, "teleport bob")

	if admin.Room().ID != home.ID {
		t.Errorf("actor room = %q, want %q (bob's room)", admin.Room().ID, home.ID)
	}
	if ev := (*got)[0].(eventbus.AdminAction); ev.Target != "home" || ev.Args != "bob" {
		t.Errorf("event = %+v, want target=home args=bob", ev)
	}
}

// An unknown room/player reports the miss, audits nothing, and the actor
// stays put.
func TestTeleport_UnknownTargetNoMove(t *testing.T) {
	w, _, field := twoRoomWorld(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminAt("Maerys", "p-admin", field)
	env := command.Env{World: w, Bus: bus}

	dispatchRole(t, env, admin, "teleport nowhere")

	if admin.Room().ID != field.ID {
		t.Errorf("actor moved on a bad target: room = %q", admin.Room().ID)
	}
	if !strings.Contains(admin.lastLine(), "no room or online player") {
		t.Errorf("message = %q, want a not-found message", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a missed teleport must not audit, got %d", len(*got))
	}
}

// Teleporting to the current room is a no-op with no audit.
func TestTeleport_SameRoomNoOp(t *testing.T) {
	w, home, _ := twoRoomWorld(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminAt("Maerys", "p-admin", home)
	env := command.Env{World: w, Bus: bus}

	dispatchRole(t, env, admin, "teleport home")

	if !strings.Contains(admin.lastLine(), "already there") {
		t.Errorf("message = %q, want 'already there'", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("same-room teleport must not audit, got %d", len(*got))
	}
}

// teleport is admin-gated (§2): a non-admin gets "Huh?", no move, no audit.
func TestTeleport_RefusedForNonAdmin(t *testing.T) {
	w, _, field := twoRoomWorld(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	bob := newRoleActor("Bob", "p-bob") // no admin role
	bob.SetRoom(field)
	env := command.Env{World: w, Bus: bus}

	dispatchRole(t, env, bob, "teleport home")

	if bob.lastLine() != "Huh?" {
		t.Errorf("refusal = %q, want 'Huh?'", bob.lastLine())
	}
	if bob.Room().ID != field.ID {
		t.Errorf("non-admin moved: room = %q", bob.Room().ID)
	}
	if len(*got) != 0 {
		t.Errorf("a refused teleport must not audit, got %d", len(*got))
	}
}

// A bare leaf name resolves against the namespaced room id by suffix, so an
// admin can `teleport meadow` without typing `tapestry-core:meadow`.
func TestTeleport_BareLeafNameResolves(t *testing.T) {
	w := world.New()
	field := &world.Room{ID: "tapestry-core:field", Name: "Field"}
	meadow := &world.Room{ID: "tapestry-core:meadow", Name: "Meadow"}
	w.AddRoom(field)
	w.AddRoom(meadow)
	admin := adminAt("Maerys", "p-admin", field)
	env := command.Env{World: w}

	dispatchRole(t, env, admin, "teleport meadow")

	if admin.Room().ID != meadow.ID {
		t.Errorf("bare-leaf teleport room = %q, want %q", admin.Room().ID, meadow.ID)
	}
}

// A leaf that exists in two packs is ambiguous: the actor does not move and
// the reply lists the candidate ids.
func TestTeleport_AmbiguousLeafReportsCandidates(t *testing.T) {
	w := world.New()
	field := &world.Room{ID: "core:field", Name: "Field"}
	m1 := &world.Room{ID: "core:meadow", Name: "Core Meadow"}
	m2 := &world.Room{ID: "expansion:meadow", Name: "Expansion Meadow"}
	w.AddRoom(field)
	w.AddRoom(m1)
	w.AddRoom(m2)
	admin := adminAt("Maerys", "p-admin", field)
	env := command.Env{World: w}

	dispatchRole(t, env, admin, "teleport meadow")

	if admin.Room().ID != field.ID {
		t.Errorf("ambiguous teleport moved the actor to %q, want no move", admin.Room().ID)
	}
	out := admin.lastLine()
	if !strings.Contains(out, "ambiguous") || !strings.Contains(out, "core:meadow") || !strings.Contains(out, "expansion:meadow") {
		t.Errorf("ambiguous reply = %q, want it to list both candidate ids", out)
	}
}
