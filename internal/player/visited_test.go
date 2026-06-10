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

// The seen-areas set round-trips through a save/load (gates the
// once-ever first-entry banner), and an unset set loads empty.
func TestSave_SeenAreasRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	if err := st.Save(ctx, &player.Save{
		Version: player.CurrentVersion, ID: "p-s", AccountID: "acct-s", Name: "Wanderer",
		SeenAreas: []string{"wot:emonds-field", "wot:westwood"},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "wanderer")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got.SeenAreas, []string{"wot:emonds-field", "wot:westwood"}) {
		t.Errorf("SeenAreas = %v, want [wot:emonds-field wot:westwood]", got.SeenAreas)
	}

	if err := st.Save(ctx, &player.Save{
		Version: player.CurrentVersion, ID: "p-n", AccountID: "acct-n", Name: "Newbie",
	}); err != nil {
		t.Fatalf("Save fresh: %v", err)
	}
	fresh, err := st.Load(ctx, "newbie")
	if err != nil {
		t.Fatalf("Load fresh: %v", err)
	}
	if len(fresh.SeenAreas) != 0 {
		t.Errorf("fresh SeenAreas = %v, want empty", fresh.SeenAreas)
	}
}

// The minimap size preset round-trips through a save/load, and an unset
// preset loads as empty (treated as "auto" by the session accessor).
func TestSave_MinimapSizeRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	if err := st.Save(ctx, &player.Save{
		Version: player.CurrentVersion, ID: "p-m", AccountID: "acct-m", Name: "Sizer",
		MinimapSize: "large",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "sizer")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.MinimapSize != "large" {
		t.Errorf("MinimapSize = %q, want large", got.MinimapSize)
	}

	if err := st.Save(ctx, &player.Save{
		Version: player.CurrentVersion, ID: "p-d", AccountID: "acct-d", Name: "Default",
	}); err != nil {
		t.Fatalf("Save default: %v", err)
	}
	def, err := st.Load(ctx, "default")
	if err != nil {
		t.Fatalf("Load default: %v", err)
	}
	if def.MinimapSize != "" {
		t.Errorf("unset MinimapSize = %q, want empty", def.MinimapSize)
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
