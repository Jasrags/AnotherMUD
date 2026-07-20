package corpse

import (
	"context"
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/loot"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

const (
	testRoom = world.RoomID("core:town-square")
	mobID    = entities.EntityID("mob-entity-1")
)

// fixedRoller returns a constant for IntN — enough for deterministic
// coin rolls in these tests.
type fixedRoller int

func (f fixedRoller) IntN(int) int { return int(f) }

// harness wires a Service against real entities/bus and returns the
// pieces a test inspects.
type harness struct {
	svc       *Service
	store     *entities.Store
	contents  *entities.Contents
	placement *entities.Placement
	bus       *eventbus.Bus
	mobs      *mob.Templates
	loot      *loot.Registry
	tick      uint64
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		store:     entities.NewStore(),
		contents:  entities.NewContents(),
		placement: entities.NewPlacement(),
		bus:       eventbus.New(),
		mobs:      mob.NewTemplates(),
		loot:      loot.NewRegistry(),
		tick:      42,
	}
	h.svc = New(Config{
		Store:     h.store,
		Contents:  h.contents,
		Placement: h.placement,
		Bus:       h.bus,
		Mobs:      h.mobs,
		Loot:      h.loot,
		Roller:    fixedRoller(0),
		Now:       func() uint64 { return h.tick },
	})
	return h
}

// spawnItemInMob mints a real item and files it under the mob's id.
func (h *harness) spawnItemInMob(t *testing.T, tplID string) entities.EntityID {
	t.Helper()
	it, err := h.store.Spawn(&item.Template{ID: item.TemplateID(tplID), Name: tplID, Type: "misc"})
	if err != nil {
		t.Fatalf("spawn item: %v", err)
	}
	h.contents.Put(mobID, it.ID())
	return it.ID()
}

func killedEvent(killer string) eventbus.MobKilled {
	return eventbus.MobKilled{
		MobID:      mobID,
		MobName:    "a village guard",
		TemplateID: "core:village-guard",
		KillerID:   killer,
		KillerName: "Bob",
		RoomID:     testRoom,
	}
}

// findCorpse returns the single corpse-tagged entity in the room, or
// fails.
func (h *harness) findCorpse(t *testing.T) *entities.ItemInstance {
	t.Helper()
	for _, id := range h.placement.InRoom(testRoom) {
		e, ok := h.store.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		if slices.Contains(it.Tags(), TagCorpse) {
			return it
		}
	}
	t.Fatal("no corpse found in room")
	return nil
}

// grouping.md §5: a party kill's corpse owner set is the killer plus their
// party (deduped, prefixed combatant ids), so any member may loot the kill.
func TestCreateOnDeath_OwnerSetIncludesParty(t *testing.T) {
	h := newHarness(t)
	// Rebuild the service with an OwnerSet that returns the killer's party —
	// including the killer themselves (as the real LootOwners does), to prove dedup.
	h.svc = New(Config{
		Store: h.store, Contents: h.contents, Placement: h.placement, Bus: h.bus,
		Mobs: h.mobs, Loot: h.loot, Roller: fixedRoller(0), Now: func() uint64 { return h.tick },
		OwnerSet: func(killerID string) []string {
			if killerID != "player:bob" {
				return nil
			}
			return []string{"player:bob", "player:amy"} // killer (dup) + a member
		},
	})
	h.spawnItemInMob(t, "trail-ration") // ensure a corpse forms

	h.svc.CreateOnDeath(context.Background(), killedEvent("player:bob"))

	owners := Owners(h.findCorpse(t))
	slices.Sort(owners)
	if !slices.Equal(owners, []string{"player:amy", "player:bob"}) {
		t.Fatalf("owner set = %v, want [player:amy player:bob] (killer + party, deduped)", owners)
	}
}

// grouping.md §9 master-looter: the OwnerSet hook returns just the designated
// master, so the corpse is owned by the master alone — the killer is excluded
// (they may not loot their own kill under this policy).
func TestCreateOnDeath_MasterLooterOwnerSetExcludesKiller(t *testing.T) {
	h := newHarness(t)
	h.svc = New(Config{
		Store: h.store, Contents: h.contents, Placement: h.placement, Bus: h.bus,
		Mobs: h.mobs, Loot: h.loot, Roller: fixedRoller(0), Now: func() uint64 { return h.tick },
		OwnerSet: func(killerID string) []string {
			return []string{"player:amy"} // master-looter: only the master, not the killer
		},
	})
	h.spawnItemInMob(t, "trail-ration")

	h.svc.CreateOnDeath(context.Background(), killedEvent("player:bob")) // bob killed it

	owners := Owners(h.findCorpse(t))
	if !slices.Equal(owners, []string{"player:amy"}) {
		t.Fatalf("owner set = %v, want [player:amy] (master only, killer excluded)", owners)
	}
}

// A nil OwnerSet hook (or one returning nothing) falls back to the solo killer.
func TestCreateOnDeath_NilOwnerSetFallsBackToKiller(t *testing.T) {
	h := newHarness(t)
	h.svc = New(Config{
		Store: h.store, Contents: h.contents, Placement: h.placement, Bus: h.bus,
		Mobs: h.mobs, Loot: h.loot, Roller: fixedRoller(0), Now: func() uint64 { return h.tick },
		OwnerSet: func(string) []string { return nil },
	})
	h.spawnItemInMob(t, "trail-ration")

	h.svc.CreateOnDeath(context.Background(), killedEvent("player:bob"))

	if owners := Owners(h.findCorpse(t)); !slices.Equal(owners, []string{"player:bob"}) {
		t.Fatalf("owner set = %v, want [player:bob] (solo fallback)", owners)
	}
}

func TestCreateOnDeath_MovesContentsIntoCorpse(t *testing.T) {
	h := newHarness(t)
	i1 := h.spawnItemInMob(t, "trail-ration")
	i2 := h.spawnItemInMob(t, "healing-draught")

	var created *eventbus.CorpseCreated
	h.bus.Subscribe(eventbus.EventCorpseCreated, func(_ context.Context, ev eventbus.Event) {
		e := ev.(eventbus.CorpseCreated)
		created = &e
	})

	h.svc.CreateOnDeath(context.Background(), killedEvent("player:bob"))

	corpse := h.findCorpse(t)
	// Contents moved off the mob, onto the corpse.
	if got := h.contents.In(mobID); len(got) != 0 {
		t.Fatalf("mob still holds %v", got)
	}
	inCorpse := h.contents.In(corpse.ID())
	if len(inCorpse) != 2 {
		t.Fatalf("corpse holds %d items, want 2", len(inCorpse))
	}
	// Same instance identities, preserved.
	got := map[entities.EntityID]bool{}
	for _, id := range inCorpse {
		got[id] = true
	}
	if !got[i1] || !got[i2] {
		t.Fatalf("corpse contents %v missing %s/%s", inCorpse, i1, i2)
	}
	// Event payload.
	if created == nil || created.ItemCount != 2 || created.CorpseID != corpse.ID() {
		t.Fatalf("corpse.created = %+v", created)
	}
}

func TestCreateOnDeath_RecordsMetadata(t *testing.T) {
	h := newHarness(t)
	h.spawnItemInMob(t, "trail-ration")
	h.svc.CreateOnDeath(context.Background(), killedEvent("player:bob"))

	corpse := h.findCorpse(t)
	if corpse.Type() != entities.ContainerType {
		t.Errorf("corpse type = %q, want container", corpse.Type())
	}
	if corpse.Name() != "the corpse of a village guard" {
		t.Errorf("corpse name = %q", corpse.Name())
	}
	if v, _ := corpse.Property(PropKiller); v != "player:bob" {
		t.Errorf("killer = %v", v)
	}
	if v, _ := corpse.Property(PropCreatedTick); v != uint64(42) {
		t.Errorf("created tick = %v (%T), want 42", v, v)
	}
	owners, _ := corpse.Property(PropOwners)
	if got, ok := owners.([]string); !ok || len(got) != 1 || got[0] != "player:bob" {
		t.Errorf("owners = %v", owners)
	}
	// no_get + no_put so the corpse can't be taken or used as storage.
	tags := map[string]bool{}
	for _, tg := range corpse.Tags() {
		tags[tg] = true
	}
	if !tags[TagNoGet] || !tags[TagNoPut] {
		t.Errorf("corpse tags = %v", corpse.Tags())
	}
}

func TestCreateOnDeath_EmptyKillerOpenOwnerSet(t *testing.T) {
	h := newHarness(t)
	h.spawnItemInMob(t, "trail-ration")
	h.svc.CreateOnDeath(context.Background(), killedEvent(""))

	corpse := h.findCorpse(t)
	owners, _ := corpse.Property(PropOwners)
	if got, ok := owners.([]string); !ok || len(got) != 0 {
		t.Errorf("empty killer should yield empty owner set, got %v", owners)
	}
}

func TestCreateOnDeath_NoItemsNoCoinsNoCorpse(t *testing.T) {
	h := newHarness(t)
	h.svc.CreateOnDeath(context.Background(), killedEvent("player:bob"))
	if got := h.placement.InRoom(testRoom); len(got) != 0 {
		t.Fatalf("expected no corpse, room holds %v", got)
	}
}

func TestCreateOnDeath_CoinsOnlyStillCreatesCorpse(t *testing.T) {
	h := newHarness(t)
	// Mob template + loot table with a fixed coin block; no items.
	h.mobs.Add(&mob.Template{ID: "core:village-guard", Name: "a village guard", Behavior: "stationary", LootTable: "core:guard-loot"})
	_ = h.loot.Register(&loot.Table{ID: "core:guard-loot", Coin: &loot.CoinBlock{Min: 5, Max: 5}})

	h.svc.CreateOnDeath(context.Background(), killedEvent("player:bob"))

	corpse := h.findCorpse(t)
	if v, _ := corpse.Property(PropCoins); v != 5 {
		t.Fatalf("corpse coins = %v, want 5", v)
	}
	if got := h.contents.In(corpse.ID()); len(got) != 0 {
		t.Fatalf("coins-only corpse should hold no items, got %v", got)
	}
}

func TestCreateOnDeath_CancelSuppressesCorpse(t *testing.T) {
	h := newHarness(t)
	i1 := h.spawnItemInMob(t, "trail-ration")
	h.bus.Subscribe(eventbus.EventCorpseCreating, func(_ context.Context, ev eventbus.Event) {
		ev.(*eventbus.CorpseCreating).Cancel()
	})

	h.svc.CreateOnDeath(context.Background(), killedEvent("player:bob"))

	if got := h.placement.InRoom(testRoom); len(got) != 0 {
		t.Fatalf("cancelled creation should leave no corpse, got %v", got)
	}
	// Contents stay on the mob (the death-cleanup path removes the mob).
	if got := h.contents.In(mobID); len(got) != 1 || got[0] != i1 {
		t.Fatalf("contents should remain on mob, got %v", got)
	}
}

func TestOnMobKilled_IgnoresOtherEvents(t *testing.T) {
	h := newHarness(t)
	// A non-MobKilled event must be a no-op (no panic, no corpse).
	h.svc.OnMobKilled(context.Background(), eventbus.MobSpawned{EntityID: mobID, RoomID: testRoom})
	if got := h.placement.InRoom(testRoom); len(got) != 0 {
		t.Fatalf("unexpected corpse from non-killed event: %v", got)
	}
}

// A mob robbed while helpless (TagLooted) drops no coins on death — the looter
// already took the purse. Combined with its already-emptied contents, it
// produces no corpse at all (the double-dip guard for the rob path).
func TestCreateOnDeath_LootedMobDropsNoCoinsOrCorpse(t *testing.T) {
	h := newHarness(t)
	h.mobs.Add(&mob.Template{ID: "core:village-guard", Name: "a village guard", LootTable: "core:guard-loot"})
	_ = h.loot.Register(&loot.Table{ID: "core:guard-loot", Coin: &loot.CoinBlock{Min: 5, Max: 5}})

	m, err := h.store.SpawnMob(&mob.Template{ID: "core:village-guard", Name: "a village guard", LootTable: "core:guard-loot"})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	m.AddTag(TagLooted) // robbed while down
	ev := eventbus.MobKilled{MobID: m.ID(), MobName: "a village guard", TemplateID: "core:village-guard", KillerID: "player:bob", RoomID: testRoom}
	h.svc.CreateOnDeath(context.Background(), ev)

	if got := h.placement.InRoom(testRoom); len(got) != 0 {
		t.Fatalf("a robbed mob should leave no corpse, room holds %v", got)
	}
}

// Control: the SAME mob NOT robbed drops its coin purse into a corpse — proving
// the skip above is the looted tag, not a broken loot table.
func TestCreateOnDeath_UnrobbedMobStillDropsCoins(t *testing.T) {
	h := newHarness(t)
	h.mobs.Add(&mob.Template{ID: "core:village-guard", Name: "a village guard", LootTable: "core:guard-loot"})
	_ = h.loot.Register(&loot.Table{ID: "core:guard-loot", Coin: &loot.CoinBlock{Min: 5, Max: 5}})

	m, err := h.store.SpawnMob(&mob.Template{ID: "core:village-guard", Name: "a village guard", LootTable: "core:guard-loot"})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	ev := eventbus.MobKilled{MobID: m.ID(), MobName: "a village guard", TemplateID: "core:village-guard", KillerID: "player:bob", RoomID: testRoom}
	h.svc.CreateOnDeath(context.Background(), ev)

	corpse := h.findCorpse(t)
	if v, _ := corpse.Property(PropCoins); v != 5 {
		t.Fatalf("unrobbed corpse coins = %v, want 5", v)
	}
}
