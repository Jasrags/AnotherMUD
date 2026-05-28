package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// TestConnActor_SatisfiesResolutionSource pins the M9.4b wiring: a
// connActor must satisfy progression.ResolutionSource (which embeds
// ValidationEntity) so the ability-resolution phase can validate +
// resolve a player's queued abilities. Compile-time assertion plus
// a spot-check of the thin-pool / no-rest defaults.
func TestConnActor_SatisfiesResolutionSource(t *testing.T) {
	a := &connActor{
		playerID:  "p-1",
		save:      &player.Save{Version: player.CurrentVersion, Name: "Tester"},
		statBlock: progression.NewWithBase(progression.DefaultPlayerBase()),
		equipment: map[string]entities.EntityID{},
	}
	var src progression.ResolutionSource = a // compile-time pin

	if src.IsResting() {
		t.Error("players have no rest state yet; IsResting must be false")
	}
	if src.InCombat() {
		t.Error("actor with nil combat manager must report not-in-combat")
	}
	if _, ok := src.CurrentTarget(); ok {
		t.Error("actor with nil combat manager must report no target")
	}
	// Thin pools read max stats; deduction is a documented no-op.
	mv := src.Movement()
	src.DeductMovement(5)
	if src.Movement() != mv {
		t.Error("DeductMovement is dormant; pool must not change")
	}
	src.SetLastAbility("slash")
	if a.LastAbility() != "slash" {
		t.Errorf("SetLastAbility not recorded, got %q", a.LastAbility())
	}
}

// TestConnActor_SatisfiesEffectTarget pins the M9.2 wiring: a
// connActor must satisfy progression.EffectTarget so the
// EffectManager can write modifiers through it without an
// adapter. Compile-time check via interface assignment plus a
// runtime Apply/RemoveBySource round-trip.
func TestConnActor_SatisfiesEffectTarget(t *testing.T) {
	a := &connActor{
		playerID:  "p-1",
		save:      &player.Save{Version: player.CurrentVersion, Name: "Tester"},
		statBlock: progression.NewWithBase(progression.DefaultPlayerBase()),
	}
	var target progression.EffectTarget = a // compile-time pin

	if id := target.EntityID(); id != "p-1" {
		t.Errorf("EntityID = %q, want p-1", id)
	}

	src := progression.EffectSourceKey("bless")
	target.AddModifiers(src, []stats.Modifier{{Stat: "str", Value: 3}})
	if !a.statBlock.HasSource(src) {
		t.Errorf("AddModifiers did not install under %s", src)
	}
	if !a.dirty {
		t.Errorf("dirty not set after AddModifiers")
	}

	// Drop dirty so we can pin RemoveBySource flips it back.
	a.dirty = false
	if !target.RemoveBySource(src) {
		t.Errorf("RemoveBySource returned false")
	}
	if a.statBlock.HasSource(src) {
		t.Errorf("RemoveBySource did not clear stat block")
	}
	if !a.dirty {
		t.Errorf("dirty not set after RemoveBySource")
	}

	// Round-trip through the EffectManager to pin the resolver
	// path works end-to-end against a real connActor.
	mgr := progression.NewEffectManager(progression.TargetResolverFunc(
		func(id string) (progression.EffectTarget, bool) {
			if id == a.EntityID() {
				return a, true
			}
			return nil, false
		}), nil)
	ok := mgr.Apply(context.Background(), "p-1", progression.EffectTemplate{
		ID: "shield", Duration: 5,
		Modifiers: []stats.Modifier{{Stat: "ac", Value: 2}},
	}, "", "spell.shield")
	if !ok {
		t.Fatalf("Apply returned false")
	}
	if !a.statBlock.HasSource(progression.EffectSourceKey("shield")) {
		t.Errorf("effect modifiers not installed via EffectManager")
	}
	if !mgr.RemoveByID(context.Background(), "p-1", "shield") {
		t.Fatalf("RemoveByID returned false")
	}
	if a.statBlock.HasSource(progression.EffectSourceKey("shield")) {
		t.Errorf("effect modifiers not reversed via EffectManager")
	}
	// Ensure the type assertion compiled (silences "declared and
	// not used" if future refactors drop the runtime calls).
	_ = entities.SourceKey("")
}
