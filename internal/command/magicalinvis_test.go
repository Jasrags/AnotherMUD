package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// applyInvisible puts the `invisible` effect flag on entityID via a real
// EffectManager (visibility §3.4 — magical invis is sourced from effects).
func applyInvisible(em *progression.EffectManager, entityID string) {
	em.Apply(context.Background(), entityID, progression.EffectTemplate{
		ID: "invisibility", Duration: 10, Flags: []string{command.InvisibleFlag},
	}, "", "")
}

// Through the visibility predicate (§3.4 / §5.4): a magically-invisible
// occupant is concealed from an ordinary viewer's target resolution but
// visible to a viewer carrying the see-invisible counter (here, as a tag).
func TestMagicalInvis_PredicateHiddenUnlessSeeInvisible(t *testing.T) {
	f := newInvFixture(t)
	em := progression.NewEffectManager(nil, nil)
	ghost := &namedActor{testActor: newTestActor(f.room), name: "Ghost", playerID: "p-ghost"}
	applyInvisible(em, "p-ghost")

	// Ordinary viewer: cannot see the invisible occupant.
	plain := newNamedTestActor("Plain", "p-plain", f.room)
	locP := &stubLocator{}
	locP.add(plain)
	locP.add(ghost)
	rcP := (&command.Context{Actor: plain, World: f.world, Items: f.store, Placement: f.place, Locator: locP, Effects: em}).BuildResolveContext()
	if rcP.CanSee == nil {
		t.Fatal("predicate should be built when a magically-invisible occupant is present")
	}
	if rcP.CanSee("p-ghost") {
		t.Error("an ordinary viewer must not see a magically-invisible occupant")
	}
	if !rcP.CanSee("p-plain") {
		t.Error("the viewer must see themselves")
	}

	// Viewer with the see_invisible tag pierces it.
	seer := newNamedTestActor("Seer", "p-seer", f.room)
	seer.tags = []string{command.SeeInvisibleFlag}
	locS := &stubLocator{}
	locS.add(seer)
	locS.add(ghost)
	rcS := (&command.Context{Actor: seer, World: f.world, Items: f.store, Placement: f.place, Locator: locS, Effects: em}).BuildResolveContext()
	if rcS.CanSee != nil && !rcS.CanSee("p-ghost") {
		t.Error("a see_invisible viewer must see a magically-invisible occupant")
	}
}

// see_invisible granted by an effect flag (not a tag) also pierces (§4.3).
func TestMagicalInvis_SeeInvisibleViaEffectFlag(t *testing.T) {
	f := newInvFixture(t)
	em := progression.NewEffectManager(nil, nil)
	ghost := &namedActor{testActor: newTestActor(f.room), name: "Ghost", playerID: "p-ghost"}
	applyInvisible(em, "p-ghost")

	seer := newNamedTestActor("Seer", "p-seer", f.room)
	em.Apply(context.Background(), "p-seer", progression.EffectTemplate{
		ID: "truesight", Duration: 10, Flags: []string{command.SeeInvisibleFlag},
	}, "", "")
	loc := &stubLocator{}
	loc.add(seer)
	loc.add(ghost)
	rc := (&command.Context{Actor: seer, World: f.world, Items: f.store, Placement: f.place, Locator: loc, Effects: em}).BuildResolveContext()
	if rc.CanSee != nil && !rc.CanSee("p-ghost") {
		t.Error("an effect-granted see_invisible must pierce magical invisibility")
	}
}

// v1 scoping (§9): magical invis is players-only — a MOB carrying an
// invisible effect is NOT concealed (magicalInvisibleOccupants iterates
// players). This test pins that documented boundary so a future §9 extension
// to mobs is a deliberate, test-visible change rather than a silent one.
func TestMagicalInvis_MobNotConcealedInV1(t *testing.T) {
	f := newInvFixture(t)
	em := progression.NewEffectManager(nil, nil)
	// A mob entity id carries the invisible flag, but mobs are not enumerated
	// by the players-only occupant scan, so nothing conceals it.
	em.Apply(context.Background(), "m-rat", progression.EffectTemplate{
		ID: "invisibility", Duration: 10, Flags: []string{command.InvisibleFlag},
	}, "", "")

	viewer := newNamedTestActor("Plain", "p-plain", f.room)
	loc := &stubLocator{}
	loc.add(viewer) // only the player is enumerated; the mob is not a PlayersInRoom entry
	rc := (&command.Context{Actor: viewer, World: f.world, Items: f.store, Placement: f.place, Locator: loc, Effects: em}).BuildResolveContext()
	// No players are concealed and the room is lit, so the predicate is nil
	// (permissive) — the invisible mob would resolve as visible (v1 boundary).
	if rc.CanSee != nil && !rc.CanSee("m-rat") {
		t.Error("v1: a magically-invisible MOB must NOT be concealed (players-only, §9)")
	}
}

// In `who`, a magically-invisible character is excluded from an ordinary
// viewer's roster/count but shown to a see_invisible viewer; self always shows.
func TestWho_MagicalInvisFilteredBySeeInvisible(t *testing.T) {
	f := newInvFixture(t)
	em := progression.NewEffectManager(nil, nil)
	applyInvisible(em, "p-ghost")

	roster := stubRoster{
		{Name: "Plain", PlayerID: "p-plain"},
		{Name: "Ghost", PlayerID: "p-ghost"},
	}

	// Ordinary viewer: Ghost hidden.
	plain := newNamedTestActor("Plain", "p-plain", f.room)
	env := f.env()
	env.Effects = em
	env.Roster = roster
	dispatchActor(t, newRegistry(t), env, plain, "who")
	if out := plain.lastLine(); strings.Contains(out, "Ghost") {
		t.Errorf("ordinary viewer must not see an invisible char in who: %q", out)
	}
	if out := plain.lastLine(); !strings.Contains(out, "1 player online.") {
		t.Errorf("count must exclude the invisible char: %q", plain.lastLine())
	}

	// see_invisible viewer: Ghost shown.
	seer := newNamedTestActor("Seer", "p-seer", f.room)
	seer.tags = []string{command.SeeInvisibleFlag}
	roster2 := stubRoster{
		{Name: "Seer", PlayerID: "p-seer"},
		{Name: "Ghost", PlayerID: "p-ghost"},
	}
	env2 := f.env()
	env2.Effects = em
	env2.Roster = roster2
	dispatchActor(t, newRegistry(t), env2, seer, "who")
	if out := seer.lastLine(); !strings.Contains(out, "Ghost") {
		t.Errorf("see_invisible viewer must see the invisible char in who: %q", out)
	}
}
