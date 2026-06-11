package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// fakeEffTemplates is a minimal EffectTemplateSource for the afflict verb.
type fakeEffTemplates map[string]progression.EffectTemplate

func (f fakeEffTemplates) Get(id string) (progression.EffectTemplate, bool) {
	t, ok := f[strings.ToLower(strings.TrimSpace(id))]
	return t, ok
}

func conditionTemplates() fakeEffTemplates {
	return fakeEffTemplates{
		"stunned": {ID: "stunned", Duration: 3, Flags: []string{"condition:stunned"}},
		"prone":   {ID: "prone", Duration: 5, Flags: []string{"condition:prone"}},
		"bless":   {ID: "bless", Duration: 300, Flags: []string{"blessed"}}, // NOT a condition
	}
}

func TestAfflict_AppliesConditionAndAudits(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	em := progression.NewEffectManager(nil, nil)
	env := f.env()
	env.Bus = bus
	env.Effects = em
	env.EffectTemplates = conditionTemplates()

	dispatchRole(t, env, admin, "afflict guard stunned")

	if !em.Has(f.guard.EntityID(), "stunned") {
		t.Error("guard not stunned after afflict")
	}
	if !strings.Contains(admin.lastLine(), "afflict") {
		t.Errorf("confirmation = %q", admin.lastLine())
	}
	if len(*got) != 1 {
		t.Fatalf("audit count = %d, want 1", len(*got))
	}
	if ev := (*got)[0].(eventbus.AdminAction); ev.Verb != "afflict" || ev.Target != f.guard.EntityID() {
		t.Errorf("audit = %+v, want verb=afflict target=%s", ev, f.guard.EntityID())
	}
}

func TestAfflict_RejectsNonCondition(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	em := progression.NewEffectManager(nil, nil)
	env := f.env()
	env.Bus = bus
	env.Effects = em
	env.EffectTemplates = conditionTemplates()

	dispatchRole(t, env, admin, "afflict guard bless") // bless is not a condition

	if em.Has(f.guard.EntityID(), "bless") {
		t.Error("afflict applied a non-condition effect")
	}
	if !strings.Contains(admin.lastLine(), "no such condition") {
		t.Errorf("message = %q, want 'no such condition'", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a rejected afflict must not audit, got %d", len(*got))
	}
}

func TestAfflict_DurationOverride(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	em := progression.NewEffectManager(nil, nil)
	env := f.env()
	env.Effects = em
	env.EffectTemplates = conditionTemplates()

	dispatchRole(t, env, admin, "afflict guard stunned 9")

	effs := em.Effects(f.guard.EntityID())
	if len(effs) != 1 || effs[0].ID != "stunned" {
		t.Fatalf("effects = %+v, want one stunned", effs)
	}
	if effs[0].Remaining != 9 {
		t.Errorf("duration override = %d, want 9", effs[0].Remaining)
	}
}

func TestCure_ByNameAndAll(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	em := progression.NewEffectManager(nil, nil)
	env := f.env()
	env.Effects = em
	env.EffectTemplates = conditionTemplates()

	dispatchRole(t, env, admin, "afflict guard stunned")
	dispatchRole(t, env, admin, "afflict guard prone")

	// Cure one by name.
	dispatchRole(t, env, admin, "cure guard stunned")
	if em.Has(f.guard.EntityID(), "stunned") {
		t.Error("stunned not cured by name")
	}
	if !em.Has(f.guard.EntityID(), "prone") {
		t.Error("prone wrongly cured when only stunned was named")
	}
	// Cure all remaining conditions.
	dispatchRole(t, env, admin, "cure guard")
	if em.Has(f.guard.EntityID(), "prone") {
		t.Error("cure-all left prone behind")
	}
	if !strings.Contains(admin.lastLine(), "cure") {
		t.Errorf("confirmation = %q", admin.lastLine())
	}
}

func TestAfflict_RefusedForNonAdmin(t *testing.T) {
	f := newConsiderFixture(t)
	bob := newRoleActor("Bob", "p-bob") // no admin role
	bob.SetRoom(f.room)
	em := progression.NewEffectManager(nil, nil)
	env := f.env()
	env.Effects = em
	env.EffectTemplates = conditionTemplates()

	dispatchRole(t, env, bob, "afflict guard stunned")

	if bob.lastLine() != "Huh?" {
		t.Errorf("refusal = %q, want 'Huh?'", bob.lastLine())
	}
	if em.Has(f.guard.EntityID(), "stunned") {
		t.Error("non-admin afflict applied a condition")
	}
}

func TestStand_ClearsProne(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	em := progression.NewEffectManager(nil, nil)
	env := f.env()
	env.Effects = em

	// Put the actor prone directly, then stand.
	em.Apply(context.Background(), admin.PlayerID(), progression.EffectTemplate{
		ID: "prone", Duration: 5, Flags: []string{"condition:prone"}}, "", "")

	dispatchRole(t, env, admin, "stand")

	if em.Has(admin.PlayerID(), "prone") {
		t.Error("stand did not clear prone")
	}
	if !strings.Contains(admin.lastLine(), "feet") {
		t.Errorf("stand message = %q, want a 'to your feet' line", admin.lastLine())
	}
}

func TestAffects_ListsConditionsAndEffects(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	em := progression.NewEffectManager(nil, nil)
	env := f.env()
	env.Effects = em

	em.Apply(context.Background(), admin.PlayerID(), progression.EffectTemplate{
		ID: "stunned", Duration: 3, Flags: []string{"condition:stunned"}}, "", "")
	em.Apply(context.Background(), admin.PlayerID(), progression.EffectTemplate{
		ID: "blessed", Duration: 10, Flags: []string{"blessed"}}, "", "")

	dispatchRole(t, env, admin, "affects")

	out := admin.lastLine()
	for _, want := range []string{"Stunned", "3 round(s)", "[condition]", "Blessed"} {
		if !strings.Contains(out, want) {
			t.Errorf("affects output missing %q\n--- got ---\n%s", want, out)
		}
	}
	// The non-condition 'blessed' must NOT carry the condition tag — check the
	// tag count equals the one condition.
	if got := strings.Count(out, "[condition]"); got != 1 {
		t.Errorf("[condition] tag count = %d, want 1 (only stunned)", got)
	}
}

func TestAffects_EmptyWhenNone(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Effects = progression.NewEffectManager(nil, nil)
	dispatchRole(t, env, admin, "affects")
	if !strings.Contains(admin.lastLine(), "no active effects") {
		t.Errorf("empty affects = %q", admin.lastLine())
	}
}
