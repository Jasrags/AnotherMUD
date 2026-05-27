package pack

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/world"
	"gopkg.in/yaml.v3"
)

// Errors callers may distinguish at the boundary.
var (
	ErrMissingArea         = errors.New("room references unknown area")
	ErrMissingExitRoom     = errors.New("exit references unknown room")
	ErrMissingItemTemplate = errors.New("room item references unknown template")
	ErrMissingMobTemplate  = errors.New("room mob references unknown template")
	ErrMissingSpawnRoom    = errors.New("spawn rule references unknown room")
	ErrInvalidContent      = errors.New("invalid content file")
)

// Spawner spawns an item template and places the resulting instance
// in a room. The loader calls it once per (room, template) entry in
// the room YAMLs' `items` list, after all content has loaded and
// validated.
//
// Implementations adapt to whatever runtime registries the host owns
// (in production: entities.Store + entities.Placement; in tests: a
// recording mock). Returning an error aborts the load — there is no
// partial-placement semantics.
//
// Spec world-rooms-movement §2.2 (boot-time room item placement).
type Spawner interface {
	SpawnAndPlace(ctx context.Context, templateID string, roomID world.RoomID) error
}

// MobSpawner mirrors Spawner for mobs (spec mobs-ai-spawning §3.1).
// Separate interface so test mocks can record items vs mobs
// independently and so a host that wants one but not the other is
// not forced to stub both. The signature shape matches Spawner; only
// the entity-kind contract differs.
type MobSpawner interface {
	SpawnAndPlaceMob(ctx context.Context, templateID string, roomID world.RoomID) error
}

// pendingPlacement is one room→item entry collected during the pack
// content pass and applied once all content has loaded. We accumulate
// these rather than spawning inline so cross-pack template references
// resolve (the target template may live in a pack that hasn't been
// loaded yet at the time the referring room is parsed).
type pendingPlacement struct {
	RoomID     world.RoomID
	TemplateID string
}

// pendingMobPlacement is the mob equivalent of pendingPlacement.
// Kept as a separate type so applyMobPlacements doesn't need a kind
// discriminator and the two error sites carry different sentinels.
type pendingMobPlacement struct {
	RoomID     world.RoomID
	TemplateID string
}

// Load discovers packs under root, orders them by dependencies, and
// populates dst's registries with the resulting content (spec §3.3
// phases 1+2).
//
// M5.1 scope: areas, rooms, item templates. Tags, properties, mobs,
// scripts arrive in later milestones. Phase 1 records the loaded
// manifest list; Phase 2 reads YAML into each registry.
//
// Filter, when non-empty, restricts discovery (spec §2.4). Pass nil to
// load every active pack under root.
func Load(ctx context.Context, root string, filter []string, dst *Registries, spawner Spawner, mobSpawner MobSpawner) error {
	if dst == nil || dst.World == nil || dst.Items == nil || dst.Slots == nil || dst.Mobs == nil || dst.Tracks == nil {
		return errors.New("pack.Load: dst has nil registry field; use pack.NewRegistries()")
	}
	logger := logging.From(ctx).With(slog.String("event", "pack.load"), slog.String("root", root))

	discovered, err := Discover(root, filter)
	if err != nil {
		return fmt.Errorf("discovery: %w", err)
	}
	ordered, err := Order(discovered)
	if err != nil {
		return fmt.Errorf("ordering: %w", err)
	}

	logger.Info("packs discovered", slog.Int("count", len(ordered)))

	// Phase 1: manifest pass. M2 records only; no tags/properties yet.
	for _, p := range ordered {
		logging.From(ctx).Info("pack manifest",
			slog.String("event", "pack.manifest"),
			slog.String("pack", p.Manifest.Name),
			slog.String("namespace", p.Namespace()),
		)
	}

	// Phase 2: content pass.
	var placements []pendingPlacement
	var mobPlacements []pendingMobPlacement
	for _, p := range ordered {
		pp, mp, err := loadPackContent(ctx, p, dst)
		if err != nil {
			return fmt.Errorf("pack %q: %w", p.Manifest.Name, err)
		}
		placements = append(placements, pp...)
		mobPlacements = append(mobPlacements, mp...)
	}

	// Cross-pack area validity check (spec §3.3 step 4) runs after every
	// pack has been read so cross-pack room→area refs resolve.
	if err := validateAreas(dst.World); err != nil {
		return err
	}

	// Exit-target validation runs last for the same reason.
	if err := validateExits(dst.World); err != nil {
		return err
	}

	// Placement post-pass. Runs after all packs have loaded so cross-pack
	// item-template references resolve. Spawner=nil means callers don't
	// want runtime instances created (tests that only need template
	// loading); the validation still runs so bad ids surface either way.
	if err := applyPlacements(ctx, dst, placements, spawner); err != nil {
		return err
	}
	if err := applyMobPlacements(ctx, dst, mobPlacements, mobSpawner); err != nil {
		return err
	}

	// Area spawn-rule references (rooms + mob templates) must resolve
	// in the final world. Runs after every pack has loaded so
	// cross-pack references (`other-pack:foo`) are valid.
	if err := validateSpawnRules(dst); err != nil {
		return err
	}

	return nil
}

// validateSpawnRules walks every area's SpawnRules and verifies that
// each rule's room id resolves in the world registry and each
// mob/rare template id resolves in the mob registry. Spec
// mobs-ai-spawning §3.1 says spawn placement is fail-silent at
// RUNTIME, but boot-time validation lets content authors catch typos
// before they ship. Sentinel errors mirror placement validation:
// `ErrMissingSpawnRoom` and `ErrMissingMobTemplate`.
func validateSpawnRules(dst *Registries) error {
	for _, area := range dst.World.Areas() {
		for i, rule := range area.SpawnRules {
			if _, err := dst.World.Room(rule.RoomID); err != nil {
				return fmt.Errorf("%w: area %q spawn_rules[%d].room %q", ErrMissingSpawnRoom, area.ID, i, rule.RoomID)
			}
			if !dst.Mobs.Has(mob.TemplateID(rule.MobTemplateID)) {
				return fmt.Errorf("%w: area %q spawn_rules[%d].mob %q", ErrMissingMobTemplate, area.ID, i, rule.MobTemplateID)
			}
			if rule.Rare != "" {
				if !dst.Mobs.Has(mob.TemplateID(rule.Rare)) {
					return fmt.Errorf("%w: area %q spawn_rules[%d].rare %q", ErrMissingMobTemplate, area.ID, i, rule.Rare)
				}
			}
		}
	}
	return nil
}

func loadPackContent(ctx context.Context, p Discovered, dst *Registries) ([]pendingPlacement, []pendingMobPlacement, error) {
	ns := p.Namespace()
	logger := logging.From(ctx).With(slog.String("pack", p.Manifest.Name), slog.String("namespace", ns))

	areaPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Areas)
	if err != nil {
		return nil, nil, err
	}
	roomPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Rooms)
	if err != nil {
		return nil, nil, err
	}
	itemPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Items)
	if err != nil {
		return nil, nil, err
	}
	slotPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Slots)
	if err != nil {
		return nil, nil, err
	}
	mobPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Mobs)
	if err != nil {
		return nil, nil, err
	}
	trackPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Tracks)
	if err != nil {
		return nil, nil, err
	}

	// Areas first — rooms reference them (spec §3.3 step 2). TryAddArea
	// catches both intra-pack and cross-pack id collisions.
	for _, ap := range areaPaths {
		a, err := decodeArea(ap, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.World.TryAddArea(a); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, ap)
		}
	}

	var placements []pendingPlacement
	var mobPlacements []pendingMobPlacement
	for _, rp := range roomPaths {
		r, items, mobs, err := decodeRoom(rp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.World.TryAddRoom(r); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, rp)
		}
		for _, tid := range items {
			placements = append(placements, pendingPlacement{
				RoomID:     r.ID,
				TemplateID: tid,
			})
		}
		for _, tid := range mobs {
			mobPlacements = append(mobPlacements, pendingMobPlacement{
				RoomID:     r.ID,
				TemplateID: tid,
			})
		}
	}

	// Item templates are namespace-scoped like rooms; TryAdd guards
	// cross-pack collisions. Spec inventory-equipment-items §2.1.
	for _, ip := range itemPaths {
		t, err := decodeItem(ip, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Items.TryAdd(t); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, ip)
		}
	}

	// Slots: names are global (not namespaced); the pack namespace
	// becomes the slot scope tag. Register surfaces collisions both
	// within a pack and across packs/engine baseline.
	for _, sp := range slotPaths {
		d, err := decodeSlot(sp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Slots.Register(d); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, sp)
		}
	}

	// Mob templates are namespace-scoped like items; TryAdd guards
	// cross-pack collisions. Spec mobs-ai-spawning §2.1. Equipment
	// id validity is NOT checked here — spec §3.1 specifies
	// fail-silent-at-spawn for missing-template lookups, and items
	// from later-loaded packs would otherwise force a post-pass.
	for _, mp := range mobPaths {
		m, err := decodeMob(mp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Mobs.TryAdd(m); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, mp)
		}
	}

	// Tracks: progression-track definitions. Name-keyed registry
	// with priority-based override semantics (spec
	// progression.md §5.1). Cross-pack overrides honored.
	for _, tp := range trackPaths {
		td, err := decodeTrack(tp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Tracks.Register(td); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, tp)
		}
	}

	logger.Info("pack content loaded",
		slog.String("event", "pack.content"),
		slog.Int("areas", len(areaPaths)),
		slog.Int("rooms", len(roomPaths)),
		slog.Int("items", len(itemPaths)),
		slog.Int("slots", len(slotPaths)),
		slog.Int("mobs", len(mobPaths)),
		slog.Int("tracks", len(trackPaths)),
		slog.Int("placements", len(placements)),
		slog.Int("mob_placements", len(mobPlacements)),
	)
	return placements, mobPlacements, nil
}

// decodeTrack reads a TrackFile and builds a progression.TrackDef.
// Spec progression.md §5.1 — name is case-sensitive; max_level
// must be > 0; xp_table must be present (formula-driven tracks
// aren't authorable from YAML until scripting lands). The
// registered Pack field records which pack registered the track
// for diagnostics.
func decodeTrack(path, ns string) (*progression.TrackDef, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading track %s: %w", path, err)
	}
	var f TrackFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	if f.MaxLevel <= 0 {
		return nil, fmt.Errorf("%w: %s: 'max_level' must be > 0 (got %d)", ErrInvalidContent, path, f.MaxLevel)
	}
	if len(f.XPTable) == 0 {
		return nil, fmt.Errorf("%w: %s: 'xp_table' required (M8.2 supports table-driven tracks only)", ErrInvalidContent, path)
	}
	// Table must declare a threshold for every reachable level
	// (index 0 unused, index 1..max_level inclusive). Without
	// this check a max_level=10 track with a 3-entry table loads
	// cleanly and cascades silently halt past level 2 — a
	// content-author footgun with no diagnostic at runtime.
	if len(f.XPTable) < f.MaxLevel+1 {
		return nil, fmt.Errorf("%w: %s: xp_table needs %d entries for max_level=%d (got %d)",
			ErrInvalidContent, path, f.MaxLevel+1, f.MaxLevel, len(f.XPTable))
	}
	// XP table must be non-decreasing — a level threshold below the
	// previous level's threshold would cause the cascade in
	// GrantExperience to never resolve cleanly. Catch the authoring
	// mistake at load.
	for i := 1; i < len(f.XPTable); i++ {
		if f.XPTable[i] < f.XPTable[i-1] {
			return nil, fmt.Errorf("%w: %s: xp_table[%d]=%d is less than xp_table[%d]=%d (must be non-decreasing)",
				ErrInvalidContent, path, i, f.XPTable[i], i-1, f.XPTable[i-1])
		}
	}
	return &progression.TrackDef{
		Name:         f.ID,
		DisplayName:  strings.TrimSpace(f.Name),
		MaxLevel:     f.MaxLevel,
		XPTable:      append([]int64(nil), f.XPTable...),
		DeathPenalty: f.DeathPenalty,
		Pack:         ns,
		Priority:     f.Priority,
	}, nil
}

// resolveGlobs expands each pattern (relative to packDir) into matching
// files. Sorted for deterministic load order. Missing patterns surface
// as errors so authors notice typos.
//
// Matches MUST stay within packDir. A pattern containing ".." (or
// otherwise escaping) is rejected — packs may not read host files
// outside their own directory.
func resolveGlobs(packDir string, patterns []string) ([]string, error) {
	cleanRoot, err := filepath.Abs(packDir)
	if err != nil {
		return nil, fmt.Errorf("resolving pack dir %s: %w", packDir, err)
	}
	prefix := cleanRoot + string(os.PathSeparator)

	var out []string
	for _, pat := range patterns {
		full := filepath.Join(cleanRoot, filepath.FromSlash(pat))
		matches, err := filepath.Glob(full)
		if err != nil {
			return nil, fmt.Errorf("bad glob %q: %w", pat, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("content pattern %q matched no files under %s", pat, packDir)
		}
		for _, m := range matches {
			absMatch, err := filepath.Abs(m)
			if err != nil {
				return nil, fmt.Errorf("resolving match %s: %w", m, err)
			}
			if absMatch != cleanRoot && !strings.HasPrefix(absMatch, prefix) {
				return nil, fmt.Errorf("content pattern %q escapes pack dir (%s)", pat, absMatch)
			}
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out, nil
}

func decodeArea(path, ns string) (*world.Area, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading area %s: %w", path, err)
	}
	var af AreaFile
	if err := yaml.Unmarshal(raw, &af); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(af.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(af.Name) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'name'", ErrInvalidContent, path)
	}
	id, err := qualifyID(af.ID, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	rules, err := decodeSpawnRules(af.SpawnRules, ns, path)
	if err != nil {
		return nil, err
	}
	return &world.Area{
		ID:            world.AreaID(id),
		Name:          af.Name,
		Description:   af.Description,
		ResetInterval: af.ResetInterval,
		SpawnRules:    rules,
	}, nil
}

// decodeSpawnRules normalizes a list of YAML SpawnRuleFile entries
// into the runtime world.SpawnRule slice. Required fields (`room`,
// `mob`, `count`) are enforced at decode; cross-references (does the
// room exist? does the mob template exist?) get checked in the
// loader's post-pass after every pack has registered its content.
//
// Bare ids are qualified against the current pack namespace; fully
// qualified `pack:id` form is preserved verbatim.
func decodeSpawnRules(in []SpawnRuleFile, ns, path string) ([]world.SpawnRule, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]world.SpawnRule, 0, len(in))
	for i, sr := range in {
		room := strings.TrimSpace(sr.Room)
		mobID := strings.TrimSpace(sr.Mob)
		if room == "" {
			return nil, fmt.Errorf("%w: %s: spawn_rules[%d]: missing 'room'", ErrInvalidContent, path, i)
		}
		if mobID == "" {
			return nil, fmt.Errorf("%w: %s: spawn_rules[%d]: missing 'mob'", ErrInvalidContent, path, i)
		}
		if sr.Count <= 0 {
			return nil, fmt.Errorf("%w: %s: spawn_rules[%d]: 'count' must be > 0 (got %d)", ErrInvalidContent, path, i, sr.Count)
		}
		qRoom, err := qualifyID(room, ns)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: spawn_rules[%d].room: %v", ErrInvalidContent, path, i, err)
		}
		qMob, err := qualifyID(mobID, ns)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: spawn_rules[%d].mob: %v", ErrInvalidContent, path, i, err)
		}
		var qRare string
		if rare := strings.TrimSpace(sr.Rare); rare != "" {
			qRare, err = qualifyID(rare, ns)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: spawn_rules[%d].rare: %v", ErrInvalidContent, path, i, err)
			}
			if sr.RareChance <= 0 || sr.RareChance > 1 {
				return nil, fmt.Errorf("%w: %s: spawn_rules[%d]: 'rare_chance' must be in (0,1] when 'rare' is set (got %v)", ErrInvalidContent, path, i, sr.RareChance)
			}
		}
		out = append(out, world.SpawnRule{
			RoomID:        world.RoomID(qRoom),
			MobTemplateID: qMob,
			Count:         sr.Count,
			Rare:          qRare,
			RareChance:    sr.RareChance,
			ResetInterval: sr.ResetInterval,
			Tags:          sr.Tags,
		})
	}
	return out, nil
}

func decodeRoom(path, ns string) (*world.Room, []string, []string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading room %s: %w", path, err)
	}
	var rf RoomFile
	if err := yaml.Unmarshal(raw, &rf); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(rf.ID) == "" {
		return nil, nil, nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(rf.Area) == "" {
		return nil, nil, nil, fmt.Errorf("%w: %s: missing 'area'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(rf.Name) == "" {
		return nil, nil, nil, fmt.Errorf("%w: %s: missing 'name'", ErrInvalidContent, path)
	}

	roomID, err := qualifyID(rf.ID, ns)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	areaID, err := qualifyID(rf.Area, ns)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %s: area: %v", ErrInvalidContent, path, err)
	}

	r := &world.Room{
		ID:          world.RoomID(roomID),
		AreaID:      world.AreaID(areaID),
		Name:        rf.Name,
		Description: rf.Description,
		Exits:       make(map[world.Direction]world.Exit, len(rf.Exits)),
	}
	for dirStr, target := range rf.Exits {
		dir, ok := world.ParseDirection(dirStr)
		if !ok {
			return nil, nil, nil, fmt.Errorf("%w: %s: unknown direction %q", ErrInvalidContent, path, dirStr)
		}
		targetID, err := qualifyID(target, ns)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%w: %s: exit %s: %v", ErrInvalidContent, path, dirStr, err)
		}
		r.Exits[dir] = world.Exit{Target: world.RoomID(targetID)}
	}

	// Item placements: qualify each template id now so we can validate
	// in a single pass at the end. We do NOT touch dst.Items here —
	// the template may live in a pack that hasn't been read yet.
	items, err := qualifyIDList(rf.Items, ns, path, "items")
	if err != nil {
		return nil, nil, nil, err
	}
	// Mob placements: same shape as item placements (spec
	// mobs-ai-spawning §3.1). Cross-pack mob refs deferred to the
	// post-pass validation in applyMobPlacements.
	mobs, err := qualifyIDList(rf.Mobs, ns, path, "mobs")
	if err != nil {
		return nil, nil, nil, err
	}
	return r, items, mobs, nil
}

// qualifyIDList qualifies each entry in a room's placement list
// (items or mobs) against the pack namespace. Empty entries are
// rejected as ErrInvalidContent so authors get a precise error.
// Extracted so the items + mobs paths can't drift.
func qualifyIDList(raws []string, ns, path, field string) ([]string, error) {
	out := make([]string, 0, len(raws))
	for i, raw := range raws {
		if strings.TrimSpace(raw) == "" {
			return nil, fmt.Errorf("%w: %s: %s[%d] is empty", ErrInvalidContent, path, field, i)
		}
		tid, err := qualifyID(raw, ns)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %s[%d]: %v", ErrInvalidContent, path, field, i, err)
		}
		out = append(out, tid)
	}
	return out, nil
}

func decodeItem(path, ns string) (*item.Template, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading item %s: %w", path, err)
	}
	var f ItemFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(f.Name) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'name'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(f.Type) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'type'", ErrInvalidContent, path)
	}
	id, err := qualifyID(f.ID, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}

	mods := make([]item.Modifier, 0, len(f.Modifiers))
	for i, m := range f.Modifiers {
		if strings.TrimSpace(m.Stat) == "" {
			return nil, fmt.Errorf("%w: %s: modifier[%d] missing 'stat'", ErrInvalidContent, path, i)
		}
		mods = append(mods, item.Modifier{Stat: m.Stat, Value: m.Value})
	}

	return &item.Template{
		ID:         item.TemplateID(id),
		Name:       f.Name,
		Type:       f.Type,
		Tags:       f.Tags,
		Keywords:   f.Keywords,
		Properties: f.Properties,
		Modifiers:  mods,
	}, nil
}

// defaultMobType is the spec-defined default for MobFile.Type when
// the YAML omits it (mobs-ai-spawning §2.2: "default `npc`").
const defaultMobType = "npc"

func decodeMob(path, ns string) (*mob.Template, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading mob %s: %w", path, err)
	}
	var f MobFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(f.Name) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'name'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(f.Behavior) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'behavior'", ErrInvalidContent, path)
	}
	id, err := qualifyID(f.ID, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}

	typ := strings.TrimSpace(f.Type)
	if typ == "" {
		typ = defaultMobType
	}

	def, err := decodeDispositionRules(f.DispositionRules, path)
	if err != nil {
		return nil, err
	}

	return &mob.Template{
		ID:               mob.TemplateID(id),
		Name:             f.Name,
		Type:             typ,
		Disposition:      f.Disposition,
		BaseDisposition:  mob.Reaction(strings.TrimSpace(f.BaseDisposition)),
		DispositionRules: def,
		Behavior:         f.Behavior,
		Tags:             f.Tags,
		Keywords:         f.Keywords,
		Properties:       f.Properties,
		Stats:            f.Stats,
		Equipment:        f.Equipment,
	}, nil
}

// decodeDispositionRules converts the YAML shape into the runtime
// Definition. Returns nil (no rules) when src is nil. Each rule must
// declare a non-empty reaction; missing reactions are an
// ErrInvalidContent surface so content authors don't ship a silently
// inert rule.
func decodeDispositionRules(src *DispositionFile, path string) (*mob.Definition, error) {
	if src == nil {
		return nil, nil
	}
	out := &mob.Definition{
		Default: mob.Reaction(strings.TrimSpace(src.Default)),
	}
	for i, r := range src.Rules {
		reaction := strings.TrimSpace(r.Reaction)
		if reaction == "" {
			return nil, fmt.Errorf("%w: %s: disposition_rules[%d]: missing 'reaction'", ErrInvalidContent, path, i)
		}
		rule := mob.Rule{
			HasTag:   strings.TrimSpace(r.HasTag),
			Reaction: mob.Reaction(reaction),
			Buckets:  r.Buckets,
		}
		if r.MinAlignment != nil {
			rule.MinAlignment = *r.MinAlignment
			rule.HasMinAlignment = true
		}
		if r.MaxAlignment != nil {
			rule.MaxAlignment = *r.MaxAlignment
			rule.HasMaxAlignment = true
		}
		out.Rules = append(out.Rules, rule)
	}
	return out, nil
}

func decodeSlot(path, ns string) (slot.Def, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return slot.Def{}, fmt.Errorf("reading slot %s: %w", path, err)
	}
	var f SlotFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return slot.Def{}, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.Name) == "" {
		return slot.Def{}, fmt.Errorf("%w: %s: missing 'name'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(f.Label) == "" {
		return slot.Def{}, fmt.Errorf("%w: %s: missing 'label'", ErrInvalidContent, path)
	}
	if f.Max <= 0 {
		// Cap-0 slots are useless and almost certainly an authoring
		// mistake; reject at decode rather than waiting for the
		// registry to surface it.
		return slot.Def{}, fmt.Errorf("%w: %s: max must be > 0", ErrInvalidContent, path)
	}
	return slot.Def{
		Name:  f.Name,
		Label: f.Label,
		Max:   f.Max,
		Scope: slot.Scope(ns),
	}, nil
}

// qualifyID applies the namespace rule (spec §5.2): if id contains ':'
// it is already qualified; otherwise prepend the current pack namespace.
// Both halves of a qualified id must be non-empty after trimming, and
// the id must contain at most one ':' so we never produce a three-part
// "ns:foo:bar" that downstream code can't interpret.
func qualifyID(id, ns string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("empty id")
	}
	if strings.Contains(id, ":") {
		parts := strings.Split(id, ":")
		if len(parts) != 2 {
			return "", fmt.Errorf("malformed qualified id %q (expected namespace:name)", id)
		}
		lhs := strings.TrimSpace(parts[0])
		rhs := strings.TrimSpace(parts[1])
		if lhs == "" || rhs == "" {
			return "", fmt.Errorf("malformed qualified id %q", id)
		}
		return lhs + ":" + rhs, nil
	}
	return ns + ":" + id, nil
}

// validateAreas walks every room in dst and ensures its area is known.
// Per spec §3.3 step 4 this is fatal regardless of validation mode.
func validateAreas(dst *world.World) error {
	for _, r := range dst.Rooms() {
		if _, err := dst.Area(r.AreaID); err != nil {
			return fmt.Errorf("%w: room %q -> area %q", ErrMissingArea, r.ID, r.AreaID)
		}
	}
	return nil
}

// applyPlacements validates each pending room→item placement and
// hands off to the spawner. Template-id validity is checked
// unconditionally so missing-template errors surface even when the
// caller passed nil spawner (tests that load content without
// instantiating). A nil spawner with valid ids is a no-op.
//
// Errors are NOT wrapped with the room/template context twice: the
// returned error already names the template; the room id is added
// here so authors get both. Spec world-rooms-movement §2.2.
func applyPlacements(ctx context.Context, dst *Registries, placements []pendingPlacement, spawner Spawner) error {
	for _, pl := range placements {
		if !dst.Items.Has(item.TemplateID(pl.TemplateID)) {
			return fmt.Errorf("%w: room %q -> item %q", ErrMissingItemTemplate, pl.RoomID, pl.TemplateID)
		}
		if spawner == nil {
			continue
		}
		if err := spawner.SpawnAndPlace(ctx, pl.TemplateID, pl.RoomID); err != nil {
			return fmt.Errorf("placement room %q item %q: %w", pl.RoomID, pl.TemplateID, err)
		}
	}
	return nil
}

// applyMobPlacements mirrors applyPlacements for mob templates.
// Validation (template existence) runs whether or not a spawner is
// supplied so missing-id content surfaces in template-only loads.
// Spec mobs-ai-spawning §3.1.
func applyMobPlacements(ctx context.Context, dst *Registries, placements []pendingMobPlacement, spawner MobSpawner) error {
	for _, pl := range placements {
		if !dst.Mobs.Has(mob.TemplateID(pl.TemplateID)) {
			return fmt.Errorf("%w: room %q -> mob %q", ErrMissingMobTemplate, pl.RoomID, pl.TemplateID)
		}
		if spawner == nil {
			continue
		}
		if err := spawner.SpawnAndPlaceMob(ctx, pl.TemplateID, pl.RoomID); err != nil {
			return fmt.Errorf("placement room %q mob %q: %w", pl.RoomID, pl.TemplateID, err)
		}
	}
	return nil
}

// validateExits walks every exit and ensures the target room exists.
func validateExits(dst *world.World) error {
	for _, r := range dst.Rooms() {
		for dir, e := range r.Exits {
			if _, err := dst.Room(e.Target); err != nil {
				return fmt.Errorf("%w: room %q exit %s -> %q", ErrMissingExitRoom, r.ID, dir, e.Target)
			}
		}
	}
	return nil
}
