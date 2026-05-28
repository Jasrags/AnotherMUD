package questwatch

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/quest"
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
