package progression_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// TestTrackNameCaseInsensitive guards the case-normalization fix: track names
// are canonicalized (lowercased) at the registry AND the Manager entry points,
// so a class's `bound_track: Martial` and a registered `martial` resolve to one
// key. Without it the grant path (case-sensitive) silently misses while `score`
// (EqualFold) still shows a level — XP that looks earned but is never banked,
// and mixed-case grants that split a character's progress across two entries.
func TestTrackNameCaseInsensitive(t *testing.T) {
	r := progression.NewTrackRegistry()
	if err := r.Register(&progression.TrackDef{Name: "Martial", MaxLevel: 3, XPTable: []int64{0, 0, 100, 300}}); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Registry lookup is case-insensitive (the key is stored canonicalized).
	if _, ok := r.Get("martial"); !ok {
		t.Error("Get(martial) missed a track registered as Martial")
	}
	if !r.Has("MARTIAL") {
		t.Error("Has(MARTIAL) missed a track registered as Martial")
	}

	m := progression.NewManager(r, nil)
	state := progression.NewProgressionState()

	// Two grants with different casing must accumulate on ONE track, not split
	// into two 60-XP state entries.
	if res := m.GrantExperience(context.Background(), state, "e1", "martial", 60, "kill"); res.TrackUnknown {
		t.Fatal("grant to 'martial' reported TrackUnknown")
	}
	if res := m.GrantExperience(context.Background(), state, "e1", "MARTIAL", 60, "kill"); res.TrackUnknown {
		t.Fatal("grant to 'MARTIAL' reported TrackUnknown")
	}

	// 120 total → level 2 (threshold 100), read back via yet another casing.
	info, ok := m.GetTrackInfo(state, "Martial")
	if !ok {
		t.Fatal("GetTrackInfo(Martial) ok=false")
	}
	if info.XP != 120 {
		t.Errorf("XP = %d, want 120 (both casings accumulate on one track, not split)", info.XP)
	}
	if info.Level != 2 {
		t.Errorf("Level = %d, want 2", info.Level)
	}

	// The DIRECT ProgressionState.Level/XP reads (used by feat credits,
	// class-save scaling, and quest MinLevel gates — they bypass the Manager)
	// must also canonicalize, or they'd miss the granted level on a mis-cased
	// bound_track and read 0. Regression guard for the go-reviewer CRITICAL.
	if lvl := state.Level("Martial"); lvl != 2 {
		t.Errorf("state.Level(Martial) = %d, want 2 (direct read must canonicalize)", lvl)
	}
	if xp := state.XP("MARTIAL"); xp != 120 {
		t.Errorf("state.XP(MARTIAL) = %d, want 120 (direct read must canonicalize)", xp)
	}

	// A save round-trip with a mixed-case key normalizes on Restore, not orphans.
	restored := progression.NewProgressionState()
	restored.Restore(progression.ProgressionSnapshot{{Name: "Martial", Level: 2, XP: 120}})
	if lvl := restored.Level("martial"); lvl != 2 {
		t.Errorf("restored.Level(martial) = %d, want 2 (Restore must canonicalize the key)", lvl)
	}
}
