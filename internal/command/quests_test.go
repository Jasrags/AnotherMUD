package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/quest"
)

// testActor satisfies quest.Player via these methods (EntityID + the
// prereq/unlock surface). Defined here since only the quest verbs need
// them.
func (a *testActor) EntityID() string  { return a.PlayerID() }
func (a *testActor) Level(string) int  { return 1 }
func (a *testActor) Class() string     { return "" }
func (a *testActor) SetClass(s string) {}
func (a *testActor) SetRace(s string)  {}

func questService(t *testing.T) *quest.Service {
	t.Helper()
	reg := quest.NewRegistry()
	err := reg.Register(&quest.Definition{
		ID: "core:patrol", Name: "Gate Patrol", Classification: "side", Abandonable: true,
		Stages: []quest.Stage{{ID: "s", Objectives: []quest.Objective{
			{Type: "visit", Target: "core:gate", Count: 1, Description: "Visit the gate."},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return quest.NewService(quest.Config{Registry: reg})
}

func acceptCtx(svc *quest.Service, args ...string) (*command.Context, *testActor) {
	a := newNamedTestActor("Hero", "p1", nil)
	return &command.Context{Actor: a, Quests: svc, Args: args}, a
}

func TestAcceptByBareIDAndName(t *testing.T) {
	svc := questService(t)
	// bare id resolves to the namespaced quest
	c, a := acceptCtx(svc, "patrol")
	if err := command.AcceptHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.lastLine(), "Gate Patrol") {
		t.Errorf("accept by bare id = %q", a.lastLine())
	}
	// already active
	c2, a2 := acceptCtx(svc, "patrol")
	c2.Actor = a // same player
	_ = a2
	if err := command.AcceptHandler(context.Background(), c2); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.lastLine(), "already on that quest") {
		t.Errorf("re-accept = %q", a.lastLine())
	}
}

func TestAcceptUnknownQuest(t *testing.T) {
	svc := questService(t)
	c, a := acceptCtx(svc, "nonsense")
	_ = command.AcceptHandler(context.Background(), c)
	if !strings.Contains(a.lastLine(), "No quest matches") {
		t.Errorf("unknown accept = %q", a.lastLine())
	}
}

func TestAcceptNoArg(t *testing.T) {
	svc := questService(t)
	c, a := acceptCtx(svc)
	_ = command.AcceptHandler(context.Background(), c)
	if !strings.Contains(a.lastLine(), "Accept which quest") {
		t.Errorf("no-arg accept = %q", a.lastLine())
	}
}

func TestQuestsJournal(t *testing.T) {
	svc := questService(t)
	// empty
	c, a := acceptCtx(svc)
	_ = command.QuestsHandler(context.Background(), c)
	if !strings.Contains(a.lastLine(), "no active quests") {
		t.Errorf("empty journal = %q", a.lastLine())
	}
	// after accepting, journal shows the quest + objective
	ac, _ := acceptCtx(svc, "patrol")
	ac.Actor = a
	_ = command.AcceptHandler(context.Background(), ac)
	_ = command.QuestsHandler(context.Background(), &command.Context{Actor: a, Quests: svc})
	out := a.lastLine()
	for _, want := range []string{"Quest Journal", "Gate Patrol", "Visit the gate.", "(0/1)"} {
		if !strings.Contains(out, want) {
			t.Errorf("journal missing %q in:\n%s", want, out)
		}
	}
}

func TestAbandonHandler(t *testing.T) {
	svc := questService(t)
	a := newNamedTestActor("Hero", "p1", nil)
	_ = command.AcceptHandler(context.Background(), &command.Context{Actor: a, Quests: svc, Args: []string{"patrol"}})
	// not on a different quest
	_ = command.AbandonHandler(context.Background(), &command.Context{Actor: a, Quests: svc, Args: []string{"ghost"}})
	if !strings.Contains(a.lastLine(), "No quest matches") {
		t.Errorf("abandon unknown = %q", a.lastLine())
	}
	// abandon the active one
	_ = command.AbandonHandler(context.Background(), &command.Context{Actor: a, Quests: svc, Args: []string{"patrol"}})
	if !strings.Contains(a.lastLine(), "abandon Gate Patrol") {
		t.Errorf("abandon = %q", a.lastLine())
	}
	// now not on it
	_ = command.AbandonHandler(context.Background(), &command.Context{Actor: a, Quests: svc, Args: []string{"patrol"}})
	if !strings.Contains(a.lastLine(), "not on that quest") {
		t.Errorf("abandon-again = %q", a.lastLine())
	}
}

func TestQuestVerbsNilService(t *testing.T) {
	a := newNamedTestActor("Hero", "p1", nil)
	for _, h := range []command.Handler{command.AcceptHandler, command.AbandonHandler, command.QuestsHandler} {
		_ = h(context.Background(), &command.Context{Actor: a, Args: []string{"x"}})
		if !strings.Contains(a.lastLine(), "not available") {
			t.Errorf("nil-service verb = %q", a.lastLine())
		}
	}
}
