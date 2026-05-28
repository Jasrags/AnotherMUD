package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// coins is a currency-tagged template carrying a positive value.
func coins(value int) *item.Template {
	return &item.Template{
		ID:         "tapestry-core:gold-coins",
		Name:       "a pile of gold coins",
		Type:       "treasure",
		Tags:       []string{"currency"},
		Keywords:   []string{"coins", "gold", "pile"},
		Properties: map[string]any{"value": value},
	}
}

// currencyEnv builds an env wired with a currency service over the
// inventory fixture's store.
func (f *invFixture) currencyEnv(svc *economy.CurrencyService) command.Env {
	e := f.env()
	e.Currency = svc
	return e
}

func dispatchBuiltin(t *testing.T, env command.Env, a command.Actor, line string) {
	t.Helper()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), env, a, line); err != nil {
		t.Fatalf("dispatch %q: %v", line, err)
	}
}

func TestGet_CurrencyAutoConverts(t *testing.T) {
	f := newInvFixture(t)
	inst := f.spawnInRoom(t, coins(25))
	a := newTestActor(f.room)
	svc := economy.NewCurrencyService(nil)

	dispatchBuiltin(t, f.currencyEnv(svc), a, "get coins")

	if got := a.Gold(); got != 25 {
		t.Errorf("gold = %d, want 25 (auto-converted)", got)
	}
	if got := a.Inventory(); len(got) != 0 {
		t.Errorf("inventory = %v, want empty (coins should not stay as item)", got)
	}
	if _, ok := f.store.GetByID(inst.ID()); ok {
		t.Error("coin item should be untracked from the world after conversion")
	}
	if !strings.Contains(a.lastLine(), "25 gold") {
		t.Errorf("reply %q should mention the gold credited", a.lastLine())
	}
}

func TestGet_CurrencyZeroValueStaysItem(t *testing.T) {
	f := newInvFixture(t)
	inst := f.spawnInRoom(t, coins(0))
	a := newTestActor(f.room)
	svc := economy.NewCurrencyService(nil)

	dispatchBuiltin(t, f.currencyEnv(svc), a, "get coins")

	// Zero value → not converted → behaves like a normal item pickup.
	if got := a.Gold(); got != 0 {
		t.Errorf("gold = %d, want 0", got)
	}
	if got := a.Inventory(); len(got) != 1 || got[0] != inst.ID() {
		t.Errorf("inventory = %v, want the coin item (zero-value not converted)", got)
	}
}

func TestGet_CurrencyNoServiceStaysItem(t *testing.T) {
	f := newInvFixture(t)
	inst := f.spawnInRoom(t, coins(25))
	a := newTestActor(f.room)

	// No currency service in env → auto-convert is a no-op.
	dispatchBuiltin(t, f.env(), a, "get coins")

	if got := a.Gold(); got != 0 {
		t.Errorf("gold = %d, want 0 (no service)", got)
	}
	if got := a.Inventory(); len(got) != 1 || got[0] != inst.ID() {
		t.Errorf("inventory = %v, want the coin item (no service → no conversion)", got)
	}
}

func TestGive_CurrencyAutoConvertsToRecipient(t *testing.T) {
	f := newInvFixture(t)
	inst, err := f.store.Spawn(coins(40))
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	giver := newNamedTestActor("Giver", "p1", f.room)
	giver.AddToInventory(inst.ID())
	recipient := newNamedTestActor("Bob", "p2", f.room)
	svc := economy.NewCurrencyService(nil)

	env := f.currencyEnv(svc)
	env.Locator = locatorFunc(func(world.RoomID, string) command.Actor { return recipient })

	dispatchBuiltin(t, env, giver, "give coins to Bob")

	if got := recipient.Gold(); got != 40 {
		t.Errorf("recipient gold = %d, want 40", got)
	}
	if got := recipient.Inventory(); len(got) != 0 {
		t.Errorf("recipient inventory = %v, want empty (converted to gold)", got)
	}
	if got := giver.Inventory(); len(got) != 0 {
		t.Errorf("giver inventory = %v, want empty (coins left)", got)
	}
	if _, ok := f.store.GetByID(inst.ID()); ok {
		t.Error("coin item should be untracked after give-conversion")
	}
}

func TestGold_VerbReportsBalance(t *testing.T) {
	f := newInvFixture(t)
	a := newTestActor(f.room)
	a.SetGold(123)
	svc := economy.NewCurrencyService(nil)

	dispatchBuiltin(t, f.currencyEnv(svc), a, "gold")

	if !strings.Contains(a.lastLine(), "123 gold") {
		t.Errorf("reply %q should report 123 gold", a.lastLine())
	}
}

func TestGold_VerbZeroBalance(t *testing.T) {
	f := newInvFixture(t)
	a := newTestActor(f.room)

	dispatchBuiltin(t, f.currencyEnv(economy.NewCurrencyService(nil)), a, "gold")

	if !strings.Contains(a.lastLine(), "no gold") {
		t.Errorf("reply %q should report no gold", a.lastLine())
	}
}
