package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/crafting"
	"github.com/Jasrags/AnotherMUD/internal/decoration"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// zeroRoller is a deterministic crafting.Roller for the gain roll.
type zeroRoller struct{}

func (zeroRoller) IntN(int) int { return 0 }

// timedCraftFixture wires a real crafting.Service with one timed recipe
// (1 iron → 1 sword, smithing, time_pulses), the actor knowing it and
// carrying the input, plus a Context with a controllable NowTick.
func timedCraftFixture(t *testing.T, timePulses int) (*command.Context, *testActor, *entities.Store, *uint64) {
	t.Helper()

	tpls := item.NewTemplates()
	inTpl := &item.Template{ID: "core:iron", Name: "an iron bar", Type: "item"}
	tpls.Add(inTpl)
	tpls.Add(&item.Template{ID: "core:sword", Name: "a sword", Type: "item"})

	store := entities.NewStore()
	recipes := recipe.NewRegistry()
	if err := recipes.TryAdd(&recipe.Recipe{
		ID: "core:forge-sword", DisplayName: "forge a sword", Discipline: "smithing",
		SkillFloor: 1, TimePulses: timePulses,
		Inputs: []recipe.Ingredient{{Template: "core:iron", Quantity: 1}},
		Output: recipe.Output{Template: "core:sword", Quantity: 1},
	}); err != nil {
		t.Fatal(err)
	}
	known := recipe.NewKnownManager(recipes)
	known.Learn("p-smith", "core:forge-sword")

	abilities := progression.NewAbilityRegistry()
	_ = abilities.Register(&progression.Ability{
		ID: "smithing", DisplayName: "Smithing",
		Type: progression.AbilityPassive, Category: progression.AbilitySkill,
		DefaultCap: 100, GainBaseChance: 30,
	})
	prof := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())
	prof.Learn("p-smith", "smithing", 50)

	svc := crafting.NewService(tpls, store, recipes, known, prof,
		decoration.NewRarityRegistry(), zeroRoller{}, crafting.DefaultConfig(), nil)

	a := newNamedTestActor("Smith", "p-smith", &world.Room{ID: "core:forge", Name: "Forge"})
	inst, err := store.Spawn(inTpl)
	if err != nil {
		t.Fatalf("spawn input: %v", err)
	}
	a.AddToInventory(inst.ID())

	now := uint64(100)
	c := &command.Context{
		Actor:   a,
		Verb:    "craft",
		Craft:   svc,
		NowTick: func() uint64 { return now },
	}
	return c, a, store, &now
}

func TestCraftHandler_TimedStartsOccupationNotImmediate(t *testing.T) {
	c, a, _, _ := timedCraftFixture(t, 30)
	c.Args = []string{"forge", "a", "sword"}

	if err := command.CraftHandler(context.Background(), c); err != nil {
		t.Fatalf("CraftHandler: %v", err)
	}
	// Announced the start, did NOT produce the output yet.
	if !strings.Contains(a.lastLine(), "begin to forge a sword") {
		t.Errorf("output = %q, want a begin message", a.lastLine())
	}
	if got := a.Inventory(); len(got) != 1 {
		t.Errorf("inventory = %v, want still 1 (input held, no output yet)", got)
	}
	p, ok := a.PendingCraft()
	if !ok {
		t.Fatal("expected an in-flight craft after a timed start")
	}
	if p.ReadyAt != 130 { // now(100) + time_pulses(30)
		t.Errorf("ReadyAt = %d, want 130", p.ReadyAt)
	}
}

func TestCraftHandler_RefusesSecondCraftWhileBusy(t *testing.T) {
	c, a, _, _ := timedCraftFixture(t, 30)
	c.Args = []string{"forge", "a", "sword"}
	_ = command.CraftHandler(context.Background(), c) // first: starts

	_ = command.CraftHandler(context.Background(), c) // second: refused
	if !strings.Contains(a.lastLine(), "still busy trying to") {
		t.Errorf("output = %q, want an already-busy refusal", a.lastLine())
	}
}

func TestCraftHandler_ZeroTimeCraftsInstantly(t *testing.T) {
	c, a, store, _ := timedCraftFixture(t, 0)
	c.Args = []string{"forge", "a", "sword"}

	if err := command.CraftHandler(context.Background(), c); err != nil {
		t.Fatalf("CraftHandler: %v", err)
	}
	// Instant: output in the bag, no pending craft.
	if _, ok := a.PendingCraft(); ok {
		t.Error("a zero-time craft should not occupy the player")
	}
	inv := a.Inventory()
	if len(inv) != 1 {
		t.Fatalf("inventory = %v, want 1 (the output)", inv)
	}
	if e, ok := store.GetByID(inv[0]); !ok || string(e.(*entities.ItemInstance).TemplateID()) != "core:sword" {
		t.Error("instant craft did not produce the sword")
	}
}
