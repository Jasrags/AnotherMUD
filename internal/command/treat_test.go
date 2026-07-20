package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// kitCharges reads a medkit's remaining supply count from its live instance.
func kitCharges(t *testing.T, kit *entities.ItemInstance) int {
	t.Helper()
	v, ok := kit.Property(economy.PropCharges)
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

// medkitTpl is a functional First Aid kit: the first_aid_kit flag + a
// starting supply of charges the treat verb spends.
func medkitTpl(charges int) *item.Template {
	return &item.Template{
		ID: "shadowrun:medkit", Name: "a medkit", Type: "item",
		Keywords: []string{"medkit", "kit"},
		Properties: map[string]any{
			"first_aid_kit":     true,
			economy.PropCharges: charges,
		},
	}
}

// giveMedkit spawns a medkit with the given charges into the actor's
// inventory and returns the live instance (so a test can read its charges).
func giveMedkit(t *testing.T, f *considerFixture, a *combatActor, charges int) *entities.ItemInstance {
	t.Helper()
	inst, err := f.store.Spawn(medkitTpl(charges))
	if err != nil {
		t.Fatalf("spawn medkit: %v", err)
	}
	a.AddToInventory(inst.ID())
	return inst
}

// treatEnv wires the treat verb: First Aid registered DEFAULTABLE (SR allows
// untrained First Aid with a kit) keyed off Logic, the actor trained to prof,
// and a roller returning the given d20 face.
func treatEnv(t *testing.T, f *considerFixture, playerID string, prof, face int) command.Env {
	t.Helper()
	abilities := progression.NewAbilityRegistry()
	if err := abilities.Register(&progression.Ability{
		ID: "first-aid", Type: progression.AbilityPassive, Category: progression.AbilitySkill,
		GainStat: progression.StatType("logic"), DefaultCap: 100,
	}); err != nil {
		t.Fatalf("register first-aid: %v", err)
	}
	pm := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())
	if prof > 0 {
		pm.Learn(playerID, "first-aid", prof)
	}
	env := f.env()
	env.Abilities = abilities
	env.Proficiency = pm
	env.SkillRoller = pickRoller{raw: face - 1}
	env.Bus = eventbus.New()
	return env
}

// A trained runner treats their own wounds: prof 25 → proficiency bonus 5,
// Logic 0 → -5, net bonus 0. Face 15 → total 15 ≥ DC 10 (success), margin 5,
// heal cap = 4 + 5 = 9, heal = 4 + 5 = 9. HP 5 → 14, and a charge is spent.
func TestTreat_SelfSuccessHeals(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Medic", "p-medic", f.room)
	a.Vitals().ApplyDamage(15) // 20 → 5
	kit := giveMedkit(t, f, a, 10)

	dispatchRole(t, treatEnv(t, f, a.PlayerID(), 25, 15), a, "treat")

	if cur, _ := a.Vitals().Snapshot(); cur != 14 {
		t.Errorf("HP = %d, want 14 (healed 9 from 5)", cur)
	}
	if got := kitCharges(t, kit); got != 9 {
		t.Errorf("charges = %d, want 9 (one spent)", got)
	}
	// The reported delta must be the HP actually restored (9), NOT the new
	// current HP (14) — Vitals.Heal returns the latter, HealAmount the former.
	if !strings.Contains(a.lastLine(), "patch yourself up. (+9 HP)") {
		t.Errorf("line = %q, want a self-treat confirmation reporting +9 HP", a.lastLine())
	}
}

// A failed check heals nothing but still burns a charge of supplies (SR: the
// field dressing is used either way; also blocks free-retry farming).
func TestTreat_FailureNoHealButSpendsCharge(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Medic", "p-medic", f.room)
	a.Vitals().ApplyDamage(15) // 20 → 5
	kit := giveMedkit(t, f, a, 10)

	dispatchRole(t, treatEnv(t, f, a.PlayerID(), 25, 1), a, "treat") // nat 1 → always fails

	if cur, _ := a.Vitals().Snapshot(); cur != 5 {
		t.Errorf("HP = %d, want 5 (unchanged on a failed treat)", cur)
	}
	if got := kitCharges(t, kit); got != 9 {
		t.Errorf("charges = %d, want 9 (a failed attempt still spends supplies)", got)
	}
	if !strings.Contains(a.lastLine(), "fumble") {
		t.Errorf("line = %q, want a fumble message", a.lastLine())
	}
}

// No medkit in inventory → refused.
func TestTreat_NoMedkitRefused(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Medic", "p-medic", f.room)
	a.Vitals().ApplyDamage(15)

	dispatchRole(t, treatEnv(t, f, a.PlayerID(), 25, 15), a, "treat")

	if !strings.Contains(a.lastLine(), "need a medkit") {
		t.Errorf("line = %q, want a no-medkit refusal", a.lastLine())
	}
	if cur, _ := a.Vitals().Snapshot(); cur != 5 {
		t.Errorf("HP = %d, want 5 (no heal without a kit)", cur)
	}
}

// An empty medkit is refused with the out-of-supplies message (distinct from
// having no kit at all), and no attempt/charge is made.
func TestTreat_EmptyMedkitRefused(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Medic", "p-medic", f.room)
	a.Vitals().ApplyDamage(15)
	giveMedkit(t, f, a, 0) // out of supplies

	dispatchRole(t, treatEnv(t, f, a.PlayerID(), 25, 15), a, "treat")

	if !strings.Contains(a.lastLine(), "out of supplies") {
		t.Errorf("line = %q, want an out-of-supplies message", a.lastLine())
	}
}

// Treating an unwounded target is a no-op that keeps the supplies.
func TestTreat_AtFullHealthKeepsSupplies(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Medic", "p-medic", f.room)
	kit := giveMedkit(t, f, a, 10) // actor at full HP

	dispatchRole(t, treatEnv(t, f, a.PlayerID(), 25, 15), a, "treat")

	if !strings.Contains(a.lastLine(), "good shape") {
		t.Errorf("line = %q, want an already-healthy message", a.lastLine())
	}
	if got := kitCharges(t, kit); got != 10 {
		t.Errorf("charges = %d, want 10 (no supplies spent on a no-op)", got)
	}
}

// A medic patches up a wounded ally in the room (the guard mob): the target's
// HP rises and the medic's own kit is spent.
func TestTreat_HealsAnotherInRoom(t *testing.T) {
	f := newConsiderFixture(t)
	a := newCombatActor("Medic", "p-medic", f.room)
	kit := giveMedkit(t, f, a, 10)
	f.guard.Vitals().ApplyDamage(25) // 40 → 15

	env := treatEnv(t, f, a.PlayerID(), 25, 15)
	healed := captureEvents(t, env.Bus, eventbus.EventEntityHealed)
	dispatchRole(t, env, a, "treat guard")

	if cur, _ := f.guard.Vitals().Snapshot(); cur != 24 {
		t.Errorf("guard HP = %d, want 24 (healed 9 from 15)", cur)
	}
	if got := kitCharges(t, kit); got != 9 {
		t.Errorf("charges = %d, want 9 (a charge spent treating the ally)", got)
	}
	if !strings.Contains(a.lastLine(), "trauma sealant into a village guard's wounds. (+9 HP)") {
		t.Errorf("line = %q, want an ally-treat confirmation reporting +9 HP", a.lastLine())
	}
	// EntityHealed.Amount must carry the true delta (9), not the new current (24).
	if len(*healed) != 1 {
		t.Fatalf("EntityHealed count = %d, want 1", len(*healed))
	}
	if ev := (*healed)[0].(eventbus.EntityHealed); ev.Amount != 9 {
		t.Errorf("EntityHealed.Amount = %d, want 9 (the HP restored, not the new current)", ev.Amount)
	}
}
