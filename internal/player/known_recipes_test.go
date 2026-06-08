package player_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// The v17 known-recipes set round-trips through a save/load.
func TestSave_KnownRecipesRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version:      player.CurrentVersion,
		ID:           "p-r",
		AccountID:    "acct-r",
		Name:         "Smith",
		KnownRecipes: []string{"tapestry-core:nail", "tapestry-core:campfire-stew"},
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "smith")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	want := []string{"tapestry-core:nail", "tapestry-core:campfire-stew"}
	if !reflect.DeepEqual(got.KnownRecipes, want) {
		t.Errorf("KnownRecipes = %v, want %v", got.KnownRecipes, want)
	}
}

// A save with no recipes loads as empty — the default for a fresh
// character and the v16→v17 migration result (no back-fill).
func TestSave_NoKnownRecipesDefaultsEmpty(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	if err := st.Save(ctx, &player.Save{
		Version: player.CurrentVersion, ID: "p-e", AccountID: "acct-e", Name: "Empty",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "empty")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.KnownRecipes) != 0 {
		t.Errorf("KnownRecipes = %v, want empty", got.KnownRecipes)
	}
}

// A legacy v16 save on disk migrates forward cleanly to v17 with no
// known_recipes key injected (the documented no-op).
func TestLoad_V16MigratesToV17NoRecipes(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "v16user")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 16\nid: p-1\naccount_id: acct-1\nname: V16User\nlocation: tapestry-core:town-square\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "v16user")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version after migrate = %d, want %d", got.Version, player.CurrentVersion)
	}
	if len(got.KnownRecipes) != 0 {
		t.Errorf("KnownRecipes = %v, want empty after migrate", got.KnownRecipes)
	}
}
