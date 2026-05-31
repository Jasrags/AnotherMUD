// Package gmcp defines the engine-facing payload types for GMCP
// packages emitted by the server. The transport layer
// (internal/conn/telnet.SendGmcp / future internal/conn/ws) writes
// the wire frames; this package owns the SHAPE of the JSON
// payloads, the canonical package-name constants, and the
// serialization helpers.
//
// Spec: docs/specs/networking-protocols.md §5 + §7.
//
// PD-2 (modern-client-plan): payload shape clones the Tapestry /
// Achaea conventions where they exist — short lowercase keys
// (`hp`, `maxhp`, `mp`, `maxmp`, `mv`, `maxmv`) so bundled Mudlet
// profiles and other off-the-shelf client packs work without
// remapping. Packages Tapestry never shipped get a new shape
// designed here.
package gmcp

// Canonical package-name constants. Spec §7 reserves dotted
// namespaces; engine packages live under `Char.*`, `Room.*`,
// `Comm.*`. Tests and senders reference these constants so a
// future renaming flows through one place.
const (
	// PackageCharVitals — current HP / max HP / future mana +
	// movement pools. Per PD-3, the session manager polls and
	// diffs per tick: at most one Char.Vitals frame per session
	// per tick, and only when the snapshot changed.
	PackageCharVitals = "Char.Vitals"

	// PackageRoomInfo — room identity + exits + ambience flags.
	// Event-driven (NOT poll-driven): emitted on every room
	// transition (movement, recall, login spawn, link-dead
	// reattach). Mudlet's room mapper relies on this package to
	// build the live map; one frame per transition is the spec
	// contract.
	PackageRoomInfo = "Room.Info"

	// PackageCharItemsList — full item list at a named location.
	// LocationInventory and LocationWear are the two locations
	// M16.4c ships; room placement and container contents follow
	// in later slices. Poll-and-diff like Char.Vitals: at most
	// one frame per location per tick, only when the snapshot
	// changed since last emission.
	PackageCharItemsList = "Char.Items.List"

	// PackageCharCombat — current combat status: in-combat flag,
	// primary target name + id + HP. Poll-and-diff like
	// Char.Vitals; at most one frame per session per tick, only
	// when the snapshot differs from the last-sent shadow.
	// Drives the Mudlet combat HUD's target panel.
	PackageCharCombat = "Char.Combat"

	// PackageCharEffects — full list of active effects on the
	// actor (spec abilities-and-effects §5). Poll-and-diff like
	// Char.Vitals; at most one frame per session per tick, only
	// when the snapshot differs from the last-sent shadow. Drives
	// the Mudlet active-effects panel (the column that shows
	// buffs/debuffs with remaining-pulse countdowns).
	//
	// Single-frame full-list emission (no per-effect Add/Remove
	// deltas) mirrors Char.Items.List: the diff is cheap and the
	// panel renders identically either way.
	PackageCharEffects = "Char.Effects"

	// PackageCharExperience — per-track progression snapshot (spec
	// progression.md §5). Drives the Mudlet XP-bar panel. Poll-
	// and-diff like Char.Vitals; at most one frame per session per
	// tick, only when any track's (level, xp, xpnext) tuple differs
	// from the last-sent shadow.
	//
	// Multi-track shape (one entry per registered track) so a MUD
	// with multiple parallel ladders — adventurer, crafting,
	// reputation — surfaces them all in the same payload. A Mudlet
	// profile rendering a single bar can pick the bound-class track
	// or the first entry.
	PackageCharExperience = "Char.Experience"

	// PackageCommChannelText — a single message on a chat channel
	// (spec chat-channels-and-tells §11). Event-driven (NOT poll-
	// driven): emitted once per delivered channel notification,
	// alongside the plain-text Deliver path that writes the
	// rendered line to the wire. Drives the Mudlet chat panel,
	// which routes per-channel rather than scraping the main
	// game window.
	PackageCommChannelText = "Comm.Channel.Text"

	// PackageCharLogin — actor identity at login (name + account).
	// Emit-once-then-watch: shipped on the first GMCP-active flush
	// after login, then re-shipped only on link-dead reattach
	// (the new peer needs the baseline). No content here ever
	// changes during a session — name is immutable; account id is
	// the persistent account row.
	PackageCharLogin = "Char.Login"

	// PackageCharStatusVars — declares the variable catalogue
	// available in Char.Status frames. Static for the engine's
	// lifetime; shipped once per GMCP-active session. Clients
	// without a hard-coded var list use this to build their
	// status panel dynamically.
	PackageCharStatusVars = "Char.StatusVars"

	// PackageCharStatus — runtime identity status (race, class,
	// alignment, alignment bucket tag). Poll-and-diff like
	// Char.Vitals; at most one frame per session per tick, only
	// when any field differs from the last-sent shadow. Drives
	// the Mudlet character-info panel without redundant scrapes
	// of `score` output.
	PackageCharStatus = "Char.Status"
)

// Char.Items "location" string constants per spec §7. Tapestry-
// compatible names so bundled Mudlet inventory modules wire up
// without renaming.
const (
	// LocationInventory — items the character is carrying (not
	// equipped, not in containers).
	LocationInventory = "inv"
	// LocationWear — items equipped in slots.
	LocationWear = "wear"
)

// CharVitals is the spec §7 Char.Vitals payload — the player's
// current vital pools (hit points, mana, movement, sustenance).
//
// Tapestry shape: `hp` / `maxhp` for hit points, `mp` / `maxmp`
// for mana, `mv` / `maxmv` for movement. The engine ships HP
// today; mana and movement are absent so their fields stay zero
// and serialize via `omitempty`. Sustenance is engine-specific
// (no Tapestry analogue) but emits under the obvious lowercase
// short key for consistency with the other pools.
//
// Zero values for hp/maxhp emit explicitly (not omitempty) —
// "HP 0" is meaningful (the player is dead) and a client panel
// that interprets a missing field as "no change" must see the
// zero. Optional fields (mana, movement, sustenance) omit when
// unset so a payload for an engine without those systems stays
// minimal on the wire.
type CharVitals struct {
	HP         int `json:"hp"`
	MaxHP      int `json:"maxhp"`
	MP         int `json:"mp,omitempty"`
	MaxMP      int `json:"maxmp,omitempty"`
	MV         int `json:"mv,omitempty"`
	MaxMV      int `json:"maxmv,omitempty"`
	Sustenance int `json:"sustenance,omitempty"`
}

// RoomInfo is the spec §7 Room.Info payload — the actor's current
// room identity, exits, and ambience flags. Mudlet's bundled
// mapper module subscribes to this package to build the live map
// (each frame becomes one map node + edges).
//
// Tapestry-shape per PD-2:
//   - `num` is the room id string (Tapestry uses an integer for
//     numeric muds; our engine uses dotted namespaced ids, which
//     Mudlet handles fine as map-key strings).
//   - `name` is the room's display name.
//   - `area` is the area id (Mudlet groups rooms by area for the
//     mapper's "zone" concept).
//   - `exits` is a map from direction code (short form: n/s/e/
//     w/ne/nw/se/sw/u/d) to the target room id. Engine cardinals
//     today are n/s/e/w/u/d; the longer diagonals are reserved
//     for content that ships them.
//   - `keywords` is the optional map of non-cardinal keyword exits
//     (portals from M15.2) mapping keyword → target room id.
//     Omitted when the room has none.
//   - `terrain` is the M15.4 terrain classifier (outdoors / indoors
//     / underground / etc.) — drives weather-eligibility and is
//     useful for the mapper's "indoor" overlay. Omitted when
//     empty (most rooms inherit the default).
//   - `details` is the room description text. Some clients render
//     it in a side panel; others ignore it. Always emitted so
//     Mudlet's room-tooltip layer has it.
type RoomInfo struct {
	Num      string            `json:"num"`
	Name     string            `json:"name"`
	Area     string            `json:"area,omitempty"`
	Exits    map[string]string `json:"exits"`
	Keywords map[string]string `json:"keywords,omitempty"`
	Terrain  string            `json:"terrain,omitempty"`
	Details  string            `json:"details,omitempty"`
}

// CharItem is one entry in a Char.Items.List payload.
//
// Tapestry shape:
//   - `id` is the runtime entity id (string in our engine; numeric
//     in Tapestry — clients consume both forms as opaque strings
//     for the panel's row key).
//   - `name` is the display name the panel renders. Mudlet's
//     inventory tile uses this directly.
//
// Tapestry also ships an `attrib` field carrying single-char
// flags (w=wearable, l=liquid, e=edible, …). Deferred until the
// engine has an item-classification surface — none of the M16.4c
// callers can populate it meaningfully, and emitting empty
// attrib would just be noise.
type CharItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CharItemsList is the spec §7 Char.Items.List payload — every
// item at one named location. Used for the initial panel
// population AND for full-refresh updates after any change at
// the location (M16.4c emits a fresh list rather than per-item
// Add/Remove deltas because the diff is cheap and the panel
// renders identically either way).
//
// Items must be a non-nil (possibly empty) slice. A nil slice
// marshals as JSON `null` which is ambiguous with "no change";
// callers initialize via `make([]CharItem, 0, n)` so the wire
// always carries `[]` for an empty list. The session flusher
// honors this via entityIDsToCharItems.
type CharItemsList struct {
	Location string     `json:"location"`
	Items    []CharItem `json:"items"`
}

// CharCombat is the spec §7 Char.Combat payload — the actor's
// current combat status and primary target snapshot.
//
// `in_combat` is the master flag. When false the target fields
// are omitted via omitempty so the panel can simply hide the
// target tile rather than render "Target: (none)".
//
// Target fields when in combat:
//   - target — display name of the primary target (head of the
//     actor's combat list per combat spec §2.5).
//   - target_id — the engine CombatantID string (`mob:...` or
//     `player:...`). Opaque to the client; useful for the panel
//     to dedupe consecutive updates on the same target.
//   - target_hp / target_max_hp — current vital pool of the
//     target. Per the spec the percent is more useful to a HUD
//     than raw HP (some MUDs hide raw HP from PvP opponents);
//     we ship both so clients can render either.
//   - target_hp_percent — convenience 0-100 derived from
//     target_hp / target_max_hp. Pre-computed so the client
//     doesn't have to handle the max=0 divide-by-zero edge.
//
// Other opponents (the rest of the actor's combat list past
// the primary) are intentionally omitted in M16.4d — multi-
// target HUDs are rare and the spec leaves the shape open.
// Add an `opponents` field here when a UI need surfaces.
type CharCombat struct {
	InCombat        bool   `json:"in_combat"`
	Target          string `json:"target,omitempty"`
	TargetID        string `json:"target_id,omitempty"`
	TargetHP        int    `json:"target_hp,omitempty"`
	TargetMaxHP     int    `json:"target_max_hp,omitempty"`
	TargetHPPercent int    `json:"target_hp_percent,omitempty"`
}

// CharEffect is one entry in a Char.Effects payload (spec
// abilities-and-effects §5). The effect manager owns lifetime; the
// session flusher snapshots and translates to this shape.
//
// Fields:
//   - `id` is the effect's stable id (lowercased at apply-time).
//     The panel uses it as the row key and to fetch a display
//     label from a client-side effect catalog.
//   - `remaining` is the remaining-pulse counter for time-bounded
//     effects. Omitted when the effect is permanent (the panel
//     should render an infinity glyph rather than a countdown).
//   - `permanent` is true for negative-duration effects per
//     progression.Effect.IsPermanent. Omitted (false) for the
//     common time-bounded case.
//   - `flags` is the effect's flag list (lowercased). Omitted
//     when empty. Lets the panel color-code by flag (`buff` vs.
//     `debuff`, etc.) without needing a client-side template
//     mirror.
//   - `source` is the SourceAbilityID — the ability that produced
//     the effect. Empty for admin-applied or world-hook effects;
//     omitted in that case so the panel can hide the source label.
type CharEffect struct {
	ID        string   `json:"id"`
	Remaining int      `json:"remaining,omitempty"`
	Permanent bool     `json:"permanent,omitempty"`
	Flags     []string `json:"flags,omitempty"`
	Source    string   `json:"source,omitempty"`
}

// CharExperienceTrack is one (track, level, xp, threshold) tuple in
// a Char.Experience payload (spec progression.md §5).
//
// Fields:
//   - `track` is the canonical case-sensitive track name. The
//     panel uses it as the row key and as the lookup into a
//     client-side track catalog.
//   - `name` is the human-facing display label. Omitted when
//     equal to track so the wire payload stays minimal for the
//     common case (no separate display name configured).
//   - `level` is the entity's current level on the track.
//   - `xp` is the entity's total XP on the track (cumulative).
//   - `xpnext` is the XP needed from the current snapshot to
//     reach the next level. Zero at max level — the panel
//     should render the max-level glyph rather than `0`.
//   - `maxlevel` is the track cap. Always emitted so the panel
//     can render "level 12 / 50" without a separate request.
//   - `at_max` is true once Level >= MaxLevel; lets the panel
//     hide the to-next progress bar without doing the compare.
//     Omitted (false) below cap.
//   - `overflow` is the over-cap XP accumulated past the
//     max-level threshold (progression spec §5.4). Zero below
//     cap; omitted in that case.
type CharExperienceTrack struct {
	Track    string `json:"track"`
	Name     string `json:"name,omitempty"`
	Level    int    `json:"level"`
	XP       int64  `json:"xp"`
	XPNext   int64  `json:"xpnext,omitempty"`
	MaxLevel int    `json:"maxlevel"`
	AtMax    bool   `json:"at_max,omitempty"`
	Overflow int64  `json:"overflow,omitempty"`
}

// CharExperience is the spec §5 Char.Experience payload — every
// registered track the actor has access to. Multi-track shape so
// a MUD with parallel XP ladders (adventurer / crafting /
// reputation) surfaces them in one panel update.
//
// Tracks must be a non-nil (possibly empty) slice. A nil slice
// marshals as JSON `null` which is ambiguous with "no change";
// the session flusher initializes via `make([]CharExperienceTrack,
// 0, n)` so the wire always carries `[]` for an engine that
// hasn't registered any tracks yet.
type CharExperience struct {
	Tracks []CharExperienceTrack `json:"tracks"`
}

// CharLogin is the boot-time identity payload (spec networking-
// protocols.md §7 — Tapestry-compatible Char.Name analogue).
//
// Fields:
//   - `name` is the actor's canonical display name (short form).
//   - `fullname` is the longer display form when distinct. Today
//     the engine carries no separate full-name surface, so it
//     mirrors `name`. Reserved for future title/honorific work.
//   - `account` is the actor's account id (opaque string). Useful
//     to the client for cross-character bookkeeping (e.g. a
//     Mudlet profile that remembers per-account UI state).
//
// All three fields always emit even when empty: a panel that
// reads `name` defensively must see the empty string rather than
// silently inheriting a stale value from a prior login.
type CharLogin struct {
	Name     string `json:"name"`
	FullName string `json:"fullname"`
	Account  string `json:"account"`
}

// CharStatusVars declares the variable catalogue available in
// future Char.Status frames. Tapestry-shape: a flat map from var
// name to human-facing caption. Clients without a hard-coded
// vocabulary use it to build the status panel dynamically.
//
// Single field so the encoder marshals as `{vars: {…}}` rather
// than as a bare top-level map; the envelope is easier for
// clients to discriminate from other Char.* packages and matches
// the pattern Tapestry shipped.
type CharStatusVars struct {
	Vars map[string]string `json:"vars"`
}

// CharStatus is the runtime identity status payload (race +
// class + alignment + alignment bucket tag).
//
// All four fields use `omitempty`: a fresh actor with no race or
// class assigned emits a minimal payload that the panel renders
// as "(none)". Alignment is an int with a meaningful zero
// (neutral) — kept always-emitted via no omitempty so the panel
// can distinguish "neutral" (alignment=0) from "missing"
// (alignment field absent).
type CharStatus struct {
	Race         string `json:"race,omitempty"`
	Class        string `json:"class,omitempty"`
	Alignment    int    `json:"alignment"`
	AlignmentTag string `json:"alignment_tag,omitempty"`
}

// CommChannelText is the spec §11 Comm.Channel.Text payload — one
// message on one channel, delivered to one subscribed actor.
//
// Tapestry-shape (PD-2) so bundled Mudlet chat profiles route
// without remapping:
//   - `channel` is the canonical channel id (`ooc`, `tapestry-
//     core:trade`). The panel uses it as the tab key.
//   - `talker` is the speaker's display name. Empty for system
//     announcements; the panel can render those without an
//     attribution prefix.
//   - `text` is the FULL rendered line as it appears in the main
//     window (`[ooc] Alice: hello`). Mudlet chat plugins
//     typically strip the channel prefix client-side from this
//     field rather than expecting a pre-stripped body — keeping
//     `text` identical to the main-window line maximizes
//     plugin compatibility.
type CommChannelText struct {
	Channel string `json:"channel"`
	Talker  string `json:"talker,omitempty"`
	Text    string `json:"text"`
}

// CharEffectsList is the spec §5 Char.Effects payload — every
// active effect on the actor. Used for the initial panel
// population AND for full-refresh updates after any change
// (apply/remove/expire) because the diff cost stays low and the
// panel renders identically either way.
//
// Effects must be a non-nil (possibly empty) slice. A nil slice
// marshals as JSON `null` which is ambiguous with "no change";
// the session flusher initializes via `make([]CharEffect, 0, n)`
// so the wire always carries `[]` for "no effects active".
type CharEffectsList struct {
	Effects []CharEffect `json:"effects"`
}
