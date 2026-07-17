package player_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/karma"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

// SR-M5: a karma-ledger character's spendable + lifetime karma round-trips
// through save/load so a relog doesn't wipe hard-earned advancement currency.
func TestSave_KarmaRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	snap := karma.Snapshot{Current: 40, Total: 170}
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-karma", AccountID: "acct-karma", Name: "Wraith",
		Karma: &snap,
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "wraith")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Karma == nil {
		t.Fatal("karma block did not round-trip: got nil")
	}
	if got.Karma.Current != 40 || got.Karma.Total != 170 {
		t.Errorf("karma did not round-trip: got %+v, want {Current:40 Total:170}", *got.Karma)
	}
}

// A level-track character stores no karma block (the nil pointer / omitempty),
// so a fantasy save never grows a `karma:` key — absent decodes back to nil.
func TestSave_KarmaAbsentForLevelTrack(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{Version: player.CurrentVersion, ID: "p-lvl", AccountID: "acct-lvl", Name: "Knight"}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "knight")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Karma != nil {
		t.Errorf("a level-track character carries a karma block: %+v, want nil", *got.Karma)
	}
}
