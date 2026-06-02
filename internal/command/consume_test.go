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
// holding it, and an Env with a wired ConsumableService. M17.2d
// migrated the consume verbs onto the §5 pipeline, so tests dispatch
// through the registry (which pre-resolves the inventory arg) rather
// than calling the handler directly.
func consumeRig(t *testing.T, props map[string]any) (command.Env, *testActor, entities.EntityID) {
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
	return command.Env{Items: store, Consumable: svc}, a, inst.ID()
}

func dispatchConsume(t *testing.T, env command.Env, a *testActor, line string) {
	t.Helper()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), env, a, line); err != nil {
		t.Fatalf("dispatch %q: %v", line, err)
	}
}

func TestEatVerb_FeedsAndDestroys(t *testing.T) {
	env, a, _ := consumeRig(t, map[string]any{"consume_method": "eat", "sustenance_value": 30})
	dispatchConsume(t, env, a, "eat ration")
	if a.sust != 80 {
		t.Errorf("sustenance = %d, want 80", a.sust)
	}
	if len(a.inventory) != 0 {
		t.Errorf("inventory = %v, want empty", a.inventory)
	}
	if a.lastLine() != "You eat a trail ration." {
		t.Errorf("output = %q, want eat message", a.lastLine())
	}
}

func TestDrinkVerb_WrongMethodRejected(t *testing.T) {
	// An eat-only item can't be drunk.
	env, a, _ := consumeRig(t, map[string]any{"consume_method": "eat", "sustenance_value": 30})
	dispatchConsume(t, env, a, "drink ration")
	if len(a.inventory) != 1 {
		t.Error("wrong-method drink must not consume the item")
	}
	if a.lastLine() != "You can't drink a trail ration." {
		t.Errorf("output = %q, want wrong-method message", a.lastLine())
	}
}

func TestUseVerb_ConsumesDrinkMethodItem(t *testing.T) {
	// `use` is a generic fallback: it consumes a drink-method item and
	// reports the verb the player typed ("use").
	env, a, _ := consumeRig(t, map[string]any{"consume_method": "drink", "sustenance_value": 20})
	dispatchConsume(t, env, a, "use ration")
	if len(a.inventory) != 0 {
		t.Errorf("inventory = %v, want empty (use consumed it)", a.inventory)
	}
	if a.lastLine() != "You use a trail ration." {
		t.Errorf("output = %q, want use message", a.lastLine())
	}
}

func TestUseVerb_RejectsNonConsumable(t *testing.T) {
	// `use` on an item with no consume_method must not destroy it.
	env, a, _ := consumeRig(t, map[string]any{"weight": 3})
	dispatchConsume(t, env, a, "use ration")
	if len(a.inventory) != 1 {
		t.Error("use must not consume a non-consumable")
	}
	if a.lastLine() != "You can't use a trail ration." {
		t.Errorf("output = %q, want can't-use message", a.lastLine())
	}
}

func TestEatVerb_NoArgsPrompts(t *testing.T) {
	env, a, _ := consumeRig(t, map[string]any{"consume_method": "eat"})
	// M17.2d: missing required arg now yields the §5.4 dispatcher
	// prompt instead of the old hand-rolled "Eat what?".
	dispatchConsume(t, env, a, "eat")
	if a.lastLine() != "What item?" {
		t.Errorf("output = %q, want 'What item?'", a.lastLine())
	}
}

func TestEatVerb_NilServiceGuards(t *testing.T) {
	env, a, _ := consumeRig(t, map[string]any{"consume_method": "eat"})
	env.Consumable = nil
	dispatchConsume(t, env, a, "eat ration")
	if a.lastLine() != "You can't do that right now." {
		t.Errorf("output = %q, want nil-service guard", a.lastLine())
	}
}
