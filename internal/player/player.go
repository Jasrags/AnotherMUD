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
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// CurrentVersion is the version stamped on every save written today.
// Append a migration to playerMigrations whenever this number bumps.
//
// v2 (M5.5): added `inventory` (list of item template ids) and
// `equipment` (slot key → item template id) blocks. Per-instance state
// is not yet persisted — items respawn fresh from their templates at
// load time.
//
// v3 (M5.6): `equipment` value type widened from string to a struct
// carrying both the template id and the runtime entity id from the
// session that wrote the save. The entity id lets respawnEquipment
// rebind persisted stat-block source keys onto the freshly-minted
// entity ids on the next login (inventory-equipment-items §3.5).
// `stats` block added to persist the holder's sourced modifier set
// applied by equipment.
//
// v4 (M5.9b): `inventory` value type widened from a bare template id
// string to a {template, contents} struct so containers can persist
// what they're carrying (inventory-equipment-items §4.5). The
// `contents` field is itself a list of InventoryEntry — nesting
// reflects real container nesting at session time. Empty Contents
// serializes via `omitempty`, so leaf items round-trip as just
// `{template: ...}`.
//
// v5 (M7.5): `vitals` block added so HP state persists across logout
// (combat spec §6.4 implies the player-death subscriber owns recovery
// but a player who logs out below full HP MUST come back at the same
// HP). Absent block (legacy v4 saves migrated forward) means "spawn at
// full HP", which is what NewVitals does.
//
// v6 (M8.1): `stats_base` block added — the persisted intrinsic
// attribute values held by the progression.StatBlock (the six classics
// + vital maxima + the M8.1-carried hit_mod / ac). Absent block (legacy
// v5 saves migrated forward) means "apply engine defaults at restore
// time", which is what progression.DefaultPlayerBase covers via the
// NewWithBase construction site before RestoreBase is even called.
//
// v7 (M8.2): `progression` block added — the per-entity (level, xp)
// state from progression.md §5.2. Absent block (legacy v6 saves
// migrated forward) means "no tracks initialized yet"; the
// ProgressionState restore path lazy-inits on first interaction.
//
// v8 (M8.3): `race` string added — the race id from progression.md
// §3.1. Empty (legacy v7 saves migrated forward) means the session
// layer applies the configured default race at construction; see
// session.applyRace for the fallback policy.
//
// v9 (M8.4): `class` id + `trains_available` integer (spec §4.1 /
// §4.6 step 4). Empty class (legacy v8 saves migrated forward)
// means the character has no class — the path processor and stat
// growth subscriber short-circuit on empty class id. Zero trains
// is the natural starting state; the M8.6 train verb is the only
// consumer.
const CurrentVersion = 9

// Sentinel errors callers may check via errors.Is.
var (
	ErrNotFound     = errors.New("player: save not found")
	ErrVersionNewer = errors.New("player: save version is newer than loader")
)

// Save is the on-disk record for a single character. The yaml tags use
// snake_case per persistence spec §3.2.
//
// Inventory stores *template ids*; runtime entity ids are reassigned
// each session so persisting them would be meaningless, and inventory
// items have no holder-side state crossing the boundary.
//
// Equipment is different: it persists both the template id AND the
// entity id the session was using when it wrote the save. The entity
// id is dead on disk (the next session mints fresh ids) but it is the
// key that the persisted Stats block uses to identify which modifier
// set came from which slot. respawnEquipment uses the saved entity id
// to rebind those modifiers onto the new instance's id. See
// inventory-equipment-items §3.5.
//
// Stats holds the holder's sourced modifier set — the cumulative
// effect of every equipped item's modifiers — persisted under the
// same source keys that were live when the save was written.
type Save struct {
	Version     int                             `yaml:"version"`
	ID          string                          `yaml:"id"`
	AccountID   string                          `yaml:"account_id"`
	Name        string                          `yaml:"name"`
	Location    string                          `yaml:"location"`
	Inventory   []InventoryEntry                `yaml:"inventory,omitempty"`
	Equipment   map[string]EquippedItem         `yaml:"equipment,omitempty"`
	Stats       stats.Snapshot                  `yaml:"stats,omitempty"`
	StatsBase   progression.BaseSnapshot        `yaml:"stats_base,omitempty"`
	Progression progression.ProgressionSnapshot `yaml:"progression,omitempty"`
	Race            string                          `yaml:"race,omitempty"`
	Class           string                          `yaml:"class,omitempty"`
	TrainsAvailable int                             `yaml:"trains_available,omitempty"`
	Vitals          *VitalsState                    `yaml:"vitals,omitempty"`
	// WimpyThreshold is the §5.1 HP-percent threshold (0 = wimpy
	// disabled). Added in M7.6 without a schema bump: zero-value
	// is indistinguishable from "field absent" so legacy v5 saves
	// round-trip unchanged. The session layer enforces [0, 100] on
	// set; load tolerates anything but treats anything < 1 or > 100
	// as disabled.
	WimpyThreshold int `yaml:"wimpy,omitempty"`
}

// VitalsState is the persisted HP block (v5+). Pointer so an absent
// vitals block (legacy v4 saves migrated forward, fresh characters
// pre-first-damage) serializes as no key at all rather than `vitals: {}`,
// and the session-load path treats nil as "spawn at full HP" without
// having to disambiguate zero-value from explicit-zero.
type VitalsState struct {
	HP    int `yaml:"hp"`
	MaxHP int `yaml:"max_hp"`
}

// InventoryEntry is one carried item in the persisted inventory list
// (v4+). Contents is non-empty only when the entry's template is a
// container that held items at save time; nesting is recursive so a
// pouch-inside-a-backpack round-trips by structure rather than by
// foreign-key id (no stable per-instance id exists on disk because
// entity ids are reassigned each session).
//
// The `omitempty` on Contents keeps the wire format compact: a leaf
// item serializes as `{template: ...}`, indistinguishable from the
// pre-v4 string shape after migration.
type InventoryEntry struct {
	Template string           `yaml:"template"`
	Contents []InventoryEntry `yaml:"contents,omitempty"`
}

// EquippedItem is one entry in the persisted equipment map (v3+). The
// pair is what lets respawnEquipment reattach the persisted Stats
// modifiers (sourced under EquipmentSourceKey(Entity)) to a freshly
// re-spawned ItemInstance with a new runtime id.
type EquippedItem struct {
	Template string `yaml:"template"`
	Entity   string `yaml:"entity"`
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
	2: migrateV2toV3,
	3: migrateV3toV4,
	4: migrateV4toV5,
	5: migrateV5toV6,
	6: migrateV6toV7,
	7: migrateV7toV8,
	8: migrateV8toV9,
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

// migrateV2toV3 widens the `equipment` value shape from a bare template
// id string to a {template, entity} struct, and admits an empty `stats`
// block (real values land when the migrated save is next written by a
// session that has actually equipped something).
//
// v2 in practice never wrote real equipment data — the field was
// declared in M5.5 but no equip command existed to populate it. The
// loop below handles the (theoretical) string-shaped legacy entries by
// promoting them to the struct shape with an empty entity id; the
// session layer treats an empty entity id as "no source key to rebind"
// so the modifier set is simply absent for that slot. Safer than
// silently dropping the equipment reference.
func migrateV2toV3(in map[string]any) (map[string]any, error) {
	raw, ok := in["equipment"]
	if !ok || raw == nil {
		return in, nil
	}
	eq, ok := toStringKeyMap(raw)
	if !ok {
		// Equipment present but not a map — drop it. A v2 save that
		// fails this shape check is malformed; the alternative is
		// returning an error and refusing to load, which is worse than
		// losing equipment that almost certainly was never there.
		delete(in, "equipment")
		return in, nil
	}
	out := make(map[string]any, len(eq))
	for slot, val := range eq {
		switch v := val.(type) {
		case string:
			out[slot] = map[string]any{"template": v, "entity": ""}
		case map[string]any:
			out[slot] = v
		case map[any]any:
			// yaml.v3 hands nested maps back as map[any]any; promote.
			promoted := make(map[string]any, len(v))
			for k, vv := range v {
				if ks, ok := k.(string); ok {
					promoted[ks] = vv
				}
			}
			out[slot] = promoted
		default:
			// Unknown shape — drop this slot, same reasoning as above.
		}
	}
	in["equipment"] = out
	return in, nil
}

// migrateV3toV4 widens the `inventory` value shape from a list of
// bare template id strings to a list of {template, contents} entries
// so containers can persist their contents (M5.9b, spec
// inventory-equipment-items §4.5).
//
// v3 entries were all leaves (containers existed as templates but the
// put verb didn't, so no save could carry container contents). The
// migration is a 1:1 lift: every old string becomes a struct with
// that template and no contents. Save-shape decoders treat a leaf
// entry and a v3 string identically once migrated.
//
// Unrecognized entry shapes (somehow a non-string in the v3 list)
// are dropped with no error: the alternative is refusing to load,
// which is worse than losing a malformed inventory slot.
func migrateV3toV4(in map[string]any) (map[string]any, error) {
	raw, ok := in["inventory"]
	if !ok || raw == nil {
		return in, nil
	}
	list, ok := raw.([]any)
	if !ok {
		// Inventory present but not a list — drop it. Mirrors the
		// equipment-malformed handling in migrateV2toV3.
		delete(in, "inventory")
		return in, nil
	}
	out := make([]any, 0, len(list))
	for _, e := range list {
		s, ok := e.(string)
		if !ok {
			// Unknown shape — drop this entry. A v3 save that
			// contains anything other than strings is malformed.
			continue
		}
		out = append(out, map[string]any{"template": s})
	}
	in["inventory"] = out
	return in, nil
}

// migrateV4toV5 adds the `vitals` block introduced in M7.5. The
// migration is a no-op on dict content: legacy v4 saves carry no HP
// state, so the absence of `vitals:` is preserved and the session
// load path's nil-Vitals branch spawns the player at full HP. New
// saves stamp the field as soon as Persist runs after first damage.
func migrateV4toV5(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV5toV6 adds the `stats_base` block introduced in M8.1. The
// migration is a no-op on dict content: legacy v5 saves carry no
// persisted base attributes, so the absence of `stats_base:` is
// preserved and the session load path's empty-snapshot branch leaves
// the StatBlock at progression.DefaultPlayerBase. New saves stamp the
// field as soon as Persist runs after any base-attribute change (M8.4
// stat growth, M8.6 train).
func migrateV5toV6(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV6toV7 adds the `progression` block introduced in M8.2.
// No-op on dict content: a legacy v6 save carries no per-track
// state, so the absence of `progression:` is preserved and the
// session load path's empty-snapshot branch leaves the
// ProgressionState empty (lazy-init on first interaction per spec
// §5.3).
func migrateV6toV7(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV7toV8 adds the `race` field introduced in M8.3. No-op on
// dict content: a legacy v7 save carries no race id, so the
// absence is preserved and the session load path applies the
// configured default race at construction (see session.applyRace).
func migrateV7toV8(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV8toV9 adds the `class` + `trains_available` fields
// introduced in M8.4 (spec progression.md §4). No-op on dict
// content: a legacy v8 save carries no class id, so the absence
// is preserved (empty class short-circuits the class-path
// processor and stat-growth subscriber). trains_available
// defaults to zero, which is the natural starting state for the
// M8.6 train verb.
func migrateV8toV9(in map[string]any) (map[string]any, error) {
	return in, nil
}

// toStringKeyMap accepts either of yaml.v3's two map decodings.
func toStringKeyMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, vv := range m {
			ks, ok := k.(string)
			if !ok {
				return nil, false
			}
			out[ks] = vv
		}
		return out, true
	default:
		return nil, false
	}
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
