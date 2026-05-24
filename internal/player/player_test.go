package player_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

func newStore(t *testing.T) (*player.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := player.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return st, dir
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	save := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "Alice",
		Location:  "tapestry-core:forge",
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := st.Load(ctx, "alice")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != "Alice" || got.Location != "tapestry-core:forge" || got.AccountID != "acct-1" {
		t.Errorf("got = %+v", got)
	}
}

func TestLoad_MissingReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	_, err := st.Load(ctx, "ghost")
	if !errors.Is(err, player.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestExists_LowercasesName(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	if err := st.Save(ctx, &player.Save{Version: player.CurrentVersion, Name: "Bob", AccountID: "x"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !st.Exists("BOB") {
		t.Error("Exists(BOB) = false, want true (case-insensitive)")
	}
	if !st.Exists("bob") {
		t.Error("Exists(bob) = false, want true")
	}
	if st.Exists("nobody") {
		t.Error("Exists(nobody) = true, want false")
	}
}

func TestLoad_NewerVersionRejected(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)

	// Write a file by hand with a too-new version.
	playerDir := filepath.Join(dir, "players", "alice")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 999\nid: p-1\naccount_id: acct-1\nname: Alice\nlocation: x\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := st.Load(ctx, "alice")
	if !errors.Is(err, player.ErrVersionNewer) {
		t.Fatalf("err = %v, want ErrVersionNewer", err)
	}
}

func TestLoad_DefaultsVersionToOneWhenMissing(t *testing.T) {
	// A pre-versioning save (no version field) should be treated as v1
	// and migrate forward — since CurrentVersion is 1, this is a no-op
	// but the path must not error.
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "carol")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("id: p-1\naccount_id: acct-1\nname: Carol\nlocation: x\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "carol")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != "Carol" {
		t.Errorf("name = %q", got.Name)
	}
}

func TestSaveLoad_V2RoundTripWithInventoryAndEquipment(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	save := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "Eve",
		Location:  "tapestry-core:town-square",
		Inventory: []string{"tapestry-core:short-sword", "tapestry-core:healing-draught"},
		Equipment: map[string]string{"main-hand": "tapestry-core:short-sword"},
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := st.Load(ctx, "eve")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("Version = %d, want 2", got.Version)
	}
	if len(got.Inventory) != 2 || got.Inventory[0] != "tapestry-core:short-sword" {
		t.Errorf("Inventory = %v", got.Inventory)
	}
	if got.Equipment["main-hand"] != "tapestry-core:short-sword" {
		t.Errorf("Equipment = %v", got.Equipment)
	}
}

func TestLoad_V1MigratesToV2(t *testing.T) {
	// A v1 file on disk must load cleanly and come back with v2 shape:
	// empty inventory, empty equipment, version bumped.
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "olduser")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 1\nid: p-1\naccount_id: acct-1\nname: OldUser\nlocation: tapestry-core:town-square\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "olduser")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("Version after migrate = %d, want 2", got.Version)
	}
	if len(got.Inventory) != 0 {
		t.Errorf("Inventory = %v, want empty", got.Inventory)
	}
	if len(got.Equipment) != 0 {
		t.Errorf("Equipment = %v, want empty", got.Equipment)
	}
	if got.Name != "OldUser" || got.Location != "tapestry-core:town-square" {
		t.Errorf("preserved fields wrong: %+v", got)
	}
}

func TestSave_RejectsUnsafeName(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	err := st.Save(ctx, &player.Save{Version: player.CurrentVersion, Name: "../etc/passwd", AccountID: "x"})
	if err == nil {
		t.Fatal("Save with traversal name succeeded, want error")
	}
}
