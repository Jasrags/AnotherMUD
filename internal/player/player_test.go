package player_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
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

func TestIsEmpty(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)

	if !st.IsEmpty() {
		t.Fatal("a fresh store should be empty")
	}

	// A stray non-directory file (e.g. a macOS .DS_Store) must not count
	// as a character — emptiness keys on character subdirectories only.
	stray := filepath.Join(dir, "players", ".DS_Store")
	if err := os.WriteFile(stray, []byte("junk"), 0o644); err != nil {
		t.Fatalf("write stray file: %v", err)
	}
	if !st.IsEmpty() {
		t.Fatal("a stray file should not make the store look non-empty")
	}

	save := &player.Save{Version: player.CurrentVersion, ID: "p-1", Name: "Alice"}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if st.IsEmpty() {
		t.Fatal("store with one character should not be empty")
	}
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

func TestSaveLoad_RolesRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	save := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "Alice",
		Roles:     []string{"admin", "builder"},
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "alice")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Roles) != 2 || got.Roles[0] != "admin" || got.Roles[1] != "builder" {
		t.Errorf("Roles = %v, want [admin builder]", got.Roles)
	}
}

func TestSaveLoad_NoRolesIsEmpty(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	// A save written with no roles (the common case) loads back with no
	// roles — the unprivileged default — and writes no `roles:` key.
	if err := st.Save(ctx, &player.Save{
		Version: player.CurrentVersion, ID: "p-2", AccountID: "a", Name: "Bob",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "bob")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Roles) != 0 {
		t.Errorf("Roles = %v, want empty (unprivileged default)", got.Roles)
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

func TestLoad_V22BackfillsWorldIDFromLocationNamespace(t *testing.T) {
	// character-identity §4: a pre-v23 save is stamped with the world derived
	// from its location room-id namespace.
	ctx := context.Background()
	st, dir := newStore(t)
	playerDir := filepath.Join(dir, "players", "wotchar")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 22\nid: p-1\naccount_id: acct-1\nname: WotChar\nlocation: wot:the-green\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := st.Load(ctx, "wotchar")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if got.WorldID != "wot" {
		t.Errorf("WorldID = %q, want wot (derived from location namespace)", got.WorldID)
	}
}

func TestLoad_V22BackfillsWorldIDFallbackWhenNoNamespace(t *testing.T) {
	// A location with no namespace falls back to the default backfill world.
	ctx := context.Background()
	st, dir := newStore(t)
	playerDir := filepath.Join(dir, "players", "oldchar")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 22\nid: p-2\naccount_id: acct-1\nname: OldChar\nlocation: orphan-room\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := st.Load(ctx, "oldchar")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.WorldID != player.DefaultBackfillWorld {
		t.Errorf("WorldID = %q, want %q (fallback)", got.WorldID, player.DefaultBackfillWorld)
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

func TestLoad_V4MigratesToV5WithNilVitals(t *testing.T) {
	// A v4 save carries no `vitals` block. After migration to v5 the
	// field is still absent — Vitals == nil — and the session-load
	// path treats that as "spawn at full HP". The migration itself is
	// a no-op on dict content; only the version stamp advances.
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "v4user")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 4\nid: p-1\naccount_id: acct-1\nname: V4User\nlocation: tapestry-core:town-square\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "v4user")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if got.Vitals != nil {
		t.Errorf("Vitals = %+v, want nil after v4 migration", got.Vitals)
	}
}

func TestSave_RoundTripsVitals(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	want := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "Vitalized",
		Location:  "tapestry-core:town-square",
		Vitals:    &player.VitalsState{HP: 12, MaxHP: 40},
	}
	if err := st.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "Vitalized")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Vitals == nil {
		t.Fatalf("Vitals nil after round-trip")
	}
	if got.Vitals.HP != 12 || got.Vitals.MaxHP != 40 {
		t.Errorf("Vitals = %+v, want {HP:12 MaxHP:40}", got.Vitals)
	}
}

func TestLoad_V5MigratesToV6WithEmptyStatsBase(t *testing.T) {
	// A v5 save carries no `stats_base` block. After migration to v6
	// the field is still absent (the migration is a no-op on dict
	// content); the session-load path leaves the StatBlock at the
	// progression.DefaultPlayerBase already seeded at construction.
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "v5user")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 5\nid: p-1\naccount_id: acct-1\nname: V5User\nlocation: tapestry-core:town-square\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "v5user")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if len(got.StatsBase) != 0 {
		t.Errorf("StatsBase = %v, want empty after v5 migration", got.StatsBase)
	}
}

func TestSave_RoundTripsStatsBase(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	want := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "BaseStats",
		Location:  "tapestry-core:town-square",
		StatsBase: progression.BaseSnapshot{
			{Stat: progression.StatSTR, Value: 14},
			{Stat: progression.StatHPMax, Value: 35},
		},
	}
	if err := st.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "BaseStats")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.StatsBase) != 2 {
		t.Fatalf("StatsBase len = %d, want 2", len(got.StatsBase))
	}
	if got.StatsBase[0].Stat != progression.StatSTR || got.StatsBase[0].Value != 14 {
		t.Errorf("StatsBase[0] = %+v, want {str 14}", got.StatsBase[0])
	}
	if got.StatsBase[1].Stat != progression.StatHPMax || got.StatsBase[1].Value != 35 {
		t.Errorf("StatsBase[1] = %+v, want {hp_max 35}", got.StatsBase[1])
	}
}

func TestLoad_V6MigratesToV7WithEmptyProgression(t *testing.T) {
	// A v6 save carries no `progression` block. After migration to v7
	// the field is still absent; the session-load path's
	// empty-snapshot branch leaves the ProgressionState empty (lazy
	// init on first interaction per spec §5.3).
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "v6user")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 6\nid: p-1\naccount_id: acct-1\nname: V6User\nlocation: tapestry-core:town-square\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "v6user")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if len(got.Progression) != 0 {
		t.Errorf("Progression = %v, want empty after v6 migration", got.Progression)
	}
}

func TestSave_RoundTripsProgression(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	want := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "Veteran",
		Location:  "tapestry-core:town-square",
		Progression: progression.ProgressionSnapshot{
			{Name: "adventurer", Level: 3, XP: 450},
			{Name: "explorer", Level: 1, XP: 0},
		},
	}
	if err := st.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "Veteran")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Progression) != 2 {
		t.Fatalf("Progression len = %d, want 2", len(got.Progression))
	}
	if got.Progression[0].Name != "adventurer" || got.Progression[0].Level != 3 || got.Progression[0].XP != 450 {
		t.Errorf("Progression[0] = %+v, want {adventurer, 3, 450}", got.Progression[0])
	}
}

func TestLoad_V7MigratesToV8WithEmptyRace(t *testing.T) {
	// A v7 save carries no `race` field. After migration to v8 the
	// field is still absent (string zero); the session-load path's
	// applyRace step seeds the configured default at construction.
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "v7user")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 7\nid: p-1\naccount_id: acct-1\nname: V7User\nlocation: tapestry-core:town-square\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "v7user")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if got.Race != "" {
		t.Errorf("Race = %q, want empty after v7 migration", got.Race)
	}
}

func TestLoad_V8MigratesToV9WithEmptyClassAndZeroTrains(t *testing.T) {
	// A v8 save carries no `class` or `trains_available` fields.
	// After migration to v9 both are absent / zero; the session-load
	// path's applyClass step is a no-op on an empty class id.
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "v8user")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 8\nid: p-1\naccount_id: acct-1\nname: V8User\nlocation: tapestry-core:town-square\nrace: human\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "v8user")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if len(got.Class) != 0 {
		t.Errorf("Class = %v, want empty after v8 migration", got.Class)
	}
	if got.TrainsAvailable != 0 {
		t.Errorf("TrainsAvailable = %d, want 0 after v8 migration", got.TrainsAvailable)
	}
	if got.Race != "human" {
		t.Errorf("Race = %q, want preserved 'human' across migration", got.Race)
	}
}

func TestSave_RoundTripsClassAndTrains(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	want := &player.Save{
		Version:         player.CurrentVersion,
		ID:              "p-1",
		AccountID:       "acct-1",
		Name:            "Brawler",
		Location:        "tapestry-core:town-square",
		Race:            "human",
		Class:           []string{"fighter"},
		TrainsAvailable: 15,
	}
	if err := st.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "Brawler")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Class) != 1 || got.Class[0] != "fighter" {
		t.Errorf("Class = %v, want [fighter]", got.Class)
	}
	if got.TrainsAvailable != 15 {
		t.Errorf("TrainsAvailable = %d, want 15", got.TrainsAvailable)
	}
}

func TestLoad_V9MigratesToV10WithZeroAlignment(t *testing.T) {
	// A v9 save carries no `alignment` field. After migration to
	// v10 the field is still absent (int zero); the session-load
	// path treats zero as neutral and the AlignmentManager
	// installs the neutral bucket tag on first call.
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "v9user")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 9\nid: p-1\naccount_id: acct-1\nname: V9User\nlocation: tapestry-core:town-square\nrace: human\nclass: fighter\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "v9user")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if got.Alignment != 0 {
		t.Errorf("Alignment = %d, want 0 after v9 migration", got.Alignment)
	}
	if len(got.Class) != 1 || got.Class[0] != "fighter" || got.Race != "human" {
		t.Errorf("v9 fields not preserved: class=%v race=%q", got.Class, got.Race)
	}
}

func TestSave_RoundTripsAlignment(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	want := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "EvilDoer",
		Location:  "tapestry-core:town-square",
		Alignment: -750,
	}
	if err := st.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "EvilDoer")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Alignment != -750 {
		t.Errorf("Alignment = %d, want -750", got.Alignment)
	}
}

func TestSave_RoundTripsRace(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	want := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "Dwarvish",
		Location:  "tapestry-core:town-square",
		Race:      "dwarf",
	}
	if err := st.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "Dwarvish")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Race != "dwarf" {
		t.Errorf("Race = %q, want dwarf", got.Race)
	}
}

func TestSave_RoundTripsAutoloot(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	want := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "Looter",
		Location:  "tapestry-core:town-square",
		Autoloot:  true,
	}
	if err := st.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "Looter")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Autoloot {
		t.Errorf("Autoloot = %v, want true (persists across logout/login)", got.Autoloot)
	}
}

func TestSave_AutolootDefaultsOffWhenAbsent(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	// A save written with autoloot off omits the field (omitempty),
	// mirroring an older save that predates it — it must load as off.
	want := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "Plain",
		Location:  "tapestry-core:town-square",
	}
	if err := st.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "Plain")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Autoloot {
		t.Errorf("Autoloot = %v, want false when absent", got.Autoloot)
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

func TestSaveLoad_V11AbilitiesRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)
	save := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "Maevyn",
		Location:  "tapestry-core:town-square",
		Abilities: progression.AbilitySnapshot{
			Proficiency: map[string]int{"slash": 12, "parry": 8},
			Cap:         map[string]int{"slash": 25, "parry": 25},
		},
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "maevyn")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if got.Abilities.Proficiency["slash"] != 12 || got.Abilities.Proficiency["parry"] != 8 {
		t.Errorf("Abilities.Proficiency = %+v", got.Abilities.Proficiency)
	}
	if got.Abilities.Cap["slash"] != 25 || got.Abilities.Cap["parry"] != 25 {
		t.Errorf("Abilities.Cap = %+v", got.Abilities.Cap)
	}
}

func TestSave_RoundTripsGold(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	want := &player.Save{
		Version:   player.CurrentVersion,
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "Moneybags",
		Location:  "tapestry-core:town-square",
		Gold:      4200,
	}
	if err := st.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "Moneybags")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Gold != 4200 {
		t.Errorf("Gold = %d, want 4200", got.Gold)
	}
}

func TestSave_RoundTripsSustenance(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	want := &player.Save{
		Version:    player.CurrentVersion,
		ID:         "p-1",
		AccountID:  "acct-1",
		Name:       "Peckish",
		Location:   "tapestry-core:town-square",
		Sustenance: 42,
	}
	if err := st.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "Peckish")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Sustenance != 42 {
		t.Errorf("Sustenance = %d, want 42", got.Sustenance)
	}
}

// A famished player at 0 serializes as an absent key (omitempty) and
// must reload as 0 — the legitimate famished floor, not a migration
// artifact. (Version is already current, so no migration runs.)
func TestSave_SustenanceZeroRoundTrips(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)

	want := &player.Save{
		Version:    player.CurrentVersion,
		ID:         "p-1",
		AccountID:  "acct-1",
		Name:       "Starving",
		Location:   "tapestry-core:town-square",
		Sustenance: 0,
	}
	if err := st.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "Starving")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Sustenance != 0 {
		t.Errorf("Sustenance = %d, want 0", got.Sustenance)
	}
}

// The v12→v13 migration is the first value-injecting migration: a
// legacy v12 save carries no sustenance, and the migration must seed it
// to full (100) so an existing character doesn't load famished.
func TestLoad_V12MigratesToV13SeedsFull(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	playerDir := filepath.Join(dir, "players", "olduser")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 12\nid: p-1\naccount_id: acct-1\nname: OldUser\nlocation: tapestry-core:town-square\ngold: 10\n"),
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
	if got.Sustenance != 100 {
		t.Errorf("v12→v13 migration seeded sustenance = %d, want 100", got.Sustenance)
	}
	if got.Gold != 10 {
		t.Errorf("migration disturbed gold = %d, want 10", got.Gold)
	}
}

// The v13→v14 migration is a no-op on dict content: a legacy v13
// save carries no recall key, and absence must decode to empty
// (the documented "no recall point set" default per recall.md §6).
// Unlike sustenance, the migration does NOT inject a value.
func TestLoad_V13MigratesToV14EmptyRecall(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	playerDir := filepath.Join(dir, "players", "olduser")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 13\nid: p-1\naccount_id: acct-1\nname: OldUser\nlocation: tapestry-core:town-square\nsustenance: 50\n"),
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
	if got.Recall != "" {
		t.Errorf("v13→v14 should preserve empty recall, got %q", got.Recall)
	}
	if got.Sustenance != 50 {
		t.Errorf("migration disturbed sustenance = %d, want 50", got.Sustenance)
	}
}

// Recall round-trips through save/load unchanged when set.
func TestSave_RoundTripsRecall(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)
	original := &player.Save{
		ID:        "p-1",
		AccountID: "acct-1",
		Name:      "Alice",
		Location:  "tapestry-core:town-square",
		Recall:    "tapestry-core:tavern",
	}
	if err := st.Save(ctx, original); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "Alice")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Recall != "tapestry-core:tavern" {
		t.Errorf("Recall round-trip = %q, want %q", got.Recall, "tapestry-core:tavern")
	}
}

func TestLoad_V21MigratesToV22(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	playerDir := filepath.Join(dir, "players", "olduser")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A v21 save carries no gender key; migration must preserve the
	// absence as the empty "unset" gender (the documented default).
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 21\nid: p-1\naccount_id: acct-1\nname: OldUser\nlocation: tapestry-core:town-square\n"),
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
	if got.Gender != "" {
		t.Errorf("v21 migration produced non-empty gender: %q", got.Gender)
	}
}

// baseStatValue returns the value of stat in snap and whether it is present.
func baseStatValue(snap progression.BaseSnapshot, stat progression.StatType) (int, bool) {
	for _, e := range snap {
		if e.Stat == stat {
			return e.Value, true
		}
	}
	return 0, false
}

// A v23 save whose persisted base predates the movement-cost feature gets
// movement_max backfilled (the on-disk shape is made explicit).
func TestLoad_V23BackfillsMovementMax(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	playerDir := filepath.Join(dir, "players", "oldwalker")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A v23 base block carries the classics + hp_max but no movement_max.
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 23\nid: p-1\naccount_id: acct-1\nname: OldWalker\nlocation: starter-world:town-square\nstats_base:\n  - stat: str\n    value: 10\n  - stat: hp_max\n    value: 20\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := st.Load(ctx, "oldwalker")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version after migrate = %d, want %d", got.Version, player.CurrentVersion)
	}
	v, ok := baseStatValue(got.StatsBase, progression.StatMovementMax)
	if !ok {
		t.Fatalf("movement_max not backfilled into StatsBase: %v", got.StatsBase)
	}
	if v != player.BackfillMovementMax {
		t.Errorf("backfilled movement_max = %d, want %d", v, player.BackfillMovementMax)
	}
	// The pre-existing entries survive (append, not replace).
	if hp, ok := baseStatValue(got.StatsBase, progression.StatHPMax); !ok || hp != 20 {
		t.Errorf("hp_max = (%d,%v), want (20,true) — backfill must not drop existing stats", hp, ok)
	}
}

// A v23 save that already carries movement_max is left untouched (no
// duplicate entry, value preserved).
func TestLoad_V23MovementMaxBackfillIdempotent(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	playerDir := filepath.Join(dir, "players", "newwalker")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 23\nid: p-2\naccount_id: acct-2\nname: NewWalker\nlocation: starter-world:town-square\nstats_base:\n  - stat: movement_max\n    value: 25\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := st.Load(ctx, "newwalker")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	n := 0
	for _, e := range got.StatsBase {
		if e.Stat == progression.StatMovementMax {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("movement_max entry count = %d, want 1 (no duplicate)", n)
	}
	if v, _ := baseStatValue(got.StatsBase, progression.StatMovementMax); v != 25 {
		t.Errorf("existing movement_max = %d, want 25 preserved", v)
	}
}

// A v23 save with no persisted base block stays empty after migration — the
// full DefaultPlayerBase (movement_max included) applies at construction, so
// the migration must NOT fabricate a partial base.
func TestLoad_V23NoStatsBaseStaysEmpty(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	playerDir := filepath.Join(dir, "players", "baselessuser")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 23\nid: p-3\naccount_id: acct-3\nname: BaselessUser\nlocation: starter-world:town-square\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := st.Load(ctx, "baselessuser")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.StatsBase) != 0 {
		t.Errorf("StatsBase = %v, want empty (no partial base fabricated)", got.StatsBase)
	}
}

func TestSaveLoad_GenderRoundTrips(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)
	in := &player.Save{
		Version: player.CurrentVersion, ID: "p-g", AccountID: "acct-g",
		Name: "Channeler", Gender: "female",
	}
	if err := st.Save(ctx, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "channeler")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Gender != "female" {
		t.Errorf("Gender round-trip = %q, want female", got.Gender)
	}
}

func TestLoad_V11MigratesToV12(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	playerDir := filepath.Join(dir, "players", "olduser")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A v11 save carries no gold key; migration must preserve the
	// absence as a zero balance (the documented default).
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 11\nid: p-1\naccount_id: acct-1\nname: OldUser\nlocation: tapestry-core:town-square\n"),
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
	if got.Gold != 0 {
		t.Errorf("v11 migration produced non-zero gold: %d", got.Gold)
	}
}

func TestLoad_V10MigratesToV11(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)
	playerDir := filepath.Join(dir, "players", "olduser")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 10\nid: p-1\naccount_id: acct-1\nname: OldUser\nlocation: tapestry-core:town-square\n"),
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
	if len(got.Abilities.Proficiency) != 0 || len(got.Abilities.Cap) != 0 {
		t.Errorf("v10 migration produced non-empty abilities: %+v", got.Abilities)
	}
}
