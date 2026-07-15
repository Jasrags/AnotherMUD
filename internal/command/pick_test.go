package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/grade"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// gradeLadderWithTool is a grade registry whose grades set ToolSkill (a
// masterwork tool adds +1 to a skill check, masterpiece +2).
func gradeLadderWithTool() *grade.Registry {
	r := grade.NewRegistry()
	r.Register(grade.Grade{Key: "masterwork", Order: 1, ToolSkill: 1})
	r.Register(grade.Grade{Key: "masterpiece", Order: 2, ToolSkill: 2})
	return r
}

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
		// Mirror the content: Open Lock is trained-only, so the untrained-refused
		// gate is now the field-driven SkillDefaulting path (skills §2.1).
		TrainedOnly: true,
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

// TestPickVerb_ArmorCheckPenaltyFailsBoundaryPick exercises armor-depth §6's
// skill-check consumer: a worn-armor check penalty reduces the pick bonus. At
// the success boundary (prof 50 → bonus 5, roll 11 → total 16, DC 16) the pick
// just succeeds with no armor; a check penalty of 3 drops the total to 13 < 16
// and the lock holds.
func TestPickVerb_ArmorCheckPenaltyFailsBoundaryPick(t *testing.T) {
	// Control: no armor → total 16 >= DC 16 → succeeds.
	f := newDoorFixture(t, pickableGate("village-key", 16), nil)
	a := newNamedTestActor("Picker", "p-pick", f.roomA(t))
	dispatchDoorEnv(t, pickEnv(t, f, a.PlayerID(), 50, 11), a, "pick gate")
	if got := a.lastLine(); !strings.Contains(got, "deftly pick") {
		t.Fatalf("control (no armor) should succeed at the boundary; got %q", got)
	}

	// Same check, but a worn-armor check penalty of 3 drops the total below the
	// DC → the pick fails and the door stays locked.
	f2 := newDoorFixture(t, pickableGate("village-key", 16), nil)
	a2 := newNamedTestActor("Picker", "p-pick2", f2.roomA(t))
	a2.armorCheck = 3
	dispatchDoorEnv(t, pickEnv(t, f2, a2.PlayerID(), 50, 11), a2, "pick gate")
	if got := a2.lastLine(); !strings.Contains(got, "fail to pick") {
		t.Fatalf("armor check penalty should fail the boundary pick; got %q", got)
	}
	if d, _ := f2.world.GetDoor("a", world.DirNorth); !d.Locked {
		t.Error("door unlocked despite the armor-check penalty failing the pick")
	}
}

// lockpickTpl is a carried tool that assists Open Lock (skills.md tool seam),
// optionally graded.
func lockpickTpl(bonus int, g string) *item.Template {
	return &item.Template{
		ID: "x:lockpick", Name: "a lockpick", Type: "tool",
		Keywords: []string{"lockpick", "pick"},
		Properties: map[string]any{
			"skill_tool":       "open-lock",
			"skill_tool_bonus": bonus,
		},
		Grade: g,
	}
}

func giveLockpick(t *testing.T, f *doorFixture, a *testActor, bonus int, g string) {
	t.Helper()
	inst, err := f.store.Spawn(lockpickTpl(bonus, g))
	if err != nil {
		t.Fatalf("spawn lockpick: %v", err)
	}
	a.AddToInventory(inst.ID())
}

// A carried lockpick adds its bonus to the Open-Lock check (the skills.md tool
// seam): at DC 18 a bare attempt (bonus 5, roll 11 → total 16) fails, but a +2
// lockpick lifts the total to 18 and the lock yields.
func TestPickVerb_CarriedLockpickAddsBonus(t *testing.T) {
	f := newDoorFixture(t, pickableGate("village-key", 18), nil)
	a := newNamedTestActor("Picker", "p-pick", f.roomA(t))
	dispatchDoorEnv(t, pickEnv(t, f, a.PlayerID(), 50, 11), a, "pick gate")
	if got := a.lastLine(); !strings.Contains(got, "fail to pick") {
		t.Fatalf("control (no tool) should fail at DC 18; got %q", got)
	}

	f2 := newDoorFixture(t, pickableGate("village-key", 18), nil)
	a2 := newNamedTestActor("Picker", "p-pick2", f2.roomA(t))
	giveLockpick(t, f2, a2, 2, "")
	dispatchDoorEnv(t, pickEnv(t, f2, a2.PlayerID(), 50, 11), a2, "pick gate")
	if got := a2.lastLine(); !strings.Contains(got, "deftly pick") {
		t.Fatalf("a carried lockpick should make the boundary pick succeed; got %q", got)
	}
}

// A masterwork lockpick aids the check more than a plain one (masterwork §3):
// at DC 19 a plain +2 pick (total 18) fails, but a masterwork +2 pick (base 2 +
// grade ToolSkill 1 = 3, total 19) succeeds.
func TestPickVerb_MasterworkLockpickAddsGradeBonus(t *testing.T) {
	grades := gradeLadderWithTool()

	f := newDoorFixture(t, pickableGate("village-key", 19), nil)
	a := newNamedTestActor("Picker", "p-pick", f.roomA(t))
	giveLockpick(t, f, a, 2, "") // plain → +2 → total 18 < 19
	env := pickEnv(t, f, a.PlayerID(), 50, 11)
	env.Grades = grades
	dispatchDoorEnv(t, env, a, "pick gate")
	if got := a.lastLine(); !strings.Contains(got, "fail to pick") {
		t.Fatalf("a plain lockpick should fail at DC 19; got %q", got)
	}

	f2 := newDoorFixture(t, pickableGate("village-key", 19), nil)
	a2 := newNamedTestActor("Picker", "p-pick2", f2.roomA(t))
	giveLockpick(t, f2, a2, 2, "masterwork") // +2 base + 1 grade = +3 → total 19
	env2 := pickEnv(t, f2, a2.PlayerID(), 50, 11)
	env2.Grades = grades
	dispatchDoorEnv(t, env2, a2, "pick gate")
	if got := a2.lastLine(); !strings.Contains(got, "deftly pick") {
		t.Fatalf("a masterwork lockpick should succeed where a plain one fails; got %q", got)
	}
}

// Carried tools toward one check do NOT stack (masterwork §3): two +2 lockpicks
// still contribute only +2 (best-applies), so at DC 19 the pick fails — were
// they stacking (+4) it would succeed.
func TestPickVerb_LockpicksDoNotStack(t *testing.T) {
	f := newDoorFixture(t, pickableGate("village-key", 19), nil)
	a := newNamedTestActor("Picker", "p-pick", f.roomA(t))
	giveLockpick(t, f, a, 2, "")
	giveLockpick(t, f, a, 2, "")
	dispatchDoorEnv(t, pickEnv(t, f, a.PlayerID(), 50, 11), a, "pick gate")
	if got := a.lastLine(); !strings.Contains(got, "fail to pick") {
		t.Fatalf("two lockpicks must not stack (best-applies, +2 → total 18 < 19); got %q", got)
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

// TestPickVerb_DefaultableSkillLetsUntrainedAttempt — skills §2.1 defaulting:
// when Open Lock is authored DEFAULTABLE (TrainedOnly=false) with a default
// penalty, an untrained actor is NOT refused — they attempt at the penalty. This
// proves the field-driven SkillDefaulting gate in door.go (the trained-only
// refusal is content, not hardcoded). A nat-20 succeeds regardless of the
// penalty, so the lock opens and the actor was clearly allowed to try.
func TestPickVerb_DefaultableSkillLetsUntrainedAttempt(t *testing.T) {
	f := newDoorFixture(t, pickableGate("village-key", 15), nil)
	a := newNamedTestActor("Picker", "p-pick", f.roomA(t))

	abilities := progression.NewAbilityRegistry()
	if err := abilities.Register(&progression.Ability{
		ID: "open-lock", Type: progression.AbilityPassive, Category: progression.AbilitySkill,
		GainStat: progression.StatDEX, DefaultCap: 100,
		TrainedOnly: false, DefaultPenalty: 4, // defaultable, at a -4 penalty
	}); err != nil {
		t.Fatalf("register open-lock: %v", err)
	}
	env := f.env()
	env.Abilities = abilities
	env.Proficiency = progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())
	env.SkillRoller = pickRoller{raw: 19} // nat-20 → auto-succeed despite the default penalty

	dispatchDoorEnv(t, env, a, "pick gate")

	if got := a.lastLine(); strings.Contains(got, "don't know how to pick") {
		t.Fatalf("a defaultable skill must not refuse an untrained picker, got %q", got)
	}
	d2, _ := f.world.GetDoor("a", world.DirNorth)
	if d2.Locked {
		t.Error("untrained defaulting pick on a nat-20 should have opened the lock")
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
