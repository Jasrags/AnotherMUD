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
	ID         string         `yaml:"id"`
	Name       string         `yaml:"name"`
	Type       string         `yaml:"type"`
	Tags       []string       `yaml:"tags,omitempty"`
	Keywords   []string       `yaml:"keywords,omitempty"`
	Properties map[string]any `yaml:"properties,omitempty"`
	Modifiers  []ModifierFile `yaml:"modifiers,omitempty"`
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
	Disposition int            `yaml:"disposition,omitempty"`
	Behavior    string         `yaml:"behavior"`
	Tags        []string       `yaml:"tags,omitempty"`
	Keywords    []string       `yaml:"keywords,omitempty"`
	Properties  map[string]any `yaml:"properties,omitempty"`
	Stats       map[string]int `yaml:"stats,omitempty"`
	Equipment   []string       `yaml:"equipment,omitempty"`
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
	ID                string               `yaml:"id"`
	Name              string               `yaml:"name,omitempty"`
	Tagline           string               `yaml:"tagline,omitempty"`
	Description       string               `yaml:"description,omitempty"`
	LevelUpFlavor     string               `yaml:"level_up_flavor,omitempty"`
	BoundTrack        string               `yaml:"bound_track,omitempty"`
	StatGrowth        map[string]string    `yaml:"stat_growth,omitempty"`
	GrowthBonuses     map[string]string    `yaml:"growth_bonuses,omitempty"`
	Path              []ClassPathEntryFile `yaml:"path,omitempty"`
	TrainsPerLevel    int                  `yaml:"trains_per_level,omitempty"`
	AllowedCategories []string             `yaml:"allowed_categories,omitempty"`
	AllowedGenders    []string             `yaml:"allowed_genders,omitempty"`
	StartingAlignment int                  `yaml:"starting_alignment,omitempty"`
	Priority          int                  `yaml:"priority,omitempty"`
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
	Cost          int      `yaml:"cost,omitempty"`
	PulseDelay    int      `yaml:"pulse_delay,omitempty"`
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
}

// EffectFile is the YAML shape for an Ability.Effect template (spec
// abilities-and-effects §5.1). Duration is in pulses; negative =
// permanent. Modifiers and Flags are optional.
type EffectFile struct {
	ID        string         `yaml:"id"`
	Duration  int            `yaml:"duration,omitempty"`
	Modifiers []ModifierFile `yaml:"modifiers,omitempty"`
	Flags     []string       `yaml:"flags,omitempty"`
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
//   transitions:
//     clear:
//       - next: cloudy
//         weight: 3
//       - next: clear
//         weight: 7
//
//   weather_messages:
//     rain:
//       outdoors:
//         start: "Rain begins to fall."
//         ongoing: "Rain patters around you."
//         end: "The rain tapers off."
//       forest:
//         start: "Drops patter against the leaves overhead."
//
//   time_messages:
//     dawn:
//       outdoors: "The first light leaks across the horizon."
type WeatherZoneFile struct {
	ID                string                                    `yaml:"id"`
	InitialState      string                                    `yaml:"initial_state,omitempty"`
	RollIntervalHours int                                       `yaml:"roll_interval_hours,omitempty"`
	Transitions       map[string][]TransitionWeightFile         `yaml:"transitions,omitempty"`
	WeatherMessages   map[string]map[string]WeatherTripleFile   `yaml:"weather_messages,omitempty"`
	TimeMessages      map[string]map[string]string              `yaml:"time_messages,omitempty"`
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

