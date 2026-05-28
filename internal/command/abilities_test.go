package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// abilityFixture wires the three ability managers a verb needs:
// a registry (display names + classification), a proficiency manager
// (learned set + caps), and an action queue (the enqueue target).
type abilityFixture struct {
	reg   *progression.AbilityRegistry
	prof  *progression.ProficiencyManager
	queue *progression.ActionQueueManager
}

func newAbilityFixture(t *testing.T) *abilityFixture {
	t.Helper()
	reg := progression.NewAbilityRegistry()
	mustRegister(t, reg, &progression.Ability{
		ID: "bless", DisplayName: "Bless",
		Type: progression.AbilityActive, Category: progression.AbilitySpell,
		DefaultCap: 30,
	})
	mustRegister(t, reg, &progression.Ability{
		ID: "kick", DisplayName: "Kick",
		Type: progression.AbilityActive, Category: progression.AbilitySkill,
		DefaultCap: 25,
	})
	return &abilityFixture{
		reg:   reg,
		prof:  progression.NewProficiencyManager(reg, progression.DefaultProficiencyConfig()),
		queue: progression.NewActionQueueManager(progression.ActionQueueConfig{}),
	}
}

func mustRegister(t *testing.T, r *progression.AbilityRegistry, a *progression.Ability) {
	t.Helper()
	if err := r.Register(a); err != nil {
		t.Fatalf("register %s: %v", a.ID, err)
	}
}

func (f *abilityFixture) env() command.Env {
	return command.Env{
		Abilities:   f.reg,
		Proficiency: f.prof,
		ActionQueue: f.queue,
	}
}

// --- abilities listing ---

func TestAbilities_EmptyWhenNoneLearned(t *testing.T) {
	f := newAbilityFixture(t)
	a := newNamedTestActor("Tester", "p-1", nil)
	ctx := &command.Context{Actor: a, Proficiency: f.prof, Abilities: f.reg}
	if err := command.AbilitiesHandler(context.Background(), ctx); err != nil {
		t.Fatalf("AbilitiesHandler: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "haven't learned any abilities") {
		t.Errorf("empty listing = %q", got)
	}
}

func TestAbilities_ListsLearnedWithProficiencyAndCap(t *testing.T) {
	f := newAbilityFixture(t)
	f.prof.Learn("p-1", "bless", 12)
	f.prof.SetCap("p-1", "bless", 30)
	a := newNamedTestActor("Tester", "p-1", nil)
	ctx := &command.Context{Actor: a, Proficiency: f.prof, Abilities: f.reg}
	if err := command.AbilitiesHandler(context.Background(), ctx); err != nil {
		t.Fatalf("AbilitiesHandler: %v", err)
	}
	got := a.lastLine()
	for _, want := range []string{"Bless", "spell", "12/30"} {
		if !strings.Contains(got, want) {
			t.Errorf("listing %q missing %q", got, want)
		}
	}
}

// A granted-but-unregistered ability (declarative class-path grant
// without content) is shown rather than dropped, flagged unlearnable.
func TestAbilities_ShowsUnlearnableForUnregistered(t *testing.T) {
	f := newAbilityFixture(t)
	f.prof.Learn("p-1", "phantom-skill", 1)
	a := newNamedTestActor("Tester", "p-1", nil)
	ctx := &command.Context{Actor: a, Proficiency: f.prof, Abilities: f.reg}
	if err := command.AbilitiesHandler(context.Background(), ctx); err != nil {
		t.Fatalf("AbilitiesHandler: %v", err)
	}
	got := a.lastLine()
	if !strings.Contains(got, "phantom-skill") || !strings.Contains(got, "unlearnable") {
		t.Errorf("unregistered ability listing = %q", got)
	}
}

func TestAbilities_DisabledWhenProficiencyUnwired(t *testing.T) {
	a := newNamedTestActor("Tester", "p-1", nil)
	ctx := &command.Context{Actor: a} // no Proficiency
	if err := command.AbilitiesHandler(context.Background(), ctx); err != nil {
		t.Fatalf("AbilitiesHandler: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "not enabled") {
		t.Errorf("disabled message = %q", got)
	}
}

// --- cast / enqueue ---

func TestCast_NoArgPrompts(t *testing.T) {
	f := newAbilityFixture(t)
	a := newNamedTestActor("Tester", "p-1", nil)
	ctx := &command.Context{Actor: a, Abilities: f.reg, ActionQueue: f.queue, Verb: "cast"}
	if err := command.CastHandler(context.Background(), ctx); err != nil {
		t.Fatalf("CastHandler: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "Cast what?") {
		t.Errorf("no-arg = %q", got)
	}
}

func TestCast_UnknownAbilityRefuses(t *testing.T) {
	f := newAbilityFixture(t)
	a := newNamedTestActor("Tester", "p-1", nil)
	ctx := &command.Context{Actor: a, Abilities: f.reg, ActionQueue: f.queue,
		Verb: "cast", Args: []string{"nonsense"}}
	if err := command.CastHandler(context.Background(), ctx); err != nil {
		t.Fatalf("CastHandler: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "don't know how") {
		t.Errorf("unknown = %q", got)
	}
	if f.queue.Len("p-1") != 0 {
		t.Error("unknown ability must not enqueue")
	}
}

func TestCast_SelfBuffEnqueues(t *testing.T) {
	f := newAbilityFixture(t)
	a := newNamedTestActor("Tester", "p-1", nil)
	ctx := &command.Context{Actor: a, Abilities: f.reg, ActionQueue: f.queue,
		Verb: "cast", Args: []string{"bless"}}
	if err := command.CastHandler(context.Background(), ctx); err != nil {
		t.Fatalf("CastHandler: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "prepare to use Bless") {
		t.Errorf("confirm = %q", got)
	}
	if f.queue.Len("p-1") != 1 {
		t.Fatalf("queue len = %d, want 1", f.queue.Len("p-1"))
	}
	action, ok := f.queue.Peek("p-1")
	if !ok || action.AbilityID != "bless" || action.TargetEntityID != "" {
		t.Errorf("queued action = %+v, want bless/no-target", action)
	}
}

// AbilityVerb (skill-named verb) enqueues a fixed id, args = target.
func TestAbilityVerb_EnqueuesFixedAbility(t *testing.T) {
	f := newAbilityFixture(t)
	a := newNamedTestActor("Tester", "p-1", nil)
	verb := command.AbilityVerb("bless")
	ctx := &command.Context{Actor: a, Abilities: f.reg, ActionQueue: f.queue}
	if err := verb(context.Background(), ctx); err != nil {
		t.Fatalf("AbilityVerb: %v", err)
	}
	if f.queue.Len("p-1") != 1 {
		t.Fatalf("queue len = %d, want 1", f.queue.Len("p-1"))
	}
}

func TestCast_TargetNotFoundRefuses(t *testing.T) {
	f := newAbilityFixture(t)
	room := &world.Room{ID: "room-1", Name: "Room", Description: "x"}
	a := newNamedTestActor("Tester", "p-1", room)
	// No Locator / Placement wired ⇒ no target resolves.
	ctx := &command.Context{Actor: a, Abilities: f.reg, ActionQueue: f.queue,
		Verb: "cast", Args: []string{"kick", "goblin"}}
	if err := command.CastHandler(context.Background(), ctx); err != nil {
		t.Fatalf("CastHandler: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "don't see them here") {
		t.Errorf("missing target = %q", got)
	}
	if f.queue.Len("p-1") != 0 {
		t.Error("unresolved target must not enqueue")
	}
}

func TestCast_ResolvesTargetViaLocator(t *testing.T) {
	f := newAbilityFixture(t)
	room := &world.Room{ID: "room-1", Name: "Room", Description: "x"}
	alice := newCombatActor("Alice", "p-1", room)
	goblin := newCombatActor("Goblin", "p-2", room)
	ctx := &command.Context{
		Actor: alice, Abilities: f.reg, ActionQueue: f.queue,
		Locator: locatorFunc(func(_ world.RoomID, name string) command.Actor {
			if strings.EqualFold(name, "Goblin") {
				return goblin
			}
			return nil
		}),
		Verb: "kick", Args: []string{"Goblin"},
	}
	if err := command.AbilityVerb("kick")(context.Background(), ctx); err != nil {
		t.Fatalf("AbilityVerb: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "prepare to use Kick on Goblin") {
		t.Errorf("confirm = %q", got)
	}
	action, ok := f.queue.Peek("p-1")
	if !ok || action.AbilityID != "kick" || action.TargetEntityID != "p-2" {
		t.Errorf("queued action = %+v, want kick→p-2", action)
	}
}

func TestCast_QueueAtCapacityRefuses(t *testing.T) {
	f := newAbilityFixture(t)
	a := newNamedTestActor("Tester", "p-1", nil)
	// Fill the queue to its limit with valid entries.
	for i := 0; i < progression.DefaultActionQueueLimit; i++ {
		if !f.queue.Push("p-1", progression.QueuedAction{AbilityID: "bless"}) {
			t.Fatalf("setup push %d failed", i)
		}
	}
	ctx := &command.Context{Actor: a, Abilities: f.reg, ActionQueue: f.queue,
		Verb: "cast", Args: []string{"bless"}}
	if err := command.CastHandler(context.Background(), ctx); err != nil {
		t.Fatalf("CastHandler: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "can't prepare any more") {
		t.Errorf("capacity refusal = %q", got)
	}
}

func TestCast_DisabledWhenManagersUnwired(t *testing.T) {
	a := newNamedTestActor("Tester", "p-1", nil)
	ctx := &command.Context{Actor: a, Verb: "cast", Args: []string{"bless"}}
	if err := command.CastHandler(context.Background(), ctx); err != nil {
		t.Fatalf("CastHandler: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "can't use abilities right now") {
		t.Errorf("disabled = %q", got)
	}
}
