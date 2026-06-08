package pack

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/decoration"
	"github.com/Jasrags/AnotherMUD/internal/effect"
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/loot"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/property"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/render"
	"github.com/Jasrags/AnotherMUD/internal/script"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stats"
	"github.com/Jasrags/AnotherMUD/internal/weather"
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
	ErrItemUnknownSlot     = errors.New("item references unknown slot")
	ErrMissingDoorKey      = errors.New("door references unknown key template")
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

// ScriptCompiler is the M17.1b seam between the pack loader and the
// scripting runtime. The loader calls Compile on every discovered
// script body so syntax errors surface with pack + path attribution
// at boot, before the engine starts ticking. Defined here at the
// use site so the pack package doesn't import internal/scripting
// directly; the composition root passes a real
// *scripting.Engine.
//
// Nil-safe: when nil is passed to Load, scripts are still
// registered but NOT compile-checked — useful for tests that
// don't want to construct a scripting engine.
type ScriptCompiler interface {
	Compile(packID, scriptPath, script string) error
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
func Load(ctx context.Context, root string, filter []string, dst *Registries, spawner Spawner, mobSpawner MobSpawner, scriptCompiler ScriptCompiler) error {
	if dst == nil || dst.World == nil || dst.Items == nil || dst.Slots == nil || dst.Mobs == nil || dst.Tracks == nil || dst.Races == nil || dst.Classes == nil || dst.Abilities == nil || dst.Theme == nil || dst.Help == nil || dst.Quests == nil || dst.Weather == nil || dst.Scripts == nil || dst.Rarity == nil || dst.Essence == nil || dst.Loot == nil {
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
		pp, mp, err := loadPackContent(ctx, p, dst, scriptCompiler)
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

	// Room-coordinate derivation (room-coordinates §3) runs once the
	// graph is fully assembled and before the world serves connections.
	// Non-fatal by design (PD-4): collisions, non-square loops, and
	// unplaced rooms degrade the local map and surface as warnings —
	// they never abort the load.
	for _, cw := range dst.World.DeriveCoordinates() {
		logCoordWarning(logger, cw)
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

	// Item slot references (eligible + companion) must resolve against the
	// fully-populated slot registry. Runs after every pack has loaded so a
	// slot defined by a later pack is visible (mirrors validateSpawnRules).
	if err := validateItemSlots(dst); err != nil {
		return err
	}

	// Door key references (the item template id a keyed door requires)
	// must resolve in the item registry. Runs after every pack has loaded
	// so a cross-pack key (`other-pack:foo-key`) is visible regardless of
	// load order (mirrors validateItemSlots).
	if err := validateDoorKeys(dst); err != nil {
		return err
	}

	return nil
}

// DiscoverScripts re-runs ONLY the pack-discovery + script-glob +
// compile-check portion of Load and returns a fresh script.Registry.
// It performs NO content parsing, entity spawning, or world mutation,
// so unlike Load it is safe to call on a live server — the M17.3
// script hot-reload path uses it to re-read pack Lua from disk without
// disturbing world.World or the content registries.
//
// Pack discovery + dependency ordering are reused so a reloaded
// script's LoadOrder (and thus dispatch order) matches boot. A syntax
// error in any script surfaces here, via scriptCompiler, BEFORE the
// caller tears the running runtime down — so a bad edit leaves the
// live scripts untouched.
func DiscoverScripts(ctx context.Context, root string, filter []string, scriptCompiler ScriptCompiler) (*script.Registry, error) {
	discovered, err := Discover(root, filter)
	if err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	ordered, err := Order(discovered)
	if err != nil {
		return nil, fmt.Errorf("ordering: %w", err)
	}
	reg := script.New()
	for _, p := range ordered {
		scriptPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Scripts)
		if err != nil {
			return nil, fmt.Errorf("pack %q scripts: %w", p.Manifest.Name, err)
		}
		if err := loadScripts(p, reg, scriptCompiler, scriptPaths); err != nil {
			return nil, fmt.Errorf("pack %q: %w", p.Manifest.Name, err)
		}
	}
	return reg, nil
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

// validateItemSlots verifies that every item template's eligible and
// companion slot names resolve in the slot registry. Runs after all packs
// load so a slot defined by a later pack is visible. Boot-time validation
// (inventory-equipment-items §3.3) turns a slot-name typo into a precise
// load failure instead of a silently never-equippable item; the sentinel
// ErrItemUnknownSlot mirrors the spawn-rule validators.
func validateItemSlots(dst *Registries) error {
	for _, t := range dst.Items.All() {
		for _, name := range t.EligibleSlots {
			if !dst.Slots.Has(name) {
				return fmt.Errorf("%w: item %q eligible slot %q", ErrItemUnknownSlot, t.ID, name)
			}
		}
		for _, name := range t.CompanionSlots {
			if !dst.Slots.Has(name) {
				return fmt.Errorf("%w: item %q companion slot %q", ErrItemUnknownSlot, t.ID, name)
			}
		}
	}
	return nil
}

// validateDoorKeys walks every exit's door and verifies that a keyed
// door's KeyID resolves to a known item template. A door's key is the
// item template id the has-key check matches against (world-rooms-
// movement §5.3); an unknown key id produces a door that can never be
// unlocked — fail-silent at the unlock attempt today. Boot-time
// validation turns that into a precise load failure, consistent with
// the spec making an unknown room-item template id fatal (§2.2) and
// with validateItemSlots / validateSpawnRules. KeyID is already
// namespace-qualified at decode (buildDoorState), so cross-pack keys
// resolve here regardless of load order.
//
// Only r.Exits is walked: the loader attaches doors solely to
// direction-keyed exits (decodeRoom), never to KeywordExits, so there is
// no keyed door to miss on the keyword side.
func validateDoorKeys(dst *Registries) error {
	for _, r := range dst.World.Rooms() {
		for dir, e := range r.Exits {
			if e.Door == nil || e.Door.KeyID == "" {
				continue
			}
			if !dst.Items.Has(item.TemplateID(e.Door.KeyID)) {
				return fmt.Errorf("%w: room %q door %s key %q", ErrMissingDoorKey, r.ID, dir, e.Door.KeyID)
			}
		}
	}
	return nil
}

func loadPackContent(ctx context.Context, p Discovered, dst *Registries, scriptCompiler ScriptCompiler) ([]pendingPlacement, []pendingMobPlacement, error) {
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
	racePaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Races)
	if err != nil {
		return nil, nil, err
	}
	classPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Classes)
	if err != nil {
		return nil, nil, err
	}
	abilityPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Abilities)
	if err != nil {
		return nil, nil, err
	}
	themePaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Theme)
	if err != nil {
		return nil, nil, err
	}
	helpPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Help)
	if err != nil {
		return nil, nil, err
	}
	questPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Quests)
	if err != nil {
		return nil, nil, err
	}
	effectPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Effects)
	if err != nil {
		return nil, nil, err
	}
	weatherZonePaths, err := resolveGlobs(p.Dir, p.Manifest.Content.WeatherZones)
	if err != nil {
		return nil, nil, err
	}
	scriptPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Scripts)
	if err != nil {
		return nil, nil, err
	}
	rarityPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Rarity)
	if err != nil {
		return nil, nil, err
	}
	essencePaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Essence)
	if err != nil {
		return nil, nil, err
	}
	lootPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.LootTables)
	if err != nil {
		return nil, nil, err
	}
	recipePaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Recipes)
	if err != nil {
		return nil, nil, err
	}

	// M17.1b: discover, compile-check, and register pack scripts.
	// Compile-check at boot is the cheapest place to surface a syntax
	// error in pack-authored Lua — the alternative is to discover at
	// load and crash at first event delivery. Done early in the
	// content pass so a broken script aborts before anything else
	// commits to disk.
	if err := loadScripts(p, dst.Scripts, scriptCompiler, scriptPaths); err != nil {
		return nil, nil, err
	}
	if len(scriptPaths) > 0 {
		logger.Info("pack scripts loaded",
			slog.String("event", "pack.scripts"),
			slog.Int("count", len(scriptPaths)))
	}

	// Weather zones load BEFORE areas so an area's `weather_zone`
	// reference resolves cleanly during area decoding. (Strictly the
	// loader only validates the reference at composition time
	// today, but the load-order is the more readable invariant —
	// "things that own ids load before things that reference them".)
	for _, wzp := range weatherZonePaths {
		z, err := decodeWeatherZone(wzp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Weather.Add(z); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, wzp)
		}
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
		r, items, mobs, err := decodeRoom(ctx, rp, ns)
		if err != nil {
			return nil, nil, err
		}
		// M14.5: validate Properties against the property registry
		// before the room is committed to the world. Snake-case,
		// known-name, and type-match are all enforced here so a
		// content error surfaces at boot rather than at first read.
		if err := validateRoomProperties(r, dst.Properties, ns); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, rp)
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

	// Races: id-keyed registry, case-insensitive lookups, same
	// priority-based override semantics (spec progression.md §3.2).
	for _, rp := range racePaths {
		r, err := decodeRace(rp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Races.Register(r); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, rp)
		}
	}

	// Classes: id-keyed registry mirroring races. Stat-growth dice
	// expressions are parsed at decode so a malformed `1d8` surfaces
	// at load rather than at first level-up (spec progression.md §4.1).
	for _, cp := range classPaths {
		c, err := decodeClass(cp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Classes.Register(c); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, cp)
		}
	}

	// Abilities: id-keyed registry with priority-based override.
	// Ids are NOT namespaced (mirrors the slot registry); a pack that
	// wants to replace a baseline ability sets Priority higher than
	// the existing entry (spec abilities-and-effects §2.1).
	for _, ap := range abilityPaths {
		a, err := decodeAbility(ap, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Abilities.Register(a); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, ap)
		}
	}

	// Theme: global (not namespaced) semantic-tag → color map. Later
	// packs override earlier entries by tag name (Register replaces),
	// mirroring the theme spec's "downstream pack can re-theme" rule.
	// The composition root compiles the registry once after Load.
	for _, tp := range themePaths {
		entries, err := decodeTheme(tp)
		if err != nil {
			return nil, nil, err
		}
		for tag, e := range entries {
			dst.Theme.Register(tag, e)
		}
	}

	// Rarity tiers + essences: global (not namespaced) decoration
	// vocabularies, later-wins like the theme. Keys are validated at this
	// boundary (decoration.ValidateKey) — a key with markup-significant
	// characters would produce broken render tags, so a bad key fails the
	// boot loudly with pack + path attribution rather than rendering wrong.
	for _, rp := range rarityPaths {
		tiers, err := decodeRarity(rp)
		if err != nil {
			return nil, nil, err
		}
		for _, t := range tiers {
			if err := decoration.ValidateKey(t.Key); err != nil {
				return nil, nil, fmt.Errorf("%w (in %s)", err, rp)
			}
			dst.Rarity.Register(t)
		}
	}
	for _, ep := range essencePaths {
		essences, err := decodeEssence(ep)
		if err != nil {
			return nil, nil, err
		}
		for _, e := range essences {
			if err := decoration.ValidateKey(e.Key); err != nil {
				return nil, nil, fmt.Errorf("%w (in %s)", err, ep)
			}
			dst.Essence.Register(e)
		}
	}

	// Loot tables: id-keyed registry with priority-based override
	// (M22.1 — mobs-ai-spawning §6.3). A mob template's loot_table
	// references a table by id; the spawn pipeline rolls it into the
	// mob's contents. Item ids inside the table are NOT validated here
	// — resolution is fail-silent at spawn (consistent with mob race/
	// class), so a typo'd item id simply produces no drop rather than
	// aborting the boot.
	for _, lp := range lootPaths {
		tbl, err := decodeLootTable(lp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Loot.Register(tbl); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, lp)
		}
	}

	// Recipes: namespace-scoped crafting recipes (crafting-and-cooking
	// §3). TryAdd guards cross-pack id collisions like items/mobs. Input/
	// output item ids and the discipline id are NOT validated here —
	// resolution is fail-soft (consistent with loot tables and mob
	// race/class), so a typo'd reference simply makes the recipe
	// uncraftable rather than aborting the boot.
	for _, rp := range recipePaths {
		r, err := decodeRecipe(rp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Recipes.TryAdd(r); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, rp)
		}
	}

	// Help: per-pack topics (spec ui-rendering-help §9.2). Topics are
	// registered with the pack's load order so a higher-order pack can
	// override an upstream topic. PackName is the pack namespace so the
	// namespaced id matches the room/item convention. Topics missing id
	// or title are skipped with a warn rather than failing the boot.
	helpTopics := 0
	for _, hp := range helpPaths {
		topics, err := decodeHelp(hp, ns)
		if err != nil {
			return nil, nil, err
		}
		for _, t := range topics {
			// Validate here so the warn fires only on genuinely invalid
			// topics. A false from AddTopic alone is ambiguous — it also
			// means "a higher load-order topic already won", which is the
			// normal, expected outcome when packs shadow each other and
			// must not be logged as an error.
			if strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Title) == "" {
				logger.Warn("skipping help topic missing id/title",
					slog.String("event", "pack.help.skip"),
					slog.String("file", hp),
					slog.String("topic_id", t.ID))
				continue
			}
			if dst.Help.AddTopic(t, p.Manifest.LoadOrder) {
				helpTopics++
			}
		}
	}

	// Quests: id-keyed registry (spec quests.md §2). Later registrations
	// replace earlier ones. decodeQuest namespaces the giver, objective
	// targets/npcs, prereq quest ids, and reward item ids against the
	// pack namespace; Register validates + normalizes objective ids.
	for _, qp := range questPaths {
		d, err := decodeQuest(qp, ns, p.Dir)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Quests.Register(d); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, qp)
		}
	}

	// Effects: id-keyed registry (M14.2). Ids are case-insensitive;
	// duplicate registration across packs is a load-time error.
	for _, ep := range effectPaths {
		data, err := os.ReadFile(ep)
		if err != nil {
			return nil, nil, fmt.Errorf("read effect %s: %w", ep, err)
		}
		tpl, err := effect.Decode(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, ep)
		}
		if dst.Effects != nil {
			if err := dst.Effects.Register(tpl); err != nil {
				return nil, nil, fmt.Errorf("%w (in %s)", err, ep)
			}
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
		slog.Int("races", len(racePaths)),
		slog.Int("classes", len(classPaths)),
		slog.Int("abilities", len(abilityPaths)),
		slog.Int("theme", len(themePaths)),
		slog.Int("help", helpTopics),
		slog.Int("quests", len(questPaths)),
		slog.Int("effects", len(effectPaths)),
		slog.Int("rarity", len(rarityPaths)),
		slog.Int("essence", len(essencePaths)),
		slog.Int("loot_tables", len(lootPaths)),
		slog.Int("placements", len(placements)),
		slog.Int("mob_placements", len(mobPlacements)),
	)
	return placements, mobPlacements, nil
}

// normalizeLowerDedup lowercases + trims + dedups a YAML string list,
// returning nil when nothing survives. Shared by ability target_types
// and item eligible/companion slot decoding so each value is inspectable
// at decode time without relying on a registry to re-normalize.
func normalizeLowerDedup(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, t := range in {
		n := strings.ToLower(strings.TrimSpace(t))
		if n == "" {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// decodeAbility reads an AbilityFile and builds a progression.Ability
// (spec abilities-and-effects §2). Required: id, type, category.
// Type and category strings are validated via the progression parsers
// so unknown values surface at load with a precise error rather than
// silently registering a malformed entry that the registry would then
// reject.
func decodeAbility(path, ns string) (*progression.Ability, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading ability %s: %w", path, err)
	}
	var f AbilityFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	typ, ok := progression.ParseAbilityType(f.Type)
	if !ok {
		return nil, fmt.Errorf("%w: %s: 'type' must be active or passive (got %q)", ErrInvalidContent, path, f.Type)
	}
	cat, ok := progression.ParseAbilityCategory(f.Category)
	if !ok {
		return nil, fmt.Errorf("%w: %s: 'category' must be skill or spell (got %q)", ErrInvalidContent, path, f.Category)
	}
	display := strings.TrimSpace(f.Name)
	if display == "" {
		display = strings.TrimSpace(f.ID)
	}

	// Dead-passive guard (spec §6.1): a passive whose §6.1 binary
	// check would always evaluate to zero can never fire. The
	// variance >= 100 branch uses max_hit_chance; with max_hit_chance
	// 0 the effective chance is prof × 0/100 = 0 forever. Reject at
	// load so a content author sees the mistake instead of shipping a
	// silently-inert passive. (Active abilities fall back to the
	// resolver's DefaultMaxHitChance, so this only applies to passives.)
	if typ == progression.AbilityPassive && f.Variance >= 100 && f.MaxHitChance == 0 {
		return nil, fmt.Errorf("%w: %s: passive with variance>=100 requires max_hit_chance>0 (else its §6.1 binary check never fires)", ErrInvalidContent, path)
	}

	// Alignment range: at least one bound set ⇒ HasAlignmentRange.
	// Missing-side defaults to the extreme so the range is open.
	var (
		hasAlignRange bool
		alignMin      int
		alignMax      int
	)
	if f.AlignmentMin != nil || f.AlignmentMax != nil {
		hasAlignRange = true
		alignMin = math.MinInt
		alignMax = math.MaxInt
		if f.AlignmentMin != nil {
			alignMin = *f.AlignmentMin
		}
		if f.AlignmentMax != nil {
			alignMax = *f.AlignmentMax
		}
	}

	// Effect template: decode modifiers; empty id is an authoring
	// error because the single-instance + removal paths key on id.
	var effect *progression.EffectTemplate
	if f.Effect != nil {
		if strings.TrimSpace(f.Effect.ID) == "" {
			return nil, fmt.Errorf("%w: %s: effect.id required when effect block is present", ErrInvalidContent, path)
		}
		mods := make([]stats.Modifier, 0, len(f.Effect.Modifiers))
		for i, m := range f.Effect.Modifiers {
			if strings.TrimSpace(m.Stat) == "" {
				return nil, fmt.Errorf("%w: %s: effect.modifiers[%d] missing 'stat'", ErrInvalidContent, path, i)
			}
			mods = append(mods, stats.Modifier{Stat: m.Stat, Value: m.Value})
		}
		var flags []string
		if len(f.Effect.Flags) > 0 {
			flags = make([]string, 0, len(f.Effect.Flags))
			for _, fl := range f.Effect.Flags {
				if t := strings.TrimSpace(fl); t != "" {
					flags = append(flags, t)
				}
			}
		}
		effect = &progression.EffectTemplate{
			ID:        f.Effect.ID,
			Duration:  f.Effect.Duration,
			Modifiers: mods,
			Flags:     flags,
		}
	}

	return &progression.Ability{
		ID:                    f.ID,
		DisplayName:           display,
		Type:                  typ,
		Category:              cat,
		DefaultCap:            f.DefaultCap,
		GainBaseChance:        f.GainBaseChance,
		GainFailureMultiplier: f.GainFailureMultiplier,
		GainStat:              progression.StatType(strings.TrimSpace(f.GainStat)),
		GainStatScale:         f.GainStatScale,
		Cost:                  f.Cost,
		PulseDelay:            f.PulseDelay,
		InitiateOnly:          f.InitiateOnly,
		TargetTypes:           normalizeLowerDedup(f.TargetTypes),
		EquipmentSlot:         strings.ToLower(strings.TrimSpace(f.EquipmentSlot)),
		EquipmentTag:          strings.ToLower(strings.TrimSpace(f.EquipmentTag)),
		Variance:              f.Variance,
		MaxHitChance:          f.MaxHitChance,
		HandlerToken:          strings.ToLower(strings.TrimSpace(f.Handler)),
		DamageDice:            strings.TrimSpace(f.Damage),
		HealDice:              strings.TrimSpace(f.Heal),
		Hook:                  strings.ToLower(strings.TrimSpace(f.Hook)),
		MaxBonus:              f.MaxBonus,
		HasAlignmentRange:     hasAlignRange,
		AlignmentMin:          alignMin,
		AlignmentMax:          alignMax,
		Effect:                effect,
		Pack:                  ns,
		Priority:              f.Priority,
	}, nil
}

// decodeTheme reads a ThemeFile and returns its tag → render.ThemeEntry
// map (spec ui-rendering-help §3.1). Color name validity is NOT checked
// here: an unrecognized fg/bg simply resolves to no SGR at Compile (the
// entry becomes declared-but-color-less), so a typo degrades to plain
// output rather than failing the boot. Entries with a blank tag name
// are skipped.
func decodeTheme(path string) (map[string]render.ThemeEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading theme %s: %w", path, err)
	}
	var f ThemeFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	out := make(map[string]render.ThemeEntry, len(f.Tags))
	for tag, e := range f.Tags {
		// Trim before keying: the registry lower-cases but does not trim
		// on lookup, so a whitespace-padded tag would register an entry
		// the renderer could never resolve.
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		out[tag] = render.ThemeEntry{FG: e.FG, BG: e.BG, HTML: e.HTML}
	}
	return out, nil
}

// decodeRarity reads a RarityFile and returns its tiers as
// decoration.Tier values (spec item-decorations §2). Color name validity
// is not checked here — like the theme, an unrecognized fg/bg degrades to
// no color at Compile rather than failing the boot. Key validity IS the
// caller's concern (decoration.ValidateKey at the load boundary).
func decodeRarity(path string) ([]decoration.Tier, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading rarity %s: %w", path, err)
	}
	var f RarityFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	out := make([]decoration.Tier, 0, len(f.Tiers))
	for _, t := range f.Tiers {
		out = append(out, decoration.Tier{
			Key:     t.Key,
			Order:   t.Order,
			Display: t.Display,
			Left:    t.Left,
			Right:   t.Right,
			Color:   render.ThemeEntry{FG: t.FG, BG: t.BG, HTML: t.HTML},
			Visible: t.Visible,
		})
	}
	return out, nil
}

// decodeEssence reads an EssenceFile and returns its essences as
// decoration.Essence values (spec item-decorations §3). Same color/key
// handling as decodeRarity.
func decodeEssence(path string) ([]decoration.Essence, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading essence %s: %w", path, err)
	}
	var f EssenceFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	out := make([]decoration.Essence, 0, len(f.Essences))
	for _, e := range f.Essences {
		out = append(out, decoration.Essence{
			Key:   e.Key,
			Glyph: e.Glyph,
			Color: render.ThemeEntry{FG: e.FG, BG: e.BG, HTML: e.HTML},
		})
	}
	return out, nil
}

// decodeLootTable reads a LootTableFile and builds a *loot.Table
// (M22.1 — mobs-ai-spawning §6.3). The table's own id and every item id
// it references are namespace-qualified against ns (unqualified names
// resolve to this pack; qualified `pack:name` forms pass through), so
// they match the namespaced keys the item registry stores and the
// runtime resolves. Item *existence* is NOT checked here — resolution is
// fail-silent at spawn — but a missing/malformed id IS rejected so a
// blank entry can't silently become a no-drop.
func decodeLootTable(path, ns string) (*loot.Table, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading loot table %s: %w", path, err)
	}
	var f LootTableFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	id, err := qualifyID(f.ID, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: id: %v", ErrInvalidContent, path, err)
	}
	t := &loot.Table{
		ID:        id,
		Priority:  f.Priority,
		PoolRolls: f.PoolRolls,
	}
	for i, g := range f.Guaranteed {
		qid, err := qualifyID(g.Item, ns)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: guaranteed[%d].item: %v", ErrInvalidContent, path, i, err)
		}
		t.Guaranteed = append(t.Guaranteed, loot.GuaranteedEntry{ItemID: qid, Count: g.Count})
	}
	for i, w := range f.Weighted {
		qid, err := qualifyID(w.Item, ns)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: weighted[%d].item: %v", ErrInvalidContent, path, i, err)
		}
		t.Weighted = append(t.Weighted, loot.WeightedEntry{ItemID: qid, Weight: w.Weight})
	}
	if f.RareBonus != nil {
		rb := &loot.RareBonus{Chance: f.RareBonus.Chance}
		for i, e := range f.RareBonus.Entries {
			qid, err := qualifyID(e.Item, ns)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: rare_bonus.entries[%d].item: %v", ErrInvalidContent, path, i, err)
			}
			rb.Entries = append(rb.Entries, loot.WeightedEntry{ItemID: qid, Weight: e.Weight})
		}
		t.RareBonus = rb
	}
	if f.Coin != nil {
		t.Coin = &loot.CoinBlock{Min: f.Coin.Min, Max: f.Coin.Max}
	}
	return t, nil
}

// decodeHelp reads a HelpFile and builds help.Topic values (spec
// ui-rendering-help §9.1), setting PackName to the pack namespace. Field
// validity (required id/title) is enforced by the help service's
// AddTopic, which the loader calls so it can warn-and-skip; decodeHelp
// only translates the YAML shape.
func decodeHelp(path, ns string) ([]*help.Topic, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading help %s: %w", path, err)
	}
	var f HelpFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	out := make([]*help.Topic, 0, len(f.Topics))
	for _, tf := range f.Topics {
		out = append(out, &help.Topic{
			ID:       tf.ID,
			Title:    tf.Title,
			Category: tf.Category,
			Brief:    tf.Brief,
			Body:     tf.Body,
			Syntax:   tf.Syntax,
			Keywords: tf.Keywords,
			SeeAlso:  tf.SeeAlso,
			Role:     help.ParseRole(tf.Role),
			PackName: ns,
		})
	}
	return out, nil
}

// decodeQuest reads a QuestFile and builds a quest.Definition (spec
// quests.md §2). Ids that reference world content — the giver, objective
// targets/npcs, prereq quest ids, and reward item template ids — are
// namespace-qualified against ns so they match the namespaced ids the
// runtime stores (qualified `pack:id` forms pass through).
//
// Ability, class (reward + prereq), and race ids are left bare BY
// DESIGN: those registries are engine-global / not namespaced (the
// ability registry mirrors the slot registry; classes/races are
// id-keyed the same way), so a quest references them by the same bare id
// the registry uses. abandonable defaults to true when absent.
func decodeQuest(path, ns, packDir string) (*quest.Definition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading quest %s: %w", path, err)
	}
	var f QuestFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}

	id, err := qualifyID(f.ID, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: id: %v", ErrInvalidContent, path, err)
	}
	giver, err := qualifyOptional(f.Giver, ns, path, "giver")
	if err != nil {
		return nil, err
	}

	stages := make([]quest.Stage, 0, len(f.Stages))
	for si, sf := range f.Stages {
		objs := make([]quest.Objective, 0, len(sf.Objectives))
		for oi, of := range sf.Objectives {
			target, err := qualifyOptional(of.Target, ns, path, fmt.Sprintf("stage[%d].objective[%d].target", si, oi))
			if err != nil {
				return nil, err
			}
			npc, err := qualifyOptional(of.NPC, ns, path, fmt.Sprintf("stage[%d].objective[%d].npc", si, oi))
			if err != nil {
				return nil, err
			}
			objs = append(objs, quest.Objective{
				ID:          of.ID,
				Type:        of.Type,
				Target:      target,
				NPC:         npc,
				Count:       of.Count,
				Description: of.Description,
			})
		}
		stages = append(stages, quest.Stage{
			ID:          sf.ID,
			Description: sf.Description,
			Hint:        sf.Hint,
			Objectives:  objs,
		})
	}

	prereqDone, err := qualifyQuestIDs(f.Prerequisite.QuestsCompleted, ns, path, "quests_completed")
	if err != nil {
		return nil, err
	}
	prereqNotDone, err := qualifyQuestIDs(f.Prerequisite.QuestsNotCompleted, ns, path, "quests_not_completed")
	if err != nil {
		return nil, err
	}
	rewardItems, err := qualifyIDList(f.Reward.Items, ns, path, "reward.items")
	if err != nil {
		return nil, err
	}

	abandonable := f.Abandonable == nil || *f.Abandonable

	return &quest.Definition{
		ID:             id,
		Name:           f.Name,
		Classification: f.Classification,
		Giver:          giver,
		Offer:          f.Offer,
		TurnIn:         f.TurnIn,
		Repeatable:     f.Repeatable,
		Abandonable:    abandonable,
		Secret:         f.Secret,
		Prereq: quest.Prerequisite{
			MinLevel:           f.Prerequisite.MinLevel,
			Class:              f.Prerequisite.Class,
			QuestsCompleted:    prereqDone,
			QuestsNotCompleted: prereqNotDone,
		},
		Stages: stages,
		Reward: quest.Reward{
			XP:          f.Reward.XP,
			Gold:        f.Reward.Gold,
			Items:       rewardItems,
			Abilities:   f.Reward.Abilities,
			ClassUnlock: f.Reward.ClassUnlock,
			RaceUnlock:  f.Reward.RaceUnlock,
		},
		Script:  f.Script,
		PackDir: packDir,
	}, nil
}

// qualifyOptional namespace-qualifies id when non-empty; an empty id
// returns "" without error (the field is optional).
func qualifyOptional(id, ns, path, field string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", nil
	}
	q, err := qualifyID(id, ns)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %s: %v", ErrInvalidContent, path, field, err)
	}
	return q, nil
}

// qualifyQuestIDs namespace-qualifies a list of quest ids (prereq
// references). Empty/absent lists return nil (not an empty slice) so the
// Prerequisite block's optional fields stay nil when unset. Empty
// entries within a non-empty list are an authoring error.
func qualifyQuestIDs(ids []string, ns, path, field string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return qualifyIDList(ids, ns, path, field)
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

// decodeRace reads a RaceFile and builds a progression.Race
// (spec progression.md §3.1). The id is required and lowercased
// at registration. Stat-cap keys are normalized to lowercase
// StatType strings; an empty stat name in the map errors out so
// authoring typos surface at load.
func decodeRace(path, ns string) (*progression.Race, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading race %s: %w", path, err)
	}
	var f RaceFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}

	var caps map[progression.StatType]int
	if len(f.StatCaps) > 0 {
		caps = make(map[progression.StatType]int, len(f.StatCaps))
		for k, v := range f.StatCaps {
			key := strings.ToLower(strings.TrimSpace(k))
			if key == "" {
				return nil, fmt.Errorf("%w: %s: stat_caps has empty key", ErrInvalidContent, path)
			}
			if v < 0 {
				return nil, fmt.Errorf("%w: %s: stat_caps[%q] = %d must be >= 0", ErrInvalidContent, path, key, v)
			}
			caps[progression.StatType(key)] = v
		}
	}

	var flags []string
	if len(f.RacialFlags) > 0 {
		flags = make([]string, 0, len(f.RacialFlags))
		for _, t := range f.RacialFlags {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			flags = append(flags, t)
		}
	}

	return &progression.Race{
		ID:                f.ID,
		DisplayName:       strings.TrimSpace(f.Name),
		Tagline:           f.Tagline,
		Description:       f.Description,
		Category:          strings.TrimSpace(f.Category),
		StartingAlignment: f.StartingAlignment,
		StatCaps:          caps,
		CastCostModifier:  f.CastCostModifier,
		RacialFlags:       flags,
		Pack:              ns,
		Priority:          f.Priority,
	}, nil
}

// decodeClass reads a ClassFile and builds a progression.Class.
// Spec progression.md §4.1. The id is required; stat-growth dice are
// parsed eagerly via combat.ParseDice so authoring errors surface at
// load time rather than at first level-up. Stat keys are lowercased
// to align with the runtime StatType convention.
func decodeClass(path, ns string) (*progression.Class, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading class %s: %w", path, err)
	}
	var f ClassFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	var growth map[progression.StatType]combat.DiceExpr
	if len(f.StatGrowth) > 0 {
		growth = make(map[progression.StatType]combat.DiceExpr, len(f.StatGrowth))
		for k, expr := range f.StatGrowth {
			key := strings.ToLower(strings.TrimSpace(k))
			if key == "" {
				return nil, fmt.Errorf("%w: %s: stat_growth has empty key", ErrInvalidContent, path)
			}
			d, err := combat.ParseDice(expr)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: stat_growth[%q]: %v", ErrInvalidContent, path, key, err)
			}
			growth[progression.StatType(key)] = d
		}
	}
	var bonuses map[progression.StatType]progression.StatType
	if len(f.GrowthBonuses) > 0 {
		bonuses = make(map[progression.StatType]progression.StatType, len(f.GrowthBonuses))
		for k, v := range f.GrowthBonuses {
			key := strings.ToLower(strings.TrimSpace(k))
			src := strings.ToLower(strings.TrimSpace(v))
			if key == "" || src == "" {
				return nil, fmt.Errorf("%w: %s: growth_bonuses[%q]=%q: empty key or source", ErrInvalidContent, path, k, v)
			}
			bonuses[progression.StatType(key)] = progression.StatType(src)
		}
	}
	var path2 []progression.ClassPathEntry
	if len(f.Path) > 0 {
		path2 = make([]progression.ClassPathEntry, 0, len(f.Path))
		for i, e := range f.Path {
			if e.Level <= 0 {
				return nil, fmt.Errorf("%w: %s: path[%d]: level must be > 0 (got %d)", ErrInvalidContent, path, i, e.Level)
			}
			if strings.TrimSpace(e.AbilityID) == "" {
				return nil, fmt.Errorf("%w: %s: path[%d]: missing 'ability'", ErrInvalidContent, path, i)
			}
			path2 = append(path2, progression.ClassPathEntry{
				Level:       e.Level,
				AbilityID:   strings.TrimSpace(e.AbilityID),
				UnlockedVia: strings.TrimSpace(e.UnlockedVia),
			})
		}
	}
	// trains_per_level: zero in YAML defaults to 5 (spec §4.1). A
	// negative value clamps to 0 at Register; we let the loader pass
	// it through so authors who explicitly write `-1` see the same
	// result (no surprise mid-layer mutation).
	trains := f.TrainsPerLevel
	if trains == 0 {
		trains = 5
	}
	return &progression.Class{
		ID:                f.ID,
		DisplayName:       strings.TrimSpace(f.Name),
		Tagline:           f.Tagline,
		Description:       f.Description,
		LevelUpFlavor:     f.LevelUpFlavor,
		BoundTrack:        strings.TrimSpace(f.BoundTrack),
		StatGrowth:        growth,
		GrowthBonuses:     bonuses,
		Path:              path2,
		TrainsPerLevel:    trains,
		AllowedCategories: append([]string(nil), f.AllowedCategories...),
		AllowedGenders:    append([]string(nil), f.AllowedGenders...),
		StartingAlignment: f.StartingAlignment,
		Pack:              ns,
		Priority:          f.Priority,
	}, nil
}

// resolveGlobs expands each pattern (relative to packDir) into matching
// files. Sorted for deterministic load order. Missing patterns surface
// as errors so authors notice typos.
//
// loadScripts reads each scriptPath, compile-checks the source via
// scriptCompiler (when supplied), and registers a script.Entry on
// dst.Scripts. Returns the first error encountered so a broken
// script aborts pack load at the same point YAML errors do.
//
// scriptCompiler may be nil — production wires a real
// scripting.Engine, tests typically pass nil and lean on integration
// tests to cover the compile-check seam.
//
// Each script's relative path inside the pack is captured as the
// Entry.Path; the runtime uses that for logging and error
// attribution at execution time (M17.1c). Source is the raw text
// read from disk — gopher-lua handles its own line-counting for
// error messages.
func loadScripts(p Discovered, reg *script.Registry, scriptCompiler ScriptCompiler, scriptPaths []string) error {
	if len(scriptPaths) == 0 {
		return nil
	}
	packAbs, err := filepath.Abs(p.Dir)
	if err != nil {
		return fmt.Errorf("resolving pack dir for scripts: %w", err)
	}
	ns := p.Namespace()
	for _, sp := range scriptPaths {
		source, err := os.ReadFile(sp)
		if err != nil {
			return fmt.Errorf("reading script %s: %w", sp, err)
		}
		// Path stored in the registry is relative to the pack
		// directory so the runtime's error messages match what a
		// pack author sees in their content tree.
		relPath, relErr := filepath.Rel(packAbs, sp)
		if relErr != nil {
			relPath = filepath.Base(sp)
		}
		relPath = filepath.ToSlash(relPath)

		if scriptCompiler != nil {
			if err := scriptCompiler.Compile(ns, relPath, string(source)); err != nil {
				return fmt.Errorf("compile %s: %w", sp, err)
			}
		}
		entry := script.Entry{
			PackID:    ns,
			Path:      relPath,
			Source:    string(source),
			LoadOrder: p.Manifest.LoadOrder,
		}
		if err := reg.Register(entry); err != nil {
			return fmt.Errorf("registering script %s: %w", sp, err)
		}
	}
	return nil
}

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
	// M15.4b₂a: qualify the weather_zone reference at decode time so
	// the runtime Area carries the same fully-qualified form the
	// service will look up. Empty stays empty (no weather).
	zoneID := ""
	if z := strings.TrimSpace(af.WeatherZone); z != "" {
		zoneID, err = qualifyID(z, ns)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: weather_zone: %v", ErrInvalidContent, path, err)
		}
	}
	return &world.Area{
		ID:            world.AreaID(id),
		Name:          af.Name,
		Description:   af.Description,
		ResetInterval: af.ResetInterval,
		SpawnRules:    rules,
		WeatherZone:   zoneID,
	}, nil
}

// decodeWeatherZone reads a WeatherZoneFile and returns the runtime
// *weather.Zone. Bare ids in transitions are qualified against the
// current pack namespace; fully-qualified `pack:state` form (rare —
// states are mostly bare strings like "rain") is preserved verbatim.
// Terrain keys and period names are passed through as-is.
//
// Validation enforced at decode:
//   - id required + qualifiable
//   - transition weights MUST be > 0
//   - roll_interval_hours MUST NOT be negative
//
// Soft validation deferred to runtime (matches the rest of the
// loader's "load fast, validate lazily" stance):
//   - a transition naming an unknown next-state surfaces only when
//     the roll lands on it (the resolver hits an empty row and
//     becomes a no-op rather than erroring).
func decodeWeatherZone(path, ns string) (*weather.Zone, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading weather zone %s: %w", path, err)
	}
	var wf WeatherZoneFile
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(wf.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	if wf.RollIntervalHours < 0 {
		return nil, fmt.Errorf("%w: %s: roll_interval_hours must be >= 0 (got %d)",
			ErrInvalidContent, path, wf.RollIntervalHours)
	}
	id, err := qualifyID(wf.ID, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	z := &weather.Zone{
		ID:                id,
		InitialState:      strings.TrimSpace(wf.InitialState),
		RollIntervalHours: wf.RollIntervalHours,
	}
	if len(wf.Transitions) > 0 {
		z.Transitions = make(map[string][]weather.TransitionWeight, len(wf.Transitions))
		for state, row := range wf.Transitions {
			out := make([]weather.TransitionWeight, 0, len(row))
			for i, tw := range row {
				next := strings.TrimSpace(tw.Next)
				if next == "" {
					return nil, fmt.Errorf("%w: %s: transitions[%s][%d]: missing 'next'",
						ErrInvalidContent, path, state, i)
				}
				if tw.Weight <= 0 {
					return nil, fmt.Errorf("%w: %s: transitions[%s][%d]: weight must be > 0 (got %d)",
						ErrInvalidContent, path, state, i, tw.Weight)
				}
				out = append(out, weather.TransitionWeight{NextState: next, Weight: tw.Weight})
			}
			z.Transitions[state] = out
		}
	}
	if len(wf.WeatherMessages) > 0 {
		z.WeatherMessages = make(map[string]map[string]weather.MessageTriple, len(wf.WeatherMessages))
		for state, terrains := range wf.WeatherMessages {
			perTerrain := make(map[string]weather.MessageTriple, len(terrains))
			for terrain, triple := range terrains {
				perTerrain[terrain] = weather.MessageTriple{
					Start:   triple.Start,
					Ongoing: triple.Ongoing,
					End:     triple.End,
				}
			}
			z.WeatherMessages[state] = perTerrain
		}
	}
	if len(wf.TimeMessages) > 0 {
		z.TimeMessages = make(map[string]map[string]string, len(wf.TimeMessages))
		for period, terrains := range wf.TimeMessages {
			out := make(map[string]string, len(terrains))
			for terrain, msg := range terrains {
				out[terrain] = msg
			}
			z.TimeMessages[period] = out
		}
	}
	return z, nil
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

// logCoordWarning emits one non-fatal coordinate-derivation finding
// (room-coordinates §4) as a structured warning. The event name encodes
// the kind (pack.coord.<kind>) so operators can filter; fields follow
// the F2 logging convention.
func logCoordWarning(logger *slog.Logger, w world.CoordWarning) {
	attrs := []any{
		slog.String("event", "pack.coord."+string(w.Kind)),
		slog.String("area", string(w.Area)),
		slog.String("room", string(w.Room)),
	}
	if w.Other != "" {
		attrs = append(attrs, slog.String("other_room", string(w.Other)))
	}
	switch w.Kind {
	case world.CoordWarnInconsistent:
		attrs = append(attrs,
			slog.String("direction", w.Dir.String()),
			slog.String("existing", coordString(w.At)),
			slog.String("expected", coordString(w.Expect)))
	case world.CoordWarnUnplaced:
		// No coordinate — the room was never placed.
	default:
		attrs = append(attrs, slog.String("coord", coordString(w.At)))
	}
	logger.Warn("room coordinate derivation conflict", attrs...)
}

// coordString renders a coordinate for log output.
func coordString(c world.Coord) string {
	return fmt.Sprintf("(%d,%d,%d)", c.X, c.Y, c.Z)
}

// pinFromFile converts an authored CoordFile to a world.Coord. It
// reports ok=false when any axis is absent (a malformed pin per
// room-coordinates §3.5); the caller warns and falls back to derived
// placement.
func pinFromFile(cf *CoordFile) (world.Coord, bool) {
	if cf == nil || cf.X == nil || cf.Y == nil || cf.Z == nil {
		return world.Coord{}, false
	}
	return world.Coord{X: *cf.X, Y: *cf.Y, Z: *cf.Z}, true
}

func decodeRoom(ctx context.Context, path, ns string) (*world.Room, []string, []string, error) {
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
		ID:             world.RoomID(roomID),
		AreaID:         world.AreaID(areaID),
		Name:           rf.Name,
		Description:    rf.Description,
		Exits:          make(map[world.Direction]world.Exit, len(rf.Exits)),
		HealingRate:    rf.HealingRate,
		Tags:           append([]string(nil), rf.Tags...),
		Properties:     copyProperties(rf.Properties),
		Terrain:        strings.TrimSpace(rf.Terrain),
		WeatherExposed: rf.WeatherExposed,
		TimeExposed:    rf.TimeExposed,
	}
	// Coordinate pin (room-coordinates §3.5). A well-formed pin becomes
	// ground truth for derivation; a malformed one (any missing axis)
	// warns and falls back to derived placement — it never aborts the
	// load (§3.5 acceptance).
	if rf.Coord != nil {
		if c, ok := pinFromFile(rf.Coord); ok {
			r.Pin = &c
		} else {
			logging.From(ctx).Warn("malformed room coordinate pin; falling back to derived placement",
				slog.String("event", "pack.coord.malformed_pin"),
				slog.String("room", roomID),
				slog.String("file", path))
		}
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

	// M15.1: decode doors, attach to the matching exit. Each door key
	// MUST name a direction that already exists in Exits — a door
	// without an exit is content authoring error caught at load.
	for dirStr, df := range rf.Doors {
		dir, ok := world.ParseDirection(dirStr)
		if !ok {
			return nil, nil, nil, fmt.Errorf("%w: %s: door direction %q is not a Direction",
				ErrInvalidContent, path, dirStr)
		}
		exit, hasExit := r.Exits[dir]
		if !hasExit {
			return nil, nil, nil, fmt.Errorf("%w: %s: door %s has no matching exit",
				ErrInvalidContent, path, dirStr)
		}
		door, err := buildDoorState(df, ns)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%w: %s: door %s: %v", ErrInvalidContent, path, dirStr, err)
		}
		exit.Door = door
		r.Exits[dir] = exit
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

	// Validate the weapon-damage dice at load so a malformed expression
	// fails the pack by file name (combat §4.5) rather than silently
	// falling back to unarmed at the first swing. The trimmed string is
	// stored on the template; the dice are parsed into a typed expression
	// when the instance is built (entities.ItemInstance).
	weaponDamage := strings.TrimSpace(f.WeaponDamage)
	if weaponDamage != "" {
		if _, derr := combat.ParseDice(weaponDamage); derr != nil {
			return nil, fmt.Errorf("%w: %s: weapon_damage %q: %v", ErrInvalidContent, path, weaponDamage, derr)
		}
	}

	// Equipment slot declarations (§3.3). Explicit eligible_slots wins;
	// otherwise the legacy single `properties.slot` string is lifted to a
	// one-element set so existing content keeps working untouched (§3.2:
	// "a single declared slot is the one-element form of this set").
	// Names are validated against the slot registry in a boot post-pass
	// (validateItemSlots) once every pack's slots are registered.
	eligible := normalizeLowerDedup(f.EligibleSlots)
	if len(eligible) == 0 {
		if legacy, ok := item.LegacySlotName(f.Properties); ok {
			eligible = []string{legacy}
		}
	}
	companion := normalizeLowerDedup(f.CompanionSlots)

	return &item.Template{
		ID:             item.TemplateID(id),
		Name:           f.Name,
		Type:           f.Type,
		Description:    strings.TrimSpace(f.Description),
		Tags:           f.Tags,
		Keywords:       f.Keywords,
		Properties:     f.Properties,
		Modifiers:      mods,
		WeaponDamage:   weaponDamage,
		EligibleSlots:  eligible,
		CompanionSlots: companion,
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

	tier, teach, err := decodeTrainer(f.Trainer, f.Tags, path)
	if err != nil {
		return nil, err
	}

	// loot_table is namespace-qualified so it matches the (qualified)
	// id the loot registry stores; empty stays empty (no loot).
	lootTable, err := qualifyOptional(f.LootTable, ns, path, "loot_table")
	if err != nil {
		return nil, err
	}

	// Equipment ids are namespace-qualified here (bare ids resolve
	// against the current pack) so the §3.3 spawn lookup can match the
	// item registry, whose template ids are stored qualified. This is a
	// pure string operation — existence is still checked fail-silent at
	// spawn (§3.1), so an id pointing at a later-loaded pack's item
	// stays valid without forcing a post-pass.
	equipment, err := qualifyIDList(f.Equipment, ns, path, "equipment")
	if err != nil {
		return nil, err
	}

	// Natural-weapon dice are validated at load like item weapon dice
	// (combat §4.5) so a typo fails the pack rather than the first swing.
	var natWeaponName, natWeaponDamage string
	if f.NaturalWeapon != nil {
		natWeaponName = strings.TrimSpace(f.NaturalWeapon.Name)
		natWeaponDamage = strings.TrimSpace(f.NaturalWeapon.Damage)
		if natWeaponDamage != "" {
			if _, derr := combat.ParseDice(natWeaponDamage); derr != nil {
				return nil, fmt.Errorf("%w: %s: natural_weapon damage %q: %v", ErrInvalidContent, path, natWeaponDamage, derr)
			}
		}
	}

	// Normalize passive-ability proficiency keys (lowercase + trim) so
	// they match the registry/proficiency-manager keying. A blank key
	// after trimming is dropped. nil stays nil (mob has no passives).
	var profs map[string]int
	if len(f.Proficiencies) > 0 {
		profs = make(map[string]int, len(f.Proficiencies))
		for k, v := range f.Proficiencies {
			key := strings.ToLower(strings.TrimSpace(k))
			if key == "" {
				continue
			}
			profs[key] = v
		}
	}

	return &mob.Template{
		ID:                  mob.TemplateID(id),
		Name:                f.Name,
		Type:                typ,
		Description:         strings.TrimSpace(f.Description),
		Disposition:         f.Disposition,
		BaseDisposition:     mob.Reaction(strings.TrimSpace(f.BaseDisposition)),
		DispositionRules:    def,
		Behavior:            f.Behavior,
		Tags:                f.Tags,
		Keywords:            f.Keywords,
		Properties:          f.Properties,
		Stats:               f.Stats,
		Equipment:           equipment,
		NaturalWeaponName:   natWeaponName,
		NaturalWeaponDamage: natWeaponDamage,
		LootTable:           lootTable,
		Proficiencies:       profs,
		Race:                strings.ToLower(strings.TrimSpace(f.Race)),
		Class:               strings.ToLower(strings.TrimSpace(f.Class)),
		Level:               f.Level,
		TrainerTier:         tier,
		TrainerTeach:        teach,
	}, nil
}

// decodeTrainer converts the YAML trainer block into a runtime
// TrainerConfig. Validates that the tier name maps to one of the
// fixed ladder values (Novice/Apprentice/Journeyman/Master, spec
// §7.2). Also enforces the §7.3 pairing rule with the
// `skill_trainer` tag: a trainer block without the tag, or the
// tag without a block, is an authoring error caught at boot.
//
// Returns (0, nil, nil) when both the block and the tag are absent.
func decodeTrainer(src *TrainerFile, tags []string, path string) (int, []string, error) {
	hasTag := false
	for _, t := range tags {
		if strings.EqualFold(strings.TrimSpace(t), progression.TagSkillTrainer) {
			hasTag = true
			break
		}
	}
	if src == nil && !hasTag {
		return 0, nil, nil
	}
	if src == nil {
		return 0, nil, fmt.Errorf("%w: %s: 'skill_trainer' tag requires a 'trainer' block", ErrInvalidContent, path)
	}
	if !hasTag {
		return 0, nil, fmt.Errorf("%w: %s: 'trainer' block requires the 'skill_trainer' tag", ErrInvalidContent, path)
	}

	tierName := strings.ToLower(strings.TrimSpace(src.Tier))
	var tier progression.CapTier
	switch tierName {
	case "novice":
		tier = progression.CapNovice
	case "apprentice":
		tier = progression.CapApprentice
	case "journeyman":
		tier = progression.CapJourneyman
	case "master":
		tier = progression.CapMaster
	default:
		return 0, nil, fmt.Errorf("%w: %s: trainer.tier must be one of novice/apprentice/journeyman/master, got %q", ErrInvalidContent, path, src.Tier)
	}

	// Dedupe the teach list so a content author shipping
	// [slash, slash] doesn't waste a linear-scan comparison on
	// every CanTeach call. Preserves first-occurrence order.
	seen := make(map[string]struct{}, len(src.Teach))
	teach := make([]string, 0, len(src.Teach))
	for _, a := range src.Teach {
		id := strings.ToLower(strings.TrimSpace(a))
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		teach = append(teach, id)
	}

	return int(tier), teach, nil
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

// decodeRecipe reads a RecipeFile and builds a recipe.Recipe
// (crafting-and-cooking §3). Required: id, name, discipline, at least one
// input, and an output template. The recipe id and the input/output item
// template ids are namespace-qualified (bare ids resolve against the
// current pack; qualified ids cross packs). The discipline is a bare
// ability id (abilities are not namespaced). Item-id and discipline-id
// validity is checked at craft time, not here — the loader stays fail-soft
// so a recipe referencing not-yet-authored content still loads.
func decodeRecipe(path, ns string) (*recipe.Recipe, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading recipe %s: %w", path, err)
	}
	var f RecipeFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(f.Name) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'name'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(f.Discipline) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'discipline'", ErrInvalidContent, path)
	}
	if len(f.Inputs) == 0 {
		return nil, fmt.Errorf("%w: %s: recipe needs at least one input", ErrInvalidContent, path)
	}
	if strings.TrimSpace(f.Output.Template) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'output.template'", ErrInvalidContent, path)
	}
	if f.StationTier < 0 {
		return nil, fmt.Errorf("%w: %s: station_tier must be >= 0", ErrInvalidContent, path)
	}
	if f.SkillFloor < 0 {
		return nil, fmt.Errorf("%w: %s: skill_floor must be >= 0", ErrInvalidContent, path)
	}
	if f.TimePulses < 0 {
		return nil, fmt.Errorf("%w: %s: time_pulses must be >= 0", ErrInvalidContent, path)
	}

	acq, ok := recipe.ParseAcquisitionTier(f.Acquisition)
	if !ok {
		return nil, fmt.Errorf("%w: %s: unknown acquisition %q (want baseline/common/uncommon/rare/regional)",
			ErrInvalidContent, path, f.Acquisition)
	}

	id, err := qualifyID(f.ID, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	outID, err := qualifyID(f.Output.Template, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: output.template: %v", ErrInvalidContent, path, err)
	}
	outQty := f.Output.Quantity
	if outQty <= 0 {
		outQty = 1
	}

	inputs := make([]recipe.Ingredient, 0, len(f.Inputs))
	for i, in := range f.Inputs {
		if strings.TrimSpace(in.Template) == "" {
			return nil, fmt.Errorf("%w: %s: inputs[%d] missing 'template'", ErrInvalidContent, path, i)
		}
		tid, err := qualifyID(in.Template, ns)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: inputs[%d]: %v", ErrInvalidContent, path, i, err)
		}
		qty := in.Quantity
		if qty <= 0 {
			qty = 1
		}
		inputs = append(inputs, recipe.Ingredient{
			Template:   tid,
			Quantity:   qty,
			MinQuality: strings.TrimSpace(in.MinQuality),
		})
	}

	return &recipe.Recipe{
		ID:          recipe.RecipeID(id),
		DisplayName: f.Name,
		Discipline:  strings.ToLower(strings.TrimSpace(f.Discipline)),
		SkillFloor:  f.SkillFloor,
		StationTier: f.StationTier,
		Tool:        strings.TrimSpace(f.Tool),
		TimePulses:  f.TimePulses,
		Acquisition: acq,
		Inputs:      inputs,
		Output:      recipe.Output{Template: outID, Quantity: outQty},
		Pack:        ns,
	}, nil
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

// copyProperties returns a fresh copy of the input map so the loaded
// world.Room owns its property bag without sharing storage with the
// transient YAML decode struct. nil input → nil output.
func copyProperties(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// validateRoomProperties checks every entry in r.Properties against
// the supplied property registry. Errors when:
//   - the property name is not registered (looked up with `ns` as
//     the current-pack shorthand context),
//   - the stored value does not match the registered ValueType.
//
// Skips silently when reg is nil (room-only tests with no registry
// wired) or when the room has no Properties — empty bags are valid.
//
// Spec: docs/specs/persistence.md §2 (registry); spec
// world-rooms-movement §2.2 (room property bag).
func validateRoomProperties(r *world.Room, reg *property.Registry, ns string) error {
	if r == nil || len(r.Properties) == 0 || reg == nil {
		return nil
	}
	for name, raw := range r.Properties {
		entry, ok := reg.Get(name, ns)
		if !ok {
			return fmt.Errorf("%w: room %q property %q is not registered",
				ErrInvalidContent, r.ID, name)
		}
		if !valueMatchesType(raw, entry.Type) {
			return fmt.Errorf("%w: room %q property %q: value %T does not match registered type %s",
				ErrInvalidContent, r.ID, name, raw, entry.Type)
		}
	}
	return nil
}

// valueMatchesType reports whether v's runtime type satisfies the
// registered ValueType. YAML decode produces Go primitives directly
// for the simple cases; the int / int64 distinction is intentional
// — int registers as int, an authoring `1` in YAML decodes as int,
// while `int64` is reserved for explicit long-form values the engine
// supplies in code.
func valueMatchesType(v interface{}, t property.ValueType) bool {
	switch t {
	case property.TypeString:
		_, ok := v.(string)
		return ok
	case property.TypeInt:
		_, ok := v.(int)
		return ok
	case property.TypeInt64:
		// go-yaml decodes untagged integers as `int` on 64-bit
		// platforms; accept both so a content-side `1000000` does
		// not bounce off a TypeInt64-registered property just
		// because the YAML decoder chose the narrower type.
		switch v.(type) {
		case int64, int:
			return true
		}
		return false
	case property.TypeFloat64:
		_, ok := v.(float64)
		return ok
	case property.TypeBool:
		_, ok := v.(bool)
		return ok
	case property.TypeMapInt:
		switch m := v.(type) {
		case map[string]int:
			return true
		case map[string]any:
			// go-yaml decodes a YAML mapping as map[string]any; accept it
			// when every value is integer-shaped (the decoder may pick int,
			// int64, or float64), mirroring the TypeInt64 int-widening above.
			for _, vv := range m {
				switch vv.(type) {
				case int, int64, float64:
				default:
					return false
				}
			}
			return true
		}
		return false
	case property.TypeMapString:
		_, ok := v.(map[string]string)
		return ok
	case property.TypeListString:
		_, ok := v.([]string)
		return ok
	}
	return false
}

// buildDoorState materializes a *world.DoorState from a DoorFile.
// Validates the constraints the YAML shape allows (locked implies
// closed) and falls back to spec §5.1 defaults for omitted fields:
//
//   - Closed defaults true.
//   - Locked defaults false.
//   - Keywords default to the space-split lowercased tokens of Name
//     when not explicitly listed.
//   - Key is namespaced against the current pack via qualifyID, so a
//     bare `key: village-gate-key` resolves to `<ns>:village-gate-key`
//     while a qualified `othr-pack:foo-key` crosses packs.
//
// DefaultClosed / DefaultLocked are seeded from the runtime values
// so area reset restores the boot-time configuration (spec §5.4).
func buildDoorState(df DoorFile, ns string) (*world.DoorState, error) {
	if strings.TrimSpace(df.Name) == "" {
		return nil, fmt.Errorf("door requires a name")
	}
	closed := true
	if df.Closed != nil {
		closed = *df.Closed
	}
	if df.Locked && !closed {
		return nil, fmt.Errorf("locked: true requires closed: true (a locked door is closed)")
	}
	kw := df.Keywords
	if len(kw) == 0 {
		kw = doorKeywordsFromName(df.Name)
	} else {
		kw = lowerStrings(kw)
	}
	var keyID string
	if df.Key != "" {
		qualified, err := qualifyID(df.Key, ns)
		if err != nil {
			return nil, fmt.Errorf("key: %v", err)
		}
		keyID = qualified
	}
	return &world.DoorState{
		Name:           df.Name,
		Keywords:       kw,
		Closed:         closed,
		Locked:         df.Locked,
		KeyID:          keyID,
		Pickable:       df.Pickable,
		PickDifficulty: df.PickDifficulty,
		DefaultClosed:  closed,
		DefaultLocked:  df.Locked,
	}, nil
}

// doorKeywordsFromName returns the lowercased non-empty tokens of
// name split on whitespace. Used when a DoorFile omits explicit
// Keywords.
func doorKeywordsFromName(name string) []string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return nil
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.ToLower(p))
	}
	return out
}

// lowerStrings returns a fresh slice with each element lowercased.
func lowerStrings(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}
