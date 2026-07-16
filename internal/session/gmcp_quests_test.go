package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/quest"
)

// questFormService wires a quest.Service over a registry holding one abandonable
// side-quest ("sw:rat-cellar") with a single stage of two objectives.
func questFormService(t *testing.T) *quest.Service {
	t.Helper()
	reg := quest.NewRegistry()
	if err := reg.Register(&quest.Definition{
		ID:             "sw:rat-cellar",
		Name:           "Clear the Cellar",
		Classification: "side",
		Abandonable:    true,
		Stages: []quest.Stage{{
			ID:          "s1",
			Description: "Deal with the rats in the cellar.",
			Hint:        "Try the trapdoor behind the bar.",
			Objectives: []quest.Objective{
				{ID: "reach", Type: "visit", Description: "reach the cellar", Count: 1},
				{ID: "kill", Type: "kill", Description: "kill cellar rats", Count: 5},
			},
		}},
	}); err != nil {
		t.Fatalf("register quest: %v", err)
	}
	return quest.NewService(quest.Config{Registry: reg})
}

// questFrames decodes the fake conn's Char.Quests frames.
func questFrames(t *testing.T, fc *gmcpFakeConn) []gmcp.CharQuests {
	t.Helper()
	raw := fc.framesSnapshot()
	out := make([]gmcp.CharQuests, 0, len(raw))
	for _, f := range raw {
		if f.pkg != gmcp.PackageCharQuests {
			continue
		}
		var cq gmcp.CharQuests
		if err := json.Unmarshal(f.payload, &cq); err != nil {
			t.Fatalf("payload unmarshal: %v (raw %s)", err, f.payload)
		}
		out = append(out, cq)
	}
	return out
}

func TestFlushGmcpQuests_NilServiceNoOp(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)
	a.flushGmcpQuests(context.Background(), nil)
	if got := len(questFrames(t, fc)); got != 0 {
		t.Errorf("nil quest service emitted %d frames, want 0", got)
	}
}

func TestFlushGmcpQuests_EmptyJournalForNoQuests(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	svc := questFormService(t)
	fc.setActive(true)

	a.flushGmcpQuests(context.Background(), svc)
	frames := questFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("first flush sent %d frames, want 1", len(frames))
	}
	if len(frames[0].Quests) != 0 {
		t.Errorf("a player with no quests should get an empty journal: %+v", frames[0])
	}
}

func TestFlushGmcpQuests_ActiveQuestShape(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	svc := questFormService(t)
	if res := svc.Accept(a, "sw:rat-cellar", true); res.Status != quest.Accepted {
		t.Fatalf("accept status = %v, want Accepted", res.Status)
	}
	// Advance the first objective to done so the frame carries a mixed set.
	svc.AdvanceObjective(a.PlayerID(), "sw:rat-cellar", "reach", 1)
	fc.setActive(true)

	a.flushGmcpQuests(context.Background(), svc)
	frames := questFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("first flush sent %d frames, want 1", len(frames))
	}
	q := frames[0].Quests
	if len(q) != 1 {
		t.Fatalf("journal entries = %d, want 1: %+v", len(q), q)
	}
	e := q[0]
	if e.ID != "sw:rat-cellar" || e.Name != "Clear the Cellar" || e.Classification != "side" {
		t.Errorf("entry identity = %+v, want sw:rat-cellar / Clear the Cellar / side", e)
	}
	if e.Stage != "Deal with the rats in the cellar." || e.Hint != "Try the trapdoor behind the bar." {
		t.Errorf("stage/hint = %q / %q, want the stage description + hint", e.Stage, e.Hint)
	}
	if !e.Abandonable || e.AbandonCmd != "abandon sw:rat-cellar" {
		t.Errorf("abandon = %v / %q, want true / abandon sw:rat-cellar", e.Abandonable, e.AbandonCmd)
	}
	if len(e.Objectives) != 2 {
		t.Fatalf("objectives = %d, want 2: %+v", len(e.Objectives), e.Objectives)
	}
	if o := e.Objectives[0]; o.Desc != "reach the cellar" || o.Current != 1 || o.Required != 1 || !o.Complete {
		t.Errorf("objective[0] = %+v, want reach the cellar 1/1 complete", o)
	}
	if o := e.Objectives[1]; o.Desc != "kill cellar rats" || o.Current != 0 || o.Required != 5 || o.Complete {
		t.Errorf("objective[1] = %+v, want kill cellar rats 0/5 incomplete", o)
	}
}

func TestFlushGmcpQuests_NoRedundantSendThenResendOnReset(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	svc := questFormService(t)
	svc.Accept(a, "sw:rat-cellar", true)
	fc.setActive(true)

	a.flushGmcpQuests(context.Background(), svc) // baseline
	pre := len(questFrames(t, fc))
	a.flushGmcpQuests(context.Background(), svc)
	a.flushGmcpQuests(context.Background(), svc)
	if got := len(questFrames(t, fc)); got != pre {
		t.Errorf("redundant flushes added %d frames, want 0", got-pre)
	}

	a.resetGmcpItemsShadow() // clears the quests shadow too (reattach seam)
	a.flushGmcpQuests(context.Background(), svc)
	if got := len(questFrames(t, fc)) - pre; got != 1 {
		t.Errorf("post-reset added %d frames, want 1", got)
	}
}
