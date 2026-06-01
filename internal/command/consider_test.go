package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// combatActor wraps testActor with the combat.Combatant surface so
// the consider self-reference path can exercise the player-as-
// Combatant branch. Carries its own Vitals so a test that damages
// the actor between calls can observe the change.
type combatActor struct {
	*testActor
	combatID combat.CombatantID
	vitals   *combat.Vitals
	stats    combat.Stats
}

func newCombatActor(name, playerID string, room *world.Room) *combatActor {
	return &combatActor{
		testActor: newNamedTestActor(name, playerID, room),
		combatID:  combat.NewPlayerCombatantID(playerID),
		vitals:    combat.NewVitals(combat.DefaultPlayerMaxHP),
		stats:     combat.DefaultPlayerStats(),
	}
}

func (a *combatActor) CombatantID() combat.CombatantID { return a.combatID }
func (a *combatActor) Vitals() *combat.Vitals          { return a.vitals }
func (a *combatActor) Stats() combat.Stats             { return a.stats }

// locatorFunc is a tiny command.Locator that hands back a pre-set
// actor when its name matches. Mirrors the production session.Manager
// path without dragging the session package into command_test.
type locatorFunc func(world.RoomID, string) command.Actor

func (f locatorFunc) FindInRoom(roomID world.RoomID, name string) command.Actor {
	return f(roomID, name)
}

func (f locatorFunc) PlayersInRoom(world.RoomID) []command.Actor { return nil }

func guardTplForConsider() *mob.Template {
	return &mob.Template{
		ID:       "tapestry-core:village-guard",
		Name:     "a village guard",
		Type:     "npc",
		Keywords: []string{"guard"},
		Stats: map[string]int{
			combat.StatKeyHPMax: 40,
			combat.StatKeyAC:    14,
			combat.StatKeySTR:   12,
		},
	}
}

// considerFixture builds the smallest environment that exercises
// consider against a mob: a room, an entity store, a Placement, and
// a spawned + placed mob.
type considerFixture struct {
	*invFixture
	guard *entities.MobInstance
}

func newConsiderFixture(t *testing.T) *considerFixture {
	t.Helper()
	inv := newInvFixture(t)
	guard, err := inv.store.SpawnMob(guardTplForConsider())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	inv.place.Place(guard.ID(), inv.room.ID)
	return &considerFixture{invFixture: inv, guard: guard}
}

func TestConsider_NoArgPrompts(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)
	dispatch(t, r, f.env(), a.testActor, "consider")
	if got := a.lastLine(); !strings.Contains(got, "Consider whom") {
		t.Errorf("no-arg consider = %q, want prompt", got)
	}
}

func TestConsider_MobByKeywordShowsHPAndAC(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)
	dispatch(t, r, f.env(), a.testActor, "consider guard")

	out := a.lastLine()
	// Expect HP fraction, descriptor, and AC line content.
	if !strings.Contains(out, "a village guard") {
		t.Errorf("output missing target name: %q", out)
	}
	if !strings.Contains(out, "40/40 HP") {
		t.Errorf("output missing 40/40 HP: %q", out)
	}
	if !strings.Contains(out, "uninjured") {
		t.Errorf("output missing 'uninjured' descriptor (full HP): %q", out)
	}
	if !strings.Contains(out, "AC 14") {
		t.Errorf("output missing 'AC 14': %q", out)
	}
}

func TestConsider_MobDescriptorTracksHPDamage(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)

	// Knock the guard down to ~25% — should land in "badly wounded".
	f.guard.Vitals().ApplyDamage(30)
	dispatch(t, r, f.env(), a.testActor, "consider guard")

	out := a.lastLine()
	if !strings.Contains(out, "10/40 HP") {
		t.Errorf("output missing 10/40 HP: %q", out)
	}
	if !strings.Contains(out, "badly wounded") {
		t.Errorf("output missing 'badly wounded': %q", out)
	}
}

func TestConsider_MobAtZeroIsDead(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)
	f.guard.Vitals().ApplyDamage(1000)
	dispatch(t, r, f.env(), a.testActor, "consider guard")
	if got := a.lastLine(); !strings.Contains(got, "dead") {
		t.Errorf("dead guard consider = %q, want 'dead' descriptor", got)
	}
}

func TestConsider_SelfAlias(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)
	for _, syn := range []string{"self", "me", "Alice", "ALICE"} {
		dispatch(t, r, f.env(), a.testActor, "consider "+syn)
		if got := a.lastLine(); !strings.Contains(got, "yourself") {
			t.Errorf("consider %q = %q, want 'yourself' in output", syn, got)
		}
	}
}

func TestConsider_PlayerViaLocator(t *testing.T) {
	f := newConsiderFixture(t)
	alice := newCombatActor("Alice", "p-1", f.room)
	bob := newCombatActor("Bob", "p-2", f.room)
	// Knock Bob to half so the descriptor differs from full.
	bob.vitals.ApplyDamage(combat.DefaultPlayerMaxHP / 2)

	env := f.env()
	env.Locator = locatorFunc(func(_ world.RoomID, name string) command.Actor {
		if strings.EqualFold(name, "Bob") {
			return bob
		}
		return nil
	})
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, alice.testActor, "consider Bob"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	out := alice.lastLine()
	if !strings.Contains(out, "Bob") {
		t.Errorf("output missing target name: %q", out)
	}
	// Default 20/2 = 10 HP remaining — descriptor band: 50% → "moderately wounded".
	if !strings.Contains(out, "moderately wounded") {
		t.Errorf("output missing 'moderately wounded': %q", out)
	}
}

func TestConsider_NotPresent(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)
	dispatch(t, r, f.env(), a.testActor, "consider dragon")
	if got := a.lastLine(); !strings.Contains(got, "don't see them") {
		t.Errorf("missing target = %q, want 'don't see them here'", got)
	}
}
