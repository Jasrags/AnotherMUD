package player_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// The v25 saidin-taint accumulator round-trips through a save/load (WoT S2
// Phase 4+ — the curse is character state, persisted, so it survives relogin).
func TestSave_MadnessRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-mad", AccountID: "acct-mad", Name: "Tainted",
		Madness: 42,
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "tainted")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if got.Madness != 42 {
		t.Errorf("Madness = %d, want 42 (taint must persist across relogin)", got.Madness)
	}
}

// A clean (untainted) channeler and every non-channeler save with no madness
// loads at 0 — the omitempty zero value, no surprise taint.
func TestSave_MadnessDefaultsZero(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-clean", AccountID: "acct-clean", Name: "Clean",
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "clean")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Madness != 0 {
		t.Errorf("Madness = %d, want 0 (a clean character carries no taint)", got.Madness)
	}
}
