package pack

// Content YAML schemas — minimal subset of the spec that M2 wires.
// Spec: scripting-and-packs §3.3, world-rooms-movement §2.

// AreaFile is the YAML shape for an area-definition file. One file
// may declare one area today. Multi-area-per-file can come later if
// authors want it.
//
// SpawnRules + ResetInterval (spec mobs-ai-spawning §3.5) drive
// area-driven respawn. Both are optional; an area with neither runs
// no spawn cycles. ResetInterval is in ticks (engine default applies
// when 0).
type AreaFile struct {
	ID            string          `yaml:"id"`
	Name          string          `yaml:"name"`
	Description   string          `yaml:"description,omitempty"`
	ResetInterval uint64          `yaml:"reset_interval,omitempty"`
	SpawnRules    []SpawnRuleFile `yaml:"spawn_rules,omitempty"`
	// WeatherZone is the zone id this area inherits climate from
	// (spec world-rooms-movement §2.4 + §6.1). Bare ids resolve
	// against the current pack namespace; fully-qualified
	// `pack:id` form crosses packs. Empty (the default) means
	// the area has no weather and the service skips it on
	// HourChanged.
	WeatherZone string `yaml:"weather_zone,omitempty"`
	// LightFloor is the area-level light floor (black/gloom/dim/lit):
	// it bakes onto every member room lacking its own `light_floor`,
	// lifting dark nights without capping daylight — the lamp-lit
	// settlement knob (spec light-and-darkness §2.4). Empty (the
	// default) means the area imposes no floor and rooms ride the sky.
	LightFloor string `yaml:"light_floor,omitempty"`
}

// SpawnRuleFile is the YAML shape for one entry of
// AreaFile.SpawnRules (spec mobs-ai-spawning §3.5). Bare ids in
// `room:` and `mob:` resolve against the current pack namespace;
// fully-qualified `pack:id` form crosses packs.
//
// Rare/RareChance are paired: Rare names an alternate template, and
// each missing slot rolls independently against RareChance to pick
// rare vs. default (spec §3.6).
//
// Tags carries rule-level flags. The engine inspects `persistent`
// today (treat Count as a ceiling). Content may carry other tags
// for content-side filtering — they're stored verbatim.
type SpawnRuleFile struct {
	Room          string   `yaml:"room"`
	Mob           string   `yaml:"mob"`
	Count         int      `yaml:"count"`
	Rare          string   `yaml:"rare,omitempty"`
	RareChance    float64  `yaml:"rare_chance,omitempty"`
	ResetInterval uint64   `yaml:"reset_interval,omitempty"`
	Tags          []string `yaml:"tags,omitempty"`
}

// ItemFile is the YAML shape for an item-template file (spec
// inventory-equipment-items §2.2). One file declares one template.
//
// Required: id, name, type. Optional: tags, keywords, properties,
// modifiers. Bare ids resolve against the current pack namespace.
type ItemFile struct {
	ID          string         `yaml:"id"`
	Name        string         `yaml:"name"`
	Type        string         `yaml:"type"`
	Description string         `yaml:"description,omitempty"`
	Tags        []string       `yaml:"tags,omitempty"`
	Keywords    []string       `yaml:"keywords,omitempty"`
	Properties  map[string]any `yaml:"properties,omitempty"`
	Modifiers   []ModifierFile `yaml:"modifiers,omitempty"`
	// WeaponDamage is the wielded-weapon damage dice (combat §4.5) as an
	// NdM±K string ("1d6+1"). Validated via combat.ParseDice at load —
	// a malformed expression fails the pack. Empty means non-weapon.
	WeaponDamage string `yaml:"weapon_damage,omitempty"`
	// EligibleSlots / CompanionSlots declare equipment-slot eligibility
	// and footprint (spec §2.2, §3.3). When eligible_slots is omitted the
	// loader falls back to the legacy single `properties.slot` string
	// (§3.2 bridge), so pre-existing content needs no edits. Slot names
	// are validated against the registry in a boot post-pass.
	EligibleSlots  []string `yaml:"eligible_slots,omitempty"`
	CompanionSlots []string `yaml:"companion_slots,omitempty"`
	// Weapon identity (weapon-identity §2). All optional. weapon_category
	// is an opaque kind; proficiency_tier validates against the engine
	// tier vocabulary (simple/martial/exotic); damage_types validate
	// against the fixed bludgeoning/piercing/slashing set. An absent tier
	// is "untiered" (treated as the lowest tier, §3); absent types are
	// untyped.
	WeaponCategory  string   `yaml:"weapon_category,omitempty"`
	ProficiencyTier string   `yaml:"proficiency_tier,omitempty"`
	DamageTypes     []string `yaml:"damage_types,omitempty"`
	// Critical threat range + multiplier (weapon-identity §4). crit_threat_low
	// is the lowest d20 face that crits (0 = unset = natural max only;
	// validated to 0 or [2,20]). crit_multiplier scales the dice on a crit
	// (0 = unset = configured default; validated non-negative).
	CritThreatLow  int `yaml:"crit_threat_low,omitempty"`
	CritMultiplier int `yaml:"crit_multiplier,omitempty"`
	// Armor depth (armor-depth §2). All optional, recorded-only this slice
	// (inert until the AC/mitigation/proficiency/check-penalty consumers
	// land). armor_bonus is the structured AC term; armor_max_dex caps the
	// Dex contribution (a pointer so 0 is a valid cap, absent = no cap);
	// armor_check_penalty is a non-negative skill-check penalty magnitude;
	// armor_tier validates against the engine light/medium/heavy vocabulary;
	// resistances maps a damage type to the amount of that damage soaked
	// (keys validated against the damage-type vocabulary, values >= 0).
	ArmorBonus        int            `yaml:"armor_bonus,omitempty"`
	ArmorMaxDex       *int           `yaml:"armor_max_dex,omitempty"`
	ArmorCheckPenalty int            `yaml:"armor_check_penalty,omitempty"`
	ArmorTier         string         `yaml:"armor_tier,omitempty"`
	Resistances       map[string]int `yaml:"resistances,omitempty"`
}

// ModifierFile is one entry of an ItemFile.Modifiers list.
type ModifierFile struct {
	Stat  string `yaml:"stat"`
	Value int    `yaml:"value"`
}

// SlotFile is the YAML shape for a pack-defined equipment slot (spec
// inventory-equipment-items §3.1). One file declares one slot. The
// pack's namespace is recorded as the slot's scope tag; the slot's
// name itself is NOT namespaced (slot names are global — see
// internal/slot package doc).
type SlotFile struct {
	Name  string `yaml:"name"`
	Label string `yaml:"label"`
	Max   int    `yaml:"max"`
}

// MobFile is the YAML shape for a mob template (spec
// mobs-ai-spawning §2.2). One file declares one template.
//
// Required: id, name, behavior. Type defaults to "npc" if omitted
// (matches spec §2.2). Disposition defaults to 0 (neutral) if
// omitted. Optional fields: tags, keywords, properties, stats,
// equipment (item template ids equipped at spawn — validated at
// spawn, not at load, per spec §3.1).
//
// Equipment ids may be bare (resolved against the current pack
// namespace) or fully qualified ("other-pack:foo") at SPAWN time;
// the loader stores them verbatim and the spawn pipeline (M6.2+)
// performs qualification.
type MobFile struct {
	ID          string         `yaml:"id"`
	Name        string         `yaml:"name"`
	Type        string         `yaml:"type,omitempty"`
	Description string         `yaml:"description,omitempty"`
	Disposition int            `yaml:"disposition,omitempty"`
	Behavior    string         `yaml:"behavior"`
	Tags        []string       `yaml:"tags,omitempty"`
	Keywords    []string       `yaml:"keywords,omitempty"`
	Properties  map[string]any `yaml:"properties,omitempty"`
	Stats       map[string]int `yaml:"stats,omitempty"`
	Equipment   []string       `yaml:"equipment,omitempty"`
	// NaturalWeapon is the mob's innate attack (combat §4.5) — a beast
	// with no item still bites/claws. Optional; an equipped weapon
	// overrides it. The damage string is validated at load.
	NaturalWeapon *NaturalWeaponFile `yaml:"natural_weapon,omitempty"`
	// LootTable is the optional loot-table id (mobs-ai-spawning §6.3).
	// Unknown ids are tolerated at load (resolution runs at spawn,
	// matching the fail-silent convention used for race/class).
	LootTable string `yaml:"loot_table,omitempty"`
	// Proficiencies maps ability id -> proficiency value for the mob's
	// passive abilities (M9.5 #3 — abilities-and-effects §6). Optional;
	// keys are lowercased + trimmed at decode. Mobs do not train, so
	// these are fixed combat content (e.g. `second-attack: 70`).
	Proficiencies map[string]int `yaml:"proficiencies,omitempty"`
	// Race is the optional race id (M8.3 — progression.md §3.1).
	// Unknown ids are tolerated at load (validation runs at spawn,
	// matching the spec's fail-silent convention).
	Race string `yaml:"race,omitempty"`

	// Class is the optional class id (M14.3 — mobs-ai-spawning §3.2).
	// When set together with a positive Level, the mob spawner
	// applies class-growth × level to stat-growth entries.
	Class string `yaml:"class,omitempty"`

	// Level is the optional level (M14.3). Zero disables class
	// growth even when Class is set.
	Level int `yaml:"level,omitempty"`

	// BaseDisposition is the static reaction string (spec §5.1).
	// Optional; when present overrides player-state-driven rules
	// only for the `hostile` value (§5.3 step 3).
	BaseDisposition string `yaml:"base_disposition,omitempty"`

	// DispositionRules carries the structured default + ordered
	// rule list (spec §5.1, §5.3). Optional; mobs without rules
	// just don't trigger reactions.
	DispositionRules *DispositionFile `yaml:"disposition_rules,omitempty"`

	// Trainer is the optional trainer config (M8.6 — progression.md
	// §7.3). When present, the mob MUST also carry the
	// `skill_trainer` tag; the pack loader rejects the pair when
	// only one side is set.
	Trainer *TrainerFile `yaml:"trainer,omitempty"`
}

// NaturalWeaponFile is the YAML shape for a mob's innate weapon
// (combat §4.5). Name is the display label shown on hit/miss events
// ("fangs"); Damage is an NdM±K dice string validated at load.
type NaturalWeaponFile struct {
	Name   string `yaml:"name"`
	Damage string `yaml:"damage"`
}

// TrainerFile is the YAML shape for a mob's TrainerConfig (spec
// progression.md §7.3). Tier is the canonical cap-tier name
// ("novice"/"apprentice"/"journeyman"/"master"); Teach lists the
// ability ids the trainer is willing to raise.
type TrainerFile struct {
	Tier  string   `yaml:"tier"`
	Teach []string `yaml:"teach,omitempty"`
}

// DispositionFile is the YAML shape for a mob's reaction policy
// (spec mobs-ai-spawning §5.1 disposition definition).
type DispositionFile struct {
	Default string                `yaml:"default,omitempty"`
	Rules   []DispositionRuleFile `yaml:"rules,omitempty"`
}

// DispositionRuleFile is one entry in DispositionFile.Rules. All
// condition fields are optional; a rule with no conditions matches
// any player (spec §5.3). Reaction is required at decode time.
//
// Alignment fields are accepted but not yet honored (M8 dependency).
// They are decoded with pointer types so omitted-vs-zero is
// distinguishable.
type DispositionRuleFile struct {
	HasTag       string   `yaml:"has_tag,omitempty"`
	MinAlignment *int     `yaml:"min_alignment,omitempty"`
	MaxAlignment *int     `yaml:"max_alignment,omitempty"`
	Buckets      []string `yaml:"buckets,omitempty"`
	Reaction     string   `yaml:"reaction"`
}

// RoomFile is the YAML shape for a single-room file.
//
// Exits is a map keyed by direction long-name ("north", "up") to keep
// the format pleasant to author. Targets may be bare (resolved against
// the current pack namespace) or fully qualified ("other-pack:foo").
//
// Items declares item-template ids the room owns at boot. The loader's
// placement post-pass spawns one instance per id and places it in this
// room. Same qualifier rules as exit targets: bare ids resolve against
// the pack namespace, "ns:name" crosses packs. Spec
// world-rooms-movement §2.2.
//
// Mobs declares mob-template ids spawned into the room at boot. Same
// qualifier rules as Items. Spec mobs-ai-spawning §3.1 (spawn
// placement) — boot-time placement is the simplest path; area-driven
// respawn cadence (spec §3.5) is a later slice. Validation runs in
// the loader's post-pass: unknown ids surface as ErrMissingMobTemplate.
type RoomFile struct {
	ID          string            `yaml:"id"`
	Area        string            `yaml:"area"`
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Exits       map[string]string `yaml:"exits,omitempty"`
	Items       []string          `yaml:"items,omitempty"`
	Mobs        []string          `yaml:"mobs,omitempty"`
	// HealingRate is the §5.7 additive room regen bonus (economy-
	// survival). Absent → 0 (no bonus).
	HealingRate int `yaml:"healing_rate,omitempty"`
	// Tags are content-defined room flags (e.g. "safe-room", "safe").
	// Copied onto world.Room.Tags at load. Absent → no tags.
	Tags []string `yaml:"tags,omitempty"`
	// Properties is the M14.5 free-form property bag (spec §2.2).
	// Each entry's name is validated against the property registry
	// at load (snake_case, registered, type-matched). Absent → empty.
	Properties map[string]any `yaml:"properties,omitempty"`
	// Doors is the M15.1 per-exit door declaration (spec §5.1).
	// Keys are the direction name (short or long) matching one of
	// the Exits entries; values declare the door's display name +
	// optional flags. Loaded into world.Exit.Door and synchronized
	// with the reverse side at load when both sides declare.
	Doors map[string]DoorFile `yaml:"doors,omitempty"`
	// HiddenExits marks exits as secret (hidden-exits §2). Keys are the
	// direction name (short or long) matching one of the Exits entries;
	// the value carries the optional search difficulty. Mirrors the Doors
	// block — a separate map so the simple `exits:` shape stays
	// backward-compatible. Each side is authored independently (§2).
	HiddenExits map[string]HiddenExitFile `yaml:"hidden_exits,omitempty"`

	// Terrain is the room's weather/time eligibility classifier
	// (spec world-rooms-movement §6.4). Empty defaults to
	// `outdoors`. `indoors` and `underground` are engine-known
	// shielding terrains that suppress ambience unless the
	// exposure flags below override.
	Terrain string `yaml:"terrain,omitempty"`
	// WeatherExposed flips a shielded room back to eligible for
	// weather messages (e.g. a covered courtyard). Spec §6.4.
	WeatherExposed bool `yaml:"weather_exposed,omitempty"`
	// TimeExposed mirrors WeatherExposed for time-of-day
	// ambience. Independent of WeatherExposed by design — a
	// room may receive one and not the other. Spec §6.4.
	TimeExposed bool `yaml:"time_exposed,omitempty"`

	// Coord is the optional authored coordinate pin
	// (room-coordinates §3.5): an area-local {x, y, z} placing this
	// room at a fixed point the derivation walk treats as ground truth
	// and never overwrites. Absent (the default) → the room's position
	// is derived from the exit graph. A malformed pin (missing axis)
	// warns at load and falls back to derived placement, never aborting
	// the load (§3.5).
	Coord *CoordFile `yaml:"coord,omitempty"`
}

// CoordFile is the YAML shape for a room coordinate pin
// (room-coordinates §3.5). All three axes are required; a pointer per
// axis lets the decoder distinguish an explicit 0 from a missing axis,
// which is the malformed-pin case (warn + fall back to derived).
type CoordFile struct {
	X *int `yaml:"x"`
	Y *int `yaml:"y"`
	Z *int `yaml:"z"`
}

// DoorFile is the YAML shape for one door declaration. All fields
// are optional except Name; the rest follow the spec §5.1 defaults
// (closed=true, locked=false, unkeyed).
type DoorFile struct {
	// Name is the door's display name; space-split tokens double
	// as match keywords for command-layer resolution.
	Name string `yaml:"name"`
	// Keywords explicitly lists match keywords. When absent the
	// decoder splits Name on whitespace and lowercases each token.
	// Provide explicit Keywords when Name contains characters
	// that should not become keywords (a quoted display name).
	Keywords []string `yaml:"keywords,omitempty"`
	// Closed defaults true (a door is closed unless content says
	// otherwise). Use `closed: false` to leave the door propped open
	// at boot.
	Closed *bool `yaml:"closed,omitempty"`
	// Locked defaults false. A locked door is also closed; the
	// decoder enforces that constraint.
	Locked bool `yaml:"locked,omitempty"`
	// Key names the item template id required to unlock the door.
	// Optional; an unkeyed door can be unlocked by anyone.
	Key string `yaml:"key,omitempty"`
	// Pickable + PickDifficulty wire the M15.1 substrate fields;
	// the lockpick verb itself is deferred.
	Pickable       bool `yaml:"pickable,omitempty"`
	PickDifficulty int  `yaml:"pick_difficulty,omitempty"`
}

// HiddenExitFile is the YAML shape for one hidden-exit declaration
// (hidden-exits §2). The presence of the entry marks the exit hidden;
// SearchDifficulty is optional (0 → the configured default applies at the
// search site).
type HiddenExitFile struct {
	// SearchDifficulty is the concealment score a searcher's perception
	// contest must beat (§2/§3). Optional; 0 → configured default.
	SearchDifficulty int `yaml:"search_difficulty,omitempty"`
}

// TrackFile is the YAML shape for a progression-track definition
// (spec progression.md §5.1). M8.2 supports the table form only;
// formula-driven tracks live as Go-side TrackDef construction
// (cmd/anothermud wiring) until the scripting milestone lands.
//
// Name is case-sensitive (spec §5.1). DisplayName falls back to
// Name when empty.
//
// XPTable is indexed by level — XPTable[L] is the total XP required
// to reach level L. Idiomatic shape: write XPTable[0] as 0 (level
// 0 unused), XPTable[1] as 0 (level 1 threshold), then the real
// thresholds for level 2 onward. The loader does not enforce that
// pattern beyond rejecting a missing/empty table.
type TrackFile struct {
	ID           string  `yaml:"id"`
	Name         string  `yaml:"name,omitempty"`
	MaxLevel     int     `yaml:"max_level"`
	XPTable      []int64 `yaml:"xp_table"`
	DeathPenalty float64 `yaml:"death_penalty,omitempty"`
	Priority     int     `yaml:"priority,omitempty"`
}

// ClassPathEntryFile is one row in a class's level path. Mirrors
// progression.ClassPathEntry. UnlockedVia (when non-empty) marks
// the entry as owned by another subsystem (quest reward, scripted
// hook); the path processor skips it at level-up per spec §4.5.
type ClassPathEntryFile struct {
	Level       int    `yaml:"level"`
	AbilityID   string `yaml:"ability"`
	UnlockedVia string `yaml:"unlocked_via,omitempty"`
}

// ClassFile is the YAML shape for a class definition (spec
// progression.md §4.1). Stat-growth dice are authored as strings
// ("1d8") and parsed via combat.ParseDice at load. Growth bonuses
// map a stat to the source stat whose modifier contributes a bonus
// on every level-up roll. Bound track is case-insensitive at runtime
// but stored verbatim for diagnostics.
type ClassFile struct {
	ID            string            `yaml:"id"`
	Name          string            `yaml:"name,omitempty"`
	Tagline       string            `yaml:"tagline,omitempty"`
	Description   string            `yaml:"description,omitempty"`
	LevelUpFlavor string            `yaml:"level_up_flavor,omitempty"`
	BoundTrack    string            `yaml:"bound_track,omitempty"`
	StatGrowth    map[string]string `yaml:"stat_growth,omitempty"`
	GrowthBonuses map[string]string `yaml:"growth_bonuses,omitempty"`
	// StartingStats is a flat base-stat grant applied once at character
	// creation (e.g. a channeler's `resource_max: 30` One Power pool). Values
	// are plain ints, not dice — StatGrowth covers per-level-up dice.
	StartingStats         map[string]int       `yaml:"starting_stats,omitempty"`
	Path                  []ClassPathEntryFile `yaml:"path,omitempty"`
	TrainsPerLevel        int                  `yaml:"trains_per_level,omitempty"`
	AllowedCategories     []string             `yaml:"allowed_categories,omitempty"`
	AllowedGenders        []string             `yaml:"allowed_genders,omitempty"`
	ProficiencyTiers      []string             `yaml:"proficiency_tiers,omitempty"`
	ProficiencyCategories []string             `yaml:"proficiency_categories,omitempty"`
	// SaveProgressions maps a save axis (fortitude/reflex/will) to a
	// strong or weak base-save curve (saves §2). An omitted axis defaults
	// to weak. Validated at load: unknown axis or progression names are an
	// authoring error.
	SaveProgressions  map[string]string `yaml:"save_progressions,omitempty"`
	StartingAlignment int               `yaml:"starting_alignment,omitempty"`
	Priority          int               `yaml:"priority,omitempty"`
}

// AbilityFile is the YAML shape for an ability definition (spec
// abilities-and-effects §2.2). M9.1 covers the minimum surface used
// by the registry + proficiency manager + training caps: identity,
// classification, and learn-time defaults. Resolution fields (cost,
// cooldown, target rules, effect template, variance, handler token,
// metadata) land in later M9 slices as their consumers come online.
//
// Ability ids are NOT namespaced — the registry is global and the
// override semantics §2.1 already permit a pack to replace a
// baseline ability by id+priority (mirrors the slot registry).
type AbilityFile struct {
	ID                    string  `yaml:"id"`
	Name                  string  `yaml:"name,omitempty"`
	Type                  string  `yaml:"type"`
	Category              string  `yaml:"category"`
	DefaultCap            int     `yaml:"default_cap,omitempty"`
	GainBaseChance        int     `yaml:"gain_base_chance,omitempty"`
	GainFailureMultiplier float64 `yaml:"gain_failure_multiplier,omitempty"`
	GainStat              string  `yaml:"gain_stat,omitempty"`
	GainStatScale         float64 `yaml:"gain_stat_scale,omitempty"`
	Priority              int     `yaml:"priority,omitempty"`
	// M9.3 validation surface (spec abilities-and-effects §2.2, §4.3).
	Cost       int `yaml:"cost,omitempty"`
	PulseDelay int `yaml:"pulse_delay,omitempty"`
	// CastTime is the interruptible warmup in combat rounds before the
	// ability resolves (WoT S2 — the channel interrupt game). 0 ⇒ instant
	// (the default for every skill and instant weave). Distinct from
	// PulseDelay (post-cast recovery cooldown).
	CastTime      int      `yaml:"cast_time,omitempty"`
	InitiateOnly  bool     `yaml:"initiate_only,omitempty"`
	TargetTypes   []string `yaml:"target_types,omitempty"`
	EquipmentSlot string   `yaml:"equipment_slot,omitempty"`
	EquipmentTag  string   `yaml:"equipment_tag,omitempty"`
	// M9.4 resolution surface (spec abilities-and-effects §4.5).
	// Variance is the hit-chance band (0 ⇒ always hits); MaxHitChance
	// optionally caps the rolled chance so even high proficiency can
	// miss. Both clamp to [0, 100] at registration.
	Variance     int `yaml:"variance,omitempty"`
	MaxHitChance int `yaml:"max_hit_chance,omitempty"`
	// AlignmentMin/Max are pointers so 0 (neutral) is distinguishable
	// from omitted. When at least one is set the range gates usage
	// (spec §4.3 step 2); the loader fills the missing bound with the
	// content-friendly extreme (Min defaults to MinInt, Max to MaxInt).
	AlignmentMin *int `yaml:"alignment_min,omitempty"`
	AlignmentMax *int `yaml:"alignment_max,omitempty"`
	// Effect is the optional template applied on hit (spec §5.1).
	Effect *EffectFile `yaml:"effect,omitempty"`
	// ApplySave is the optional entry save the target rolls to resist the
	// effect (conditions §4 — a save-gated trip/bash). Validated at load.
	ApplySave *SaveFile `yaml:"apply_save,omitempty"`
	// M9.6b resolution side-effect surface (spec §4.5 step 8, §4.6).
	// Handler is the dispatch token the "ability used" subscriber
	// switches on ("damage" / "heal" / empty). Damage / Heal are
	// NdM±K dice the matching handler rolls; Damage also makes a
	// no-effect spell offensive (§4.6). Dice are parsed by the host
	// (cmd/anothermud) so the pack/progression layers stay free of
	// the combat dice type.
	Handler string `yaml:"handler,omitempty"`
	Damage  string `yaml:"damage,omitempty"`
	Heal    string `yaml:"heal,omitempty"`
	// M9.5 passive surface (spec §6). Hook is the discovery key a
	// subsystem iterates ("extra_attack", "defensive"); MaxBonus is
	// the §6.2 scaling ceiling. Only meaningful on passive abilities.
	Hook     string `yaml:"hook,omitempty"`
	MaxBonus int    `yaml:"max_bonus,omitempty"`
	// Elements is the open-vocabulary list of "powers" a spell weaves
	// (WoT S2: the Five Powers air/earth/fire/water/spirit). Authoring
	// metadata only; a setting layer (affinities) reads it. Normalized
	// lowercase + deduped at registration.
	Elements []string `yaml:"elements,omitempty"`
}

// EffectFile is the YAML shape for an Ability.Effect template (spec
// abilities-and-effects §5.1). Duration is in pulses; negative =
// permanent. Modifiers and Flags are optional.
type EffectFile struct {
	ID          string         `yaml:"id"`
	Duration    int            `yaml:"duration,omitempty"`
	Modifiers   []ModifierFile `yaml:"modifiers,omitempty"`
	Flags       []string       `yaml:"flags,omitempty"`
	Refreshable bool           `yaml:"refreshable,omitempty"`
	// RecurringSave gives the effect a per-tick shake-off save (conditions
	// §4) — an inline-ability effect (trip/bash) can declare it just like a
	// standalone effect file. Validated at load.
	RecurringSave *SaveFile `yaml:"recurring_save,omitempty"`
}

// SaveFile is the YAML shape for a condition save (conditions §4): an axis
// (fortitude/reflex/will) and a difficulty class. Shared by an ability's
// entry save (apply_save) and an effect's shake-off save (recurring_save).
type SaveFile struct {
	Axis string `yaml:"axis"`
	DC   int    `yaml:"dc"`
}

// ThemeFile is the YAML shape for a pack theme (spec
// ui-rendering-help §3.1). Each entry under `tags` maps a semantic tag
// name to a foreground/background color name (§2.3) and/or an HTML
// color for GMCP clients. Tag names are global (not namespaced) and
// later packs override earlier entries.
//
//	tags:
//	  highlight: { fg: bright-yellow }
//	  danger:    { fg: red, bg: black }
//	  item.rare: { fg: cyan, html: "#00FFFF" }
type ThemeFile struct {
	Tags map[string]ThemeTagEntry `yaml:"tags"`
}

// ThemeTagEntry is one tag's presentation in a ThemeFile.
type ThemeTagEntry struct {
	FG   string `yaml:"fg,omitempty"`
	BG   string `yaml:"bg,omitempty"`
	HTML string `yaml:"html,omitempty"`
}

// ChannelMapFile is the YAML shape for a pack's combat-channel derivation
// (the channel layer — docs/themes/channel-vocabulary.md §7). Each entry
// under `channels` maps a curated channel name (attack/defense/…) to an
// arithmetic formula over the entity's attributes. Channel names are
// global (not namespaced); later packs override earlier entries per
// channel. (The `channels` key here names combat channels, NOT the chat
// channels loaded from a pack's separate `channels:` glob.)
//
//	channels:
//	  attack:  hit_mod
//	  defense: 10 + mod(dex) + armor
type ChannelMapFile struct {
	Channels map[string]string `yaml:"channels"`
}

// RarityFile is the YAML shape for a pack's rarity-tier vocabulary (spec
// item-decorations §2). Each tier carries a key, an order (low → high),
// optional display text + a decorator pair, a color (fg/bg/html, seeded as
// the theme tag `item.<key>`), and a visible flag. A tier that is invisible
// or lacks display/decorators renders as nothing — the baseline pattern
// (e.g. `common`).
//
//	tiers:
//	  - { key: common, order: 10 }
//	  - { key: rare, order: 30, display: RARE, left: "[", right: "]", fg: cyan, visible: true }
type RarityFile struct {
	Tiers []RarityTierEntry `yaml:"tiers"`
}

// RarityTierEntry is one tier in a RarityFile.
type RarityTierEntry struct {
	Key     string `yaml:"key"`
	Order   int    `yaml:"order"`
	Display string `yaml:"display,omitempty"`
	Left    string `yaml:"left,omitempty"`
	Right   string `yaml:"right,omitempty"`
	FG      string `yaml:"fg,omitempty"`
	BG      string `yaml:"bg,omitempty"`
	HTML    string `yaml:"html,omitempty"`
	Visible bool   `yaml:"visible,omitempty"`
}

// EssenceFile is the YAML shape for a pack's essence vocabulary (spec
// item-decorations §3). Each essence carries a key, a glyph, and a color
// (seeded as the theme tag `essence.<key>`).
//
//	essences:
//	  - { key: fire, glyph: "✦", fg: red }
type EssenceFile struct {
	Essences []EssenceEntry `yaml:"essences"`
}

// EssenceEntry is one essence in an EssenceFile.
type EssenceEntry struct {
	Key   string `yaml:"key"`
	Glyph string `yaml:"glyph"`
	FG    string `yaml:"fg,omitempty"`
	BG    string `yaml:"bg,omitempty"`
	HTML  string `yaml:"html,omitempty"`
}

// LootTableFile is the YAML shape for a pack's loot table (M22.1 —
// mobs-ai-spawning §6.3). A mob template's `loot_table` field references
// a table by id; the spawn pipeline rolls it into the mob's contents.
//
//	id: goblin-loot
//	guaranteed:
//	  - { item: copper-coin, count: 3 }
//	weighted:
//	  - { item: rusty-dagger, weight: 3 }
//	  - { item: health-potion, weight: 1 }
//	pool_rolls: 1
//	rare_bonus:
//	  chance: 5            # percent (0-100)
//	  entries:
//	    - { item: signet-of-the-king, weight: 1 }
type LootTableFile struct {
	ID         string           `yaml:"id"`
	Priority   int              `yaml:"priority,omitempty"`
	Guaranteed []LootGuaranteed `yaml:"guaranteed,omitempty"`
	Weighted   []LootWeighted   `yaml:"weighted,omitempty"`
	PoolRolls  int              `yaml:"pool_rolls,omitempty"`
	RareBonus  *LootRareBonus   `yaml:"rare_bonus,omitempty"`
	// Coin is the optional currency drop, rolled at corpse creation
	// (loot-and-corpses §3). e.g. `coin: { min: 1, max: 10 }`.
	Coin *LootCoin `yaml:"coin,omitempty"`
}

// LootCoin is the optional coin block: an inclusive currency range.
type LootCoin struct {
	Min int `yaml:"min"`
	Max int `yaml:"max"`
}

// LootGuaranteed is one always-drops entry: an item id dropped Count times.
type LootGuaranteed struct {
	Item  string `yaml:"item"`
	Count int    `yaml:"count"`
}

// LootWeighted is one weighted-pool candidate.
type LootWeighted struct {
	Item   string `yaml:"item"`
	Weight int    `yaml:"weight"`
}

// LootRareBonus is the optional one-roll bonus pool. Chance is a percent.
type LootRareBonus struct {
	Chance  int            `yaml:"chance"`
	Entries []LootWeighted `yaml:"entries,omitempty"`
}

// HelpFile is the YAML shape for a pack help file (spec
// ui-rendering-help §9.1). A file carries one or more topics under
// `topics`. Each topic's `role` is optional (player/builder/admin;
// absent = visible to all).
//
//	topics:
//	  - id: look
//	    title: Look
//	    category: commands
//	    brief: Examine your surroundings.
//	    body: |
//	      Shows the current room and its exits.
//	    syntax: ["look", "look <target>"]
//	    keywords: [examine, l]
//	    see_also: [examine, scan]
type HelpFile struct {
	Topics []HelpTopicFile `yaml:"topics"`
}

// HelpTopicFile is one topic in a HelpFile.
type HelpTopicFile struct {
	ID       string   `yaml:"id"`
	Title    string   `yaml:"title"`
	Category string   `yaml:"category,omitempty"`
	Brief    string   `yaml:"brief,omitempty"`
	Body     string   `yaml:"body,omitempty"`
	Syntax   []string `yaml:"syntax,omitempty"`
	Keywords []string `yaml:"keywords,omitempty"`
	SeeAlso  []string `yaml:"see_also,omitempty"`
	Role     string   `yaml:"role,omitempty"`
}

// QuestFile is the YAML shape for a quest definition (spec quests.md
// §2). `abandonable` is a pointer so an absent value defaults to true
// (the spec default); repeatable/secret default false via the zero
// value. Objective/giver/target ids are namespace-qualified by the
// loader; ability/class/race reward ids and objective types are not.
type QuestFile struct {
	ID             string           `yaml:"id"`
	Name           string           `yaml:"name,omitempty"`
	Classification string           `yaml:"classification,omitempty"`
	Giver          string           `yaml:"giver,omitempty"`
	Offer          string           `yaml:"offer,omitempty"`
	TurnIn         bool             `yaml:"turn_in,omitempty"`
	Repeatable     bool             `yaml:"repeatable,omitempty"`
	Abandonable    *bool            `yaml:"abandonable,omitempty"`
	Secret         bool             `yaml:"secret,omitempty"`
	Prerequisite   PrerequisiteFile `yaml:"prerequisite,omitempty"`
	Stages         []QuestStageFile `yaml:"stages,omitempty"`
	Reward         RewardFile       `yaml:"reward,omitempty"`
	Script         string           `yaml:"script,omitempty"`
}

// PrerequisiteFile is the YAML shape for a quest prerequisite block.
type PrerequisiteFile struct {
	MinLevel           int      `yaml:"min_level,omitempty"`
	Class              string   `yaml:"class,omitempty"`
	QuestsCompleted    []string `yaml:"quests_completed,omitempty"`
	QuestsNotCompleted []string `yaml:"quests_not_completed,omitempty"`
}

// QuestStageFile is the YAML shape for a quest stage.
type QuestStageFile struct {
	ID          string               `yaml:"id,omitempty"`
	Description string               `yaml:"description,omitempty"`
	Hint        string               `yaml:"hint,omitempty"`
	Objectives  []QuestObjectiveFile `yaml:"objectives,omitempty"`
}

// QuestObjectiveFile is the YAML shape for a quest objective.
type QuestObjectiveFile struct {
	ID          string `yaml:"id,omitempty"`
	Type        string `yaml:"type,omitempty"`
	Target      string `yaml:"target,omitempty"`
	NPC         string `yaml:"npc,omitempty"`
	Count       int    `yaml:"count,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// RewardFile is the YAML shape for a quest reward block.
type RewardFile struct {
	XP          int64    `yaml:"xp,omitempty"`
	Gold        int      `yaml:"gold,omitempty"`
	Items       []string `yaml:"items,omitempty"`
	Abilities   []string `yaml:"abilities,omitempty"`
	Recipes     []string `yaml:"recipes,omitempty"`
	ClassUnlock string   `yaml:"class_unlock,omitempty"`
	RaceUnlock  string   `yaml:"race_unlock,omitempty"`
}

// RaceFile is the YAML shape for a race definition (spec
// progression.md §3.1). Stat caps are keyed by lowercase StatType
// strings ("str", "hp_max", etc.); the loader maps them onto the
// typed StatType keys at registration. CastCostModifier may be
// negative — cost.AdjustCost clamps the final ability cost at
// zero.
type RaceFile struct {
	ID                string         `yaml:"id"`
	Name              string         `yaml:"name,omitempty"`
	Tagline           string         `yaml:"tagline,omitempty"`
	Description       string         `yaml:"description,omitempty"`
	Category          string         `yaml:"category,omitempty"`
	StartingAlignment int            `yaml:"starting_alignment,omitempty"`
	StatCaps          map[string]int `yaml:"stat_caps,omitempty"`
	CastCostModifier  int            `yaml:"cast_cost_modifier,omitempty"`
	RacialFlags       []string       `yaml:"racial_flags,omitempty"`
	Priority          int            `yaml:"priority,omitempty"`
}

// BackgroundFile is the YAML shape for a character-creation background
// (backgrounds §2). A background grants a starting package — skills, items,
// gold — applied once at creation. Item ids + skill ability ids are content
// references resolved (fail-soft) at grant time, not at load.
type BackgroundFile struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name,omitempty"`
	Tagline     string `yaml:"tagline,omitempty"`
	Description string `yaml:"description,omitempty"`
	// Skills: ability id → starting proficiency (default the baseline trained
	// value when omitted/<=0).
	Skills            []BackgroundSkillFile `yaml:"skills,omitempty"`
	Items             []string              `yaml:"items,omitempty"`
	Feats             []string              `yaml:"feats,omitempty"`
	Gold              int                   `yaml:"gold,omitempty"`
	AllowedCategories []string              `yaml:"allowed_categories,omitempty"`
	AllowedGenders    []string              `yaml:"allowed_genders,omitempty"`
	Priority          int                   `yaml:"priority,omitempty"`
}

// BackgroundSkillFile is one skill grant in a BackgroundFile (backgrounds §2).
type BackgroundSkillFile struct {
	Ability     string `yaml:"ability"`
	Proficiency int    `yaml:"proficiency,omitempty"`
}

// FeatFile is the YAML shape for a player-chosen feat (EPIC S4 Phase 0 —
// docs/proposals/wot-feats.md §2.1). Phase 0 carries identity + prerequisites
// + multi-take rule + class gate; the grant payload (what the feat confers) is
// Phase 3. Prereq targets (feat ids, skill ability ids) are content references
// resolved fail-soft when the feat is taken, not at load.
type FeatFile struct {
	ID             string           `yaml:"id"`
	Name           string           `yaml:"name,omitempty"`
	Description    string           `yaml:"description,omitempty"`
	Prerequisites  []FeatPrereqFile `yaml:"prerequisites,omitempty"`
	Grants         []FeatGrantFile  `yaml:"grants,omitempty"`
	MultiTake      string           `yaml:"multi_take,omitempty"`
	AllowedClasses []string         `yaml:"allowed_classes,omitempty"`
	Priority       int              `yaml:"priority,omitempty"`
}

// FeatGrantFile is one bonus a feat confers (EPIC S4 Phase 3 — §2.4). Kind
// selects the bonus shape (Phase 3a: save_bonus); target + magnitude are
// interpreted per kind (save_bonus: target = fortitude/reflex/will, magnitude
// = the bonus).
type FeatGrantFile struct {
	Kind      string `yaml:"kind"`
	Target    string `yaml:"target,omitempty"`
	Magnitude int    `yaml:"magnitude,omitempty"`
}

// FeatPrereqFile is one prerequisite gate in a FeatFile (feats §2.1). Kind is
// one of ability_score / feat / skill / level; target names the stat / feat id
// / skill ability id (omitted for level); min is the threshold.
type FeatPrereqFile struct {
	Kind   string `yaml:"kind"`
	Target string `yaml:"target,omitempty"`
	Min    int    `yaml:"min,omitempty"`
}

// WeatherZoneFile is the YAML shape for one weather-zone definition
// (spec world-rooms-movement §6; see internal/weather).
//
// Required: id. Optional: initial_state (defaults to "clear"),
// roll_interval_hours (defaults to 1), transitions (a row missing
// for a state is treated as "stay"), weather_messages, time_messages.
//
// Zone ids are namespaced like every other content id: bare in YAML,
// fully-qualified at runtime after pack-namespace qualification.
//
// Shape of the nested maps:
//
//	transitions:
//	  clear:
//	    - next: cloudy
//	      weight: 3
//	    - next: clear
//	      weight: 7
//
//	weather_messages:
//	  rain:
//	    outdoors:
//	      start: "Rain begins to fall."
//	      ongoing: "Rain patters around you."
//	      end: "The rain tapers off."
//	    forest:
//	      start: "Drops patter against the leaves overhead."
//
//	time_messages:
//	  dawn:
//	    outdoors: "The first light leaks across the horizon."
type WeatherZoneFile struct {
	ID                string                                  `yaml:"id"`
	InitialState      string                                  `yaml:"initial_state,omitempty"`
	RollIntervalHours int                                     `yaml:"roll_interval_hours,omitempty"`
	Transitions       map[string][]TransitionWeightFile       `yaml:"transitions,omitempty"`
	WeatherMessages   map[string]map[string]WeatherTripleFile `yaml:"weather_messages,omitempty"`
	TimeMessages      map[string]map[string]string            `yaml:"time_messages,omitempty"`
}

// TransitionWeightFile is one outcome in a zone transition row.
// Weight MUST be > 0; the decoder rejects non-positive weights.
type TransitionWeightFile struct {
	Next   string `yaml:"next"`
	Weight int    `yaml:"weight"`
}

// WeatherTripleFile is the YAML shape for one (start, ongoing, end)
// message set per (state, terrain). Any field may be empty; absent
// fields decode to "" and the dispatcher skips empty messages
// (spec §6.2 step 7).
type WeatherTripleFile struct {
	Start   string `yaml:"start,omitempty"`
	Ongoing string `yaml:"ongoing,omitempty"`
	End     string `yaml:"end,omitempty"`
}

// RecipeFile is the YAML shape for a crafting-recipe definition
// (crafting-and-cooking §3). Inputs/output reference item template ids;
// discipline references a crafting proficiency (ability id). Numeric
// levers (skill floor, station tier, craft time) come from content per
// the §10 configuration surface. Decoded by decodeRecipe.
type RecipeFile struct {
	ID          string           `yaml:"id"`
	Name        string           `yaml:"name"`
	Discipline  string           `yaml:"discipline"`
	SkillFloor  int              `yaml:"skill_floor,omitempty"`
	StationTier int              `yaml:"station_tier,omitempty"`
	Tool        string           `yaml:"tool,omitempty"`
	TimePulses  int              `yaml:"time_pulses,omitempty"`
	Acquisition string           `yaml:"acquisition,omitempty"`
	Inputs      []IngredientFile `yaml:"inputs"`
	Output      OutputFile       `yaml:"output"`
}

// IngredientFile is one input in a RecipeFile (§3). Template is the item
// template id consumed; Quantity defaults to 1 when omitted. MinQuality is
// an optional rarity-tier key floor (§5).
type IngredientFile struct {
	Template   string `yaml:"template"`
	Quantity   int    `yaml:"quantity,omitempty"`
	MinQuality string `yaml:"min_quality,omitempty"`
}

// OutputFile is the produced item in a RecipeFile (§3). Quantity defaults
// to 1 when omitted.
type OutputFile struct {
	Template string `yaml:"template"`
	Quantity int    `yaml:"quantity,omitempty"`
}

// ChannelFile is the YAML shape of a chat-channel definition
// (chat-channels-and-tells §3). ID is namespace-qualified at load;
// DisplayName is the player-typed verb. Kind defaults to "public" when
// omitted; BufferCap 0 means the registry default. SpeakGate/ListenGate
// are role-tag sets for gated channels (read but not enforced in v1).
type ChannelFile struct {
	ID          string   `yaml:"id"`
	DisplayName string   `yaml:"display_name"`
	Kind        string   `yaml:"kind,omitempty"`
	DefaultOn   bool     `yaml:"default_on,omitempty"`
	Persisted   bool     `yaml:"persisted,omitempty"`
	BufferCap   int      `yaml:"buffer_cap,omitempty"`
	SpeakGate   []string `yaml:"speak_gate,omitempty"`
	ListenGate  []string `yaml:"listen_gate,omitempty"`
}

// EmoteFile is the YAML shape of a social-emote definition (emotes.md §2).
// ID is namespace-qualified at load; DisplayName is the verb. NoTarget is
// required unless RequiresTarget is set; Targeted is always required. Each
// view block carries actor/target/room templates ($n = actor, $N = target).
type EmoteFile struct {
	ID             string        `yaml:"id"`
	DisplayName    string        `yaml:"display_name"`
	Aliases        []string      `yaml:"aliases,omitempty"`
	RequiresTarget bool          `yaml:"requires_target,omitempty"`
	NoTarget       EmoteViewFile `yaml:"no_target,omitempty"`
	Targeted       EmoteViewFile `yaml:"targeted"`
}

// EmoteViewFile is one view block in an EmoteFile: the actor's line, the
// target's line (targeted forms only), and the third-person room line.
type EmoteViewFile struct {
	Actor  string `yaml:"actor,omitempty"`
	Target string `yaml:"target,omitempty"`
	Room   string `yaml:"room,omitempty"`
}
