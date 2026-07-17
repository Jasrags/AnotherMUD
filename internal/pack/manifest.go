// Package pack discovers content packs, parses their manifests, and
// orders them for the two-phase loader (spec: scripting-and-packs §2–§3).
//
// M2 scope: manifest parse, discovery walk, dependency-ordered load
// sequence. Content loading (rooms, areas) lives in later phases of M2.
package pack

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// EngineNamespace is the reserved namespace for engine-scope packs
// (spec §2.3). Tags/properties declared by a pack with this namespace
// are visible to all packs without a qualifier.
const EngineNamespace = "tapestry-core"

// Advancement strategy tokens for the manifest `advancement:` field
// (shadowrun-mvp.md SR-M5). See Manifest.Advancement.
const (
	// AdvancementLevelTrack is the default: rewards bank XP onto the class
	// bound_track and level up. Also the value an empty `advancement:` means.
	AdvancementLevelTrack = "level-track"
	// AdvancementKarmaLedger routes rewards into a spendable karma balance;
	// the player raises skills/attributes/qualities via `improve` (no levels).
	AdvancementKarmaLedger = "karma-ledger"
)

// ManifestFilenames are the filenames the loader recognizes as manifests,
// in lookup order (spec §2.4 acceptance criteria).
var ManifestFilenames = []string{"pack.yaml", "tapestry.yaml"}

// Sentinel errors so callers can switch on classes of failure.
var (
	ErrManifestMissing = errors.New("pack manifest missing")
	ErrManifestInvalid = errors.New("pack manifest invalid")
)

// Manifest is the parsed pack.yaml / tapestry.yaml content.
//
// Only fields meaningful to M2 are wired today. Cosmetic and
// help-precedence fields are accepted (so manifests don't fail YAML
// strict-mode) but unused.
type Manifest struct {
	// Name is the pack name — bare ("legends-forgotten") or scoped
	// ("@scope/pack"). Required.
	Name string `yaml:"name"`

	// Active controls whether the pack is loaded at all (spec §2.1).
	// Default true; only false skips the pack entirely.
	Active *bool `yaml:"active,omitempty"`

	// Kind flags whether this pack is a "world" (a leaf ruleset a
	// character may belong to) or a "library" (a shared baseline
	// dependency). Empty defaults to "library", so a pack must opt in to
	// being a world and a baseline like the engine pack is never a valid
	// world stamp (character-identity §2). Validated at load: only
	// "", "world", or "library" are accepted.
	Kind string `yaml:"kind,omitempty"`

	// Version is informational today (spec §2.2).
	Version string `yaml:"version,omitempty"`

	// Dependencies maps pack-name → version-constraint. Only the keys
	// are consulted by the loader (spec §2.2, §3.2); the values are
	// not interpreted today.
	Dependencies map[string]string `yaml:"dependencies,omitempty"`

	// LoadOrder affects help-topic precedence only (spec §2.2).
	// Pack discovery/load order is alphabetical with dep-aware sort.
	LoadOrder int `yaml:"load_order,omitempty"`

	// Validation is "strict" (default) or "lenient" (spec §2.2).
	Validation string `yaml:"validation,omitempty"`

	// Splash is the path (relative to the pack dir) of the connect splash
	// screen shown before the account prompt (character-select §4 / login).
	// REQUIRED for kind:world packs — the world's door identity — and
	// validated at load: a world pack with no splash, or an unreadable/empty
	// splash file, fails boot. Library packs ignore it (they are never a
	// connect door). The file is plain text with engine color markup
	// ({Y}…{x}); it is rendered through the theme at display time.
	Splash string `yaml:"splash,omitempty"`

	// AttributeSet selects which content-declared base attribute set a
	// world seeds its characters from (SR-M1 — shadowrun-mvp.md Appendix A).
	// Names an id registered via some pack's `attribute_sets:` content (e.g.
	// "classic", "shadowrun5"). Empty → the engine `classic` fallback. Only
	// meaningful on kind:world packs (a library declares attribute-set content
	// but does not select one for characters).
	//
	// BREAKING MIGRATION: changing this after characters of this world exist on
	// disk corrupts their base stats. The seed uses the NEW set's keys while the
	// save persists the OLD set's keys, and RestoreBase merges them — leaving
	// both sets' keys on the character (the carries-both-sets bug this feature
	// exists to prevent, re-triggered via content). Treat a change like a save
	// shape change: bump player.CurrentVersion and add a migration that rewrites
	// the persisted base to the new set, the same discipline the save version
	// chain follows.
	AttributeSet string `yaml:"attribute_set,omitempty"`

	// Currency selects this world's money-DISPLAY vocabulary (the Shadowrun
	// nuyen/¥ reskin vs the "gold" default). Only meaningful on kind:world packs;
	// resolved boot-wide from the primary world, like Splash. Absent → the gold
	// default. Display-only — the ledger is a single int either way — so unlike
	// AttributeSet this is NOT a breaking/migration field; change it freely.
	Currency *CurrencyManifest `yaml:"currency,omitempty"`

	// StealthSkill selects the single skill ability that BOTH concealment axes
	// (hide + move-silently) read for this world (skills §2 — SR merges D&D's
	// two stealth skills into one "Sneaking"). Only meaningful on kind:world
	// packs. Absent → the engine default, where the hide verb reads the `hide`
	// ability and the sneak verb reads `move-silently` (the two-axis fantasy
	// model). Set it to a single id (SR: `sneaking`) to route both verbs' checks
	// through that one proficiency. Behavior-only, not a save field — change it
	// freely.
	StealthSkill string `yaml:"stealth_skill,omitempty"`

	// Advancement selects this world's advancement STRATEGY (shadowrun-mvp.md
	// SR-M5, Decision D3): how earned rewards (kills, quests) turn into character
	// growth. Only meaningful on kind:world packs. Two values:
	//   - "" / "level-track" (default): rewards bank XP onto the character's
	//     class bound_track; crossing a threshold levels up and the class grants
	//     stat growth + abilities. The engine's original model (WoT/core).
	//   - "karma-ledger": rewards bank spendable KARMA (no track XP, no levels);
	//     the player raises skills/attributes/qualities à la carte via `improve`.
	//     Canon-faithful Shadowrun (level-less). A karma-ledger world's classes
	//     are a creation package only — their bound_track never accrues XP.
	// Behavior/routing-only, NOT a save-shape field (a level-track character
	// simply carries no karma block); change it freely. An unknown value is
	// rejected at load so a typo can't silently fall back to level-track.
	Advancement string `yaml:"advancement,omitempty"`

	// Content paths — only the categories M2 cares about are listed.
	// Unknown keys are tolerated (no strict YAML decoding) so future
	// content types do not fail manifests authored ahead of time.
	Content ContentPaths `yaml:"content,omitempty"`
}

// CurrencyManifest is a world's money-display vocabulary in YAML form (the
// decoded shape of the manifest `currency:` block), converted to an
// economy.CurrencyLabel at load. Both fields optional; an omitted field falls
// back to the gold default at display time.
type CurrencyManifest struct {
	// Name is the currency noun for prose ("nuyen"); empty → "gold".
	Name string `yaml:"name,omitempty"`
	// Suffix is appended to an amount, INCLUDING any leading space the format
	// wants: "¥" → "725¥", " gold" → "725 gold". Empty → " gold".
	//
	// The leading space is significant beyond spacing: it classifies the unit.
	// A suffix that starts with a regular ASCII space is treated as a spelled-out
	// WORD (" gold", " credits") and is dropped from the score purse row, where a
	// noun label already names the currency (so it reads "Gold: 1,250", not
	// "Gold: 1,250 gold"). A suffix with no leading space is treated as a SYMBOL
	// ("¥", "$") and is kept everywhere ("Nuyen: 1,250¥"). So: word units MUST be
	// written with a leading ASCII space; symbol units MUST hug the number.
	Suffix string `yaml:"suffix,omitempty"`
}

// ContentPaths enumerates per-category file globs (spec §2.2 "content"
// block). Paths are relative to the pack directory.
type ContentPaths struct {
	Areas []string `yaml:"areas,omitempty"`
	Rooms []string `yaml:"rooms,omitempty"`
	// Properties declares content-defined property keys (persistence §2.2).
	// Each file registers a PACK-scoped property (namespaced to this pack, and
	// visible to packs that depend on it); an entry that shadows an engine
	// baseline property is a load error. Loaded EARLY — before this pack's
	// areas/rooms — so their `properties:` bags can reference keys this pack
	// declares. This is the content-side counterpart to the engine baseline in
	// `properties.go`; it lets a world pack add its own metadata keys without
	// an engine edit.
	Properties  []string `yaml:"properties,omitempty"`
	Items       []string `yaml:"items,omitempty"`
	Slots       []string `yaml:"slots,omitempty"`
	Mobs        []string `yaml:"mobs,omitempty"`
	Tracks      []string `yaml:"tracks,omitempty"`
	Races       []string `yaml:"races,omitempty"`
	Classes     []string `yaml:"classes,omitempty"`
	Backgrounds []string `yaml:"backgrounds,omitempty"`
	// AttributeSets declares content-defined base attribute sets (SR-M1 —
	// shadowrun-mvp.md Appendix A). Loaded into Registries.AttributeSets; set
	// ids are GLOBAL (not namespace-qualified), higher priority wins. The core
	// pack ships the `classic` six; a world pack may declare its own.
	AttributeSets []string `yaml:"attribute_sets,omitempty"`
	// Pools declares content-defined resource pools (shadowrun-mvp SR-M3a).
	// Loaded into Registries.Pools; pool kinds are GLOBAL (not
	// namespace-qualified), higher priority wins. The core pack declares
	// mana/movement; a world pack (Shadowrun) declares its Stun/Physical monitors.
	Pools     []string `yaml:"pools,omitempty"`
	Languages []string `yaml:"languages,omitempty"`
	Feats     []string `yaml:"feats,omitempty"`
	Abilities []string `yaml:"abilities,omitempty"`
	// Factions declares faction/standing definition files (faction.md §2).
	// Loaded into Registries.Factions; ids are namespace-qualified at load.
	Factions []string `yaml:"factions,omitempty"`
	Theme    []string `yaml:"theme,omitempty"`
	// ChannelMap declares combat-channel derivation files (the channel
	// layer — docs/themes/channel-vocabulary.md §7). Loaded into
	// Registries.ChannelMap; later packs override earlier per-channel.
	// Distinct from Channels (chat) further down.
	ChannelMap []string `yaml:"channel_map,omitempty"`
	Help       []string `yaml:"help,omitempty"`
	Quests     []string `yaml:"quests,omitempty"`
	Effects    []string `yaml:"effects,omitempty"`
	// Rarity / Essence are the M20 item-decoration vocabularies.
	// Loaded into Registries.Rarity / .Essence; an item's reserved
	// rarity/essence property references a key these define.
	Rarity  []string `yaml:"rarity,omitempty"`
	Essence []string `yaml:"essence,omitempty"`
	// Grades are the masterwork quality-grade vocabulary files (masterwork
	// §2). Loaded into Registries.Grades; an item's `grade:` key references
	// a grade these define.
	Grades []string `yaml:"grades,omitempty"`
	// LootTables are loot-table definition files (M22.1 —
	// mobs-ai-spawning §6.3). Loaded into Registries.Loot; a mob
	// template's `loot_table` field references a table by id.
	LootTables []string `yaml:"loot_tables,omitempty"`
	// WeatherZones are zone-definition files for the M15.4
	// weather substrate (spec world-rooms-movement §6).
	// Loaded into Registries.Weather; areas reference zones
	// by id through their `weather_zone` field.
	WeatherZones []string `yaml:"weather_zones,omitempty"`
	// Recipes are crafting-recipe definition files
	// (crafting-and-cooking §3). Loaded into Registries.Recipes; a
	// recipe's inputs/output reference item template ids and its
	// discipline references a crafting proficiency (ability id).
	Recipes []string `yaml:"recipes,omitempty"`
	// Biomes are biome-definition files (biomes.md §2). Loaded into
	// Registries.Biomes; a room's `terrain` value keys into this
	// registry for shielding, ambience, and the gathering resource
	// tables. Unregistered terrain values keep today's bare-string
	// behavior (§2.3).
	Biomes []string `yaml:"biomes,omitempty"`
	// ForageTables are ambient-forage resource pools (gathering.md §2).
	// Loaded into Registries.ForageTables; a biome's `forage_table` id
	// references one. Item ids + the table id are namespace-qualified at
	// load (like loot tables).
	ForageTables []string `yaml:"forage_tables,omitempty"`
	// NodeTemplates / NodeSpawnTables are resource-node definitions +
	// per-biome node spawn tables (gathering.md §3). Loaded into
	// Registries.Nodes; a node references a forage table as its yield, and
	// a biome's `node_spawn_table` id references a spawn table. Ids + refs
	// are namespace-qualified at load.
	NodeTemplates   []string `yaml:"node_templates,omitempty"`
	NodeSpawnTables []string `yaml:"node_spawn_tables,omitempty"`
	// Channels are chat-channel definitions (chat-channels-and-tells
	// §3). Loaded into Registries.Channels; the engine baseline (ooc)
	// ships in the core pack. Ids are namespace-qualified at load.
	Channels []string `yaml:"channels,omitempty"`
	// Emotes are social-emote definitions (emotes.md §2). Loaded into
	// Registries.Emotes; the engine baseline (smile/nod/…) ships in the
	// core pack. Ids are namespace-qualified at load.
	Emotes []string `yaml:"emotes,omitempty"`
	// RangedFlavor are ranged-weapon flavor-style files (rangedflavor). Loaded
	// into Registries.RangedFlavor; a weapon's `ranged_style` keys into it. The
	// engine-namespace baseline (bow/crossbow/thrown + a `default`) ships in the
	// core pack. Ids are a GLOBAL vocabulary (like slot names), NOT namespace-
	// qualified, so `ranged_style: bow` resolves across packs.
	RangedFlavor []string `yaml:"ranged_flavor,omitempty"`
	// Scripts are Lua source files discovered by the M17.1b
	// loader. Each path glob expands relative to the pack
	// directory (e.g. `scripts/*.lua`). The loader compiles
	// each file via scripting.Engine.Compile to surface
	// syntax errors at boot, then stashes (PackID, Path,
	// Source) into Registries.Scripts. Execution (M17.1c)
	// is the runtime's concern, not the loader's.
	Scripts []string `yaml:"scripts,omitempty"`
}

// IsActive reports whether the manifest is active (default true).
func (m *Manifest) IsActive() bool {
	return m.Active == nil || *m.Active
}

// Pack-kind values for Manifest.Kind (character-identity §2). Empty is
// treated as KindLibrary — a pack opts in to being a world.
const (
	KindWorld   = "world"
	KindLibrary = "library"
)

// IsWorld reports whether this pack is a world — a leaf ruleset a
// character may be stamped to (character-identity §2). Only an explicit
// `kind: world` is a world; empty or "library" is not. Case-insensitive.
func (m *Manifest) IsWorld() bool {
	return strings.EqualFold(m.Kind, KindWorld)
}

// ValidKind reports whether a manifest Kind value is one the engine
// accepts: empty (⇒ library), "world", or "library" (case-insensitive).
// Anything else is an authoring error caught at load.
func ValidKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", KindWorld, KindLibrary:
		return true
	default:
		return false
	}
}

// Namespace derives the pack namespace per spec §2.3.
//
// A scoped name `@scope/name` becomes `scope-name`. A bare name passes
// through unchanged.
func (m *Manifest) Namespace() string {
	return DeriveNamespace(m.Name)
}

// DeriveNamespace applies the spec §2.3 rule to an arbitrary pack name.
//
// Exposed because discovery needs to produce namespaces before a
// Manifest value exists (e.g. for error messages on bad parses).
func DeriveNamespace(name string) string {
	trimmed := strings.TrimPrefix(name, "@")
	return strings.ReplaceAll(trimmed, "/", "-")
}

// LoadManifest reads and parses the manifest at path. It does not
// validate cross-pack references — that happens in the two-phase
// loader.
func LoadManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrManifestMissing, path)
		}
		return nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrManifestInvalid, path, err)
	}

	if strings.TrimSpace(m.Name) == "" {
		return nil, fmt.Errorf("%w: %s: missing required field 'name'", ErrManifestInvalid, path)
	}

	return &m, nil
}
