package command

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/quest"
)

// stubQuestPlayer is a minimal quest.Player for driving the production
// questScope adapter against a real quest.Service.
type stubQuestPlayer struct{ id string }

func (p stubQuestPlayer) EntityID() string { return p.id }
func (p stubQuestPlayer) Level(string) int { return 0 }
func (p stubQuestPlayer) Class() string    { return "" }
func (p stubQuestPlayer) SetClass(string)  {}
func (p stubQuestPlayer) SetRace(string)   {}

func questScopeDef(id, name, giver string, abandonable bool) *quest.Definition {
	return &quest.Definition{
		ID: id, Name: name, Giver: giver, Abandonable: abandonable,
		Stages: []quest.Stage{{ID: "s0", Objectives: []quest.Objective{
			{ID: "s0-kill-0", Type: "kill", Target: "core:rat"},
		}}},
	}
}

func refBareIDs(refs []QuestRef) map[string]bool {
	m := make(map[string]bool, len(refs))
	for _, r := range refs {
		m[r.BareID] = true
	}
	return m
}

// TestQuestScope_Production exercises the real questScope (not the
// completion fake): the bare-id mapping, the offered vs. active split, and
// the abandonable filter that keeps `abandon` from suggesting dead ends.
func TestQuestScope_Production(t *testing.T) {
	reg := quest.NewRegistry()
	if err := reg.Register(questScopeDef("core:gate-patrol", "Gate Patrol", "core:maerys", true)); err != nil {
		t.Fatalf("register gate-patrol: %v", err)
	}
	// Eternal Vow is NON-abandonable — abandon refuses it, so EnumerateActive
	// must omit it once accepted.
	if err := reg.Register(questScopeDef("core:eternal-vow", "Eternal Vow", "core:maerys", false)); err != nil {
		t.Fatalf("register eternal-vow: %v", err)
	}
	svc := quest.NewService(quest.Config{Registry: reg})
	player := stubQuestPlayer{id: "p1"}
	scope := questScope{svc: svc, player: player, givers: []string{"core:maerys"}}

	// Before accepting: both are offered, bare ids stripped of the pack
	// namespace ("core:gate-patrol" → "gate-patrol").
	if offered := refBareIDs(scope.EnumerateAcceptable()); !offered["gate-patrol"] || !offered["eternal-vow"] {
		t.Fatalf("EnumerateAcceptable = %v, want both quests by bare id", offered)
	}
	// Nothing active yet.
	if got := scope.EnumerateActive(); len(got) != 0 {
		t.Fatalf("EnumerateActive (none accepted) = %v, want empty", refBareIDs(got))
	}

	if r := svc.Accept(player, "core:gate-patrol", false); r.Status != quest.Accepted {
		t.Fatalf("accept gate-patrol: status %v", r.Status)
	}
	if r := svc.Accept(player, "core:eternal-vow", false); r.Status != quest.Accepted {
		t.Fatalf("accept eternal-vow: status %v", r.Status)
	}

	// EnumerateActive returns ONLY the abandonable active quest.
	active := refBareIDs(scope.EnumerateActive())
	if !active["gate-patrol"] {
		t.Errorf("EnumerateActive should include the abandonable gate-patrol, got %v", active)
	}
	if active["eternal-vow"] {
		t.Errorf("EnumerateActive must omit the non-abandonable eternal-vow, got %v", active)
	}

	// Accepted (non-repeatable) quests drop out of the offer set.
	if offered := refBareIDs(scope.EnumerateAcceptable()); offered["gate-patrol"] || offered["eternal-vow"] {
		t.Errorf("active quests should no longer be offered, got %v", offered)
	}

	// Display carries the friendly name; Completion (BareID) the token.
	for _, ref := range scope.EnumerateActive() {
		if ref.BareID == "gate-patrol" && ref.Name != "Gate Patrol" {
			t.Errorf("ref Name = %q, want %q", ref.Name, "Gate Patrol")
		}
	}
}

// A scope with no service or no player enumerates nothing (the nil-safety
// the completion path relies on).
func TestQuestScope_NilSafe(t *testing.T) {
	var empty questScope
	if got := empty.EnumerateAcceptable(); got != nil {
		t.Errorf("nil-svc EnumerateAcceptable = %v, want nil", got)
	}
	if got := empty.EnumerateActive(); got != nil {
		t.Errorf("nil-svc EnumerateActive = %v, want nil", got)
	}
}
