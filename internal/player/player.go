// Package player owns the on-disk player save record and its file store.
//
// Spec: docs/specs/persistence.md §4 (player serialization) and §7
// (versioning + migrations). M3 carries the minimum: version, ids, name,
// location. Stats, properties, inventory, equipment, and the tagged-
// value envelope land with M5/M8 when there's live state worth saving.
//
// The migration table is scaffolded empty: CurrentVersion is 1 and there
// are no registered migrations. The Load path still exercises the
// drift-detection and newer-version-rejection branches so the §7
// acceptance criteria are testable today.
package player

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/persistence"
)

// CurrentVersion is the version stamped on every save written today.
// Append a migration to playerMigrations whenever this number bumps.
//
// v2 (M5.5): added `inventory` (list of item template ids) and
// `equipment` (slot key → item template id) blocks. Per-instance state
// is not yet persisted — items respawn fresh from their templates at
// load time. When per-instance state lands (charges, condition, fill
// amount), bump to v3 with a richer inventory entry shape.
const CurrentVersion = 2

// Sentinel errors callers may check via errors.Is.
var (
	ErrNotFound     = errors.New("player: save not found")
	ErrVersionNewer = errors.New("player: save version is newer than loader")
)

// Save is the on-disk record for a single character. The yaml tags use
// snake_case per persistence spec §3.2.
//
// Inventory and Equipment store *template ids* (e.g. "tapestry-core:short-sword"),
// not runtime entity ids. Runtime ids are reassigned each session, so
// persisting them would be meaningless; the inventory feature respawns
// fresh instances from the template registry at login. Equipment maps
// slot key (e.g. "main-hand", "ring:0") to template id. Both are
// optional and empty for v1 saves migrated forward.
type Save struct {
	Version   int               `yaml:"version"`
	ID        string            `yaml:"id"`
	AccountID string            `yaml:"account_id"`
	Name      string            `yaml:"name"`
	Location  string            `yaml:"location"`
	Inventory []string          `yaml:"inventory,omitempty"`
	Equipment map[string]string `yaml:"equipment,omitempty"`
}

// Store is a file-backed player store. Directories live at
// <root>/players/<lowercased-name>/player.yaml so concurrent reads see
// either the prior or the new file, never a partial one (atomic writes
// in internal/persistence).
//
// A coarse per-store mutex serializes Save against concurrent
// Save/Load on the same name. Without it, two writers' tmp→bak→rename
// sequences can interleave so one .bak rotation clobbers another's
// .tmp before the rename, leaving both writes only partially applied.
// Per-name locking would be more efficient; the single mutex is the
// M3-scale cut and revisits with the SessionManager rework in M4.
type Store struct {
	root string // <save-root>/players
	mu   sync.Mutex
}

// NewStore opens (creating if needed) the players subdirectory under
// root.
func NewStore(root string) (*Store, error) {
	dir := filepath.Join(root, "players")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("player: mkdir: %w", err)
	}
	return &Store{root: dir}, nil
}

// CanonicalName lowercases name for both filesystem path computation and
// in-game equality checks. Spec §3.2 mandates lowercased-name keying.
func CanonicalName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (s *Store) playerDir(name string) (string, error) {
	canon := CanonicalName(name)
	if canon == "" {
		return "", fmt.Errorf("player: empty name: %w", persistence.ErrUnsafePath)
	}
	return persistence.SafeJoin(s.root, canon)
}

func (s *Store) playerFile(name string) (string, error) {
	dir, err := s.playerDir(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "player.yaml"), nil
}

// Exists is a cheap stat used by the login flow.
func (s *Store) Exists(name string) bool {
	path, err := s.playerFile(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Save writes the record atomically. Save.Version is stamped to
// CurrentVersion if zero so callers don't have to remember.
func (s *Store) Save(ctx context.Context, save *Save) error {
	if save.Version == 0 {
		save.Version = CurrentVersion
	}
	path, err := s.playerFile(save.Name)
	if err != nil {
		return fmt.Errorf("player.Save: %w", err)
	}
	data, err := yaml.Marshal(save)
	if err != nil {
		return fmt.Errorf("player.Save: encode: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return persistence.AtomicWrite(path, data)
}

// Load reads the record for name. Older versions are migrated forward
// in memory before the structured Save is returned; newer versions are
// rejected (spec §7).
func (s *Store) Load(ctx context.Context, name string) (*Save, error) {
	path, err := s.playerFile(name)
	if err != nil {
		return nil, fmt.Errorf("player.Load %q: %w", name, err)
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("player.Load %q: %w", name, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("player.Load %q: %w", name, err)
	}

	// Decode into a generic dict first so migrations can mutate it
	// before we bind into the structured shape (spec §7).
	var generic map[string]any
	if err := yaml.Unmarshal(data, &generic); err != nil {
		return nil, fmt.Errorf("player.Load %q: decode generic: %w", name, err)
	}
	migrated, err := migrate(ctx, generic, name)
	if err != nil {
		return nil, err
	}

	// Re-marshal then unmarshal into the structured Save. Slightly
	// roundabout but avoids hand-rolling field binding and keeps yaml
	// tag handling in one place.
	out, err := yaml.Marshal(migrated)
	if err != nil {
		return nil, fmt.Errorf("player.Load %q: re-marshal: %w", name, err)
	}
	var save Save
	if err := yaml.Unmarshal(out, &save); err != nil {
		return nil, fmt.Errorf("player.Load %q: bind: %w", name, err)
	}
	return &save, nil
}

// playerMigrations is the append-only table from spec §7. Key N means
// "transforms a v{N} dict into a v{N+1} dict". Never delete an entry;
// existing saves out there may still be at that version.
var playerMigrations = map[int]func(map[string]any) (map[string]any, error){
	1: migrateV1toV2,
}

// migrateV1toV2 adds the empty inventory/equipment blocks introduced
// in M5.5. Pre-existing fields are left untouched.
//
// The migrated dict is left without explicit `inventory` / `equipment`
// keys when they're empty — yaml `omitempty` handles the serialization,
// and the structured Save decoder treats absence and empty list /
// empty map identically.
func migrateV1toV2(in map[string]any) (map[string]any, error) {
	return in, nil
}

func migrate(ctx context.Context, generic map[string]any, name string) (map[string]any, error) {
	v, _ := asInt(generic["version"])
	if v == 0 {
		// Pre-versioning saves: treat as v1.
		v = 1
		generic["version"] = 1
	}
	if v > CurrentVersion {
		return nil, fmt.Errorf("player.migrate %q: file v%d, loader v%d: %w",
			name, v, CurrentVersion, ErrVersionNewer)
	}
	if v < CurrentVersion {
		logging.From(ctx).Info("migrating player save",
			slog.String("name", name),
			slog.Int("from_version", v),
			slog.Int("to_version", CurrentVersion))
	}
	for cur := v; cur < CurrentVersion; cur++ {
		mig, ok := playerMigrations[cur]
		if !ok {
			return nil, fmt.Errorf("player.migrate %q: no migration v%d -> v%d", name, cur, cur+1)
		}
		next, err := mig(generic)
		if err != nil {
			return nil, fmt.Errorf("player.migrate %q: v%d -> v%d: %w", name, cur, cur+1, err)
		}
		generic = next
		generic["version"] = cur + 1
	}
	return generic, nil
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	default:
		return 0, false
	}
}
