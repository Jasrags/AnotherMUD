package player_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// The v16 fog-of-war visited set round-trips through a save/load.
func TestSave_VisitedRoomsRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version:      player.CurrentVersion,
		ID:           "p-v",
		AccountID:    "acct-v",
		Name:         "Mapper",
		VisitedRooms: []string{"core:a", "core:b", "core:c"},
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "mapper")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if !reflect.DeepEqual(got.VisitedRooms, []string{"core:a", "core:b", "core:c"}) {
		t.Errorf("VisitedRooms = %v, want [core:a core:b core:c]", got.VisitedRooms)
	}
}

// A save with no visited set loads as empty — the fog-of-war default for
// a fresh character and the v15→v16 migration result (no back-fill).
func TestSave_NoVisitedRoomsDefaultsEmpty(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	if err := st.Save(ctx, &player.Save{
		Version: player.CurrentVersion, ID: "p-b", AccountID: "acct-b", Name: "Blank",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "blank")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.VisitedRooms) != 0 {
		t.Errorf("VisitedRooms = %v, want empty", got.VisitedRooms)
	}
}
