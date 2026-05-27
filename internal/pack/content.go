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

	// BaseDisposition is the static reaction string (spec §5.1).
	// Optional; when present overrides player-state-driven rules
	// only for the `hostile` value (§5.3 step 3).
	BaseDisposition string `yaml:"base_disposition,omitempty"`

	// DispositionRules carries the structured default + ordered
	// rule list (spec §5.1, §5.3). Optional; mobs without rules
	// just don't trigger reactions.
	DispositionRules *DispositionFile `yaml:"disposition_rules,omitempty"`
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
