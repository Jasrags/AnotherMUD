package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// pickRoller serves a fixed raw IntN value (a d20 face N is programmed as N-1,
// since ResolveSkillCheck does IntN(20)+1).
type pickRoller struct{ raw int }

func (r pickRoller) IntN(int) int { return r.raw }

func pickableGate(keyID string, difficulty int) *world.DoorState {
	d := ironGate(keyID)
	d.Pickable = true
	d.PickDifficulty = difficulty
	return d
}

// pickEnv builds a door env wired for the pick verb, with the actor trained in
// open-lock at the given proficiency and a roller that returns the given d20
// face.
func pickEnv(t *testing.T, f *doorFixture, playerID string, prof, face int) command.Env {
	t.Helper()
	abilities := progression.NewAbilityRegistry()
	if err := abilities.Register(&progression.Ability{
		ID: "open-lock", Type: progression.AbilityPassive, Category: progression.AbilitySkill,
		GainStat: progression.StatDEX, DefaultCap: 100,
	}); err != nil {
		t.Fatalf("register open-lock: %v", err)
	}
	pm := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())
	if prof > 0 {
		pm.Learn(playerID, "open-lock", prof)
	}
	env := f.env()
	env.Abilities = abilities
	env.Proficiency = pm
	env.SkillRoller = pickRoller{raw: face - 1}
	return env
}

func dispatchDoorEnv(t *testing.T, env command.Env, a *testActor, line string) {
	t.Helper()
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, line); err != nil {
		t.Fatalf("dispatch %q: %v", line, err)
	}
}

func TestPickVerb_SuccessUnlocks(t *testing.T) {
	f := newDoorFixture(t, pickableGate("village-key", 15), nil)
	a := newNamedTestActor("Picker", "p-pick", f.roomA(t))
	env := pickEnv(t, f, a.PlayerID(), 50, 20) // natural 20 → always succeeds

	dispatchDoorEnv(t, env, a, "pick gate")

	if got := a.lastLine(); !strings.Contains(got, "deftly pick") {
		t.Errorf("success message = %q, want a 'deftly pick' line", got)
	}
	d2, _ := f.world.GetDoor("a", world.DirNorth)
	if d2.Locked {
		t.Error("door still locked after a successful pick")
	}
}

func TestPickVerb_FailureLeavesLocked(t *testing.T) {
	f := newDoorFixture(t, pickableGate("village-key", 15), nil)
	a := newNamedTestActor("Picker", "p-pick", f.roomA(t))
	env := pickEnv(t, f, a.PlayerID(), 50, 1) // natural 1 → always fails

	dispatchDoorEnv(t, env, a, "pick gate")

	if got := a.lastLine(); !strings.Contains(got, "fail to pick") {
		t.Errorf("failure message = %q, want 'fail to pick'", got)
	}
	d2, _ := f.world.GetDoor("a", world.DirNorth)
	if !d2.Locked {
		t.Error("door unlocked after a FAILED pick")
	}
}

func TestPickVerb_UntrainedRefused(t *testing.T) {
	f := newDoorFixture(t, pickableGate("village-key", 15), nil)
	a := newNamedTestActor("Picker", "p-pick", f.roomA(t))
	env := pickEnv(t, f, a.PlayerID(), 0, 20) // prof 0 → not learned → untrained

	dispatchDoorEnv(t, env, a, "pick gate")

	if got := a.lastLine(); !strings.Contains(got, "don't know how to pick") {
		t.Errorf("untrained message = %q", got)
	}
	d2, _ := f.world.GetDoor("a", world.DirNorth)
	if !d2.Locked {
		t.Error("an untrained actor picked the lock")
	}
}

func TestPickVerb_NotPickableRefused(t *testing.T) {
	d := ironGate("village-key") // locked but NOT pickable
	f := newDoorFixture(t, d, nil)
	a := newNamedTestActor("Picker", "p-pick", f.roomA(t))
	env := pickEnv(t, f, a.PlayerID(), 50, 20)

	dispatchDoorEnv(t, env, a, "pick gate")

	if got := a.lastLine(); !strings.Contains(got, "can't be picked") {
		t.Errorf("non-pickable message = %q", got)
	}
}

func TestPickVerb_KeylessHasNoLock(t *testing.T) {
	d := pickableGate("", 15) // pickable flag but no lock (keyless)
	d.Locked = false
	f := newDoorFixture(t, d, nil)
	a := newNamedTestActor("Picker", "p-pick", f.roomA(t))
	env := pickEnv(t, f, a.PlayerID(), 50, 20)

	dispatchDoorEnv(t, env, a, "pick gate")

	if got := a.lastLine(); !strings.Contains(got, "no lock") {
		t.Errorf("keyless message = %q", got)
	}
}
