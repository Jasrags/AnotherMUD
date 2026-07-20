package session

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// newNotifierRig wires a questNotifier over a manager holding one online
// actor ("Alice"/p1), a registry with a single turn-in quest, and name
// resolvers for the giver and reward item.
func newNotifierRig(t *testing.T) (quest.EventSink, *fakeConn) {
	t.Helper()
	reg := quest.NewRegistry()
	if err := reg.Register(&quest.Definition{
		ID: "q", Name: "Gate Patrol", Giver: "core:master", TurnIn: true,
		Stages: []quest.Stage{
			{ID: "s0", Objectives: []quest.Objective{
				{ID: "s0-kill-0", Type: "kill", Target: "core:bandit", Count: 2, Description: "Slay the bandit"},
			}},
		},
		Reward: quest.Reward{XP: 100, Gold: 25, Items: []string{"core:healing-draught"}},
	}); err != nil {
		t.Fatal(err)
	}
	mgr := NewManager()
	a, fc := newFakeActor("c1", "p1", "acc1", "Alice", &world.Room{ID: "x:1"})
	mgr.Add(a)
	giver := func(tid string) string {
		if tid == "core:master" {
			return "a training master"
		}
		return ""
	}
	itemN := func(tid string) string {
		if tid == "core:healing-draught" {
			return "a healing draught"
		}
		return ""
	}
	factionN := func(id string) string {
		if id == "the-streets" {
			return "The Streets"
		}
		return ""
	}
	return NewQuestNotifier(mgr, reg, giver, itemN, factionN, economy.DefaultCurrency, nil), fc
}

func TestQuestNotifier_ObjectiveProgress(t *testing.T) {
	n, fc := newNotifierRig(t)
	n.ObjectiveAdvanced(quest.ObjectiveAdvancedEvent{
		PlayerID: "p1", QuestID: "q", ObjectiveID: "s0-kill-0", Current: 1, Required: 2,
	})
	out := strings.Join(fc.writes(), "\n")
	for _, want := range []string{"Gate Patrol", "Slay the bandit", "(1/2)"} {
		if !strings.Contains(out, want) {
			t.Errorf("objective message missing %q:\n%s", want, out)
		}
	}
}

func TestQuestNotifier_ReadyToTurnIn(t *testing.T) {
	n, fc := newNotifierRig(t)
	n.ReadyToTurnIn(quest.ReadyToTurnInEvent{PlayerID: "p1", QuestID: "q", Giver: "core:master"})
	out := strings.Join(fc.writes(), "\n")
	if !strings.Contains(out, "Gate Patrol complete") || !strings.Contains(out, "a training master") {
		t.Errorf("ready-to-turn-in message wrong:\n%s", out)
	}
}

func TestQuestNotifier_CompletionBannerListsRewards(t *testing.T) {
	n, fc := newNotifierRig(t)
	n.Completed(quest.CompletedEvent{
		PlayerID: "p1", QuestID: "q", XP: 100, Gold: 25,
		Items:      []string{"core:healing-draught"},
		Faction:    []quest.FactionReward{{Faction: "the-streets", Delta: 50}},
		Reputation: 10,
	})
	out := strings.Join(fc.writes(), "\n")
	for _, want := range []string{
		"QUEST COMPLETE",            // the prominent header rule
		"Gate Patrol",               // quest name
		"100 experience", "25 gold", // xp + currency-labeled gold
		"a healing draught",              // item reward
		"+ 50 standing with The Streets", // faction reward (was silently dropped before)
		"+ 10 renown",                    // reputation reward
	} {
		if !strings.Contains(out, want) {
			t.Errorf("completion banner missing %q:\n%s", want, out)
		}
	}
	// The old "Quest complete: <name>" one-liner is gone (name is now its own line).
	if strings.Contains(out, "Quest complete: Gate Patrol") {
		t.Errorf("banner still uses the old one-liner form:\n%s", out)
	}
}

// A quest that COSTS standing with a rival renders a signed "- N" line, and a
// zero-delta faction reward (author slip) shows nothing.
func TestQuestNotifier_CompletionBannerSignedAndZeroFaction(t *testing.T) {
	n, fc := newNotifierRig(t)
	n.Completed(quest.CompletedEvent{
		PlayerID: "p1", QuestID: "q", XP: 10,
		Faction: []quest.FactionReward{
			{Faction: "the-streets", Delta: -25}, // a cost
			{Faction: "the-streets", Delta: 0},   // slip → omitted
		},
		Reputation: -5,
	})
	out := strings.Join(fc.writes(), "\n")
	for _, want := range []string{"- 25 standing with The Streets", "- 5 renown"} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "+ 0 standing") || strings.Contains(out, "0 standing with") {
		t.Errorf("zero-delta faction reward rendered a line:\n%s", out)
	}
}

func TestQuestNotifier_StartedSilentInGame(t *testing.T) {
	// The accept command owns the acceptance banner; the notifier must not
	// re-write it on Started (would double-message).
	n, fc := newNotifierRig(t)
	n.Started(quest.StartedEvent{PlayerID: "p1", QuestID: "q", Banner: "ignored"})
	if got := fc.writes(); len(got) != 0 {
		t.Errorf("Started wrote in-game, want silent: %v", got)
	}
}

func TestQuestNotifier_OfflineRecipientNoPanic(t *testing.T) {
	// An event for a player with no live actor must be a silent no-op.
	n, fc := newNotifierRig(t)
	n.Completed(quest.CompletedEvent{PlayerID: "ghost", QuestID: "q", XP: 1})
	if got := fc.writes(); len(got) != 0 {
		t.Errorf("offline recipient should not touch the online actor: %v", got)
	}
}
