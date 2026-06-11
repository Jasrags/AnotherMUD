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

// --- wot-character-model D1: v17 scalar class → v18 list -------------------

// writePlayerYAML writes a raw player.yaml for migration tests.
func writePlayerYAML(t *testing.T, dir, name, body string) {
	t.Helper()
	pd := filepath.Join(dir, "players", name)
	if err := os.MkdirAll(pd, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pd, "player.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLoad_V17ScalarClassMigratesToList(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	writePlayerYAML(t, dir, "warder",
		"version: 17\nid: p-1\naccount_id: acct-1\nname: Warder\nlocation: tapestry-core:town-square\nclass: fighter\n")

	got, err := st.Load(ctx, "warder")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if len(got.Class) != 1 || got.Class[0] != "fighter" {
		t.Errorf("Class = %v, want [fighter] (scalar wrapped to a 1-element list)", got.Class)
	}
}

func TestLoad_V17AbsentClassMigratesToNil(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	// A classless v17 save — no `class:` key at all.
	writePlayerYAML(t, dir, "wanderer",
		"version: 17\nid: p-2\naccount_id: acct-1\nname: Wanderer\nlocation: tapestry-core:town-square\n")

	got, err := st.Load(ctx, "wanderer")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Class) != 0 {
		t.Errorf("Class = %v, want empty (classless stays classless)", got.Class)
	}
}

func TestLoad_V17EmptyClassStringMigratesToNil(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	writePlayerYAML(t, dir, "blank",
		"version: 17\nid: p-3\naccount_id: acct-1\nname: Blank\nlocation: tapestry-core:town-square\nclass: \"\"\n")

	got, err := st.Load(ctx, "blank")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Class) != 0 {
		t.Errorf("Class = %v, want empty (an empty scalar is classless)", got.Class)
	}
}

// A v17 save whose class is already a YAML list (hand-edited, or forward-
// dated) migrates idempotently — the wrap branch leaves a list untouched.
func TestLoad_V17YamlListClassIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	writePlayerYAML(t, dir, "listchar",
		"version: 17\nid: p-5\naccount_id: acct-1\nname: Listchar\nlocation: tapestry-core:town-square\nclass:\n  - fighter\n")

	got, err := st.Load(ctx, "listchar")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Class) != 1 || got.Class[0] != "fighter" {
		t.Errorf("Class = %v, want [fighter] (a list at v17 is left as-is)", got.Class)
	}
}

// A v18 save round-trips its class list unchanged (save then load).
func TestSaveLoad_V18ClassListRoundTrips(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)
	in := &player.Save{
		Version: player.CurrentVersion, ID: "p-4", AccountID: "acct-1",
		Name: "Aiel", Location: "tapestry-core:town-square", Class: []string{"fighter"},
	}
	if err := st.Save(ctx, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "aiel")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Class) != 1 || got.Class[0] != "fighter" {
		t.Errorf("Class round-trip = %v, want [fighter]", got.Class)
	}
}

// --- backgrounds §5: v18 → v19 (background field) -------------------------

func TestLoad_V18NoBackgroundMigratesToEmpty(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	// A v18 save with a class list but no background field.
	writePlayerYAML(t, dir, "noback",
		"version: 18\nid: p-1\naccount_id: acct-1\nname: Noback\nlocation: tapestry-core:town-square\nclass:\n  - fighter\n")

	got, err := st.Load(ctx, "noback")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if got.Background != "" {
		t.Errorf("Background = %q, want empty (v18 had no background)", got.Background)
	}
	// Class still migrates/round-trips alongside.
	if len(got.Class) != 1 || got.Class[0] != "fighter" {
		t.Errorf("Class = %v, want [fighter]", got.Class)
	}
}

func TestSaveLoad_V19BackgroundRoundTrips(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)
	in := &player.Save{
		Version: player.CurrentVersion, ID: "p-2", AccountID: "acct-1",
		Name: "Originful", Location: "tapestry-core:town-square",
		Class: []string{"fighter"}, Background: "soldier",
	}
	if err := st.Save(ctx, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "originful")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Background != "soldier" {
		t.Errorf("Background round-trip = %q, want soldier", got.Background)
	}
}
