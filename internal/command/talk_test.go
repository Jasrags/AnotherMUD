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

// TestTalk_DialogueNPCSpeaksIntro covers the onboarding fallback: a bare
// `talk`/`ask` at an NPC that has no quest to give or turn in, but DOES carry
// a `dialogue` property, speaks its intro (a `started`/`default` topic) instead
// of dead-ending on "nothing for you right now". This is what makes `ask rook`
// / `ask patch` helpful for a new player.
func TestTalk_DialogueNPCSpeaksIntro(t *testing.T) {
	svc := offerSvc(t) // its only quest is given by core:master, not core:mentor
	room := &world.Room{ID: "x:1", Name: "Square"}
	store := entities.NewStore()
	place := entities.NewPlacement()
	npc, err := store.SpawnMob(&mob.Template{
		ID: "core:mentor", Name: "Rook", Type: "npc",
		Keywords: []string{"rook", "mentor"},
		Properties: map[string]any{
			"dialogue": map[string]any{
				"started": "First run? Ask me about gear, chrome, or the streets.",
				"default": "I deal in work and advice, not small talk.",
			},
		},
	})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	place.Place(npc.ID(), room.ID)
	a := newNamedTestActor("Hero", "p1", room)
	c := &command.Context{Actor: a, Quests: svc, Items: store, Placement: place, Args: []string{"rook"}}

	if err := command.TalkHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	out := a.lastLine()
	if strings.Contains(out, "nothing for you") {
		t.Errorf("dialogue NPC should not dead-end; got %q", out)
	}
	if !strings.Contains(out, "Rook") || !strings.Contains(out, "says") || !strings.Contains(out, "Ask me about gear") {
		t.Errorf("want Rook's intro spoken; got %q", out)
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
