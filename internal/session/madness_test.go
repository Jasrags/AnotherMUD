package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// connActor.AddMadness/Madness is the neutral saidin-taint counter the
// composition root drives (WoT S2 Phase 4+): it accrues, cures (floored at 0),
// and marks the save dirty so the value persists.

func newMadnessActor() *connActor {
	return &connActor{save: &player.Save{Version: player.CurrentVersion, Name: "Tainted"}}
}

func TestAddMadness_AccruesAndPersistsDirty(t *testing.T) {
	a := newMadnessActor()
	a.dirty = false

	if got := a.AddMadness(5); got != 5 {
		t.Errorf("AddMadness(5) = %d, want 5", got)
	}
	if a.Madness() != 5 {
		t.Errorf("Madness() = %d, want 5", a.Madness())
	}
	if !a.dirty {
		t.Error("dirty not set after AddMadness — the taint would be lost on next persist")
	}
}

func TestAddMadness_FloorsAtZero(t *testing.T) {
	a := newMadnessActor()
	a.AddMadness(3)
	if got := a.AddMadness(-10); got != 0 {
		t.Errorf("AddMadness(-10) from 3 = %d, want 0 (a cure cannot drive taint negative)", got)
	}
	if a.Madness() != 0 {
		t.Errorf("Madness() = %d, want 0", a.Madness())
	}
}

func TestMadness_NilSaveIsZero(t *testing.T) {
	a := &connActor{}
	if got := a.Madness(); got != 0 {
		t.Errorf("Madness() with nil save = %d, want 0", got)
	}
	if got := a.AddMadness(5); got != 0 {
		t.Errorf("AddMadness with nil save = %d, want 0 (no-op)", got)
	}
}

// HasFeat backs the Mental Stability madness-resilience seam (WoT S2 Phase 4+):
// a case-insensitive query over the persisted KnownFeats.
func TestHasFeat(t *testing.T) {
	a := newMadnessActor()
	a.save.KnownFeats = []player.KnownFeat{{FeatID: "Mental-Stability", Count: 1}}

	if !a.HasFeat("mental-stability") {
		t.Error("HasFeat should match case-insensitively against KnownFeats")
	}
	if a.HasFeat("toughness") {
		t.Error("HasFeat returned true for an untaken feat")
	}
	if a.HasFeat("") {
		t.Error("HasFeat(\"\") should be false")
	}
	if (&connActor{}).HasFeat("mental-stability") {
		t.Error("HasFeat with nil save should be false")
	}
}
