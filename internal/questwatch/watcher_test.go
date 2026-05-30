package questwatch

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// player satisfies quest.Player for accepting a quest in tests.
type player struct{ id string }

func (p player) EntityID() string { return p.id }
func (player) Level(string) int   { return 1 }
func (player) Class() string      { return "" }
func (player) SetClass(string)    {}
func (player) SetRace(string)     {}

func allTypesQuest() *quest.Definition {
	return &quest.Definition{
		ID: "q", Abandonable: true,
		Stages: []quest.Stage{{ID: "s", Objectives: []quest.Objective{
			{ID: "s-kill-0", Type: "kill", Target: "core:rat", Count: 1},
			{ID: "s-collect-1", Type: "collect", Target: "core:gem", Count: 1},
			{ID: "s-deliver-2", Type: "deliver", Target: "core:letter", NPC: "core:mayor", Count: 1},
			{ID: "s-visit-3", Type: "visit", Target: "core:home", Count: 1},
		}}},
	}
}

func setup(t *testing.T) (*quest.Service, *entities.Store, *Watcher) {
	t.Helper()
	reg := quest.NewRegistry()
	if err := reg.Register(allTypesQuest()); err != nil {
		t.Fatal(err)
	}
	svc := quest.NewService(quest.Config{Registry: reg})
	svc.Accept(player{id: "p1"}, "q", false)
	store := entities.NewStore()
	return svc, store, New(svc, store)
}

func progressOf(t *testing.T, svc *quest.Service, objID string) int {
	t.Helper()
	snap := svc.Snapshot("p1")
	for _, o := range snap.Active[0].Objectives {
		if o.ObjectiveID == objID {
			return o.Current
		}
	}
	t.Fatalf("objective %q not found", objID)
	return 0
}

func TestKillAdvances(t *testing.T) {
	svc, _, w := setup(t)
	// KillerID arrives combat-prefixed ("player:<id>") from the real
	// death pipeline; the watcher must strip it to the bare player id the
	// quest state is keyed by.
	w.onMobKilled(context.Background(), eventbus.MobKilled{KillerID: "player:p1", TemplateID: "core:rat"})
	if progressOf(t, svc, "s-kill-0") != 1 {
		t.Error("kill did not advance (prefixed killer id not normalized?)")
	}
	// wrong template doesn't advance
	w.onMobKilled(context.Background(), eventbus.MobKilled{KillerID: "p1", TemplateID: "core:wolf"})
	if progressOf(t, svc, "s-kill-0") != 1 {
		t.Error("non-matching kill advanced")
	}
}

func TestVisitAdvances(t *testing.T) {
	svc, _, w := setup(t)
	w.onPlayerMoved(context.Background(), eventbus.PlayerMoved{PlayerID: "p1", To: "core:home"})
	if progressOf(t, svc, "s-visit-3") != 1 {
		t.Error("visit did not advance")
	}
}

func TestCollectResolvesTemplate(t *testing.T) {
	svc, store, w := setup(t)
	gem, _ := store.Spawn(&item.Template{ID: "core:gem", Name: "Gem", Type: "treasure"})
	w.onItemPickedUp(context.Background(), eventbus.ItemPickedUp{HolderID: "p1", ItemID: gem.ID()})
	if progressOf(t, svc, "s-collect-1") != 1 {
		t.Error("collect did not advance via template resolution")
	}
	// picking up an unrelated item doesn't advance
	rock, _ := store.Spawn(&item.Template{ID: "core:rock", Name: "Rock", Type: "junk"})
	w.onItemPickedUp(context.Background(), eventbus.ItemPickedUp{HolderID: "p1", ItemID: rock.ID()})
	if progressOf(t, svc, "s-collect-1") != 1 {
		t.Error("non-matching collect advanced")
	}
}

func TestDeliverResolvesRecipientTemplate(t *testing.T) {
	svc, store, w := setup(t)
	smith, _ := store.SpawnMob(&mob.Template{ID: "core:smith", Name: "Smith", Type: "npc"})
	mayor, _ := store.SpawnMob(&mob.Template{ID: "core:mayor", Name: "Mayor", Type: "npc"})

	// right item, wrong recipient (smith, not mayor) → no advance.
	w.onItemGiven(context.Background(), eventbus.ItemGiven{
		GiverID: "p1", TemplateID: "core:letter", RecipientID: smith.ID(),
	})
	if progressOf(t, svc, "s-deliver-2") != 0 {
		t.Error("deliver advanced with wrong recipient")
	}

	// item target + recipient npc both match → advance.
	w.onItemGiven(context.Background(), eventbus.ItemGiven{
		GiverID: "p1", TemplateID: "core:letter", RecipientID: mayor.ID(),
	})
	if progressOf(t, svc, "s-deliver-2") != 1 {
		t.Error("deliver did not advance")
	}
}

func TestMissingSourceIgnored(t *testing.T) {
	svc, _, w := setup(t)
	// empty source ids must be ignored without panic or advance
	w.onMobKilled(context.Background(), eventbus.MobKilled{TemplateID: "core:rat"})
	w.onPlayerMoved(context.Background(), eventbus.PlayerMoved{To: "core:home"})
	w.onItemPickedUp(context.Background(), eventbus.ItemPickedUp{ItemID: "x"})
	w.onItemGiven(context.Background(), eventbus.ItemGiven{TemplateID: "core:letter"})
	snap := svc.Snapshot("p1")
	for _, o := range snap.Active[0].Objectives {
		if o.Current != 0 {
			t.Errorf("objective %q advanced from a sourceless event", o.ObjectiveID)
		}
	}
}

func TestMissingEntityTolerated(t *testing.T) {
	svc, _, w := setup(t)
	// ItemID/RecipientID that don't resolve → no advance, no panic
	w.onItemPickedUp(context.Background(), eventbus.ItemPickedUp{HolderID: "p1", ItemID: "ghost"})
	w.onItemGiven(context.Background(), eventbus.ItemGiven{GiverID: "p1", TemplateID: "core:letter", RecipientID: "ghost"})
	if progressOf(t, svc, "s-collect-1") != 0 || progressOf(t, svc, "s-deliver-2") != 0 {
		t.Error("missing entity should not advance")
	}
}

func TestSubscribeRoutesThroughBus(t *testing.T) {
	svc, store, w := setup(t)
	bus := eventbus.New()
	w.Subscribe(bus)
	bus.Publish(context.Background(), eventbus.MobKilled{KillerID: "p1", TemplateID: "core:rat"})
	if progressOf(t, svc, "s-kill-0") != 1 {
		t.Error("bus-published kill did not advance through subscription")
	}
	_ = store
}

func TestQuestGrantOnPickup(t *testing.T) {
	reg := quest.NewRegistry()
	if err := reg.Register(&quest.Definition{
		ID: "core:scroll-quest", Name: "Scroll Quest", Abandonable: true,
		Stages: []quest.Stage{{ID: "s", Objectives: []quest.Objective{{Type: "visit", Target: "core:x", Count: 1}}}},
	}); err != nil {
		t.Fatal(err)
	}
	svc := quest.NewService(quest.Config{Registry: reg})
	store := entities.NewStore()
	w := New(svc, store)
	w.SetItemGrant(func(id string) (quest.Player, bool) {
		if id != "p1" {
			return nil, false
		}
		return player{id: id}, true
	})

	// quest_grant uses a bare id; ResolveID maps it to the namespaced id.
	scroll, _ := store.Spawn(&item.Template{
		ID: "core:scroll", Name: "a scroll", Type: "item",
		Properties: map[string]any{"quest_grant": "scroll-quest"},
	})
	w.onItemPickedUp(context.Background(), eventbus.ItemPickedUp{HolderID: "p1", ItemID: scroll.ID()})

	snap := svc.Snapshot("p1")
	if snap == nil || len(snap.Active) != 1 || snap.Active[0].QuestID != "core:scroll-quest" {
		t.Errorf("quest_grant did not accept the quest: %+v", snap)
	}
}

func TestQuestGrantNoResolverNoop(t *testing.T) {
	svc, store, w := setup(t) // no SetItemGrant called
	scroll, _ := store.Spawn(&item.Template{
		ID: "core:scroll", Name: "a scroll", Type: "item",
		Properties: map[string]any{"quest_grant": "q"},
	})
	// picking up a plain item (no collect objective for it) with no
	// resolver must not panic or grant anything.
	before := svc.Snapshot("p1")
	n := 0
	if before != nil {
		n = len(before.Active)
	}
	w.onItemPickedUp(context.Background(), eventbus.ItemPickedUp{HolderID: "p1", ItemID: scroll.ID()})
	after := svc.Snapshot("p1")
	if after != nil && len(after.Active) != n {
		t.Error("quest_grant fired without a resolver")
	}
}

// TestQuestGrantOnRoomEntry pins the M14.6 room-side variant: a
// PlayerMoved event into a room whose quest_grant resolver returns
// a quest id auto-accepts that quest for the mover.
func TestQuestGrantOnRoomEntry(t *testing.T) {
	reg := quest.NewRegistry()
	if err := reg.Register(&quest.Definition{
		ID: "core:village-welcome", Name: "Welcome", Abandonable: true,
		Stages: []quest.Stage{{ID: "s", Objectives: []quest.Objective{{Type: "visit", Target: "core:x", Count: 1}}}},
	}); err != nil {
		t.Fatal(err)
	}
	svc := quest.NewService(quest.Config{Registry: reg})
	w := New(svc, entities.NewStore())
	w.SetItemGrant(func(id string) (quest.Player, bool) {
		if id != "p1" {
			return nil, false
		}
		return player{id: id}, true
	})
	w.SetRoomGrant(func(rid world.RoomID) string {
		if rid == "core:town-square" {
			return "village-welcome" // bare id resolved via ResolveID
		}
		return ""
	})

	w.onPlayerMoved(context.Background(), eventbus.PlayerMoved{
		PlayerID: "p1",
		From:     "core:somewhere-else",
		To:       "core:town-square",
	})

	snap := svc.Snapshot("p1")
	if snap == nil || len(snap.Active) != 1 || snap.Active[0].QuestID != "core:village-welcome" {
		t.Errorf("room quest_grant did not accept: %+v", snap)
	}
}

// TestQuestGrantOnRoomEntryNoResolverNoop confirms the room-side
// path is dormant without both resolvers wired.
func TestQuestGrantOnRoomEntryNoResolverNoop(t *testing.T) {
	svc, _, w := setup(t) // no SetItemGrant, no SetRoomGrant
	// Just SetRoomGrant without item grant: player resolver missing
	// so the grant path drops silently.
	w.SetRoomGrant(func(world.RoomID) string { return "q" })

	w.onPlayerMoved(context.Background(), eventbus.PlayerMoved{
		PlayerID: "p1", From: "core:a", To: "core:b",
	})
	snap := svc.Snapshot("p1")
	if snap == nil || len(snap.Active) != 1 {
		t.Errorf("snap = %+v", snap)
	}
}

// TestQuestGrantOnRoomEntryEmptyPropertyNoop confirms moving into a
// room whose resolver returns "" is a no-op.
func TestQuestGrantOnRoomEntryEmptyPropertyNoop(t *testing.T) {
	svc, _, w := setup(t)
	w.SetItemGrant(func(id string) (quest.Player, bool) {
		return player{id: id}, true
	})
	w.SetRoomGrant(func(world.RoomID) string { return "" }) // no grant on any room

	before := svc.Snapshot("p1")
	beforeN := len(before.Active)

	w.onPlayerMoved(context.Background(), eventbus.PlayerMoved{
		PlayerID: "p1", From: "core:a", To: "core:b",
	})
	after := svc.Snapshot("p1")
	if len(after.Active) != beforeN {
		t.Errorf("empty room grant changed active count: %d → %d", beforeN, len(after.Active))
	}
}

// TestQuestGrantOnRoomEntryUnknownQuestIDNoop confirms a room
// resolver returning a string that doesn't resolve to any
// registered quest is a no-op.
func TestQuestGrantOnRoomEntryUnknownQuestIDNoop(t *testing.T) {
	svc, _, w := setup(t)
	w.SetItemGrant(func(id string) (quest.Player, bool) {
		return player{id: id}, true
	})
	w.SetRoomGrant(func(world.RoomID) string { return "bogus-quest" })

	beforeN := len(svc.Snapshot("p1").Active)
	w.onPlayerMoved(context.Background(), eventbus.PlayerMoved{
		PlayerID: "p1", From: "core:a", To: "core:b",
	})
	if got := len(svc.Snapshot("p1").Active); got != beforeN {
		t.Errorf("unknown quest id should be a no-op; active %d → %d", beforeN, got)
	}
}
