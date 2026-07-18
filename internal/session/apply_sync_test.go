package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// These tests pin the m8-4 data-loss guard: a class/race id that is on the save
// but NOT in the active registry (content removed, or a character loaded under a
// pack that lacks the class) must be PRESERVED on the save, never overwritten
// with empty. Before the guard, run()'s post-apply sync compared
// loaded != resolved (true: "fighter" != "") and wrote "" back, so the next
// autosave permanently erased the class. The fix keeps the original id so
// re-adding the content reattaches the character.

func TestShouldSyncRace(t *testing.T) {
	tests := []struct {
		name     string
		resolved string // applyRace output (a.raceID)
		loaded   string // save's race id
		want     bool
	}{
		{"fail-soft preserves saved id", "", "extinct-species", false},
		{"default applied to fresh char syncs", "human", "", true},
		{"resolved differs from loaded syncs", "human", "orc", true},
		{"no-op when already equal", "human", "human", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSyncRace(tt.resolved, tt.loaded); got != tt.want {
				t.Errorf("shouldSyncRace(%q, %q) = %v, want %v", tt.resolved, tt.loaded, got, tt.want)
			}
		})
	}
}

func TestShouldSyncClass(t *testing.T) {
	tests := []struct {
		name     string
		resolved []string // applyClass output (a.classIDs)
		loaded   []string // save's class list
		want     bool
	}{
		{"fail-soft (all unregistered) preserves saved list", nil, []string{"fighter"}, false},
		{"empty resolved with loaded present preserves", []string{}, []string{"wizard"}, false},
		{"resolved differs from loaded syncs", []string{"fighter"}, []string{"wizard"}, true},
		{"canonical list shrank (one id removed) syncs", []string{"fighter"}, []string{"fighter", "ghost-class"}, true},
		{"no-op when already equal", []string{"fighter"}, []string{"fighter"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSyncClass(tt.resolved, tt.loaded); got != tt.want {
				t.Errorf("shouldSyncClass(%v, %v) = %v, want %v", tt.resolved, tt.loaded, got, tt.want)
			}
		})
	}
}

// TestApplyClass_UnknownIDLeavesClassless pins the fail-soft PRECONDITION that
// makes shouldSyncClass preserve the save: an unregistered saved class id
// resolves to an empty classIDs list (mirrors TestApplyRace_UnknownIDLeavesRaceless).
func TestApplyClass_UnknownIDLeavesClassless(t *testing.T) {
	cr := progression.NewClassRegistry()
	if err := cr.Register(&progression.Class{ID: "fighter", DisplayName: "Fighter"}); err != nil {
		t.Fatalf("register fighter: %v", err)
	}
	a := &connActor{}
	cfg := &Config{Classes: cr}

	applyClass(a, cfg, []string{"removed-class"})

	if len(a.classIDs) != 0 {
		t.Errorf("classIDs = %v, want empty (unregistered class must not stick)", a.classIDs)
	}
	// The whole point: with classIDs empty, the sync guard preserves the save.
	if shouldSyncClass(a.classIDs, []string{"removed-class"}) {
		t.Error("shouldSyncClass returned true for a fail-soft resolution — the saved class would be erased")
	}
}

// TestApplyClass_DropsUnregisteredKeepsRegistered pins the mixed case: a
// multiclass save where one id is removed content keeps the registered id and
// drops the ghost (so shouldSyncClass then syncs the shrunk canonical list).
func TestApplyClass_DropsUnregisteredKeepsRegistered(t *testing.T) {
	cr := progression.NewClassRegistry()
	if err := cr.Register(&progression.Class{ID: "fighter", DisplayName: "Fighter"}); err != nil {
		t.Fatalf("register fighter: %v", err)
	}
	a := &connActor{}
	cfg := &Config{Classes: cr}

	applyClass(a, cfg, []string{"fighter", "ghost-class"})

	if len(a.classIDs) != 1 || a.classIDs[0] != "fighter" {
		t.Fatalf("classIDs = %v, want [fighter] (ghost dropped, fighter kept)", a.classIDs)
	}
	if !shouldSyncClass(a.classIDs, []string{"fighter", "ghost-class"}) {
		t.Error("shouldSyncClass = false, want true — the shrunk canonical list should be synced")
	}
}
