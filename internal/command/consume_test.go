package command_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// consumeRig builds a store with one item (from props), a testActor
// holding it, and a wired ConsumableService.
func consumeRig(t *testing.T, props map[string]any) (*command.Context, *testActor, entities.EntityID) {
	t.Helper()
	tpl := &item.Template{ID: "ration", Name: "a trail ration", Type: "item", Keywords: []string{"ration"}, Properties: props}
	store := entities.NewStore()
	inst, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a := newTestActor(&world.Room{ID: "x:1"})
	a.playerID = "p1"
	a.inventory = []entities.EntityID{inst.ID()}
	a.sust = 50
	svc := economy.NewConsumableService(store, economy.NewSustenanceService(economy.DefaultSustenanceConfig()), nil)
	c := &command.Context{Actor: a, Items: store, Consumable: svc, Args: []string{"ration"}}
	return c, a, inst.ID()
}

func TestEatVerb_FeedsAndDestroys(t *testing.T) {
	c, a, _ := consumeRig(t, map[string]any{"consume_method": "eat", "sustenance_value": 30})
	if err := command.EatHandler(context.Background(), c); err != nil {
		t.Fatalf("EatHandler: %v", err)
	}
	if a.sust != 80 {
		t.Errorf("sustenance = %d, want 80", a.sust)
	}
	if len(a.inventory) != 0 {
		t.Errorf("inventory = %v, want empty", a.inventory)
	}
	if len(a.lines) != 1 || a.lines[0] != "You eat a trail ration." {
		t.Errorf("output = %v, want eat message", a.lines)
	}
}

func TestDrinkVerb_WrongMethodRejected(t *testing.T) {
	// An eat-only item can't be drunk.
	c, a, _ := consumeRig(t, map[string]any{"consume_method": "eat", "sustenance_value": 30})
	if err := command.DrinkHandler(context.Background(), c); err != nil {
		t.Fatalf("DrinkHandler: %v", err)
	}
	if len(a.inventory) != 1 {
		t.Error("wrong-method drink must not consume the item")
	}
	if len(a.lines) != 1 || a.lines[0] != "You can't drink a trail ration." {
		t.Errorf("output = %v, want wrong-method message", a.lines)
	}
}

func TestEatVerb_NoArgsPrompts(t *testing.T) {
	c, a, _ := consumeRig(t, map[string]any{"consume_method": "eat"})
	c.Args = nil
	if err := command.EatHandler(context.Background(), c); err != nil {
		t.Fatalf("EatHandler: %v", err)
	}
	if len(a.lines) != 1 || a.lines[0] != "Eat what?" {
		t.Errorf("output = %v, want 'Eat what?'", a.lines)
	}
}

func TestEatVerb_NilServiceGuards(t *testing.T) {
	c, a, _ := consumeRig(t, map[string]any{"consume_method": "eat"})
	c.Consumable = nil
	if err := command.EatHandler(context.Background(), c); err != nil {
		t.Fatalf("EatHandler: %v", err)
	}
	if len(a.lines) != 1 || a.lines[0] != "You can't do that right now." {
		t.Errorf("output = %v, want nil-service guard", a.lines)
	}
}
