package pack

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/biome"
	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/chat"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/decoration"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/effect"
	"github.com/Jasrags/AnotherMUD/internal/emote"
	"github.com/Jasrags/AnotherMUD/internal/faction"
	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/gathering"
	"github.com/Jasrags/AnotherMUD/internal/grade"
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/loot"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/mount"
	"github.com/Jasrags/AnotherMUD/internal/pool"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/property"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/rangedflavor"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/render"
	"github.com/Jasrags/AnotherMUD/internal/script"
	"github.com/Jasrags/AnotherMUD/internal/size"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stats"
	"github.com/Jasrags/AnotherMUD/internal/weather"
	"github.com/Jasrags/AnotherMUD/internal/world"
	"gopkg.in/yaml.v3"
)

// Errors callers may distinguish at the boundary.
var (
	ErrMissingArea           = errors.New("room references unknown area")
	ErrMissingExitRoom       = errors.New("exit references unknown room")
	ErrMissingItemTemplate   = errors.New("room item references unknown template")
	ErrMissingMobTemplate    = errors.New("room mob references unknown template")
	ErrMissingSpawnRoom      = errors.New("spawn rule references unknown room")
	ErrInvalidContent        = errors.New("invalid content file")
	ErrItemUnknownSlot       = errors.New("item references unknown slot")
	ErrItemUnknownGrade      = errors.New("item references unknown grade")
	ErrProjectileNoAmmo      = errors.New("projectile weapon's ammo_kind is supplied by no item")
	ErrMissingDoorKey        = errors.New("door references unknown key template")
	ErrAttributeReservedName = errors.New("attribute name collides with a reserved synthetic combat input")
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
	if dst == nil || dst.World == nil || dst.Items == nil || dst.Slots == nil || dst.Mobs == nil || dst.Tracks == nil || dst.Races == nil || dst.Classes == nil || dst.Backgrounds == nil || dst.AttributeSets == nil || dst.Pools == nil || dst.Languages == nil || dst.Feats == nil || dst.Abilities == nil || dst.Theme == nil || dst.Help == nil || dst.Quests == nil || dst.Weather == nil || dst.Scripts == nil || dst.Rarity == nil || dst.Essence == nil || dst.Grades == nil || dst.Loot == nil || dst.Channels == nil || dst.Emotes == nil || dst.RangedFlavor == nil || dst.ChannelMap == nil {
		return errors.New("pack.Load: dst has nil registry field; use pack.NewRegistries()")
	}
	logger := logging.From(ctx).With(slog.String("event", "pack.load"), slog.String("root", root))

	discovered, err := Discover(root, nil)
	if err != nil {
		return fmt.Errorf("discovery: %w", err)
	}
	// Boot-time pack selection (allowlist + dependency closure). Empty filter
	// = every active pack. Naming a world pack auto-keeps its baseline deps.
	discovered = filterClosure(discovered, filter)
	if len(filter) > 0 && len(discovered) == 0 {
		logger.Warn("pack allowlist matched no packs; world will be empty",
			slog.Any("packs", filter))
	}
	ordered, err := Order(discovered)
	if err != nil {
		return fmt.Errorf("ordering: %w", err)
	}

	logger.Info("packs discovered", slog.Int("count", len(ordered)))

	// Phase 1: manifest pass. M2 records only; no tags/properties yet.
	// Also validate each pack's kind and collect the active world set
	// (character-identity §2): the namespaces of `kind: world` packs, in
	// load order. Library/baseline packs are loaded but excluded.
	dst.Worlds = dst.Worlds[:0]
	dst.Splashes = make(map[string]string)
	dst.WorldAttributeSets = make(map[string]string)
	dst.WorldCurrencies = make(map[string]economy.CurrencyLabel)
	for _, p := range ordered {
		if !ValidKind(p.Manifest.Kind) {
			return fmt.Errorf("%w: pack %q: kind %q is not valid (expected \"world\", \"library\", or empty)",
				ErrInvalidContent, p.Manifest.Name, p.Manifest.Kind)
		}
		logging.From(ctx).Info("pack manifest",
			slog.String("event", "pack.manifest"),
			slog.String("pack", p.Manifest.Name),
			slog.String("namespace", p.Namespace()),
			slog.String("kind", p.Manifest.Kind),
		)
		if p.Manifest.IsWorld() {
			dst.Worlds = append(dst.Worlds, p.Namespace())
			// A world pack MUST declare a connect splash (login/character-select):
			// the door identity. Required + validated here so a world can't boot
			// faceless. Library packs are never a connect door and are exempt.
			splash, err := loadPackSplash(p)
			if err != nil {
				return err
			}
			dst.Splashes[p.Namespace()] = splash
			// Record the world's selected attribute set (SR-M1). Empty →
			// omitted, so the seed falls back to `classic`. The referenced id
			// is validated fail-soft at seed time (unknown → classic), not
			// here, mirroring how a background's home_language resolves.
			if id := strings.TrimSpace(p.Manifest.AttributeSet); id != "" {
				dst.WorldAttributeSets[p.Namespace()] = strings.ToLower(id)
			}
			// Record the world's money-display vocabulary (the nuyen/¥ reskin).
			// Absent → omitted, so display falls back to the gold default. The
			// suffix is kept verbatim (a leading space is meaningful: " gold");
			// only the noun is trimmed. Display-only, never validated as breaking.
			if cm := p.Manifest.Currency; cm != nil {
				dst.WorldCurrencies[p.Namespace()] = economy.CurrencyLabel{
					Noun:   strings.TrimSpace(cm.Name),
					Suffix: cm.Suffix,
				}
			}
		}
	}

	// Wire the property registry's dependency resolver (property Get §2.4 step 3)
	// before the content pass validates any `properties:` bag, so a pack can
	// reference a property declared by a pack it depends on via the bare `name`
	// shorthand. Maps each pack's namespace → its DIRECT dependencies' namespaces
	// (sorted for a stable first-hit-wins order — a manifest's dependency map has no
	// declaration order). A pack that wants a transitive dependency's property must
	// declare it directly or use the fully-qualified `pack:name` form.
	if dst.Properties != nil {
		deps := make(map[string][]string, len(ordered))
		for _, p := range ordered {
			ds := make([]string, 0, len(p.Manifest.Dependencies))
			for depName := range p.Manifest.Dependencies {
				ds = append(ds, DeriveNamespace(depName))
			}
			sort.Strings(ds)
			deps[p.Namespace()] = ds
		}
		dst.Properties.SetDependencyResolver(func(pack string) []string {
			return deps[pack]
		})
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

	// Light-floor inheritance (light-and-darkness §2.4 room→area tier):
	// an area's `light_floor` default bakes onto each member room that
	// does not declare its own, collapsing the floor cascade at load so
	// the per-viewer resolver stays a pure room read. An invalid area
	// level is a boot error (mirrors the weather-zone id check).
	if err := bakeAreaLightFloors(dst.World); err != nil {
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

	// Derive each room's map point-of-interest class from its pinned
	// NPCs (shop / skill_trainer tags) and rest bonus (player-maps §6).
	// Runs after mob placements are known so a room's fixtures are
	// visible. Content-derived, recomputed every load.
	deriveRoomPOIs(dst.World, mobPlacements, dst.Mobs)

	// Area spawn-rule references (rooms + mob templates) must resolve
	// in the final world. Runs after every pack has loaded so
	// cross-pack references (`other-pack:foo`) are valid.
	if err := validateSpawnRules(dst); err != nil {
		return err
	}

	// Quest-scoped spawn references (mob/item templates + rooms) must resolve,
	// too (quest-spawns.md §2 — a typo fails the pack, not the run). Same
	// after-all-packs timing as validateSpawnRules so cross-pack refs resolve.
	if err := validateQuestSpawns(dst); err != nil {
		return err
	}

	// Item slot references (eligible + companion) must resolve against the
	// fully-populated slot registry. Runs after every pack has loaded so a
	// slot defined by a later pack is visible (mirrors validateSpawnRules).
	if err := validateItemSlots(dst); err != nil {
		return err
	}

	// Item grade references (masterwork §2) must resolve against the
	// fully-populated grade registry. Runs after every pack loads so a grade
	// defined by a dependency is visible (mirrors validateItemSlots).
	if err := validateItemGrades(dst); err != nil {
		return err
	}

	// Door key references (the item template id a keyed door requires)
	// must resolve in the item registry. Runs after every pack has loaded
	// so a cross-pack key (`other-pack:foo-key`) is visible regardless of
	// load order (mirrors validateItemSlots).
	if err := validateDoorKeys(dst); err != nil {
		return err
	}

	// Attribute-name reserved-word guard (sr-m3c-deferred-fixes): no content
	// attribute set may declare a stat key that collides with a synthetic
	// combat-input name (channel.ReservedInputs) — the combat stat lookup
	// special-cases those before StatBlock.Effective, so a colliding attribute
	// would be silently shadowed. Runs after every pack loads so every
	// registered set is checked (mirrors validateDoorKeys).
	if err := validateAttributeReservedNames(dst); err != nil {
		return err
	}

	// Projectile ammo references (ranged-combat §3): every projectile weapon's
	// ammo_kind must be supplied by at least one ammo item, so a `ranged_class:
	// projectile` weapon can never be unfireable for want of any matching ammo
	// in the world. Runs after every pack loads so a cross-pack ammo item is
	// visible (mirrors validateItemSlots).
	if err := validateProjectileAmmo(dst); err != nil {
		return err
	}

	// Economy guardrail (crafting-and-cooking §8 / plan D3): flag any recipe
	// that loses money (output value ≤ Σ input value). Advisory only — the
	// D2.1 invariant is a content-pricing discipline, not a load gate — so
	// these surface as warnings, like the coordinate-derivation warnings.
	for _, w := range validateRecipeEconomy(dst) {
		logger.Warn("recipe economy: output value does not exceed inputs (crafting adds no value — §8/D2.1)",
			slog.String("recipe", string(w.Recipe)),
			slog.Int("output_value", w.OutputValue),
			slog.Int("input_value", w.InputValue),
		)
	}

	// Authoring guardrail (sr-m3c-deferred-fixes): a class growth_bonuses source
	// stat capped below 12 makes the d20 (v-10)/2 growth modifier always <= 0, so
	// the bonus silently no-ops (SR raw-value attrs cap at 6). Advisory, like the
	// recipe-economy warnings above.
	for _, w := range validateGrowthBonuses(dst) {
		logger.Warn("class growth_bonuses source is inert: its cap under this attribute set is below the d20 growth threshold (12), so (source-10)/2 is always <= 0",
			slog.String("class", w.Class),
			slog.String("stat", w.Stat),
			slog.String("source", w.Source),
			slog.String("attribute_set", w.Set),
			slog.Int("source_cap", w.Cap),
		)
	}

	// Size-and-wielding §4.1 authoring guardrail: an item that declares BOTH a
	// size and static companion_slots has the static list silently overridden
	// by the size-derived equip footprint. Advisory only — the item still loads.
	for _, id := range sizedCompanionConflicts(dst) {
		logger.Warn("item declares both size and companion_slots; the size-derived equip footprint overrides the static companion slots (size-and-wielding §4.1)",
			slog.String("item", id),
		)
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
	discovered, err := Discover(root, nil)
	if err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	// Honor the same boot-time pack selection so a script hot-reload sees the
	// same packs (and dependency closure) the server booted with.
	discovered = filterClosure(discovered, filter)
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

// validateQuestSpawns verifies that every quest-scoped spawn (quest-spawns.md
// §2) names a room and a mob/item template that resolve in the fully-populated
// registries. Runs after all packs load so cross-pack (dependency) references
// are valid, mirroring validateSpawnRules. A miss is a load-time error: a spawn
// declaration names concrete content the quest owns, so a typo should fail the
// pack rather than surface as a swallowed runtime warning at stage activation.
func validateQuestSpawns(dst *Registries) error {
	for _, def := range dst.Quests.All() {
		for si, stage := range def.Stages {
			for spi, sp := range stage.Spawns {
				if _, err := dst.World.Room(world.RoomID(sp.Room)); err != nil {
					return fmt.Errorf("%w: quest %q stage[%d].spawn[%d].room %q", ErrMissingSpawnRoom, def.ID, si, spi, sp.Room)
				}
				if sp.Kind == "item" {
					if !dst.Items.Has(item.TemplateID(sp.Template)) {
						return fmt.Errorf("%w: quest %q stage[%d].spawn[%d].template %q", ErrMissingItemTemplate, def.ID, si, spi, sp.Template)
					}
					continue
				}
				if !dst.Mobs.Has(mob.TemplateID(sp.Template)) {
					return fmt.Errorf("%w: quest %q stage[%d].spawn[%d].template %q", ErrMissingMobTemplate, def.ID, si, spi, sp.Template)
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

// validateItemGrades verifies that every item template's quality grade (if
// set) resolves in the grade registry (masterwork §2). Runs after all packs
// load so a grade defined by a dependency is visible. Boot-time validation
// turns a `grade:` typo into a precise load failure instead of a silently
// ungraded item; the sentinel ErrItemUnknownGrade mirrors ErrItemUnknownSlot.
func validateItemGrades(dst *Registries) error {
	for _, t := range dst.Items.All() {
		if t.Grade != "" && !dst.Grades.Has(t.Grade) {
			return fmt.Errorf("%w: item %q grade %q", ErrItemUnknownGrade, t.ID, t.Grade)
		}
		// quality_grades (masterwork §7): the map VALUES are grade keys a
		// craft stamps onto the result, so a typo here would silently produce
		// an ungraded crafted item — validate them too.
		for _, g := range qualityGradeValues(t.Properties) {
			if !dst.Grades.Has(g) {
				return fmt.Errorf("%w: item %q quality_grades value %q", ErrItemUnknownGrade, t.ID, g)
			}
		}
	}
	return nil
}

// validateProjectileAmmo verifies that every projectile weapon's ammo_kind
// (ranged-combat §2/§3) is supplied by at least one item declaring the same
// ammo_kind. Runs after all packs load so a cross-pack ammo item is visible.
// Boot-time validation turns "a bow nothing can feed" into a precise load
// failure rather than a weapon that silently never fires; the sentinel
// ErrProjectileNoAmmo mirrors ErrItemUnknownGrade. The decode pass already
// guarantees a projectile declares a non-empty ammo_kind (§2), so this only
// checks that some item supplies it.
func validateProjectileAmmo(dst *Registries) error {
	items := dst.Items.All()
	supplied := make(map[string]bool) // ammo_kind → at least one item supplies it
	for _, t := range items {
		// A projectile weapon's ammo_kind is what it CONSUMES, not what it
		// supplies — exclude it so a bow can't satisfy its own requirement.
		// Ammo items (and any non-projectile) declaring ammo_kind are suppliers.
		if t.AmmoKind != "" && t.RangedClass != item.RangedProjectile {
			supplied[t.AmmoKind] = true
		}
	}
	for _, t := range items {
		if t.RangedClass == item.RangedProjectile && !supplied[t.AmmoKind] {
			return fmt.Errorf("%w: weapon %q needs ammo_kind %q", ErrProjectileNoAmmo, t.ID, t.AmmoKind)
		}
	}
	return nil
}

// qualityGradeValues returns the grade keys referenced by a template's
// `quality_grades` map (the crafting quality→grade map, masterwork §7) — its
// VALUES. Handles the two YAML map decodings (map[string]any / map[any]any);
// non-string or blank values are ignored (they map to no grade).
func qualityGradeValues(props map[string]any) []string {
	if props == nil {
		return nil
	}
	raw, ok := props["quality_grades"] // matches crafting.propQualityGrades (a content wire key)
	if !ok {
		return nil
	}
	collect := func(v any) string {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
		return ""
	}
	var out []string
	switch m := raw.(type) {
	case map[string]any:
		for _, v := range m {
			if s := collect(v); s != "" {
				out = append(out, s)
			}
		}
	case map[any]any:
		for _, v := range m {
			if s := collect(v); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// poiRank orders the map point-of-interest classes by precedence so a
// room that is several things at once shows the most useful marker:
// shop > trainer > inn (player-maps §6). Empty ranks lowest.
func poiRank(poi string) int {
	switch poi {
	case "shop":
		return 3
	case "trainer":
		return 2
	case "inn":
		return 1
	default:
		return 0
	}
}

// poiFromMobTags maps an NPC's tags to a point-of-interest class: a
// `shop` tag wins over `skill_trainer` (a vendor who also trains shows
// as a shop). Empty when the NPC is neither.
func poiFromMobTags(tags []string) string {
	var trainer bool
	for _, t := range tags {
		switch t {
		case "shop":
			return "shop"
		case "skill_trainer":
			trainer = true
		}
	}
	if trainer {
		return "trainer"
	}
	return ""
}

// deriveRoomPOIs sets each room's POI marker class from its pinned NPCs
// and rest bonus (player-maps §6). Shop/trainer come from the fixtures
// placed in the room (the strongest such marker wins); a room with no
// vendor/trainer but a positive HealingRate (inn / infirmary / shrine)
// is marked an inn. Content-derived, recomputed every load; never
// persisted.
//
// Mutates *world.Room fields directly through the bare pointer Room()
// returns. Safe because Load runs single-threaded and completes before
// any tick/session goroutine starts — the same load-phase mutation
// pattern as bakeAreaLightFloors and DeriveCoordinates.
func deriveRoomPOIs(w *world.World, placements []pendingMobPlacement, mobs *mob.Templates) {
	if w == nil || mobs == nil {
		return
	}
	for _, p := range placements {
		tpl, err := mobs.Get(mob.TemplateID(p.TemplateID))
		if err != nil {
			continue
		}
		poi := poiFromMobTags(tpl.Tags)
		if poi == "" {
			continue
		}
		room, err := w.Room(p.RoomID)
		if err != nil {
			continue
		}
		if poiRank(poi) > poiRank(room.POI) {
			room.POI = poi
		}
	}
	// Inn/rest is the weakest marker — only when no vendor/trainer claimed
	// the room. A positive HealingRate is the rest-room signal.
	for _, r := range w.Rooms() {
		if r.POI == "" && r.HealingRate > 0 {
			r.POI = "inn"
		}
	}
}

// bakeAreaLightFloors resolves the room→area tier of the
// light-and-darkness §2.4 floor cascade at load: each area that
// declares a `light_floor` copies it onto every member room that does
// not already carry its own `light_floor`, so the room-level light
// resolver (light.FloorFor) stays a pure room read with no World
// threading into the render/combat/movement hot paths. A room's own
// `light_floor` wins (the bake skips it), preserving room-over-area
// precedence. An area `light_floor` outside the four-level vocabulary
// is a boot error, mirroring the weather-zone id check — a typo must
// not silently vanish into "no floor".
func bakeAreaLightFloors(w *world.World) error {
	for _, a := range w.Areas() {
		if a.LightFloor == "" {
			continue
		}
		if _, ok := light.ParseLevel(a.LightFloor); !ok {
			return fmt.Errorf("%w: area %q: light_floor %q is not one of black/gloom/dim/lit",
				ErrInvalidContent, a.ID, a.LightFloor)
		}
		for _, r := range w.RoomsInArea(a.ID) {
			if _, has := r.PropertyString(light.PropRoomLightFloor); has {
				continue // room-level floor wins over the area default
			}
			if r.Properties == nil {
				r.Properties = map[string]any{}
			}
			r.Properties[light.PropRoomLightFloor] = a.LightFloor
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

// validateAttributeReservedNames asserts that no registered attribute set
// declares a stat key colliding with a synthetic combat-input name
// (channel.ReservedInputs). The combat stat lookup special-cases those names
// before StatBlock.Effective, so a colliding attribute would resolve to the
// synthetic value and never its stored stat — a fail-silent authoring trap.
// See sr-m3c-deferred-fixes.
func validateAttributeReservedNames(dst *Registries) error {
	reserved := make(map[string]struct{}, len(channel.ReservedInputs()))
	for _, name := range channel.ReservedInputs() {
		reserved[name] = struct{}{}
	}
	for _, set := range dst.AttributeSets.All() {
		for _, key := range set.Keys() {
			if _, ok := reserved[string(key)]; ok {
				return fmt.Errorf("%w: attribute set %q key %q", ErrAttributeReservedName, set.ID, key)
			}
		}
	}
	return nil
}

// recipeValueProp is the item property the economy guardrail reads. It is the
// same key the shop prices off (economy.PropValue = "value"); referenced by
// literal here to keep the pack loader free of an economy import.
const recipeValueProp = "value"

// recipeEconomyWarning flags a recipe whose output is worth no more than the
// items it consumes — a "crafting loses money" smell (crafting-and-cooking §8,
// biomes/gathering plan D2.1/D3). It is advisory: Load logs it but never
// fails on it, mirroring the non-fatal coordinate-derivation warnings. A
// third-party pack with deliberately valueless craft outputs is a content
// choice, not a load error.
type recipeEconomyWarning struct {
	Recipe      recipe.RecipeID
	OutputValue int
	InputValue  int
}

// validateRecipeEconomy walks every registered recipe and flags those where
// output value ≤ Σ(input value × quantity) — the D3 guardrail that keeps the
// D2.1 "crafting always adds value" invariant from silently rotting as
// content evolves. Recipes whose output template is unknown are skipped (the
// value can't be assessed; the loader is fail-soft on recipe item ids
// elsewhere). Missing input templates contribute 0, which only makes the
// check more lenient — never a false positive. Returns the warnings sorted by
// recipe id for deterministic logging; Load emits them via slog.Warn.
func validateRecipeEconomy(dst *Registries) []recipeEconomyWarning {
	if dst == nil || dst.Recipes == nil || dst.Items == nil {
		return nil
	}
	var warns []recipeEconomyWarning
	for _, r := range dst.Recipes.All() {
		outTpl, err := dst.Items.Get(item.TemplateID(r.Output.Template))
		if err != nil {
			continue // unknown output — can't assess; fail-soft like the rest.
		}
		outValue := itemValueProp(outTpl) * maxInt(1, r.Output.Quantity)

		sumIn := 0
		for _, in := range r.Inputs {
			tpl, err := dst.Items.Get(item.TemplateID(in.Template))
			if err != nil {
				continue // unknown input contributes 0 (lenient).
			}
			sumIn += itemValueProp(tpl) * maxInt(1, in.Quantity)
		}

		if outValue <= sumIn {
			warns = append(warns, recipeEconomyWarning{
				Recipe:      r.ID,
				OutputValue: outValue,
				InputValue:  sumIn,
			})
		}
	}
	sort.Slice(warns, func(i, j int) bool { return warns[i].Recipe < warns[j].Recipe })
	return warns
}

// growthBonusMinSource is the smallest source-stat value that yields a positive
// d20 growth modifier: level_up applies max(0, (source-10)/2), which is > 0 only
// at source >= 12. A source stat capped below this can never contribute.
const growthBonusMinSource = 12

type growthBonusWarning struct {
	Class  string
	Stat   string
	Source string
	Set    string
	Cap    int
}

// validateGrowthBonuses flags a class whose growth_bonuses names a source stat
// that no world can push high enough to matter. A class level-up adds
// max(0, (Effective(source)-10)/2) to the grown stat (level_up.go); for a
// source stat capped below growthBonusMinSource under some registered attribute
// set, that modifier is always <= 0, so the bonus is a silent no-op — the SR
// footgun (raw-value attrs cap at 6, so the d20 modifier never fires). Advisory,
// like validateRecipeEconomy: a warn per (class, source, capping-set), not a
// load error. Sorted for deterministic logging.
func validateGrowthBonuses(dst *Registries) []growthBonusWarning {
	if dst == nil || dst.Classes == nil || dst.AttributeSets == nil {
		return nil
	}
	var warns []growthBonusWarning
	for _, c := range dst.Classes.All() {
		for stat, src := range c.GrowthBonuses {
			if src == "" {
				continue
			}
			for _, set := range dst.AttributeSets.All() {
				attr, ok := set.Get(src)
				if !ok {
					continue // this set doesn't declare the source stat.
				}
				// Cap 0 means "no set-level cap" (race caps / the engine default
				// apply) — the source can still reach 12, so it's viable. Only an
				// explicit sub-12 cap guarantees the modifier is dead.
				if attr.Cap > 0 && attr.Cap < growthBonusMinSource {
					warns = append(warns, growthBonusWarning{
						Class: c.ID, Stat: string(stat), Source: string(src),
						Set: set.ID, Cap: attr.Cap,
					})
				}
			}
		}
	}
	// Group by (class, source): the finding is about the source stat's low cap,
	// so warnings sharing a source cluster together even when one source drives
	// several grown stats. Stat + Set are deterministic tie-breakers.
	sort.Slice(warns, func(i, j int) bool {
		if warns[i].Class != warns[j].Class {
			return warns[i].Class < warns[j].Class
		}
		if warns[i].Source != warns[j].Source {
			return warns[i].Source < warns[j].Source
		}
		if warns[i].Stat != warns[j].Stat {
			return warns[i].Stat < warns[j].Stat
		}
		return warns[i].Set < warns[j].Set
	})
	return warns
}

// sizedCompanionConflicts returns the ids of item templates that declare BOTH a
// `size` and static `companion_slots`. A sized item's equip footprint is
// DERIVED from its size (size-and-wielding §4.1): a light/one-handed weapon
// discards its static companion slots entirely, a two-handed one merges them
// with the off-hand slot — so the authored companion list is silently (wholly
// or partly) overridden. Flagging the co-occurrence surfaces the authoring
// mistake without failing the pack (advisory, like the recipe-economy /
// coordinate warnings). Sorted by id for deterministic logging.
func sizedCompanionConflicts(dst *Registries) []string {
	if dst == nil || dst.Items == nil {
		return nil
	}
	var ids []string
	for _, t := range dst.Items.All() {
		if t.Size != "" && len(t.CompanionSlots) > 0 {
			ids = append(ids, string(t.ID))
		}
	}
	sort.Strings(ids)
	return ids
}

// itemValueProp reads the integer `value` property off a template, tolerating
// the int / int64 / float64 shapes yaml.v3 produces. Zero when absent or
// non-numeric (mirrors economy.templateValue).
func itemValueProp(tpl *item.Template) int {
	if tpl == nil || tpl.Properties == nil {
		return 0
	}
	switch n := tpl.Properties[recipeValueProp].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

// maxInt is a tiny local helper (Go 1.21 builtin max is available, but the
// explicit name keeps the value-clamp intent obvious at the call sites).
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
	backgroundPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Backgrounds)
	if err != nil {
		return nil, nil, err
	}
	languagePaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Languages)
	if err != nil {
		return nil, nil, err
	}
	attributeSetPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.AttributeSets)
	if err != nil {
		return nil, nil, err
	}
	poolPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Pools)
	if err != nil {
		return nil, nil, err
	}
	featPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Feats)
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
	channelMapPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.ChannelMap)
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
	gradePaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Grades)
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
	biomePaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Biomes)
	if err != nil {
		return nil, nil, err
	}
	factionPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Factions)
	if err != nil {
		return nil, nil, err
	}
	foragePaths, err := resolveGlobs(p.Dir, p.Manifest.Content.ForageTables)
	if err != nil {
		return nil, nil, err
	}
	nodePaths, err := resolveGlobs(p.Dir, p.Manifest.Content.NodeTemplates)
	if err != nil {
		return nil, nil, err
	}
	nodeSpawnPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.NodeSpawnTables)
	if err != nil {
		return nil, nil, err
	}
	channelPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Channels)
	if err != nil {
		return nil, nil, err
	}
	emotePaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Emotes)
	if err != nil {
		return nil, nil, err
	}
	rangedFlavorPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.RangedFlavor)
	if err != nil {
		return nil, nil, err
	}
	propertyPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Properties)
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

	// Content-declared properties register FIRST — before this pack's areas and
	// rooms, whose `properties:` bags are validated against the registry. Each
	// installs a pack-scoped key (visible to this pack + its dependents);
	// shadowing an engine baseline property is a load error (RegisterPack).
	for _, pp := range propertyPaths {
		entry, err := decodeProperty(pp)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Properties.RegisterPack(ns, entry); err != nil {
			return nil, nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, pp, err)
		}
	}
	if len(propertyPaths) > 0 {
		logger.Info("pack properties declared",
			slog.String("event", "pack.properties"),
			slog.Int("count", len(propertyPaths)))
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
		// Validate the area property bag against the registry before commit
		// (same snake-case / registered / type-match contract as rooms), so a
		// bad area property surfaces at boot, not at first read.
		if err := validateAreaProperties(a, dst.Properties, ns); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, ap)
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

	// Backgrounds: id-keyed registry mirroring races/classes (backgrounds
	// §2). Skill/item grants are content references resolved at creation
	// (fail-soft), so the loader validates only id presence.
	for _, bp := range backgroundPaths {
		b, err := decodeBackground(bp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Backgrounds.Register(b); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, bp)
		}
	}

	// Languages: id-keyed registry mirroring backgrounds (languages.md §2). A
	// background's home_language references one (resolved fail-soft at grant);
	// the loader validates only id presence.
	for _, lp := range languagePaths {
		l, err := decodeLanguage(lp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Languages.Register(l); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, lp)
		}
	}

	// Attribute sets: global-id registry (SR-M1 — shadowrun-mvp.md Appendix A).
	// A world seeds its characters + score sheet + trainable gate from the set
	// its manifest selects; the core pack ships `classic` (the engine six).
	// Register validates set/attribute ids; malformed sets fail here with pack
	// attribution rather than seeding a broken character later.
	for _, ap := range attributeSetPaths {
		s, err := decodeAttributeSet(ap, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.AttributeSets.Register(s); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, ap)
		}
	}

	// Resource pools: kind-keyed global registry (shadowrun-mvp SR-M3a). The
	// core pack declares mana/movement; a world pack (Shadowrun) declares its
	// Stun/Physical monitors. Read at creation + spawn to seed each entity's
	// pool.Set. Register validates the kind + priority-overrides on collision.
	for _, pp := range poolPaths {
		d, err := decodePool(pp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Pools.Register(d); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, pp)
		}
	}

	// Feats: id-keyed global registry (EPIC S4 Phase 0 — docs/proposals/
	// wot-feats.md §2.1). Prereq feat/skill ids are content references
	// resolved fail-soft when a feat is taken, so the loader validates only
	// id presence + the prereq-kind / multi-take vocabulary.
	for _, fp := range featPaths {
		ft, err := decodeFeat(fp, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := dst.Feats.Register(ft); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, fp)
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

	// Channel map: global stat→combat-channel derivation (the channel
	// layer), later-wins per channel like the theme. Channel names +
	// formulas are validated at decode for pack + path attribution, so a
	// typo fails the boot rather than silently reading a default.
	for _, cp := range channelMapPaths {
		entries, err := decodeChannelMap(cp)
		if err != nil {
			return nil, nil, err
		}
		for ch, src := range entries {
			dst.ChannelMap.Register(ch, src)
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
	// Quality grades: global vocabulary, later-wins (masterwork §2). Keys
	// validated at this boundary; a malformed grade key fails the boot
	// loudly with pack + path attribution.
	for _, gp := range gradePaths {
		grades, err := decodeGrades(gp)
		if err != nil {
			return nil, nil, err
		}
		for _, g := range grades {
			if err := grade.ValidateKey(g.Key); err != nil {
				return nil, nil, fmt.Errorf("%w (in %s)", err, gp)
			}
			dst.Grades.Register(g)
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

	// Biomes: a room's `terrain` value keys into this registry
	// (biomes.md §2). Pack biomes register under their BARE id (terrain
	// strings are bare, PD-3) via RegisterPack, which rejects shadowing an
	// engine-baseline biome. A `terrain` value with no registered biome
	// keeps today's bare-string behavior (§2.3), so no room reference is
	// validated against this registry.
	for _, bp := range biomePaths {
		data, err := os.ReadFile(bp)
		if err != nil {
			return nil, nil, fmt.Errorf("read biome %s: %w", bp, err)
		}
		b, err := biome.Decode(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, bp)
		}
		// Qualify the biome's forage-table reference against this pack's
		// namespace so a bare `forage_table: forest-forage` resolves to the
		// namespaced key the ForageTables registry stores (gathering.md §2).
		// Empty = the biome offers no forage.
		if b.ForageTable != "" {
			qid, err := qualifyID(b.ForageTable, ns)
			if err != nil {
				return nil, nil, fmt.Errorf("%w: %s: forage_table: %v", ErrInvalidContent, bp, err)
			}
			b.ForageTable = qid
		}
		if b.NodeSpawnTable != "" {
			qid, err := qualifyID(b.NodeSpawnTable, ns)
			if err != nil {
				return nil, nil, fmt.Errorf("%w: %s: node_spawn_table: %v", ErrInvalidContent, bp, err)
			}
			b.NodeSpawnTable = qid
		}
		if dst.Biomes != nil {
			if err := dst.Biomes.RegisterPack(ns, b); err != nil {
				return nil, nil, fmt.Errorf("%w (in %s)", err, bp)
			}
		}
	}

	// Factions (faction.md §2): per-character standing groups. The definition
	// id is namespace-qualified (like loot/biomes); its rank ladder, bounds,
	// and starting standing default from the registry config when omitted.
	// Re-adding an id overrides (last-wins, like other content registries).
	for _, fp := range factionPaths {
		data, err := os.ReadFile(fp)
		if err != nil {
			return nil, nil, fmt.Errorf("read faction %s: %w", fp, err)
		}
		def, hasMin, hasMax, hasStarting, err := faction.Decode(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, fp)
		}
		qid, err := qualifyID(def.ID, ns)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s: id: %v", ErrInvalidContent, fp, err)
		}
		def.ID = qid
		if dst.Factions != nil {
			stored := dst.Factions.AddWithFlags(def, hasMin, hasMax, hasStarting)
			// Validate the resolved (defaults-filled) definition at LOAD so a
			// content misconfiguration fails the pack rather than silently
			// pinning every standing at the floor on the first shift — the
			// "catch at load" guarantee the alignment manager has (which panics
			// on a bad config; here a bad content file is an ErrInvalidContent,
			// not a crash).
			if stored.Min > stored.Max {
				return nil, nil, fmt.Errorf("%w: %s: faction min (%d) exceeds max (%d)", ErrInvalidContent, fp, stored.Min, stored.Max)
			}
			if n := len(stored.Ranks); n == 0 || stored.Ranks[0].Threshold > stored.Max {
				return nil, nil, fmt.Errorf("%w: %s: faction rank ladder is unreachable (no rank at or below max %d)", ErrInvalidContent, fp, stored.Max)
			}
		}
	}

	// Forage tables (gathering.md §2): the ambient-forage pools a biome's
	// forage_table id references. The gathering package owns the parse +
	// validation; the loader qualifies the table id + each entry's item id
	// against this pack's namespace (like loot tables), so bare ids resolve
	// to this pack and `pack:name` forms cross packs. The Ceiling is a
	// rarity key (global vocabulary), left unqualified.
	for _, fp := range foragePaths {
		data, err := os.ReadFile(fp)
		if err != nil {
			return nil, nil, fmt.Errorf("read forage table %s: %w", fp, err)
		}
		t, err := gathering.DecodeForageTable(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, fp)
		}
		qid, err := qualifyID(t.ID, ns)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s: id: %v", ErrInvalidContent, fp, err)
		}
		t.ID = qid
		for i := range t.Entries {
			qitem, err := qualifyID(t.Entries[i].Item, ns)
			if err != nil {
				return nil, nil, fmt.Errorf("%w: %s: entries[%d].item: %v", ErrInvalidContent, fp, i, err)
			}
			t.Entries[i].Item = qitem
		}
		if dst.ForageTables != nil {
			if err := dst.ForageTables.Register(t); err != nil {
				return nil, nil, fmt.Errorf("%w (in %s)", err, fp)
			}
		}
	}

	// Node templates (gathering.md §3.1): the gathering package parses +
	// validates; the loader qualifies the node id + its yield-table ref
	// (a forage table). RequiredTool is a tag (global), left bare.
	for _, np := range nodePaths {
		data, err := os.ReadFile(np)
		if err != nil {
			return nil, nil, fmt.Errorf("read node template %s: %w", np, err)
		}
		n, err := gathering.DecodeNodeTemplate(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, np)
		}
		qid, err := qualifyID(n.ID, ns)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s: id: %v", ErrInvalidContent, np, err)
		}
		n.ID = qid
		qyield, err := qualifyID(n.YieldTable, ns)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s: yield_table: %v", ErrInvalidContent, np, err)
		}
		n.YieldTable = qyield
		if dst.Nodes != nil {
			if err := dst.Nodes.RegisterNode(n); err != nil {
				return nil, nil, fmt.Errorf("%w (in %s)", err, np)
			}
		}
	}

	// Node spawn tables (gathering.md §3.1): qualify the table id + each
	// entry's node-template ref against this pack's namespace.
	for _, np := range nodeSpawnPaths {
		data, err := os.ReadFile(np)
		if err != nil {
			return nil, nil, fmt.Errorf("read node spawn table %s: %w", np, err)
		}
		st, err := gathering.DecodeNodeSpawnTable(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, np)
		}
		qid, err := qualifyID(st.ID, ns)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s: id: %v", ErrInvalidContent, np, err)
		}
		st.ID = qid
		for i := range st.Entries {
			qnode, err := qualifyID(st.Entries[i].Node, ns)
			if err != nil {
				return nil, nil, fmt.Errorf("%w: %s: entries[%d].node: %v", ErrInvalidContent, np, i, err)
			}
			st.Entries[i].Node = qnode
		}
		if dst.Nodes != nil {
			if err := dst.Nodes.RegisterSpawnTable(st); err != nil {
				return nil, nil, fmt.Errorf("%w (in %s)", err, np)
			}
		}
	}

	// Channels (chat-channels-and-tells §3): namespace-qualify the id, then
	// register. A duplicate id or display-name collision across packs is a
	// loud error (the registry rejects it), same as item/recipe id clashes.
	for _, cp := range channelPaths {
		ch, err := decodeChannel(cp, ns)
		if err != nil {
			return nil, nil, err
		}
		// dst.Channels is guaranteed non-nil by Load's required-registry
		// check — channels are engine-baseline content, not optional like
		// Biomes/Nodes.
		if err := dst.Channels.Register(ch); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, cp)
		}
	}

	// Emotes (emotes.md §2): namespace-qualify the id, then register. A
	// duplicate id or verb/alias collision across packs is a loud error.
	for _, ep := range emotePaths {
		em, err := decodeEmote(ep, ns)
		if err != nil {
			return nil, nil, err
		}
		// dst.Emotes is guaranteed non-nil by Load's required-registry check.
		if err := dst.Emotes.Register(em); err != nil {
			return nil, nil, fmt.Errorf("%w (in %s)", err, ep)
		}
	}

	// Ranged flavor (rangedflavor): a global style vocabulary keyed by
	// `ranged_style`. Ids are NOT namespace-qualified (like slot names), and
	// registration is last-writer-wins so a pack may override the core baseline.
	for _, rp := range rangedFlavorPaths {
		style, err := decodeRangedFlavor(rp)
		if err != nil {
			return nil, nil, err
		}
		dst.RangedFlavor.Register(style)
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
		slog.Int("backgrounds", len(backgroundPaths)),
		slog.Int("attribute_sets", len(attributeSetPaths)),
		slog.Int("pools", len(poolPaths)),
		slog.Int("languages", len(languagePaths)),
		slog.Int("feats", len(featPaths)),
		slog.Int("abilities", len(abilityPaths)),
		slog.Int("theme", len(themePaths)),
		slog.Int("channel_map", len(channelMapPaths)),
		slog.Int("help", helpTopics),
		slog.Int("quests", len(questPaths)),
		slog.Int("effects", len(effectPaths)),
		slog.Int("rarity", len(rarityPaths)),
		slog.Int("essence", len(essencePaths)),
		slog.Int("loot_tables", len(lootPaths)),
		slog.Int("biomes", len(biomePaths)),
		slog.Int("forage_tables", len(foragePaths)),
		slog.Int("node_templates", len(nodePaths)),
		slog.Int("node_spawn_tables", len(nodeSpawnPaths)),
		slog.Int("channels", len(channelPaths)),
		slog.Int("emotes", len(emotePaths)),
		slog.Int("ranged_flavor", len(rangedFlavorPaths)),
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

	// Faction requirements (faction.md §6): qualify each faction id against the
	// pack namespace (faction ids are namespaced content, like reward factions).
	var factionReqs []progression.AbilityFactionRequirement
	for i, fr := range f.FactionRequirements {
		fid, err := qualifyOptional(fr.Faction, ns, path, fmt.Sprintf("faction_requirements[%d].faction", i))
		if err != nil {
			return nil, err
		}
		if fid == "" {
			continue
		}
		factionReqs = append(factionReqs, progression.AbilityFactionRequirement{Faction: fid, MinStanding: fr.MinStanding})
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
		recurring, err := decodeConditionSave(f.Effect.RecurringSave, path, "effect.recurring_save")
		if err != nil {
			return nil, err
		}
		effect = &progression.EffectTemplate{
			ID:            f.Effect.ID,
			Duration:      f.Effect.Duration,
			Modifiers:     mods,
			Flags:         flags,
			Refreshable:   f.Effect.Refreshable,
			RecurringSave: recurring,
		}
	}

	applySave, err := decodeConditionSave(f.ApplySave, path, "apply_save")
	if err != nil {
		return nil, err
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
		CastTime:              f.CastTime,
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
		Elements:              normalizeLowerDedup(f.Elements),
		HasAlignmentRange:     hasAlignRange,
		AlignmentMin:          alignMin,
		AlignmentMax:          alignMax,
		FactionRequirements:   factionReqs,
		Effect:                effect,
		ApplySave:             applySave,
		Pack:                  ns,
		Priority:              f.Priority,
	}, nil
}

// decodeConditionSave validates + converts a content SaveFile (axis + DC)
// into a progression.ConditionSave (conditions §4). nil in ⇒ nil out (no
// save). An unknown axis is an authoring error at load. field names the
// surface (apply_save / effect.recurring_save) for the error.
func decodeConditionSave(in *SaveFile, path, field string) (*progression.ConditionSave, error) {
	if in == nil {
		return nil, nil
	}
	axis := progression.SaveType(strings.ToLower(strings.TrimSpace(in.Axis)))
	switch axis {
	case progression.SaveFortitude, progression.SaveReflex, progression.SaveWill:
	default:
		return nil, fmt.Errorf("%w: %s: %s axis %q must be fortitude/reflex/will", ErrInvalidContent, path, field, in.Axis)
	}
	return &progression.ConditionSave{Axis: axis, DC: in.DC}, nil
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

// decodeChannelMap reads a ChannelMapFile and returns its channel →
// formula-source map (the channel layer — docs/themes/channel-vocabulary.md
// §7). Each channel name is validated against the curated vocabulary
// (channel.IsKnown) and each formula is parsed here, so an unknown channel
// or a malformed formula fails the boot with pack + path attribution rather
// than silently reading a default at runtime. Blank channel names are
// skipped.
func decodeChannelMap(path string) (map[channel.Channel]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading channel map %s: %w", path, err)
	}
	var f ChannelMapFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	out := make(map[channel.Channel]string, len(f.Channels))
	for name, src := range f.Channels {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		ch := channel.Channel(name)
		if !channel.IsKnown(ch) {
			return nil, fmt.Errorf("%w: %s: unknown combat channel %q (not in the engine vocabulary)", ErrInvalidContent, path, name)
		}
		// Parse here purely to validate WITH pack+path attribution (the
		// later Registry.Build re-parses the same source — Build's error
		// carries only the channel, not the file). The tiny double-parse at
		// boot buys a precise load-time diagnostic.
		if _, err := channel.Parse(src); err != nil {
			return nil, fmt.Errorf("%w: %s: channel %q formula %q: %v", ErrInvalidContent, path, name, src, err)
		}
		out[ch] = src
	}
	return out, nil
}

// decodeRarity reads a RarityFile and returns its tiers as
// decoration.Tier values (spec item-decorations §2). Color name validity
// is not checked here — like the theme, an unrecognized fg/bg degrades to
// no color at Compile rather than failing the boot. Key validity IS the
// caller's concern (decoration.ValidateKey at the load boundary).
// decodeGrades reads a GradeFile and returns its grades as grade.Grade
// values (masterwork §2). Mirrors decodeRarity.
func decodeGrades(path string) ([]grade.Grade, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading grades %s: %w", path, err)
	}
	var f GradeFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	out := make([]grade.Grade, 0, len(f.Grades))
	for _, g := range f.Grades {
		out = append(out, grade.Grade{
			Key:               g.Key,
			Order:             g.Order,
			WeaponToHit:       g.WeaponToHit,
			WeaponDamage:      g.WeaponDamage,
			ArmorCheckImprove: g.ArmorCheckImprove,
			ToolSkill:         g.ToolSkill,
			Unbreakable:       g.Unbreakable,
		})
	}
	return out, nil
}

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
		spawns, err := decodeQuestSpawns(sf, si, ns, path, objs)
		if err != nil {
			return nil, err
		}
		stages = append(stages, quest.Stage{
			ID:          sf.ID,
			Description: sf.Description,
			Hint:        sf.Hint,
			Objectives:  objs,
			Spawns:      spawns,
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
	// Recipe ids are namespaced concrete content (like items, unlike the
	// global bare-id abilities), so reward recipes qualify against the pack
	// namespace too (crafting-and-cooking §7).
	rewardRecipes, err := qualifyIDList(f.Reward.Recipes, ns, path, "reward.recipes")
	if err != nil {
		return nil, err
	}
	// Faction ids are namespaced content (faction registry), so reward/prereq
	// faction references qualify against the pack namespace (faction.md §5.1).
	var rewardFactions []quest.FactionReward
	for i, fr := range f.Reward.Faction {
		fid, err := qualifyOptional(fr.Faction, ns, path, fmt.Sprintf("reward.faction[%d].faction", i))
		if err != nil {
			return nil, err
		}
		if fid == "" {
			continue
		}
		rewardFactions = append(rewardFactions, quest.FactionReward{Faction: fid, Delta: fr.Delta})
	}
	var prereqFactions []quest.FactionRequirement
	for i, fr := range f.Prerequisite.Faction {
		fid, err := qualifyOptional(fr.Faction, ns, path, fmt.Sprintf("prerequisite.faction[%d].faction", i))
		if err != nil {
			return nil, err
		}
		if fid == "" {
			continue
		}
		prereqFactions = append(prereqFactions, quest.FactionRequirement{Faction: fid, MinStanding: fr.MinStanding})
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
			Faction:            prereqFactions,
		},
		Stages: stages,
		Reward: quest.Reward{
			XP:          f.Reward.XP,
			Gold:        f.Reward.Gold,
			Items:       rewardItems,
			Abilities:   f.Reward.Abilities,
			Recipes:     rewardRecipes,
			Faction:     rewardFactions,
			Reputation:  f.Reward.Reputation,
			ClassUnlock: f.Reward.ClassUnlock,
			RaceUnlock:  f.Reward.RaceUnlock,
		},
		Script:  f.Script,
		PackDir: packDir,
	}, nil
}

// questSpawnCapPerStage bounds the total entities a single stage may spawn
// (quest-spawns.md §2 / §9) — a runaway guard, not a gameplay limit.
const questSpawnCapPerStage = 32

// decodeQuestSpawns validates the SHAPE of a stage's `spawns` block and
// namespace-qualifies its ids (quest-spawns.md §2). kind must be mob|item;
// template is required and qualified; room is qualified, defaulting to the
// stage's first `visit` objective target (a room) when omitted; count defaults
// to 1; the per-stage total is capped. objs are the already-qualified
// objectives of the same stage, used for the room default. Whether the
// qualified template/room ids actually RESOLVE is checked post-load in
// validateQuestSpawns (once every pack's registries are populated, so
// cross-pack dependency refs are visible) — a miss there fails the pack too.
// Returns nil for a stage with no spawns.
func decodeQuestSpawns(sf QuestStageFile, si int, ns, path string, objs []quest.Objective) ([]quest.Spawn, error) {
	if len(sf.Spawns) == 0 {
		return nil, nil
	}
	// The room to default an omitted spawn.room to: the first visit objective's
	// (already-qualified) target in this stage, if any.
	defaultRoom := ""
	for _, o := range objs {
		if strings.EqualFold(o.Type, "visit") && o.Target != "" {
			defaultRoom = o.Target
			break
		}
	}
	out := make([]quest.Spawn, 0, len(sf.Spawns))
	total := 0
	for spi, spf := range sf.Spawns {
		field := fmt.Sprintf("stage[%d].spawn[%d]", si, spi)
		kind := strings.ToLower(strings.TrimSpace(spf.Kind))
		if kind != "mob" && kind != "item" {
			return nil, fmt.Errorf("%w: %s: %s.kind %q must be \"mob\" or \"item\"", ErrInvalidContent, path, field, spf.Kind)
		}
		if strings.TrimSpace(spf.Template) == "" {
			return nil, fmt.Errorf("%w: %s: %s.template is required", ErrInvalidContent, path, field)
		}
		template, err := qualifyID(spf.Template, ns)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %s.template: %v", ErrInvalidContent, path, field, err)
		}
		room := defaultRoom
		if strings.TrimSpace(spf.Room) != "" {
			room, err = qualifyID(spf.Room, ns)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: %s.room: %v", ErrInvalidContent, path, field, err)
			}
		}
		if room == "" {
			return nil, fmt.Errorf("%w: %s: %s.room omitted and the stage has no visit objective to default from", ErrInvalidContent, path, field)
		}
		count := spf.Count
		if count < 1 {
			count = 1
		}
		total += count
		if total > questSpawnCapPerStage {
			return nil, fmt.Errorf("%w: %s: stage[%d] spawns %d entities, over the per-stage cap of %d", ErrInvalidContent, path, si, total, questSpawnCapPerStage)
		}
		out = append(out, quest.Spawn{Kind: kind, Template: template, Room: room, Count: count})
	}
	return out, nil
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

	// stat_bonuses is the metatype's starting-attribute skew (applied at
	// creation via AdjustBase). Unlike stat_caps, a negative value is allowed
	// (a metatype attribute penalty). Mirrors the empty-key guard.
	var bonuses map[progression.StatType]int
	if len(f.StatBonuses) > 0 {
		bonuses = make(map[progression.StatType]int, len(f.StatBonuses))
		for k, v := range f.StatBonuses {
			key := strings.ToLower(strings.TrimSpace(k))
			if key == "" {
				return nil, fmt.Errorf("%w: %s: stat_bonuses has empty key", ErrInvalidContent, path)
			}
			bonuses[progression.StatType(key)] = v
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

	// Size (size-and-wielding §3.1). Optional; empty ⇒ baseline at resolution.
	raceSize := strings.ToLower(strings.TrimSpace(f.Size))
	if raceSize != "" && !size.Valid(raceSize) {
		return nil, fmt.Errorf("%w: %s: size %q is not a known size %v",
			ErrInvalidContent, path, raceSize, size.Names())
	}

	return &progression.Race{
		ID:                f.ID,
		DisplayName:       strings.TrimSpace(f.Name),
		Tagline:           f.Tagline,
		Description:       f.Description,
		Category:          strings.TrimSpace(f.Category),
		StartingAlignment: f.StartingAlignment,
		StatCaps:          caps,
		StatBonuses:       bonuses,
		CastCostModifier:  f.CastCostModifier,
		RacialFlags:       flags,
		Size:              raceSize,
		Pack:              ns,
		Priority:          f.Priority,
	}, nil
}

// decodeBackground reads a BackgroundFile and builds a progression.Background
// (backgrounds §2). Mirrors decodeRace: validates the id, normalizes the skill
// grants (an empty ability id is a load error; a non-positive proficiency is
// left as-is and defaulted to the baseline at grant time). Skill/item id
// existence is NOT checked here — those are resolved fail-soft at creation.
func decodeBackground(path, ns string) (*progression.Background, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading background %s: %w", path, err)
	}
	var f BackgroundFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	var skills []progression.BackgroundSkill
	if len(f.Skills) > 0 {
		skills = make([]progression.BackgroundSkill, 0, len(f.Skills))
		for i, s := range f.Skills {
			if strings.TrimSpace(s.Ability) == "" {
				return nil, fmt.Errorf("%w: %s: skills[%d] missing 'ability'", ErrInvalidContent, path, i)
			}
			skills = append(skills, progression.BackgroundSkill{
				AbilityID:   strings.TrimSpace(s.Ability),
				Proficiency: s.Proficiency,
			})
		}
	}
	// Item grants are namespace-qualified at decode (matching recipes/loot,
	// loader.go decodeRecipe): a bare id resolves against this pack's ns, a
	// qualified id crosses packs. The granter looks templates up by their
	// qualified key, so storing bare ids here would never resolve at grant
	// time (backgrounds §4). An empty entry is a load error.
	items, err := qualifyIDList(f.Items, ns, path, "items")
	if err != nil {
		return nil, err
	}
	// Equipment packages are namespace-qualified per package (each inner list
	// is a mutually-exclusive bundle of item template ids), mirroring Items.
	var packages [][]string
	if len(f.EquipmentPackages) > 0 {
		packages = make([][]string, 0, len(f.EquipmentPackages))
		for i, pkg := range f.EquipmentPackages {
			q, err := qualifyIDList(pkg, ns, path, fmt.Sprintf("equipment_packages[%d]", i))
			if err != nil {
				return nil, err
			}
			packages = append(packages, q)
		}
	}
	// Language refs are namespace-qualified like items (a bare id resolves
	// against this pack's ns); the granter resolves the home language by its
	// qualified key, so a bare id stored here would never match at grant time
	// (languages.md §3). An empty home_language is fine (no grant).
	homeLang, err := qualifyOptional(f.HomeLanguage, ns, path, "home_language")
	if err != nil {
		return nil, err
	}
	bonusLangs, err := qualifyIDList(f.BonusLanguages, ns, path, "bonus_languages")
	if err != nil {
		return nil, err
	}
	return &progression.Background{
		ID:          f.ID,
		DisplayName: strings.TrimSpace(f.Name),
		Tagline:     f.Tagline,
		Description: f.Description,
		Skills:      skills,
		Items:       items,
		// Feat ids are global (not namespace-qualified, like abilities); the
		// granter resolves them fail-soft. Register lowercases them.
		Feats:             append([]string(nil), f.Feats...),
		FeatOptions:       append([]string(nil), f.FeatOptions...),
		EquipmentPackages: packages,
		Gold:              f.Gold,
		HomeLanguage:      homeLang,
		BonusLanguages:    bonusLangs,
		// Weapon-restriction categories are global strings (not namespace-
		// qualified, like feat ids + weapon_category itself); Register lowercases.
		WeaponRestrictions:       append([]string(nil), f.WeaponRestrictions...),
		WeaponRestrictionMessage: strings.TrimSpace(f.WeaponRestrictionMessage),
		AllowedCategories:        append([]string(nil), f.AllowedCategories...),
		AllowedGenders:           append([]string(nil), f.AllowedGenders...),
		Pack:                     ns,
		Priority:                 f.Priority,
	}, nil
}

// decodeLanguage reads a LanguageFile and builds a progression.Language
// (languages.md §2). Mirrors decodeBackground's id check; the language id is
// namespace-qualified (like items) so a background's qualified home_language
// reference resolves. The comprehension family is a grouping label, not an id
// reference, so it is NOT qualified (lowercased at Register).
func decodeLanguage(path, ns string) (*progression.Language, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading language %s: %w", path, err)
	}
	var f LanguageFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	id, err := qualifyID(f.ID, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: id: %v", ErrInvalidContent, path, err)
	}
	return &progression.Language{
		ID:          id,
		Name:        strings.TrimSpace(f.Name),
		Family:      strings.TrimSpace(f.Family),
		Description: f.Description,
		Pack:        ns,
		Priority:    f.Priority,
	}, nil
}

// decodeAttributeSet reads an AttributeSetFile and builds a
// progression.AttributeSet (SR-M1 — shadowrun-mvp.md Appendix A). The set id
// and the attribute ids are GLOBAL (not namespace-qualified): the set id is a
// vocabulary selected by manifest reference (like feats/abilities), and the
// attribute ids are the raw stat keys the StatBlock stores under. Structural
// validation (empty/duplicate ids) happens in Register; this only checks the
// set id is present and normalizes whitespace.
func decodeAttributeSet(path, ns string) (*progression.AttributeSet, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading attribute set %s: %w", path, err)
	}
	var f AttributeSetFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	attrs := make([]progression.Attribute, 0, len(f.Attributes))
	for _, a := range f.Attributes {
		attrs = append(attrs, progression.Attribute{
			ID:        progression.StatType(strings.TrimSpace(a.ID)),
			Name:      strings.TrimSpace(a.Name),
			Abbrev:    strings.TrimSpace(a.Abbrev),
			Default:   a.Default,
			Cap:       a.Cap,
			Trainable: a.Trainable,
			Category:  strings.TrimSpace(a.Category),
		})
	}
	return &progression.AttributeSet{
		ID:         strings.TrimSpace(f.ID),
		Name:       strings.TrimSpace(f.Name),
		Attributes: attrs,
		Pack:       ns,
		Priority:   f.Priority,
	}, nil
}

// decodePool reads a PoolFile and builds a pool.Decl (shadowrun-mvp SR-M3a).
// The pool kind is GLOBAL (not namespace-qualified) — it is a vocabulary the
// seeders and channel formulas reference by raw name, like an attribute stat
// key. Register lowercases the kind + overflow target and enforces the
// priority-override; this only checks the kind is present and normalizes
// whitespace/booleans into pool.Rules.
func decodePool(path, ns string) (*pool.Decl, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading pool %s: %w", path, err)
	}
	var f PoolFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	maxChannel := strings.TrimSpace(f.MaxChannel)
	maxFormula := strings.TrimSpace(f.MaxFormula)
	// A pool's ceiling comes from EITHER a flat stat (max_channel) OR a derived
	// formula (max_formula) — never both. Two sources would race at seed time;
	// reject the ambiguity loudly at load rather than silently pick one.
	if maxChannel != "" && maxFormula != "" {
		return nil, fmt.Errorf("%w: %s: pool %q sets both max_channel and max_formula (mutually exclusive)", ErrInvalidContent, path, f.ID)
	}
	// Validate the formula parses NOW so a malformed expression fails at boot,
	// not at the first entity seed (the seeder re-parses the stored source).
	if maxFormula != "" {
		if _, err := channel.Parse(maxFormula); err != nil {
			return nil, fmt.Errorf("%w: %s: pool %q max_formula: %v", ErrInvalidContent, path, f.ID, err)
		}
	}
	return &pool.Decl{
		Kind: pool.Kind(strings.TrimSpace(f.ID)),
		Rules: pool.Rules{
			Floor:          f.Floor,
			OverflowTo:     pool.Kind(strings.TrimSpace(f.OverflowTo)),
			Degrades:       strings.TrimSpace(f.Degrades),
			DepletionEvent: f.DepletionEvent,
			Nonlethal:      f.Nonlethal,
		},
		MaxChannel:   maxChannel,
		MaxFormula:   maxFormula,
		SeedOnPlayer: f.SeedOnPlayer,
		SeedOnMob:    f.SeedOnMob,
		Pack:         ns,
		Priority:     f.Priority,
	}, nil
}

// decodeFeat reads a FeatFile and builds a feat.Feat (EPIC S4 Phase 0 —
// docs/proposals/wot-feats.md §2.1). Validates the id, the multi-take rule,
// and each prerequisite's kind (+ that a non-level prereq names a target).
// Feat ids are GLOBAL (not namespace-qualified) like abilities; prereq feat /
// skill ids are content references resolved fail-soft when a feat is taken,
// not checked here.
func decodeFeat(path, ns string) (*feat.Feat, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading feat %s: %w", path, err)
	}
	var f FeatFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	mt := feat.MultiTake(strings.ToLower(strings.TrimSpace(f.MultiTake)))
	if mt == "" {
		mt = feat.MultiTakeSingle
	}
	if !feat.ValidMultiTake(mt) {
		return nil, fmt.Errorf("%w: %s: unknown multi_take %q (want single/per_param/stackable)",
			ErrInvalidContent, path, f.MultiTake)
	}
	var prereqs []feat.Prerequisite
	if len(f.Prerequisites) > 0 {
		prereqs = make([]feat.Prerequisite, 0, len(f.Prerequisites))
		for i, p := range f.Prerequisites {
			kind := feat.PrereqKind(strings.ToLower(strings.TrimSpace(p.Kind)))
			if !feat.ValidPrereqKind(kind) {
				return nil, fmt.Errorf("%w: %s: prerequisites[%d] unknown kind %q (want ability_score/feat/skill/level)",
					ErrInvalidContent, path, i, p.Kind)
			}
			target := strings.TrimSpace(p.Target)
			if kind != feat.PrereqLevel && target == "" {
				return nil, fmt.Errorf("%w: %s: prerequisites[%d] (%s) missing 'target'",
					ErrInvalidContent, path, i, kind)
			}
			prereqs = append(prereqs, feat.Prerequisite{Kind: kind, Target: target, Min: p.Min})
		}
	}
	var grants []feat.Grant
	if len(f.Grants) > 0 {
		grants = make([]feat.Grant, 0, len(f.Grants))
		for i, g := range f.Grants {
			kind := feat.GrantKind(strings.ToLower(strings.TrimSpace(g.Kind)))
			if !feat.ValidGrantKind(kind) {
				return nil, fmt.Errorf("%w: %s: grants[%d] unknown kind %q (see feat.GrantKind for valid kinds)",
					ErrInvalidContent, path, i, g.Kind)
			}
			// Per-kind validation.
			switch kind {
			// v1 grants are beneficial-only: a non-positive magnitude is a
			// content typo, not a drawback feat (a future flaw/drawback system
			// would relax this to allow negatives).
			case feat.GrantSaveBonus:
				if !feat.ValidSaveAxis(g.Target) {
					return nil, fmt.Errorf("%w: %s: grants[%d] save_bonus target %q is not a save axis (fortitude/reflex/will)",
						ErrInvalidContent, path, i, g.Target)
				}
				if g.Magnitude <= 0 {
					return nil, fmt.Errorf("%w: %s: grants[%d] save_bonus needs a positive magnitude",
						ErrInvalidContent, path, i)
				}
			case feat.GrantMaxHP:
				if g.Magnitude <= 0 {
					return nil, fmt.Errorf("%w: %s: grants[%d] max_hp needs a positive magnitude",
						ErrInvalidContent, path, i)
				}
			case feat.GrantACBonus:
				// Global AC grant (Dodge): beneficial-only, Target unused.
				if g.Magnitude <= 0 {
					return nil, fmt.Errorf("%w: %s: grants[%d] ac_bonus needs a positive magnitude",
						ErrInvalidContent, path, i)
				}
			case feat.GrantRenownBonus:
				// Global renown grant (Fame): beneficial-only, Target unused.
				if g.Magnitude <= 0 {
					return nil, fmt.Errorf("%w: %s: grants[%d] renown_bonus needs a positive magnitude",
						ErrInvalidContent, path, i)
				}
			case feat.GrantInfamy, feat.GrantLowProfile:
				// Boolean global flags (Infamy / Low Profile): Target and Magnitude
				// are unused — the presence of the grant is the whole effect.
			case feat.GrantAbility:
				if strings.TrimSpace(g.Target) == "" {
					return nil, fmt.Errorf("%w: %s: grants[%d] ability needs a target (ability id)",
						ErrInvalidContent, path, i)
				}
			case feat.GrantWeaponProficiency:
				// Fixed-target proficiency grant (Militia): Target = the weapon
				// category id; Magnitude unused (proficiency is boolean).
				if strings.TrimSpace(g.Target) == "" {
					return nil, fmt.Errorf("%w: %s: grants[%d] weapon_proficiency needs a target (weapon category id)",
						ErrInvalidContent, path, i)
				}
			case feat.GrantTwoWeaponHit, feat.GrantOffHandHit, feat.GrantOffHandAttack:
				// Global two-weapon grants (Two-Weapon Fighting / Ambidexterity /
				// Improved Two-Weapon Fighting): beneficial-only, so a non-positive
				// magnitude is a content typo. Target is unused.
				if g.Magnitude <= 0 {
					return nil, fmt.Errorf("%w: %s: grants[%d] %s needs a positive magnitude",
						ErrInvalidContent, path, i, kind)
				}
				if strings.TrimSpace(g.Target) != "" {
					return nil, fmt.Errorf("%w: %s: grants[%d] %s is a global grant and takes no target",
						ErrInvalidContent, path, i, kind)
				}
				// These are single-take perks in v1: a stackable penalty-reduction
				// would over-reduce (the consumer clamps at zero, silently wasting
				// extra ranks), and a stackable off-hand-attack count is intentionally
				// not yet a thing (the cap is two — one Improved TWF). Reject it at
				// load so the content surface stays honest.
				if mt == feat.MultiTakeStackable {
					return nil, fmt.Errorf("%w: %s: grants[%d] %s cannot be stackable (single-take in v1)",
						ErrInvalidContent, path, i, kind)
				}
			}
			// Per-weapon/skill kinds (hit_bonus/crit_threat/skill_bonus) take
			// their target from the take's PARAM, so the feat must be
			// per_param + carry a positive magnitude. Exception: a skill_bonus
			// MAY instead be a fixed-axis feat (Alertness → perception) that
			// names its skill via a grant Target; that form is single-take,
			// mirroring save_bonus.
			if feat.IsPerWeaponOrSkill(kind) {
				if g.Magnitude <= 0 {
					return nil, fmt.Errorf("%w: %s: grants[%d] %s needs a positive magnitude",
						ErrInvalidContent, path, i, kind)
				}
				fixedSkill := kind == feat.GrantSkillBonus && strings.TrimSpace(g.Target) != ""
				if !fixedSkill && mt != feat.MultiTakeParam {
					return nil, fmt.Errorf("%w: %s: grants[%d] %s requires multi_take: per_param (the param names the weapon/skill)",
						ErrInvalidContent, path, i, kind)
				}
			}
			grants = append(grants, feat.Grant{Kind: kind, Target: strings.TrimSpace(g.Target), Magnitude: g.Magnitude})
		}
	}
	return &feat.Feat{
		ID:             f.ID,
		DisplayName:    strings.TrimSpace(f.Name),
		Description:    f.Description,
		Prerequisites:  prereqs,
		Grants:         grants,
		MultiTake:      mt,
		AllowedClasses: append([]string(nil), f.AllowedClasses...),
		Pack:           ns,
		Priority:       f.Priority,
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
	var startingStats map[progression.StatType]int
	if len(f.StartingStats) > 0 {
		startingStats = make(map[progression.StatType]int, len(f.StartingStats))
		for k, v := range f.StartingStats {
			key := strings.ToLower(strings.TrimSpace(k))
			if key == "" {
				return nil, fmt.Errorf("%w: %s: starting_stats has empty key", ErrInvalidContent, path)
			}
			startingStats[progression.StatType(key)] = v
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
	// save_progressions (saves §2): validate axis + progression names at
	// load so a typo is an authoring error, not a silent weak-default. The
	// axis values match progression.SaveType; the curve values strong/weak.
	var saveProg map[progression.SaveType]progression.SaveProgression
	if len(f.SaveProgressions) > 0 {
		saveProg = make(map[progression.SaveType]progression.SaveProgression, len(f.SaveProgressions))
		for axis, prog := range f.SaveProgressions {
			a := progression.SaveType(strings.ToLower(strings.TrimSpace(axis)))
			switch a {
			case progression.SaveFortitude, progression.SaveReflex, progression.SaveWill:
			default:
				return nil, fmt.Errorf("%w: %s: save_progressions: unknown save axis %q (want fortitude/reflex/will)", ErrInvalidContent, path, axis)
			}
			p := progression.SaveProgression(strings.ToLower(strings.TrimSpace(prog)))
			switch p {
			case progression.SaveStrong, progression.SaveWeak:
			default:
				return nil, fmt.Errorf("%w: %s: save_progressions[%s]: unknown progression %q (want strong/weak)", ErrInvalidContent, path, axis, prog)
			}
			saveProg[a] = p
		}
	}
	return &progression.Class{
		ID:                    f.ID,
		DisplayName:           strings.TrimSpace(f.Name),
		Tagline:               f.Tagline,
		Description:           f.Description,
		LevelUpFlavor:         f.LevelUpFlavor,
		BoundTrack:            strings.TrimSpace(f.BoundTrack),
		StatGrowth:            growth,
		GrowthBonuses:         bonuses,
		StartingStats:         startingStats,
		Path:                  path2,
		TrainsPerLevel:        trains,
		AllowedCategories:     append([]string(nil), f.AllowedCategories...),
		AllowedGenders:        append([]string(nil), f.AllowedGenders...),
		AllowedGifts:          append([]string(nil), f.AllowedGifts...),
		ProficiencyTiers:      append([]string(nil), f.ProficiencyTiers...),
		ProficiencyCategories: append([]string(nil), f.ProficiencyCategories...),
		ArmorProficiencyTiers: append([]string(nil), f.ArmorProficiencyTiers...),
		SaveProgressions:      saveProg,
		StartingAlignment:     f.StartingAlignment,
		Pack:                  ns,
		Priority:              f.Priority,
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
// loadPackSplash reads and validates a world pack's connect splash file. The
// path is taken from the manifest's `splash:` field, resolved relative to the
// pack dir with the same escape guard resolveGlobs applies. It is a boot error
// (ErrInvalidContent) for a world pack to declare no splash, point at an
// unreadable file, or supply an empty one — a world must have a door identity.
// The returned text is the raw file contents (trailing newline trimmed); color
// markup is rendered downstream at display time.
func loadPackSplash(p Discovered) (string, error) {
	rel := strings.TrimSpace(p.Manifest.Splash)
	if rel == "" {
		return "", fmt.Errorf("%w: world pack %q declares no splash (add `splash: <file>` to the manifest)",
			ErrInvalidContent, p.Manifest.Name)
	}
	cleanRoot, err := filepath.Abs(p.Dir)
	if err != nil {
		return "", fmt.Errorf("resolving pack dir %s: %w", p.Dir, err)
	}
	full, err := filepath.Abs(filepath.Join(cleanRoot, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("resolving splash %s: %w", rel, err)
	}
	if full != cleanRoot && !strings.HasPrefix(full, cleanRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: world pack %q splash %q escapes the pack dir",
			ErrInvalidContent, p.Manifest.Name, rel)
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("%w: world pack %q splash %q: %v",
			ErrInvalidContent, p.Manifest.Name, rel, err)
	}
	text := strings.TrimRight(string(raw), "\n")
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%w: world pack %q splash %q is empty",
			ErrInvalidContent, p.Manifest.Name, rel)
	}
	return text, nil
}

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
		LightFloor:    af.LightFloor,
		Properties:    copyProperties(af.Properties),
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
			maps.Copy(out, terrains)
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

	// Hidden exits (hidden-exits §2): mark the matching exit secret. Like
	// doors, each key MUST name an existing exit — a hidden_exits entry with
	// no exit is a content authoring error caught at load.
	for dirStr, hf := range rf.HiddenExits {
		dir, ok := world.ParseDirection(dirStr)
		if !ok {
			return nil, nil, nil, fmt.Errorf("%w: %s: hidden_exits direction %q is not a Direction",
				ErrInvalidContent, path, dirStr)
		}
		exit, hasExit := r.Exits[dir]
		if !hasExit {
			return nil, nil, nil, fmt.Errorf("%w: %s: hidden_exits %s has no matching exit",
				ErrInvalidContent, path, dirStr)
		}
		if hf.SearchDifficulty < 0 {
			return nil, nil, nil, fmt.Errorf("%w: %s: hidden_exits %s: search_difficulty must be non-negative",
				ErrInvalidContent, path, dirStr)
		}
		exit.Hidden = true
		exit.SearchDifficulty = hf.SearchDifficulty
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

	// Weapon identity (weapon-identity §2). Category is an opaque label;
	// the tier and damage types validate against the engine vocabularies
	// so an authoring typo fails the pack by file name (like weapon_damage
	// above) rather than silently producing an unrecognized tier. All
	// three are optional — an absent tier is "untiered" (treated as the
	// lowest tier at proficiency-check time, §3); absent types are untyped.
	weaponCategory := strings.ToLower(strings.TrimSpace(f.WeaponCategory))
	weaponTier := strings.ToLower(strings.TrimSpace(f.ProficiencyTier))
	if weaponTier != "" && !item.ValidTier(weaponTier) {
		return nil, fmt.Errorf("%w: %s: proficiency_tier %q is not a known weapon tier %v",
			ErrInvalidContent, path, weaponTier, item.WeaponTierNames())
	}
	damageTypes := normalizeLowerDedup(f.DamageTypes)
	for _, dt := range damageTypes {
		if !item.ValidDamageType(dt) {
			return nil, fmt.Errorf("%w: %s: damage_types entry %q is not a valid damage type %v",
				ErrInvalidContent, path, dt, item.DamageTypeNames())
		}
	}

	// §4 critical threat range + multiplier. Unset (0) is valid — the
	// engine defaults at attack time. A declared threat-low must be in
	// [2,20] (a natural 1 is always a fumble, a face above 20 cannot
	// roll); a declared multiplier must be non-negative.
	if f.CritThreatLow != 0 && (f.CritThreatLow < 2 || f.CritThreatLow > 20) {
		return nil, fmt.Errorf("%w: %s: crit_threat_low %d out of range (expected 0 or 2..20)",
			ErrInvalidContent, path, f.CritThreatLow)
	}
	if f.CritMultiplier < 0 {
		return nil, fmt.Errorf("%w: %s: crit_multiplier %d must be non-negative",
			ErrInvalidContent, path, f.CritMultiplier)
	}

	// Ranged weapon metadata (ranged-combat §2). Optional; recorded only
	// this slice (no ammo/Strength consumer wired yet). ranged_class
	// validates against the engine vocabulary; a projectile weapon must name
	// the ammo kind it fires (a bow with no ammo_kind is an authoring error);
	// range_increment and a declared str_rating are non-negative.
	rangedClass := strings.ToLower(strings.TrimSpace(f.RangedClass))
	if rangedClass != "" && !item.ValidRangedClass(rangedClass) {
		return nil, fmt.Errorf("%w: %s: ranged_class %q is not a known ranged class %v",
			ErrInvalidContent, path, rangedClass, item.RangedClassNames())
	}
	ammoKind := strings.ToLower(strings.TrimSpace(f.AmmoKind))
	if rangedClass == item.RangedProjectile && ammoKind == "" {
		return nil, fmt.Errorf("%w: %s: a projectile weapon must declare ammo_kind (what it fires)",
			ErrInvalidContent, path)
	}
	// ranged_style is presentational (rangedflavor): normalized but NOT validated
	// against the flavor vocabulary here — an unknown/missing style resolves to
	// the default style and then the engine floor, so it never blocks a boot.
	rangedStyle := strings.ToLower(strings.TrimSpace(f.RangedStyle))
	if f.RangeIncrement < 0 {
		return nil, fmt.Errorf("%w: %s: range_increment %d must be non-negative",
			ErrInvalidContent, path, f.RangeIncrement)
	}
	if f.ReloadTicks < 0 {
		return nil, fmt.Errorf("%w: %s: reload_ticks %d must be non-negative",
			ErrInvalidContent, path, f.ReloadTicks)
	}
	if f.Magazine < 0 {
		return nil, fmt.Errorf("%w: %s: magazine %d must be non-negative",
			ErrInvalidContent, path, f.Magazine)
	}
	// essence_cost (Shadowrun SR-M4): the SR decimal an installed augmentation
	// spends is stored as integer tenths (the essence pool, like every pool, is
	// integer-valued). Convert authored 2.0 → 20, 0.2 → 2, rounding half-away.
	// Negative is rejected — an implant cannot refund Essence.
	if f.EssenceCost < 0 {
		return nil, fmt.Errorf("%w: %s: essence_cost %.2f must be non-negative",
			ErrInvalidContent, path, f.EssenceCost)
	}
	essenceCost := int(math.Round(f.EssenceCost * 10))
	// Item modification (item-modification.md — Slice A). A host declares a
	// non-negative capacity budget; a modification declares mod_host (the host
	// class it fits) + a non-negative capacity cost. mod_host is normalized
	// lowercase; mod_capacity_cost is meaningful only on a mod.
	if f.Capacity < 0 {
		return nil, fmt.Errorf("%w: %s: capacity %d must be non-negative",
			ErrInvalidContent, path, f.Capacity)
	}
	if f.ModCapacityCost < 0 {
		return nil, fmt.Errorf("%w: %s: mod_capacity_cost %d must be non-negative",
			ErrInvalidContent, path, f.ModCapacityCost)
	}
	modHost := strings.ToLower(strings.TrimSpace(f.ModHost))
	if modHost == "" && f.ModCapacityCost > 0 {
		return nil, fmt.Errorf("%w: %s: mod_capacity_cost %d set without mod_host (only a modification consumes capacity)",
			ErrInvalidContent, path, f.ModCapacityCost)
	}
	// Weapon accessories (weapon-accessories.md §2/§3): normalize the mount lists.
	// An accessory (accessory_mounts set) must declare mod_host and must NOT also
	// declare a capacity cost — a mod installs by the mount rule OR the capacity
	// rule, never both.
	mounts := normalizeLowerDedup(f.Mounts)
	accessoryMounts := normalizeLowerDedup(f.AccessoryMounts)
	modProtection := normalizeLowerDedup(f.Protection)
	if len(modProtection) > 0 && modHost == "" {
		return nil, fmt.Errorf("%w: %s: protection set without mod_host (only a modification grants protection)",
			ErrInvalidContent, path)
	}
	modGrants := normalizeLowerDedup(f.Grants)
	if len(modGrants) > 0 && modHost == "" {
		return nil, fmt.Errorf("%w: %s: grants set without mod_host (only a modification grants a capability)",
			ErrInvalidContent, path)
	}
	if len(mounts) > 0 && f.Capacity > 0 {
		return nil, fmt.Errorf("%w: %s: a host declares mounts (mount rule) OR capacity (capacity rule), not both",
			ErrInvalidContent, path)
	}
	if len(accessoryMounts) > 0 {
		if modHost == "" {
			return nil, fmt.Errorf("%w: %s: accessory_mounts set without mod_host (a mount accessory names the host class it fits)",
				ErrInvalidContent, path)
		}
		if f.ModCapacityCost > 0 {
			return nil, fmt.Errorf("%w: %s: a modification declares accessory_mounts (mount rule) OR mod_capacity_cost (capacity rule), not both",
				ErrInvalidContent, path)
		}
	}
	// reload_method is normalized (lowercase) but not vocabulary-validated —
	// only "clip" is consumed today; other SR5 methods are recorded-only. A
	// magazine weapon with no method defaults to "clip".
	reloadMethod := strings.ToLower(strings.TrimSpace(f.ReloadMethod))
	if f.Magazine > 0 && reloadMethod == "" {
		reloadMethod = "clip"
	}
	// Ammo-holder fields (ammo-and-reloading §2). A holder (holder_fits set)
	// carries rounds and needs both a capacity (magazine) and a round kind
	// (ammo_kind). A holder-fed weapon (accepts_holder set) fires from an
	// inserted holder and must NOT also declare its own magazine (that marks an
	// internally-fed weapon) — the two feed models are mutually exclusive.
	holderFits := strings.ToLower(strings.TrimSpace(f.HolderFits))
	acceptsHolder := strings.ToLower(strings.TrimSpace(f.AcceptsHolder))
	if holderFits != "" {
		if f.Magazine <= 0 {
			return nil, fmt.Errorf("%w: %s: an ammunition holder (holder_fits %q) must declare a positive magazine capacity",
				ErrInvalidContent, path, holderFits)
		}
		if ammoKind == "" {
			return nil, fmt.Errorf("%w: %s: an ammunition holder (holder_fits %q) must declare ammo_kind (the round it holds)",
				ErrInvalidContent, path, holderFits)
		}
	}
	if acceptsHolder != "" && f.Magazine > 0 {
		return nil, fmt.Errorf("%w: %s: a holder-fed weapon (accepts_holder %q) must not also declare magazine (that marks an internally-fed weapon)",
			ErrInvalidContent, path, acceptsHolder)
	}
	// preload seeds a holder's spawn load; only meaningful on a holder, clamped
	// to its capacity.
	preload := f.Preload
	if preload < 0 {
		preload = 0
	}
	if holderFits != "" && preload > f.Magazine {
		preload = f.Magazine
	}
	if f.StrRating != nil && *f.StrRating < 0 {
		return nil, fmt.Errorf("%w: %s: str_rating %d must be non-negative",
			ErrInvalidContent, path, *f.StrRating)
	}
	var strRating *int
	if f.StrRating != nil {
		v := *f.StrRating
		strRating = &v
	}

	// Size (size-and-wielding §2). Optional; empty ⇒ baseline at resolution.
	// A declared size must be in the engine vocabulary, caught at load.
	weaponSize := strings.ToLower(strings.TrimSpace(f.Size))
	if weaponSize != "" && !size.Valid(weaponSize) {
		return nil, fmt.Errorf("%w: %s: size %q is not a known size %v",
			ErrInvalidContent, path, weaponSize, size.Names())
	}

	// Armor depth (armor-depth §2). All optional; validated at load so an
	// authoring typo fails the pack by file name rather than producing
	// silent garbage (as the weapon fields above do). Recorded only this
	// slice — no combat/skill consumer wires them yet.
	armorTier := strings.ToLower(strings.TrimSpace(f.ArmorTier))
	if armorTier != "" && !item.ValidArmorTier(armorTier) {
		return nil, fmt.Errorf("%w: %s: armor_tier %q is not a known armor tier %v",
			ErrInvalidContent, path, armorTier, item.ArmorTierNames())
	}
	if f.ArmorBonus < 0 {
		return nil, fmt.Errorf("%w: %s: armor_bonus %d must be non-negative",
			ErrInvalidContent, path, f.ArmorBonus)
	}
	if f.ArmorMaxDex != nil && *f.ArmorMaxDex < 0 {
		return nil, fmt.Errorf("%w: %s: armor_max_dex %d must be non-negative",
			ErrInvalidContent, path, *f.ArmorMaxDex)
	}
	if f.ArmorCheckPenalty < 0 {
		return nil, fmt.Errorf("%w: %s: armor_check_penalty %d must be non-negative (it is a penalty magnitude)",
			ErrInvalidContent, path, f.ArmorCheckPenalty)
	}
	var resistances map[string]int
	for rawType, amount := range f.Resistances {
		dt := strings.ToLower(strings.TrimSpace(rawType))
		if !item.ValidDamageType(dt) {
			return nil, fmt.Errorf("%w: %s: resistances key %q is not a valid damage type %v",
				ErrInvalidContent, path, rawType, item.DamageTypeNames())
		}
		if amount < 0 {
			return nil, fmt.Errorf("%w: %s: resistances[%q] %d must be non-negative",
				ErrInvalidContent, path, dt, amount)
		}
		if resistances == nil {
			resistances = make(map[string]int, len(f.Resistances))
		}
		resistances[dt] = amount
	}

	// Copy the optional max-Dex pointer so the template does not alias the
	// decoded file struct (which the caller may reuse/free).
	var armorMaxDex *int
	if f.ArmorMaxDex != nil {
		v := *f.ArmorMaxDex
		armorMaxDex = &v
	}

	// Angreal (wot-the-one-power.md S2). An item is an angreal when it declares
	// either a non-zero rating or a gender; both are then required and validated
	// so an authoring slip (a power with no gender, or vice versa) fails the
	// pack by file name rather than producing a silently-inert device.
	angrealGender := strings.ToLower(strings.TrimSpace(f.AngrealGender))
	if f.AngrealPower != 0 || angrealGender != "" {
		if f.AngrealPower <= 0 {
			return nil, fmt.Errorf("%w: %s: angreal_power %d must be positive (it is an amplification rating)",
				ErrInvalidContent, path, f.AngrealPower)
		}
		if angrealGender != "male" && angrealGender != "female" {
			return nil, fmt.Errorf("%w: %s: angreal_gender %q must be \"male\" or \"female\"",
				ErrInvalidContent, path, f.AngrealGender)
		}
	}

	// Special-weapon tags (special-weapons.md §2, increment J). Each tag
	// validates against the engine vocabulary; normalized lowercase. The bonus
	// scalars are non-negative, and a bonus with no matching tag is an authoring
	// slip (an inert magnitude) that fails the pack by file name.
	special := normalizeLowerDedup(f.Special) // normalize + dedup, like elements/target_types
	hasTrip, hasDisarm := false, false
	for _, tag := range special {
		if !item.ValidSpecialTag(tag) {
			return nil, fmt.Errorf("%w: %s: special %q is not a known special-weapon tag %v",
				ErrInvalidContent, path, tag, item.SpecialTagNames())
		}
		switch tag {
		case item.SpecialTrip:
			hasTrip = true
		case item.SpecialDisarm:
			hasDisarm = true
		}
	}
	if f.TripBonus < 0 {
		return nil, fmt.Errorf("%w: %s: trip_bonus %d must be non-negative",
			ErrInvalidContent, path, f.TripBonus)
	}
	if f.DisarmBonus < 0 {
		return nil, fmt.Errorf("%w: %s: disarm_bonus %d must be non-negative",
			ErrInvalidContent, path, f.DisarmBonus)
	}
	if f.TripBonus > 0 && !hasTrip {
		return nil, fmt.Errorf("%w: %s: trip_bonus %d set without the \"trip\" special tag",
			ErrInvalidContent, path, f.TripBonus)
	}
	if f.DisarmBonus > 0 && !hasDisarm {
		return nil, fmt.Errorf("%w: %s: disarm_bonus %d set without the \"disarm\" special tag",
			ErrInvalidContent, path, f.DisarmBonus)
	}
	if f.Reach < 0 {
		return nil, fmt.Errorf("%w: %s: reach %d must be non-negative",
			ErrInvalidContent, path, f.Reach)
	}

	// Recorded-only equipment-depth metadata (no consumer yet). double_damage is
	// a dice string validated like weapon_damage so an authoring typo fails the
	// pack; armor_speed is non-negative; subdual/reputation need no validation
	// (a bool and a signed delta).
	doubleDamage := strings.TrimSpace(f.DoubleDamage)
	if doubleDamage != "" {
		if _, derr := combat.ParseDice(doubleDamage); derr != nil {
			return nil, fmt.Errorf("%w: %s: double_damage %q: %v", ErrInvalidContent, path, doubleDamage, derr)
		}
	}
	if f.ArmorSpeed < 0 {
		return nil, fmt.Errorf("%w: %s: armor_speed %d must be non-negative",
			ErrInvalidContent, path, f.ArmorSpeed)
	}

	return &item.Template{
		ID:                item.TemplateID(id),
		Name:              f.Name,
		Type:              f.Type,
		Description:       strings.TrimSpace(f.Description),
		Tags:              f.Tags,
		Keywords:          f.Keywords,
		Properties:        f.Properties,
		Modifiers:         mods,
		WeaponDamage:      weaponDamage,
		EligibleSlots:     eligible,
		CompanionSlots:    companion,
		WeaponCategory:    weaponCategory,
		ProficiencyTier:   weaponTier,
		DamageTypes:       damageTypes,
		TargetPool:        strings.ToLower(strings.TrimSpace(f.TargetPool)),
		Grade:             strings.ToLower(strings.TrimSpace(f.Grade)),
		CritThreatLow:     f.CritThreatLow,
		CritMultiplier:    f.CritMultiplier,
		Size:              weaponSize,
		RangedClass:       rangedClass,
		AmmoKind:          ammoKind,
		RangedStyle:       rangedStyle,
		RangeIncrement:    f.RangeIncrement,
		ReloadTicks:       f.ReloadTicks,
		Magazine:          f.Magazine,
		ReloadMethod:      reloadMethod,
		HolderFits:        holderFits,
		Preload:           preload,
		AcceptsHolder:     acceptsHolder,
		StrRating:         strRating,
		ArmorBonus:        f.ArmorBonus,
		ArmorMaxDex:       armorMaxDex,
		ArmorCheckPenalty: f.ArmorCheckPenalty,
		ArmorTier:         armorTier,
		Resistances:       resistances,
		AngrealPower:      f.AngrealPower,
		AngrealGender:     angrealGender,
		Special:           special,
		Reach:             f.Reach,
		TripBonus:         f.TripBonus,
		DisarmBonus:       f.DisarmBonus,
		Subdual:           f.Subdual,
		DoubleDamage:      doubleDamage,
		ArmorSpeed:        f.ArmorSpeed,
		Reputation:        f.Reputation,
		EssenceCost:       essenceCost,
		Capacity:          f.Capacity,
		ModHost:           modHost,
		ModCapacityCost:   f.ModCapacityCost,
		Mounts:            mounts,
		AccessoryMounts:   accessoryMounts,
		Protection:        modProtection,
		Grants:            modGrants,
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

	def, err := decodeDispositionRules(f.DispositionRules, path, ns)
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

	// faction is namespace-qualified so it matches the (qualified) id the
	// faction registry stores (faction.md §5.2); empty stays empty.
	factionID, err := qualifyOptional(f.Faction, ns, path, "faction")
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

	// Size (size-and-wielding §3.2). Optional; empty ⇒ baseline at resolution.
	mobSize := strings.ToLower(strings.TrimSpace(f.Size))
	if mobSize != "" && !size.Valid(mobSize) {
		return nil, fmt.Errorf("%w: %s: size %q is not a known size %v",
			ErrInvalidContent, path, mobSize, size.Names())
	}

	// Mount block (mounts.md §2). Optional; presence marks the mob a mount.
	mountSpec, err := decodeMount(f.Mount, path)
	if err != nil {
		return nil, err
	}

	// Hireling block (hireable-mobs.md §2). Optional; presence marks it hireable.
	hirelingSpec, err := decodeHireling(f.Hireling, path)
	if err != nil {
		return nil, err
	}

	// Recruiter block (hireable-mobs.md §3.1). Optional; presence marks it a
	// hiring access point.
	recruiterSpec, err := decodeRecruiter(f.Recruiter, path)
	if err != nil {
		return nil, err
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
		Faction:             factionID,
		XPValue:             f.XPValue,
		Proficiencies:       profs,
		Race:                strings.ToLower(strings.TrimSpace(f.Race)),
		Class:               strings.ToLower(strings.TrimSpace(f.Class)),
		Level:               f.Level,
		Size:                mobSize,
		TrainerTier:         tier,
		TrainerTeach:        teach,
		Mount:               mountSpec,
		Hireling:            hirelingSpec,
		Recruiter:           recruiterSpec,
	}, nil
}

// decodeHireling converts a mob's optional `hireling:` block (hireable-mobs.md
// §2) into a validated *mob.HirelingSpec. Returns (nil, nil) when the block is
// absent — an ordinary, non-hireable mob. Requires non-negative gold sinks.
func decodeHireling(f *HirelingFile, path string) (*mob.HirelingSpec, error) {
	if f == nil {
		return nil, nil
	}
	if f.HireCost < 0 {
		return nil, fmt.Errorf("%w: %s: hireling hire_cost must not be negative (got %d)",
			ErrInvalidContent, path, f.HireCost)
	}
	if f.Upkeep < 0 {
		return nil, fmt.Errorf("%w: %s: hireling upkeep must not be negative (got %d)",
			ErrInvalidContent, path, f.Upkeep)
	}
	return &mob.HirelingSpec{HireCost: f.HireCost, Upkeep: f.Upkeep}, nil
}

// decodeRecruiter converts a mob's optional `recruiter:` block (hireable-mobs.md
// §3.1) into a validated *mob.RecruiterSpec. Returns (nil, nil) when the block is
// absent — a mob that is not a recruiter. Requires a non-empty offers list of
// non-empty (trimmed) entries; the offered ids are resolved to hireable templates
// at hire time, not here (load order within a pack is not guaranteed).
func decodeRecruiter(f *RecruiterFile, path string) (*mob.RecruiterSpec, error) {
	if f == nil {
		return nil, nil
	}
	offers := make([]string, 0, len(f.Offers))
	for _, o := range f.Offers {
		if t := strings.TrimSpace(o); t != "" {
			offers = append(offers, t)
		}
	}
	if len(offers) == 0 {
		return nil, fmt.Errorf("%w: %s: recruiter offers must be a non-empty list of hireling ids",
			ErrInvalidContent, path)
	}
	return &mob.RecruiterSpec{Offers: offers}, nil
}

// decodeMount converts a mob's optional `mount:` block (mounts.md §2.1) into a
// validated *mob.MountSpec. Returns (nil, nil) when the block is absent — an
// ordinary mob. Validates the temperament against the mount vocabulary (like
// size against the size vocabulary) and requires a positive travel_max so a
// mount can never spawn unable to move while ridden.
func decodeMount(f *MountFile, path string) (*mob.MountSpec, error) {
	if f == nil {
		return nil, nil
	}
	temperament := strings.ToLower(strings.TrimSpace(f.Temperament))
	if temperament != "" && !mount.Valid(temperament) {
		return nil, fmt.Errorf("%w: %s: mount temperament %q is not a known temperament %v",
			ErrInvalidContent, path, temperament, mount.Names())
	}
	if f.TravelMax <= 0 {
		return nil, fmt.Errorf("%w: %s: mount travel_max must be positive (got %d)",
			ErrInvalidContent, path, f.TravelMax)
	}
	if f.TravelRegen < 0 {
		return nil, fmt.Errorf("%w: %s: mount travel_regen must not be negative (got %d)",
			ErrInvalidContent, path, f.TravelRegen)
	}
	var impassable []string
	seen := make(map[string]struct{})
	for _, terr := range f.Impassable {
		t := strings.ToLower(strings.TrimSpace(terr))
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		impassable = append(impassable, t)
	}
	return &mob.MountSpec{
		Temperament: temperament,
		TravelMax:   f.TravelMax,
		TravelRegen: f.TravelRegen,
		Impassable:  impassable,
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
func decodeDispositionRules(src *DispositionFile, path, ns string) (*mob.Definition, error) {
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
		// faction.md §6 standing clause: qualify the faction id against this
		// pack's namespace so a bare `faction: children-of-the-light` matches
		// the qualified id the faction registry + player standing bag store.
		if fid := strings.TrimSpace(r.Faction); fid != "" {
			qid, err := qualifyID(fid, ns)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: disposition_rules[%d]: faction: %v", ErrInvalidContent, path, i, err)
			}
			rule.Faction = qid
		}
		if r.MinStanding != nil {
			rule.MinStanding = *r.MinStanding
			rule.HasMinStanding = true
		}
		if r.MaxStanding != nil {
			rule.MaxStanding = *r.MaxStanding
			rule.HasMaxStanding = true
		}
		// reputation.md §6/§7 renown clause (single-axis, no id to qualify).
		if r.MinRenown != nil {
			rule.MinRenown = *r.MinRenown
			rule.HasMinRenown = true
		}
		if r.Infamous != nil {
			rule.RequireInfamous = *r.Infamous
			rule.HasInfamous = true
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

// decodeChannel reads a ChannelFile and builds a chat.Channel
// (chat-channels-and-tells §3). Required: id, display_name. The id is
// namespace-qualified; kind defaults to public and must be public or gated.
func decodeChannel(path, ns string) (chat.Channel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return chat.Channel{}, fmt.Errorf("reading channel %s: %w", path, err)
	}
	var f ChannelFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return chat.Channel{}, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return chat.Channel{}, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(f.DisplayName) == "" {
		return chat.Channel{}, fmt.Errorf("%w: %s: missing 'display_name'", ErrInvalidContent, path)
	}
	if f.BufferCap < 0 {
		return chat.Channel{}, fmt.Errorf("%w: %s: buffer_cap must be >= 0", ErrInvalidContent, path)
	}
	kind := chat.KindPublic
	if k := strings.ToLower(strings.TrimSpace(f.Kind)); k != "" {
		switch chat.Kind(k) {
		case chat.KindPublic, chat.KindGated:
			kind = chat.Kind(k)
		default:
			return chat.Channel{}, fmt.Errorf("%w: %s: unknown kind %q (want public or gated)",
				ErrInvalidContent, path, f.Kind)
		}
	}
	id, err := qualifyID(f.ID, ns)
	if err != nil {
		return chat.Channel{}, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	return chat.Channel{
		ID:          id,
		DisplayName: strings.TrimSpace(f.DisplayName),
		Kind:        kind,
		DefaultOn:   f.DefaultOn,
		Persisted:   f.Persisted,
		BufferCap:   f.BufferCap,
		SpeakGate:   f.SpeakGate,
		ListenGate:  f.ListenGate,
	}, nil
}

// decodeEmote reads an EmoteFile and builds an emote.Emote (emotes.md §2).
// Required: id, display_name. The id is namespace-qualified. View-shape
// validity (NoTarget required unless RequiresTarget; Targeted required) is
// enforced by emote.Registry.Register via Emote.Validate at registration.
func decodeEmote(path, ns string) (emote.Emote, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return emote.Emote{}, fmt.Errorf("reading emote %s: %w", path, err)
	}
	var f EmoteFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return emote.Emote{}, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return emote.Emote{}, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(f.DisplayName) == "" {
		return emote.Emote{}, fmt.Errorf("%w: %s: missing 'display_name'", ErrInvalidContent, path)
	}
	id, err := qualifyID(f.ID, ns)
	if err != nil {
		return emote.Emote{}, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	return emote.Emote{
		ID:             id,
		DisplayName:    strings.TrimSpace(f.DisplayName),
		Aliases:        f.Aliases,
		RequiresTarget: f.RequiresTarget,
		NoTarget: emote.View{
			ActorView:  f.NoTarget.Actor,
			TargetView: f.NoTarget.Target,
			RoomView:   f.NoTarget.Room,
		},
		Targeted: emote.View{
			ActorView:  f.Targeted.Actor,
			TargetView: f.Targeted.Target,
			RoomView:   f.Targeted.Room,
		},
	}, nil
}

// decodeRangedFlavor reads a RangedFlavorFile and builds a rangedflavor.Style.
// Required: id (the `ranged_style` key). Ids are a GLOBAL vocabulary (like slot
// names), so they are NOT namespace-qualified. Every message key/audience is
// optional — an omitted one falls through the resolver's default→floor chain.
func decodeRangedFlavor(path string) (rangedflavor.Style, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return rangedflavor.Style{}, fmt.Errorf("reading ranged_flavor %s: %w", path, err)
	}
	var f RangedFlavorFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return rangedflavor.Style{}, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	id := strings.ToLower(strings.TrimSpace(f.ID))
	if id == "" {
		return rangedflavor.Style{}, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	msgs := make(map[string]rangedflavor.Line, len(f.Messages))
	for key, line := range f.Messages {
		msgs[strings.ToLower(strings.TrimSpace(key))] = rangedflavor.Line{Self: line.Self, Room: line.Room}
	}
	return rangedflavor.Style{ID: id, Msgs: msgs}, nil
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
	maps.Copy(out, src)
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

// decodeProperty reads a content-declared property file into a property.Entry.
// The `type` string is mapped to a property.ValueType; `name` is required and
// the registry enforces snake_case + uniqueness at registration. Pack scoping
// (Entry.Pack) is set by RegisterPack, so it is left empty here.
func decodeProperty(path string) (property.Entry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return property.Entry{}, fmt.Errorf("reading property %s: %w", path, err)
	}
	var pf PropertyFile
	if err := yaml.Unmarshal(raw, &pf); err != nil {
		return property.Entry{}, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(pf.Name) == "" {
		return property.Entry{}, fmt.Errorf("%w: %s: missing 'name'", ErrInvalidContent, path)
	}
	vt, ok := parseValueType(pf.Type)
	if !ok {
		return property.Entry{}, fmt.Errorf("%w: %s: unknown property type %q (want one of string/int/int64/float64/bool/map_int/map_string/list_string)",
			ErrInvalidContent, path, pf.Type)
	}
	return property.Entry{
		Name:          strings.TrimSpace(pf.Name),
		Type:          vt,
		Description:   pf.Description,
		AppliesTo:     pf.AppliesTo,
		AdminSettable: pf.AdminSettable,
		Transient:     pf.Transient,
	}, nil
}

// parseValueType maps a content `type:` string to a property.ValueType,
// mirroring property.ValueType.String(). Returns false for an unknown type.
func parseValueType(s string) (property.ValueType, bool) {
	switch strings.TrimSpace(s) {
	case "string":
		return property.TypeString, true
	case "int":
		return property.TypeInt, true
	case "int64":
		return property.TypeInt64, true
	case "float64":
		return property.TypeFloat64, true
	case "bool":
		return property.TypeBool, true
	case "map_int":
		return property.TypeMapInt, true
	case "map_string":
		return property.TypeMapString, true
	case "list_string":
		return property.TypeListString, true
	default:
		return 0, false
	}
}

// validateAreaProperties is validateRoomProperties for the area property bag:
// every entry must be a registered property whose registered type matches the
// authored value, resolved with `ns` as the current-pack shorthand context.
func validateAreaProperties(a *world.Area, reg *property.Registry, ns string) error {
	if a == nil || len(a.Properties) == 0 || reg == nil {
		return nil
	}
	for name, raw := range a.Properties {
		entry, ok := reg.Get(name, ns)
		if !ok {
			return fmt.Errorf("%w: area %q property %q is not registered",
				ErrInvalidContent, a.ID, name)
		}
		if !valueMatchesType(raw, entry.Type) {
			return fmt.Errorf("%w: area %q property %q: value %T does not match registered type %s",
				ErrInvalidContent, a.ID, name, raw, entry.Type)
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
func valueMatchesType(v any, t property.ValueType) bool {
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
