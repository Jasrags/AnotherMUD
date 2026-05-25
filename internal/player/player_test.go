package player_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/stats"
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

func TestSaveLoad_V4RoundTripWithInventoryEquipmentStats(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	src := entities.EquipmentSourceKey("entity-1")
	save := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "Eve",
		Location:  "tapestry-core:town-square",
		Inventory: []player.InventoryEntry{
			{Template: "tapestry-core:short-sword"},
			{Template: "tapestry-core:healing-draught"},
		},
		Equipment: map[string]player.EquippedItem{
			"wield": {Template: "tapestry-core:short-sword", Entity: "entity-1"},
		},
		Stats: stats.Snapshot{
			{Source: src, Modifiers: []stats.Modifier{{Stat: "str", Value: 1}}},
		},
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := st.Load(ctx, "eve")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if len(got.Inventory) != 2 || got.Inventory[0].Template != "tapestry-core:short-sword" {
		t.Errorf("Inventory = %+v", got.Inventory)
	}
	eq, ok := got.Equipment["wield"]
	if !ok {
		t.Fatalf("Equipment missing wield slot: %v", got.Equipment)
	}
	if eq.Template != "tapestry-core:short-sword" || eq.Entity != "entity-1" {
		t.Errorf("Equipment[wield] = %+v", eq)
	}
	if len(got.Stats) != 1 || got.Stats[0].Source != src {
		t.Errorf("Stats = %+v", got.Stats)
	}
	if got.Stats[0].Modifiers[0].Stat != "str" || got.Stats[0].Modifiers[0].Value != 1 {
		t.Errorf("Stats modifiers = %+v", got.Stats[0].Modifiers)
	}
}

func TestLoad_V1MigratesToCurrent(t *testing.T) {
	// A v1 file on disk must traverse every migration step cleanly and
	// come back at CurrentVersion with empty inventory, equipment, and
	// stats.
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
	if got.Version != player.CurrentVersion {
		t.Errorf("Version after migrate = %d, want %d", got.Version, player.CurrentVersion)
	}
	if len(got.Inventory) != 0 {
		t.Errorf("Inventory = %v, want empty", got.Inventory)
	}
	if len(got.Equipment) != 0 {
		t.Errorf("Equipment = %v, want empty", got.Equipment)
	}
	if len(got.Stats) != 0 {
		t.Errorf("Stats = %v, want empty", got.Stats)
	}
	if got.Name != "OldUser" || got.Location != "tapestry-core:town-square" {
		t.Errorf("preserved fields wrong: %+v", got)
	}
}

func TestLoad_V2EquipmentMigratesToV3Struct(t *testing.T) {
	// A v2 save with the old string-shaped equipment (theoretical — M5.5
	// never wrote one in practice) should promote into the v3 struct
	// shape with an empty entity id; the missing entity id leaves the
	// stats block with no source key to rebind against, so the slot is
	// effectively unequipped on next login. This is the documented
	// behavior in migrateV2toV3.
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "v2user")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 2\nid: p-1\naccount_id: acct-1\nname: V2User\nlocation: tapestry-core:town-square\nequipment:\n  wield: tapestry-core:short-sword\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "v2user")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	eq, ok := got.Equipment["wield"]
	if !ok {
		t.Fatalf("Equipment[wield] missing after migrate: %v", got.Equipment)
	}
	if eq.Template != "tapestry-core:short-sword" {
		t.Errorf("Equipment[wield].Template = %q", eq.Template)
	}
	if eq.Entity != "" {
		t.Errorf("Equipment[wield].Entity = %q, want empty", eq.Entity)
	}
}

func TestLoad_V3InventoryMigratesToV4Entries(t *testing.T) {
	// A v3 save with the old flat string-list inventory must lift to
	// v4 InventoryEntry{Template, Contents=nil} entries. The migration
	// is a 1:1 promote per pre-v4 entries; v3 had no container nesting
	// so Contents is always empty after migration.
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "v3user")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 3\nid: p-1\naccount_id: acct-1\nname: V3User\nlocation: tapestry-core:town-square\ninventory:\n  - tapestry-core:short-sword\n  - tapestry-core:healing-draught\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "v3user")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if len(got.Inventory) != 2 {
		t.Fatalf("Inventory length = %d, want 2: %+v", len(got.Inventory), got.Inventory)
	}
	if got.Inventory[0].Template != "tapestry-core:short-sword" {
		t.Errorf("Inventory[0].Template = %q", got.Inventory[0].Template)
	}
	if got.Inventory[0].Contents != nil {
		t.Errorf("Inventory[0].Contents = %v, want nil after v3 migration", got.Inventory[0].Contents)
	}
	if got.Inventory[1].Template != "tapestry-core:healing-draught" {
		t.Errorf("Inventory[1].Template = %q", got.Inventory[1].Template)
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
