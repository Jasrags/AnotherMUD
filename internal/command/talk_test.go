package command_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// talkRig builds a room with one NPC (template id "core:master") placed
// in it, an actor co-located, and a quest service the caller seeds.
func talkRig(t *testing.T, svc *quest.Service) (*command.Context, *testActor, *entities.Store) {
	t.Helper()
	room := &world.Room{ID: "x:1", Name: "Square"}
	store := entities.NewStore()
	place := entities.NewPlacement()
	npc, err := store.SpawnMob(&mob.Template{
		ID: "core:master", Name: "a training master", Type: "npc",
		Keywords: []string{"master", "trainer"},
	})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	place.Place(npc.ID(), room.ID)
	a := newNamedTestActor("Hero", "p1", room)
	return &command.Context{Actor: a, Quests: svc, Items: store, Placement: place}, a, store
}

func offerSvc(t *testing.T) *quest.Service {
	t.Helper()
	reg := quest.NewRegistry()
	if err := reg.Register(&quest.Definition{
		ID: "core:patrol", Name: "Gate Patrol", Giver: "core:master", TurnIn: true,
		Offer:       "The gate needs watching.",
		Abandonable: true,
		Stages: []quest.Stage{{ID: "s", Objectives: []quest.Objective{
			{ID: "s-visit-0", Type: "visit", Target: "core:gate", Count: 1, Description: "Visit the gate."},
		}}},
		Reward: quest.Reward{XP: 50},
	}); err != nil {
		t.Fatal(err)
	}
	return quest.NewService(quest.Config{Registry: reg})
}

func containsStr(s []string, want string) bool {
	return slices.Contains(s, want)
}

func TestTalk_ListsOffers(t *testing.T) {
	svc := offerSvc(t)
	c, a, _ := talkRig(t, svc)
	c.Args = []string{"master"}
	if err := command.TalkHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	out := a.lastLine()
	if !strings.Contains(out, "Gate Patrol") || !strings.Contains(out, "The gate needs watching.") || !strings.Contains(out, "accept Gate Patrol") {
		t.Errorf("offer listing wrong:\n%s", out)
	}
}

func TestTalk_TurnsInReadyQuest(t *testing.T) {
	svc := offerSvc(t)
	c, a, _ := talkRig(t, svc)

	// Accept and complete the objective so the quest parks awaiting turn-in.
	if r := svc.Accept(a, "core:patrol", true); r.Status != quest.Accepted {
		t.Fatalf("accept: %v", r.Status)
	}
	svc.AdvanceObjective("p1", "core:patrol", "s-visit-0", 1)
	if snap := svc.Snapshot("p1"); snap == nil || len(snap.Active) != 1 || !snap.Active[0].AwaitingTurnIn {
		t.Fatalf("quest should be awaiting turn-in: %+v", snap)
	}

	// Talking to the giver claims it (no offers left, so a thanks line).
	c.Args = []string{"master"}
	if err := command.TalkHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	snap := svc.Snapshot("p1")
	if snap == nil || len(snap.Active) != 0 || !containsStr(snap.Completed, "core:patrol") {
		t.Errorf("quest should be completed after turn-in: %+v", snap)
	}
	if !strings.Contains(a.lastLine(), "thanks you") {
		t.Errorf("turn-in reply = %q", a.lastLine())
	}
}

func TestTalk_NoNPC(t *testing.T) {
	svc := offerSvc(t)
	c, a, _ := talkRig(t, svc)
	c.Args = []string{"goblin"}
	if err := command.TalkHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.lastLine(), "no \"goblin\" here") {
		t.Errorf("missing-npc reply = %q", a.lastLine())
	}
}

func TestTalk_NothingToOffer(t *testing.T) {
	svc := offerSvc(t)
	c, a, _ := talkRig(t, svc)
	// Already completed + non-repeatable → no offer, nothing ready.
	if r := svc.Accept(a, "core:patrol", true); r.Status != quest.Accepted {
		t.Fatalf("accept: %v", r.Status)
	}
	svc.AdvanceObjective("p1", "core:patrol", "s-visit-0", 1)
	_ = svc.TurnIn(a, "core:patrol") // claim it
	c.Args = []string{"master"}
	if err := command.TalkHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.lastLine(), "nothing for you") {
		t.Errorf("nothing-to-offer reply = %q", a.lastLine())
	}
}
