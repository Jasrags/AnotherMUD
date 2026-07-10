package command_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/corpse"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

type lootFixture struct {
	*invFixture
	contents *entities.Contents
	bus      *eventbus.Bus
	currency *economy.CurrencyService
	now      uint64
	window   uint64
}

func newLootFixture(t *testing.T) *lootFixture {
	t.Helper()
	return &lootFixture{
		invFixture: newInvFixture(t),
		contents:   entities.NewContents(),
		bus:        eventbus.New(),
		currency:   economy.NewCurrencyService(nil),
		now:        110,
		window:     100,
	}
}

func (f *lootFixture) env() command.Env {
	e := f.invFixture.env()
	e.Contents = f.contents
	e.Bus = f.bus
	e.Currency = f.currency
	e.NowTick = func() uint64 { return f.now }
	e.CorpseOwnershipWindow = f.window
	return e
}

// placeCorpse mints a corpse in the room with the given owner set,
// creation tick, coins, and item contents.
func (f *lootFixture) placeCorpse(t *testing.T, owners []string, createdTick uint64, coins int, items ...*item.Template) *entities.ItemInstance {
	t.Helper()
	cor, err := f.store.SpawnContainer("the corpse of a goblin",
		[]string{corpse.TagCorpse, corpse.TagNoGet, corpse.TagNoPut},
		[]string{"corpse", "goblin"},
		map[string]any{
			corpse.PropOwners:      owners,
			corpse.PropCreatedTick: createdTick,
			corpse.PropCoins:       coins,
		})
	if err != nil {
		t.Fatalf("SpawnContainer: %v", err)
	}
	f.place.Place(cor.ID(), f.room.ID)
	for _, tpl := range items {
		it, err := f.store.Spawn(tpl)
		if err != nil {
			t.Fatalf("Spawn: %v", err)
		}
		f.contents.Put(cor.ID(), it.ID())
	}
	return cor
}

func ration() *item.Template {
	return &item.Template{ID: "tapestry-core:trail-ration", Name: "a trail ration", Type: "food", Keywords: []string{"ration"}}
}

func dispatchLoot(t *testing.T, f *lootFixture, a command.Actor, line string) {
	t.Helper()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), f.env(), a, line); err != nil {
		t.Fatalf("dispatch %q: %v", line, err)
	}
}

func TestLoot_OwnerTakesItemsAndCoins(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	cor := f.placeCorpse(t, []string{"player:p-alice"}, 100, 5, ration(), sword())

	var looted *eventbus.CorpseLooted
	f.bus.Subscribe(eventbus.EventCorpseLooted, func(_ context.Context, ev eventbus.Event) {
		e := ev.(eventbus.CorpseLooted)
		looted = &e
	})

	dispatchLoot(t, f, a, "loot corpse")

	if got := len(a.Inventory()); got != 2 {
		t.Fatalf("inventory = %d items, want 2", got)
	}
	if got := a.Gold(); got != 5 {
		t.Errorf("gold = %d, want 5", got)
	}
	// Corpse emptied → removed from world + placement.
	if _, ok := f.store.GetByID(cor.ID()); ok {
		t.Error("emptied corpse should be untracked")
	}
	if got, ok := f.place.RoomOf(cor.ID()); ok {
		t.Errorf("emptied corpse still placed in %q", got)
	}
	if looted == nil || looted.ItemCount != 2 || looted.Coins != 5 {
		t.Errorf("corpse.looted = %+v", looted)
	}
}

func TestLoot_NonOwnerRefusedDuringWindow(t *testing.T) {
	f := newLootFixture(t)
	owner := newNamedTestActor("Alice", "p-alice", f.room)
	_ = owner
	eve := newNamedTestActor("Eve", "p-eve", f.room)
	cor := f.placeCorpse(t, []string{"player:p-alice"}, 100, 5, ration())

	dispatchLoot(t, f, eve, "loot corpse")

	if got := len(eve.Inventory()); got != 0 {
		t.Errorf("non-owner inventory = %d, want 0 (refused)", got)
	}
	if got := eve.Gold(); got != 0 {
		t.Errorf("non-owner gold = %d, want 0", got)
	}
	if _, ok := f.store.GetByID(cor.ID()); !ok {
		t.Error("refused corpse should remain")
	}
}

func TestLoot_OpenAfterWindow(t *testing.T) {
	f := newLootFixture(t)
	f.now = 250 // created 100 + window 100 = 150 < 250 → open
	eve := newNamedTestActor("Eve", "p-eve", f.room)
	f.placeCorpse(t, []string{"player:p-alice"}, 100, 0, ration())

	dispatchLoot(t, f, eve, "loot corpse")

	if got := len(eve.Inventory()); got != 1 {
		t.Errorf("after window, anyone loots: inventory = %d, want 1", got)
	}
}

func TestLoot_NoArgPicksCorpse(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	f.placeCorpse(t, []string{}, 100, 0, ration()) // empty owner → open

	dispatchLoot(t, f, a, "loot")

	if got := len(a.Inventory()); got != 1 {
		t.Errorf("no-arg loot: inventory = %d, want 1", got)
	}
}

func TestLoot_CoinsOnlyCorpseRemovedAfterCredit(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	cor := f.placeCorpse(t, []string{"player:p-alice"}, 100, 9) // no items

	dispatchLoot(t, f, a, "loot corpse")

	if got := a.Gold(); got != 9 {
		t.Errorf("gold = %d, want 9", got)
	}
	if _, ok := f.store.GetByID(cor.ID()); ok {
		t.Error("coins-only corpse should be removed after looting")
	}
}

func TestLoot_NothingHere(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	// No corpse placed.
	dispatchLoot(t, f, a, "loot")
	if got := len(a.Inventory()); got != 0 {
		t.Errorf("inventory = %d, want 0", got)
	}
}

func TestAutoloot_Toggles(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)

	dispatchLoot(t, f, a, "autoloot on")
	if !a.Autoloot() {
		t.Error("autoloot on did not enable")
	}
	dispatchLoot(t, f, a, "autoloot off")
	if a.Autoloot() {
		t.Error("autoloot off did not disable")
	}

	// No argument flips: off → on → off.
	dispatchLoot(t, f, a, "autoloot")
	if !a.Autoloot() {
		t.Error("bare `autoloot` should flip off → on")
	}
	dispatchLoot(t, f, a, "autoloot")
	if a.Autoloot() {
		t.Error("bare `autoloot` should flip on → off")
	}
}

// The loot message renders coins through the pack's currency label (currency-label
// seam), so a Shadowrun-style label shows "25¥", never "gold".
func TestLoot_UsesCurrencyLabel(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	f.placeCorpse(t, []string{"player:p-alice"}, 100, 25, ration())

	env := f.env()
	env.Money = economy.CurrencyLabel{Noun: "nuyen", Suffix: "¥"}
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), env, a, "loot corpse"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := a.lastLine(); !contains(got, "25¥") || contains(got, "gold") {
		t.Errorf("loot message = %q, want it to show 25¥ (not gold)", got)
	}
}

// TransferCorpse is the shared primitive both the loot verb and the
// autoloot path use; this exercises it directly with no rights gate.
func TestTransferCorpse_MovesItemsAndCoins(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	cor := f.placeCorpse(t, []string{"player:p-alice"}, 100, 8, ration(), sword())

	grant := command.LootGrant{Items: f.store, Contents: f.contents, Placement: f.place, Currency: f.currency, Bus: f.bus}
	taken, coins := command.TransferCorpse(context.Background(), grant, a, cor, f.room.ID, "player:p-alice")

	if len(taken) != 2 || coins != 8 {
		t.Fatalf("transfer = %d items, %d coins; want 2, 8", len(taken), coins)
	}
	if len(a.Inventory()) != 2 {
		t.Errorf("inventory = %d, want 2", len(a.Inventory()))
	}
	if a.Gold() != 8 {
		t.Errorf("gold = %d, want 8", a.Gold())
	}
	if _, ok := f.store.GetByID(cor.ID()); ok {
		t.Error("emptied corpse should be removed by TransferCorpse")
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestLoot_NoArgSkipsCorpseNotYetLootable(t *testing.T) {
	f := newLootFixture(t)
	// A corpse owned by someone else, still inside its window — the
	// no-arg picker must not select it for a non-owner.
	cor := f.placeCorpse(t, []string{"player:p-alice"}, 100, 0, ration())
	eve := newNamedTestActor("Eve", "p-eve", f.room)

	dispatchLoot(t, f, eve, "loot")

	if got := len(eve.Inventory()); got != 0 {
		t.Errorf("no-arg loot picked someone else's corpse: inventory = %d", got)
	}
	if _, ok := f.store.GetByID(cor.ID()); !ok {
		t.Error("corpse should remain")
	}
}

// Regression for the M22.3a coin double-credit + double-event races:
// two players looting the same open corpse concurrently must split the
// items without duplication and credit the coin pile exactly once.
func TestLoot_ConcurrentLootNoDuplication(t *testing.T) {
	f := newLootFixture(t)
	f.window = 0 // open to everyone
	a := newNamedTestActor("Alice", "p-alice", f.room)
	b := newNamedTestActor("Bob", "p-bob", f.room)
	f.placeCorpse(t, []string{}, 100, 10, ration(), sword())

	var looted int
	var lm sync.Mutex
	f.bus.Subscribe(eventbus.EventCorpseLooted, func(_ context.Context, _ eventbus.Event) {
		lm.Lock()
		looted++
		lm.Unlock()
	})

	env := f.env()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}

	var wg sync.WaitGroup
	for _, actor := range []command.Actor{a, b} {
		wg.Go(func() {
			_ = r.Dispatch(context.Background(), env, actor, "loot corpse")
		})
	}
	wg.Wait()

	if total := a.Gold() + b.Gold(); total != 10 {
		t.Errorf("total gold = %d, want 10 (coin pile credited once)", total)
	}
	if total := len(a.Inventory()) + len(b.Inventory()); total != 2 {
		t.Errorf("total items = %d, want 2 (no duplication)", total)
	}
	if looted != 1 {
		t.Errorf("corpse.looted fired %d times, want 1", looted)
	}
}
