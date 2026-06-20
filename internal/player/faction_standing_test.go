package player_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// faction.md §8.1: after a shift the standing bag holds the value and
// round-trips through save/load (the durable per-character reputation).
func TestSave_FactionStandingRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-fs", AccountID: "acct-fs", Name: "Galad",
		FactionStanding: map[string]int{
			"wot:children-of-the-light": 750,
			"wot:darkfriends":           -900,
		},
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "galad")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if got.FactionStanding["wot:children-of-the-light"] != 750 ||
		got.FactionStanding["wot:darkfriends"] != -900 {
		t.Errorf("FactionStanding did not round-trip: %v", got.FactionStanding)
	}
}

// faction.md §8.1: a fresh character has an empty standing bag (the omitempty
// zero value) — reads every faction at its starting standing.
func TestSave_FactionStandingDefaultsEmpty(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-fresh", AccountID: "acct-fresh", Name: "Egwene",
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "egwene")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.FactionStanding) != 0 {
		t.Errorf("FactionStanding = %v, want empty for a fresh character", got.FactionStanding)
	}
}
