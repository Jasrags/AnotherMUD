package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/pool"
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
func (a *combatActor) Pools() *pool.Set                { return nil }

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

func TestConsider_NoArgPointsToScore(t *testing.T) {
	// Bare `consider` no longer sizes yourself up — self stats moved to
	// `score`, so it points there.
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "consider")
	if got := a.lastLine(); !strings.Contains(got, "score") {
		t.Errorf("no-arg consider = %q, want a pointer to `score`", got)
	}
}

func TestConsider_NoArgNonCombatant(t *testing.T) {
	// A bare consider points to score regardless of the actor's combat
	// state — no self render, no room search for a stranger.
	f := newConsiderFixture(t)
	a := newNamedTestActor("Plain", "p-1", f.room)
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "consider")
	if got := a.lastLine(); !strings.Contains(got, "score") {
		t.Errorf("no-arg consider (non-combatant) = %q, want a pointer to `score`", got)
	}
}

func TestConsider_MobShowsConditionAndThreatNoNumbers(t *testing.T) {
	// The tactical lens is qualitative: a condition word + a relative-
	// threat read, and NO raw HP/AC numbers leak (those live on `score`).
	// Viewer is the combatActor so the threat branch runs. Guard power
	// (40 + STR 12 + AC 14 = 66) vs default player (20 + 10 + 10 = 40) →
	// ratio 1.65 → "advantage" band.
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "consider guard")

	out := a.lastLine()
	if !strings.Contains(out, "a village guard") {
		t.Errorf("output missing target name: %q", out)
	}
	if !strings.Contains(out, "uninjured") {
		t.Errorf("output missing 'uninjured' condition (full HP): %q", out)
	}
	if !strings.Contains(out, "advantage") {
		t.Errorf("output missing threat read (tougher target): %q", out)
	}
	// No raw stat numbers may leak.
	for _, leak := range []string{"40/40", "HP", "AC "} {
		if strings.Contains(out, leak) {
			t.Errorf("qualitative consider leaked %q: %q", leak, out)
		}
	}
}

func TestConsider_ThreatReadScalesWithViewerStrength(t *testing.T) {
	// A weak target relative to the viewer reads as a clear advantage.
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	// Beef the viewer well past the guard (66 power) so the ratio drops
	// below 0.5 → "crush" band.
	a.vitals = combat.NewVitals(500)
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "consider guard")
	if got := a.lastLine(); !strings.Contains(got, "crush") {
		t.Errorf("strong viewer vs weak guard = %q, want a 'crush' threat read", got)
	}
}

func TestConsider_NonCombatantViewerOmitsThreat(t *testing.T) {
	// A viewer that isn't a combatant (test stub) gets the condition line
	// only — the threat read degrades out rather than panicking.
	f := newConsiderFixture(t)
	a := newNamedTestActor("Plain", "p-1", f.room)
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "consider guard")
	out := a.lastLine()
	if !strings.Contains(out, "uninjured") {
		t.Errorf("non-combatant viewer missing condition: %q", out)
	}
	for _, threat := range []string{"crush", "upper hand", "even fight", "advantage", "stand a chance"} {
		if strings.Contains(out, threat) {
			t.Errorf("non-combatant viewer leaked a threat read %q: %q", threat, out)
		}
	}
}

func TestConsider_MobDescriptorTracksHPDamage(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)

	// Knock the guard down to ~25% — should land in "badly wounded".
	f.guard.Vitals().ApplyDamage(30)
	dispatchActor(t, r, f.env(), a, "consider guard")

	out := a.lastLine()
	if !strings.Contains(out, "badly wounded") {
		t.Errorf("output missing 'badly wounded': %q", out)
	}
	if strings.Contains(out, "10/40") || strings.Contains(out, "HP") {
		t.Errorf("qualitative consider leaked HP numbers: %q", out)
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

func TestConsider_SelfReferencePointsToScore(t *testing.T) {
	// self / me / own-name all point to `score` rather than rendering a
	// self status line.
	f := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	r := newRegistry(t)
	for _, syn := range []string{"self", "me", "Alice", "ALICE"} {
		dispatch(t, r, f.env(), a.testActor, "consider "+syn)
		if got := a.lastLine(); !strings.Contains(got, "score") {
			t.Errorf("consider %q = %q, want a pointer to `score`", syn, got)
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
	// M17.2d₄b: combat targeting resolves through the §5 entity arg,
	// which enumerates room players via Locator.PlayersInRoom — so the
	// fixture must surface Bob there (stubLocator does both that and
	// FindInRoom), not the name-only locatorFunc.
	loc := &stubLocator{}
	loc.add(bob)
	env.Locator = loc
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

// TestConsider_PlayerPartialName pins the M17.2d₄b behavior change:
// players are now keyword/partial-matchable through the §5 entity
// resolver, so "consider bo" resolves Bob — the old name-only path
// required the full name.
func TestConsider_PlayerPartialName(t *testing.T) {
	f := newConsiderFixture(t)
	alice := newCombatActor("Alice", "p-1", f.room)
	bob := newCombatActor("Bob", "p-2", f.room)

	env := f.env()
	loc := &stubLocator{}
	loc.add(bob)
	env.Locator = loc
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, alice.testActor, "consider bo"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "Bob") {
		t.Errorf("consider bo = %q, want Bob resolved by partial name", got)
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
