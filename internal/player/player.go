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
	"github.com/Jasrags/AnotherMUD/internal/pool"
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
//
// v10 (M8.5): `alignment` integer (spec progression.md §6.1).
// Zero (legacy v9 saves migrated forward) is the neutral
// default; AlignmentManager.Bucket lazy-resolves the bucket tag
// on first read. History is runtime-only by design (spec §6.3
// open question resolved to "no" for M8.5).
//
// v11 (M9.1): `abilities` block — parallel proficiency + cap maps
// keyed by lowercase ability id (spec abilities-and-effects §3.1).
// Absent block (legacy v10 saves migrated forward) means "no
// abilities learned"; the session-load path passes an empty
// AbilitySnapshot to the ProficiencyManager and Restore is a
// no-op. Caps are clamped to [0,100] and proficiency to [1,cap]
// on ingest.
//
// v12 (M11.1): `gold` integer (spec economy-survival §2.1). Zero
// (legacy v11 saves migrated forward, omitempty-absent) is the
// valid default — "missing entries are treated as zero". The
// CurrencyService floors it at zero on every mutation, so a save
// never carries a negative balance.
//
// v13 (M11.3): `sustenance` integer (spec economy-survival §4.1) in
// [0, 100]. Unlike the gold no-op, the v12→v13 migration is the first
// value-injecting migration: it seeds existing characters to full
// (100) so a returning player isn't suddenly famished. A fresh
// character is seeded to 100 inline at login; a value legitimately
// drained to 0 serializes as absent (omitempty) and reloads as 0 —
// which is the famished floor, so the round-trip is lossless.
//
// v14 (M15.3): `recall` string — the per-character recall room id
// from recall.md §6. Empty (legacy v13 saves migrated forward,
// fresh characters) is the documented "no recall point set"
// default; the `recall` verb short-circuits on empty with the
// no-point message. No injected value: a legacy character loads
// with no recall point and must bind one explicitly with
// `recall set`.
//
// v17 (Crafting & Cooking Phase 0): `known_recipes` list — the
// namespace-qualified ids of crafting recipes this character knows
// (crafting-and-cooking §7, §9). Persisted like proficiencies/abilities.
// Empty / absent (legacy v16 saves migrated forward, fresh characters)
// means "knows no recipes beyond what a discipline grants at runtime";
// the migration injects nothing. A known id whose recipe was removed from
// content loads cleanly and is ignored at restore (§9), never an error.
const CurrentVersion = 34

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
	Race        string                          `yaml:"race,omitempty"`
	// Class is the character's class id list (v18+). A character holds one
	// class today (single-element list), but the field is a list so a future
	// second class-track (multiclass — wot-character-model D1) is additive
	// content, not another save migration. Empty/absent = classless. The v17
	// scalar `class: fighter` is wrapped to `class: [fighter]` by
	// migrateV17toV18; the primary class (first element) is what single-value
	// readers (quest gate, score, GMCP) surface.
	Class []string `yaml:"class,omitempty"`
	// Background is the character-creation origin id (v19+ — backgrounds §5).
	// One per character (a scalar, unlike the class list). Empty/absent =
	// background-less. The granted starting package (skills, items, gold)
	// persists through the proficiency/inventory/gold surfaces; this field is
	// the label for display + future reference.
	Background string `yaml:"background,omitempty"`
	// BackgroundFeat is the feat the character chose from its background's
	// FeatOptions at creation (v29+ — the pick-one chooser). Empty = the
	// background offered no feat choice (the always-granted feats applied). Read
	// once by the character.created grant to apply the chosen feat; the result
	// persists as a KnownFeat, so this is the choice record, not the effect.
	BackgroundFeat string `yaml:"background_feat,omitempty"`
	// BackgroundEquipmentChoice is the index of the equipment package the
	// character chose from its background's EquipmentPackages at creation (v29+).
	// 0 (the default) is the first package or "no packages offered" — the
	// granter bounds-checks against the live background. Like BackgroundFeat,
	// the chosen items persist in inventory; this is the choice record.
	BackgroundEquipmentChoice int `yaml:"background_equipment_choice,omitempty"`
	// Gender is the character's gender, chosen at creation (v22+). A general
	// character attribute that fills the engine's pre-existing AllowedGenders
	// contract (class/background eligibility) and, in the WoT pack, derives a
	// channeler's saidin/saidar affinity (WoT S2 Phase 3). Stored lowercase
	// ("male"/"female" in v1). Empty/absent = unset (pre-v22 saves, or a pack
	// whose flow omits the step) — readers treat unset as "no affinity".
	Gender string `yaml:"gender,omitempty"`
	// ChannelingGift is the character's relationship to the One Power, chosen
	// at creation in the WoT pack's flow (v28+): "spark" (born channeling),
	// "learn" (can be taught), or "none" (the Source is closed). A durable
	// origin trait distinct from the chosen class — the WoT creation flow's
	// decoupled capability gate uses it to offer channeler vs non-channeler
	// classes (see progression.Class.AllowsGift), and it persists so `score`
	// and future S2 hooks can read it. Empty/absent = unset (pre-v28 saves, or
	// any non-WoT pack whose flow omits the channeling step). Stored lowercase.
	ChannelingGift  string `yaml:"channeling_gift,omitempty"`
	TrainsAvailable int    `yaml:"trains_available,omitempty"`
	Alignment       int    `yaml:"alignment,omitempty"`
	// Gold is the §2.1 integer currency balance (v12+). Zero serializes
	// as no `gold:` key via omitempty, indistinguishable from a legacy
	// v11 save where the field never existed — both load as a zero
	// balance, which is the documented default.
	Gold int `yaml:"gold,omitempty"`
	// Sustenance is the §4.1 hunger pool in [0, 100] (v13+). Seeded to
	// 100 at character creation; a value of 0 (famished floor) and an
	// absent key both decode to 0 — consistent, since 0 is the
	// legitimate famished state. The v12→v13 migration injects 100 for
	// legacy saves so existing characters don't load famished.
	Sustenance int          `yaml:"sustenance,omitempty"`
	Vitals     *VitalsState `yaml:"vitals,omitempty"`
	// Recall is the saved recall room id (v14+). Empty = no recall
	// point set (the documented default per recall.md §6); the
	// recall verb short-circuits on empty. Stored as a bare string
	// (not world.RoomID) so the save package doesn't import world.
	Recall string `yaml:"recall,omitempty"`
	// WimpyThreshold is the §5.1 HP-percent threshold (0 = wimpy
	// disabled). Added in M7.6 without a schema bump: zero-value
	// is indistinguishable from "field absent" so legacy v5 saves
	// round-trip unchanged. The session layer enforces [0, 100] on
	// set; load tolerates anything but treats anything < 1 or > 100
	// as disabled.
	WimpyThreshold int `yaml:"wimpy,omitempty"`

	// Autoloot is the per-character autoloot preference (loot-and-corpses
	// §6; off by default). Added without a schema bump on the same logic
	// as WimpyThreshold: the false zero-value is indistinguishable from
	// "field absent" (omitempty), so older saves load with autoloot off
	// and round-trip unchanged.
	Autoloot bool `yaml:"autoloot,omitempty"`

	// AutoAssist is the per-character auto-assist preference (grouping.md
	// §9; off by default — opt-in so a party member's engage doesn't yank
	// everyone into every fight). Added without a schema bump on the same
	// logic as Autoloot/WimpyThreshold: the false zero-value is
	// indistinguishable from "field absent" (omitempty), so older saves load
	// with auto-assist off and round-trip unchanged.
	AutoAssist bool `yaml:"auto_assist,omitempty"`

	// PromptTemplate is the player's custom prompt format
	// (ui-rendering-help §7.1). Empty means "use the engine default".
	// Added in M10.3b without a schema bump: an absent field decodes to
	// "" which is exactly the default-template signal, so legacy saves
	// round-trip unchanged. No verb sets it yet (a `prompt` command
	// lands with the M10 command surface); the field exists so the
	// flush path honors per-player templates the moment one can be set.
	PromptTemplate string `yaml:"prompt_template,omitempty"`

	// Abilities holds the persisted proficiency + cap maps for
	// learned abilities (spec abilities-and-effects §3.1). Both
	// maps key on lowercase ability id. Zero-value AbilitySnapshot
	// (empty maps) round-trips as no `abilities:` key via the
	// snapshot's own omitempty tags.
	Abilities progression.AbilitySnapshot `yaml:"abilities,omitempty"`

	// Roles is the character's set of authorization role strings
	// (roles-and-permissions.md §2, §6). Added in v15. Each entry is a
	// normalized (lowercased, trimmed) role name; the engine treats the
	// list as a set. Empty / absent means the character holds no roles —
	// the default for an unprivileged player. Distinct namespace from
	// gameplay tags: a role never participates in tag matching. omitempty
	// so a roleless save (the common case) writes no `roles:` key and a
	// legacy pre-v15 save round-trips as the empty set.
	Roles []string `yaml:"roles,omitempty"`

	// ShowRoomData is the admin/builder preference to append a room
	// metadata block (ids, coordinates, terrain, tags, properties, exit
	// targets) to `look` output. Off by default; the `roomdata` admin
	// verb toggles it and it persists across logins. Added without a
	// schema bump on the same logic as Autoloot/PromptTemplate: the
	// false zero-value is indistinguishable from "field absent"
	// (omitempty), so legacy saves load with it off and round-trip
	// unchanged. The display still gates on the admin role at render
	// time, so a saved-true flag does nothing for a non-admin.
	ShowRoomData bool `yaml:"show_room_data,omitempty"`

	// VisitedRooms is the persisted fog-of-war set (player-maps §3,§8):
	// the namespaced ids of rooms this character has entered at least
	// once. The map surfaces draw only visited rooms. Stored as a slice
	// (set semantics — uniqueness — are enforced at the room-entry hook,
	// not in the file); first-seen order is preserved but not relied on.
	// Added in v16. Empty / absent means "explored nothing" — the
	// default for a fresh character and the v15→v16 migration result:
	// a returning character re-explores to rebuild their map, which is
	// the intended fog-of-war behavior, so the migration injects nothing.
	VisitedRooms []string `yaml:"visited_rooms,omitempty"`

	// SeenAreas is the persisted set of area ids this character has ever
	// entered (player-maps §4): it gates the once-ever "first time!"
	// banner so it fires exactly once per area, across sessions. Slice
	// with set semantics enforced at the entry hook, like VisitedRooms.
	// Added without a schema bump (the empty/absent default means "no
	// areas entered yet", so legacy saves load with every area
	// un-greeted and round-trip unchanged); a returning character simply
	// re-greets each area once, which is acceptable for a cosmetic line.
	SeenAreas []string `yaml:"seen_areas,omitempty"`

	// MinimapEnabled is the per-character preference for the active
	// minimap appended to the room view (player-maps §4). Off by
	// default; the `minimap` verb toggles it. Added without a schema
	// bump on the Autoloot precedent — the false zero-value is
	// indistinguishable from absent (omitempty), so legacy saves load
	// with it off and round-trip unchanged.
	MinimapEnabled bool `yaml:"minimap,omitempty"`

	// MinimapSize is the per-character active-minimap size preset
	// (player-maps §4): "auto" (default), "small", "medium", or
	// "large". The `minimap` verb sets it. Empty is treated as "auto"
	// — the radius scales to the client's terminal width. Added without
	// a schema bump on the MinimapEnabled precedent: the "" zero-value
	// is indistinguishable from absent (omitempty), so legacy saves
	// load as auto and round-trip unchanged.
	MinimapSize string `yaml:"minimap_size,omitempty"`

	// KnownRecipes is the per-character set of crafting recipes this
	// character knows (crafting-and-cooking §7, §9), stored as a slice of
	// namespace-qualified recipe ids. Added in v17. Set semantics
	// (uniqueness) are enforced at the learn site, not in the file.
	// Empty / absent means "no recipes learned"; a discipline grants its
	// baseline recipes into this set at runtime (Phase 1). A known id
	// whose recipe is no longer in content is ignored at restore (§9),
	// never an error. omitempty so a recipeless save (the common case)
	// writes no key and a legacy pre-v17 save round-trips as the empty set.
	KnownRecipes []string `yaml:"known_recipes,omitempty"`

	// FeatCredits is the count of banked-but-unspent feat slots (EPIC S4
	// Phase 2 — docs/proposals/wot-feats.md §2.2). Earned 1 at character
	// creation + 1 per 3 character levels; spent by the feat verb (Phase 4).
	// Added in v20; absent/zero = no banked credits, the correct default for a
	// pre-v20 save.
	FeatCredits int `yaml:"feat_credits,omitempty"`
	// KnownFeats is the per-character set of taken feats (EPIC S4 Phase 2).
	// Added in v20; empty/absent = no feats. A known feat whose definition is
	// no longer in content is ignored when its bonus is recomputed, never an
	// error (fail-soft, like KnownRecipes).
	KnownFeats []KnownFeat `yaml:"known_feats,omitempty"`

	// KnownLanguages is the per-character set of tongues the character speaks,
	// read, and writes (languages.md §5). Added in v30; empty/absent = no
	// languages. Entries are language ids (namespace-qualified, like the
	// background home_language that seeds them); an id with no registered
	// language renders by id rather than erroring (fail-soft, like KnownFeats).
	KnownLanguages []string `yaml:"known_languages,omitempty"`

	// FactionStanding is the per-character standing bag (faction.md §8): a
	// map of faction id → signed standing. Added in v31; empty/absent = an
	// untouched character, who reads every faction at its starting standing.
	// Written through the faction.Entity adapter (SetStanding) on a Shift/Set;
	// rank tags are NOT persisted — they are re-derived from this bag on login
	// (the manager's Rank sync), exactly as alignment re-mirrors its bucket
	// tag. The combined faction history is runtime-only in v1 (matching
	// alignment's runtime-only history; §8 history persistence deferred).
	FactionStanding map[string]int `yaml:"faction_standing,omitempty"`

	// Reputation is the per-character single-axis renown score (reputation.md
	// §10): how widely known the character is — fame positive, infamy negative,
	// Unknown at 0. Added in v32; absent (the common case — a pre-v32 save, or a
	// character who has earned no renown) decodes to 0 = Unknown, which is the
	// engine's default Starting renown, so absent and a stored 0 are correctly
	// indistinguishable. A class/background that begins a character "already
	// known" applies its non-zero starting renown once at creation and stores it
	// explicitly (omitempty writes a non-zero value). Written through the
	// reputation.Entity adapter (SetRenown) on a Shift/Set; the tier tag is NOT
	// persisted — it is re-derived from this score on login (the manager's Tier
	// sync), exactly as faction re-mirrors its rank tag. Renown history is
	// runtime-only in v1 (matching faction/alignment).
	Reputation int `yaml:"reputation,omitempty"`

	// Pools is the persisted current value of the actor's generalized
	// resource pools — mana / movement today, the One Power tomorrow (WoT
	// S2 Phase 0). Added in v21. Only pools that are NOT full at save time
	// are written (current < max); a full or unseeded pool is omitted, so
	// the login path reseeds it full from the stat-derived max. The
	// persisted `max` is informational — restore re-derives the ceiling
	// from stats and applies only `current`, so a rebalanced max stat never
	// needs a migration. Empty/absent (the common case: HP is in Vitals,
	// mana/movement default full) writes no `pools:` key and a pre-v21 save
	// round-trips as "all pools full on login".
	Pools pool.Snapshot `yaml:"pools,omitempty"`

	// WorldID is the leaf ruleset pack this character belongs to
	// (character-identity §3) — e.g. "starter-world" or "wot". Stamped at
	// creation from the active world; the login gate refuses a character
	// whose WorldID is not in the server's active world set. Added in v23;
	// pre-v23 saves are backfilled from the Location namespace by
	// migrateV22toV23. Never empty after migration.
	WorldID string `yaml:"world_id,omitempty"`

	// Madness is the saidin taint a MALE channeler has accumulated (WoT S2
	// Phase 4+ — the One Power's signature asymmetry: saidin is tainted, saidar
	// is clean). It rises as a man weaves (overchannel adds more), decays slowly
	// when he abstains, and above thresholds drives a per-tick chance to suffer a
	// condition (S5). Female channelers never accrue it. Unlike active effects
	// (ephemeral), the accumulator is character state, so it persists across
	// relogin — no escape hatch. Added in v25; 0 (clean) for every pre-v25 save
	// and every non-channeler. Cured by the Heal-the-Mind weave / Mental
	// Stability feat.
	Madness int `yaml:"madness,omitempty"`

	// Mounts is the set of rideable mounts this character owns (mounts.md
	// §2.2, §10) — durable, exclusive ownership colocated with the owner. Each
	// record carries the mount's durable identity (its template) and, in later
	// slices, its barding/saddlebag/upkeep. Added in v26; empty/absent (the
	// common case: most characters own no mount) writes no `mounts:` key and a
	// pre-v26 save round-trips as the empty set. The live ride relationship is
	// NOT persisted (§10): on logout every owned mount resolves to a resting
	// (stabled) record and the player re-mounts after login. A record whose
	// template is no longer in content is ignored at materialization, never an
	// error (fail-soft, like KnownRecipes).
	Mounts []MountRecord `yaml:"mounts,omitempty"`

	// PowerAttackActive is the Power Attack combat stance (feats Bucket C — a
	// melee accuracy-for-power trade). A persistent posture, not a per-swing
	// d20 choice (Decision 0: tick/chance, not action economy): while on, the
	// attacker trades to-hit for damage every melee swing. Toggled by the
	// `powerattack on|off` verb. Added in v27; the false zero-value (the common
	// case — stance off, or the character never took the feat) writes no key
	// and a pre-v27 save round-trips as off. The trade only applies if the
	// character also holds the power-attack ability, so a stale-on stance on a
	// character without the feat is inert rather than wrong.
	PowerAttackActive bool `yaml:"power_attack_active,omitempty"`

	// Hirelings is the list of hire contracts this character owns (hireable-mobs.md
	// §9) — the durable resting form of a hireling while the live MobInstance does
	// not persist. Added in v33; empty/absent (the common case: most characters own
	// no hireling) writes no `hirelings:` key and a pre-v33 save round-trips as the
	// empty set. The live creature + its follow/combat state are NOT persisted (§9):
	// on logout each contract dematerializes its creature and re-materializes it on
	// login. A record whose template left content is ignored at materialization,
	// never an error (fail-soft, like Mounts).
	Hirelings []HirelingRecord `yaml:"hirelings,omitempty"`

	// AdminTags is the free-form gameplay-tag bag an admin `set tag` writes
	// onto a character (admin-verbs §4). Distinct from the manager-derived
	// tags (racial flags, alignment bucket, faction rank, reputation tier),
	// which their owning managers reconstruct at login: those are never
	// stored here, and `set tag` refuses to write a tag in a manager-owned
	// namespace (see internal/command/set.go). This bag is the one category
	// of character tag that is authored by hand and therefore must persist —
	// a player is not transient, so an admin-applied tag (a curse, a watch
	// flag, a party marker) is expected to survive relog. Added in v34;
	// empty/absent (the common case) writes no `admin_tags:` key and a pre-v34
	// save round-trips as the empty set. Folded into connActor.Tags() at
	// runtime so the AI disposition evaluator's `has_tag` rules match on it.
	AdminTags []string `yaml:"admin_tags,omitempty"`
}

// HirelingRecord is one hire contract on a player save (hireable-mobs.md §9). It
// is the durable form of a hireling — what persists between sessions while the
// live MobInstance does not. v1 carries the hireling's identity (its template);
// upkeep/condition state are additive fields for later slices (each omitempty so
// the shape stays migration-free as it grows).
type HirelingRecord struct {
	// TemplateID is the namespace-qualified mob-template id the hireling is
	// materialized from (hireable-mobs.md §9 "template identity"). Required.
	TemplateID string `yaml:"template_id"`
}

// MountRecord is one owned mount on a player save (mounts.md §10). It is the
// durable resting form of a mount — what persists between sessions while the
// live MobInstance does not. v1 carries the mount's identity (its template);
// barding, saddlebag contents, and upkeep state are additive fields added by
// later slices (each omitempty so the shape stays migration-free as it grows).
type MountRecord struct {
	// TemplateID is the namespace-qualified mob-template id the mount is
	// materialized from (mounts.md §10 "type/identity"). Required.
	TemplateID string `yaml:"template_id"`
}

// KnownFeat is one taken feat on a player save (EPIC S4 Phase 2 —
// docs/proposals/wot-feats.md §2.5). FeatID is the global feat id; Param binds
// the instance for a per-parameter feat (a weapon/skill id; empty otherwise);
// Count is the number of takes for a stackable feat (0/1 for a non-stackable
// feat). The conferred bonuses are recomputed from this set, not separately
// persisted — KnownFeats is the source of truth.
type KnownFeat struct {
	FeatID string `yaml:"feat"`
	Param  string `yaml:"param,omitempty"`
	Count  int    `yaml:"count,omitempty"`
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
	// Loaded persists a magazine weapon's current loaded-round count so a
	// reloaded firearm keeps its rounds across relog. A pointer so absent
	// (nil) — the common non-firearm / empty-magazine case — stays out of the
	// wire format and reads as an unloaded weapon on respawn. Additive with a
	// safe zero-value, so no save-version bump (an old save simply has none).
	Loaded *int `yaml:"loaded,omitempty"`
}

// EquippedItem is one entry in the persisted equipment map (v3+). The
// pair is what lets respawnEquipment reattach the persisted Stats
// modifiers (sourced under EquipmentSourceKey(Entity)) to a freshly
// re-spawned ItemInstance with a new runtime id.
type EquippedItem struct {
	Template string `yaml:"template"`
	Entity   string `yaml:"entity"`
	// Loaded persists a wielded INTERNALLY-FED magazine weapon's loaded-round
	// count across relog (SR-M3e; see InventoryEntry.Loaded). nil = not an
	// internally-fed firearm / empty magazine.
	Loaded *int `yaml:"loaded,omitempty"`
	// Holder persists the ammunition holder inserted in a wielded HOLDER-FED
	// weapon (ammo-and-reloading §9). nil = no holder inserted / not holder-fed.
	Holder *EquippedHolder `yaml:"holder,omitempty"`
}

// EquippedHolder is the persisted state of the holder inserted in a holder-fed
// weapon: its template id and its remaining round count. Re-established on
// respawn so a loaded firearm stays loaded across relog.
type EquippedHolder struct {
	Template string `yaml:"template"`
	Loaded   int    `yaml:"loaded"`
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

// IsEmpty reports whether the store holds no character records yet. Used
// by the session layer to detect the very first character in a fresh
// deployment so it can be bootstrapped as admin. Each character is a
// subdirectory (players/<name>/), so emptiness is "no directory entries"
// — stray files such as a macOS .DS_Store are ignored so they can't
// silently disable the bootstrap. A read error is treated as non-empty —
// fail safe: an unreadable store should not hand out admin.
func (s *Store) IsEmpty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			return false
		}
	}
	return true
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

// Delete removes a character's entire on-disk record — the players/<name>/
// directory and every sibling file under it (player.yaml plus quest.yaml,
// notifications.yaml, the chat-subscriptions file): a hard delete
// (character-select §8 roster operations). Removing a non-existent record is
// not an error (idempotent), so a racing double-delete is harmless. The
// account unlink is the caller's separate step (account.RemoveCharacter).
func (s *Store) Delete(ctx context.Context, name string) error {
	dir, err := s.playerDir(name)
	if err != nil {
		return fmt.Errorf("player.Delete: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("player.Delete %q: %w", name, err)
	}
	return nil
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
	1:  migrateV1toV2,
	2:  migrateV2toV3,
	3:  migrateV3toV4,
	4:  migrateV4toV5,
	5:  migrateV5toV6,
	6:  migrateV6toV7,
	7:  migrateV7toV8,
	8:  migrateV8toV9,
	9:  migrateV9toV10,
	10: migrateV10toV11,
	11: migrateV11toV12,
	12: migrateV12toV13,
	13: migrateV13toV14,
	14: migrateV14toV15,
	15: migrateV15toV16,
	16: migrateV16toV17,
	17: migrateV17toV18,
	18: migrateV18toV19,
	19: migrateV19toV20,
	20: migrateV20toV21,
	21: migrateV21toV22,
	22: migrateV22toV23,
	23: migrateV23toV24,
	24: migrateV24toV25,
	25: migrateV25toV26,
	26: migrateV26toV27,
	27: migrateV27toV28,
	28: migrateV28toV29,
	29: migrateV29toV30,
	30: migrateV30toV31,
	31: migrateV31toV32,
	32: migrateV32toV33,
	33: migrateV33toV34,
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

// migrateV9toV10 adds the `alignment` integer introduced in M8.5
// (spec progression.md §6.1). No-op on dict content: a legacy
// v9 save carries no alignment, so the absence is preserved
// (zero = neutral default). The session load path resolves the
// bucket lazily via AlignmentManager.Bucket on first read.
func migrateV9toV10(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV10toV11 adds the `abilities` block introduced in M9.1
// (spec abilities-and-effects §3.1). No-op on dict content: a
// legacy v10 save carries no proficiency/cap maps, so the absence
// is preserved (zero-value AbilitySnapshot = nothing learned).
// The ProficiencyManager's Restore short-circuits on empty input.
func migrateV10toV11(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV11toV12 adds the `gold` integer introduced in M11.1 (spec
// economy-survival §2.1). No-op on dict content: a legacy v11 save
// carries no gold key, and absence decodes to a zero balance — the
// documented default ("missing entries are treated as zero").
func migrateV11toV12(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV12toV13 adds the `sustenance` integer introduced in M11.3
// (spec economy-survival §4.1). Unlike the prior no-op migrations, this
// one INJECTS a value: a legacy v12 character has no sustenance, and
// letting it decode to the zero default would land them at the famished
// floor on first login. Seeding 100 (full) matches the
// character-creation default so an existing character is unaffected by
// the feature's arrival. Idempotent on a dict already carrying a value
// (only fills when absent), though in practice this only runs on v12
// dicts that never had the key.
func migrateV12toV13(in map[string]any) (map[string]any, error) {
	if _, ok := in["sustenance"]; !ok {
		in["sustenance"] = 100
	}
	return in, nil
}

// migrateV13toV14 adds the `recall` string introduced in M15.3 (spec
// recall.md §6). No-op on dict content: a legacy v13 save carries no
// recall key, and absence decodes to an empty string — the
// documented "no recall point set" default. Unlike sustenance, this
// migration does NOT inject a value: a returning character should
// log in with no recall point and bind one explicitly with
// `recall set`, not be quietly bound to wherever they last logged
// out.
func migrateV13toV14(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV14toV15 adds the `roles` list introduced for
// roles-and-permissions.md §6. No-op on dict content: a legacy v14 save
// carries no roles key, and absence decodes to a nil slice — the
// documented "no roles / unprivileged" default. The migration injects
// nothing: privilege enters only via the config seed (§5) or an
// in-game grant (§4), never by quietly elevating a returning character.
func migrateV14toV15(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV15toV16 adds the `visited_rooms` fog-of-war set introduced
// for player-maps.md §8. No-op on dict content: a legacy v15 save
// carries no visited_rooms key, and absence decodes to a nil slice —
// the documented "explored nothing" default. The migration injects
// nothing: a returning character starts with a blank map and re-explores
// to rebuild it, which is the intended fog-of-war behavior, not a set to
// back-fill.
func migrateV15toV16(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV16toV17 adds the `known_recipes` list introduced for
// crafting-and-cooking.md §7. No-op on dict content: a legacy v16 save
// carries no known_recipes key, and absence decodes to a nil slice — the
// documented "no recipes learned" default. The migration injects nothing:
// a returning character learns recipes at runtime (a discipline grants its
// baseline set, §2), it is not a set to back-fill.
func migrateV16toV17(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV17toV18 widens the `class` value from a scalar id to a list of ids
// (wot-character-model D1 — multi-track-as-multiclass enablement). A v17 save
// carries `class: fighter`; v18 carries `class: [fighter]`, so a future second
// class-track is additive content rather than another migration.
//
// Cases: absent/nil → left absent (classless, decodes to a nil slice); a
// non-empty string → wrapped in a 1-element list; an empty string → dropped
// (classless). An already-list value (idempotent re-run) is left untouched.
func migrateV17toV18(in map[string]any) (map[string]any, error) {
	raw, ok := in["class"]
	if !ok || raw == nil {
		return in, nil
	}
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			delete(in, "class")
			return in, nil
		}
		in["class"] = []any{v}
	case []any:
		// Already a list (idempotent) — leave it.
	default:
		// Unknown shape — drop it, same fail-soft reasoning as the other
		// shape-widening migrations (refusing to load is worse than losing a
		// malformed class field).
		delete(in, "class")
	}
	return in, nil
}

// migrateV18toV19 introduces the `background` field (backgrounds §5). A v18
// save has no background key; its absence decodes to "" — a background-less
// character — which is the correct default, so the migration is a no-op (the
// granted package, if any, already lives in the proficiency/inventory/gold
// surfaces). Pre-existing characters simply have no recorded origin label.
func migrateV18toV19(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV19toV20 introduces the feat-system save fields — `feat_credits` and
// `known_feats` (EPIC S4 Phase 2 — docs/proposals/wot-feats.md §2.5). A v19
// save has neither key; their absence decodes to 0 credits + no feats, the
// correct default (a pre-feats character has earned nothing yet), so the
// migration is a no-op. Banked credits begin accruing on the next level-up.
func migrateV19toV20(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV20toV21 introduces the `pools` block — the persisted current value
// of the generalized resource pools (mana / movement; the One Power later, WoT
// S2 Phase 0). A v20 save has no `pools` key; its absence decodes to an empty
// snapshot, which the login path treats as "reseed every pool full from its
// stat-derived max" — the correct default (a pre-pools character was always
// reseeded full anyway), so the migration is a no-op.
func migrateV20toV21(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV21toV22 introduces the `gender` field (WoT S2 Phase 3 — a channeler's
// saidin/saidar affinity derives from it). A pre-v22 save has no `gender` key;
// its absence decodes to the empty string, which readers treat as "unset / no
// affinity". Existing characters keep their weaves at full potency until a
// gender is assigned, so the migration is a no-op.
func migrateV21toV22(in map[string]any) (map[string]any, error) {
	return in, nil
}

// DefaultBackfillWorld is the world a pre-v23 save is stamped with when its
// Location has no parseable namespace (character-identity §4). It matches the
// engine's default start world.
const DefaultBackfillWorld = "starter-world"

// migrateV22toV23 backfills the `world_id` field introduced for world-locking
// (character-identity §4). It derives the world from the namespace of the
// save's `location` room id ("starter-world:town-square" → "starter-world"),
// falling back to DefaultBackfillWorld when location is missing or carries no
// namespace. Deterministic; needs no operator input. After this every save has
// a non-empty world_id, so the login gate never sees an unstamped character.
func migrateV22toV23(in map[string]any) (map[string]any, error) {
	world := DefaultBackfillWorld
	if locRaw, ok := in["location"]; ok {
		if loc, ok := locRaw.(string); ok {
			if ns, _, found := strings.Cut(loc, ":"); found {
				if ns = strings.TrimSpace(ns); ns != "" {
					world = ns
				}
			}
		}
	}
	in["world_id"] = world
	return in, nil
}

// BackfillMovementMax is the movement_max base-stat value migrateV23toV24
// stamps onto a pre-v24 save. Frozen at the v24-era
// progression.DefaultMovementMax so the migration stays deterministic if that
// default is later rebalanced (a migration must produce the same result
// regardless of future code).
const BackfillMovementMax = 30

// migrateV23toV24 backfills the `movement_max` base stat onto saves whose
// persisted `stats_base` block predates the movement-cost feature
// (world-rooms-movement §3.3 — walking spends movement points). Such a block
// carries the classic attributes + hp_max but no movement_max.
//
// Belt-and-suspenders by design: at login RestoreBase MERGES the persisted
// base onto a block already seeded from DefaultPlayerBase (which now carries
// movement_max), so a pre-v24 character already resolves the pool correctly at
// runtime — the merge keeps the construction default for any key the save
// omits. This migration makes the value explicit ON DISK (so an unread save
// already carries it) and decouples it from the merge + the live default.
//
// Only a present, non-empty stats_base that is missing movement_max is
// touched: an absent block already loads the FULL default (movement_max
// included, since RestoreBase isn't invoked for an empty snapshot), and a
// block already carrying the key is left as-is.
func migrateV23toV24(in map[string]any) (map[string]any, error) {
	raw, ok := in["stats_base"]
	if !ok || raw == nil {
		return in, nil // no persisted base — the full default applies at load
	}
	list, ok := raw.([]any)
	if !ok {
		return in, nil // malformed shape — leave it for the decoder to reject
	}
	movementMax := string(progression.StatMovementMax)
	for _, e := range list {
		if m, ok := toStringKeyMap(e); ok {
			if s, _ := m["stat"].(string); s == movementMax {
				return in, nil // already present
			}
		}
	}
	in["stats_base"] = append(list, map[string]any{
		"stat":  movementMax,
		"value": BackfillMovementMax,
	})
	return in, nil
}

// migrateV24toV25 is a no-op: the v25 addition (Save.Madness, the saidin taint
// accumulator) defaults to 0 (clean) for every pre-v25 character, which is the
// correct starting value — a returning channeler simply begins untainted. The
// field is append-only; the bump exists so the on-disk version reflects the new
// shape. Like every migration in this chain it must never be edited once shipped.
func migrateV24toV25(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV25toV26 is a no-op: the v26 addition (Save.Mounts, the owned-mount
// list — mounts.md §10) is absent on a pre-v26 save, which decodes to a nil
// slice — the correct default for a character who owns no mount. No on-disk
// shape needs to change.
func migrateV25toV26(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV26toV27 is a no-op: the v27 addition (Save.PowerAttackActive, the
// Power Attack combat stance — feats Bucket C) is absent on a pre-v27 save,
// which decodes to false — the correct default (stance off). No on-disk shape
// needs to change.
func migrateV26toV27(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV27toV28 is a no-op: the v28 addition (Save.ChannelingGift, the WoT
// channeling origin chosen at creation) is absent on a pre-v28 save, which
// decodes to "" — the correct default (unset; no gift gate, no affinity). Old
// characters predate the WoT per-pack creation flow, so none carry a gift. No
// on-disk shape needs to change.
func migrateV27toV28(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV28toV29 is a no-op: the v29 additions (Save.BackgroundFeat +
// Save.BackgroundEquipmentChoice, the pick-one background chooser) are absent on
// a pre-v29 save, which decode to "" / 0 — the correct default (no choice;
// older backgrounds granted their package without a choice anyway). No on-disk
// shape needs to change.
func migrateV28toV29(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV29toV30 is a no-op: the v30 addition (Save.KnownLanguages, the
// per-character set of tongues a character speaks — languages.md §5) is absent
// on a pre-v30 save, which decodes to an empty set — the correct default (no
// character loses or gains a language across the bump; older characters predate
// the languages substrate, and a returning character's home language is not
// re-granted). No on-disk shape needs to change.
func migrateV29toV30(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV30toV31 is a no-op: the v31 addition (Save.FactionStanding, the
// per-character faction standing bag — faction.md §8) is absent on a pre-v31
// save, which decodes to an empty bag — the correct default (an untouched
// character reads every faction at its starting standing; §8.1). No on-disk
// shape needs to change.
func migrateV30toV31(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV31toV32 is a no-op: the v32 addition (Save.Reputation, the
// single-axis renown score — reputation.md §10) is absent on a pre-v32 save,
// which decodes to 0 = Unknown — the engine's default starting renown (§2). No
// on-disk shape needs to change; a character who predates the renown substrate
// is correctly Unknown.
func migrateV31toV32(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV32toV33 introduces the hireling list (hireable-mobs.md §9). No-op: an
// absent `hirelings` key loads as the empty set (the common case — no hireling
// owned), so absent and stored-empty are correctly indistinguishable.
func migrateV32toV33(in map[string]any) (map[string]any, error) {
	return in, nil
}

// migrateV33toV34 introduces the admin_tags bag (admin-verbs §4 — the
// free-form gameplay-tag set an admin `set tag` writes onto a character).
// A no-op: every pre-v34 save had no admin-applied tags, so the field is
// absent and Restore reads it as the empty set (nil), exactly matching a
// character no admin has tagged. The manager-derived tags (racial, alignment,
// faction, reputation) are reconstructed at login as before and were never
// stored here, so nothing needs backfilling.
func migrateV33toV34(in map[string]any) (map[string]any, error) {
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
