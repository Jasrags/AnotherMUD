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
