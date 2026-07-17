package player_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// security-response.md §7 v2: a character's heat + wanted level round-trip through
// save/load so a relog doesn't wipe the law's attention.
func TestSave_HeatRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-heat", AccountID: "acct-heat", Name: "Raze",
		Heat: 72, WantedLevel: 3,
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "raze")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Heat != 72 || got.WantedLevel != 3 {
		t.Errorf("heat/wanted did not round-trip: got (%d,%d), want (72,3)", got.Heat, got.WantedLevel)
	}
}

// A clean character stores no heat (the omitempty zero value) — absent and a
// stored 0 are indistinguishable, both reading as cold.
func TestSave_HeatDefaultsCold(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{Version: player.CurrentVersion, ID: "p-cold", AccountID: "acct-cold", Name: "Clean"}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "clean")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Heat != 0 || got.WantedLevel != 0 {
		t.Errorf("fresh character has heat: (%d,%d), want (0,0)", got.Heat, got.WantedLevel)
	}
}
