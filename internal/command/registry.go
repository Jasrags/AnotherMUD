// Package command implements the keyword registry and player-input
// dispatcher described in docs/specs/commands-and-dispatch.md.
//
// M1 scope (intentionally small):
//   - Registration: keyword + handler. No aliases, no priority, no
//     roles, no arg types, no GMCP, no help generation.
//   - Resolution: exact match first, then prefix match against all
//     registered keywords; ties broken by registration order.
//   - Dispatch: player route only. Empty input → no-op. Unknown verb
//     → "Huh?". No chain (";"), no repeat ("3n"), no flood control.
//
// The narrow surface is deliberate: M1 only needs look / movement /
// quit, and any additional machinery now would be guesswork before a
// real consumer (pack-registered commands, mob route) shows up.
package command

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/chat"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/decoration"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/property"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stacking"
	"github.com/Jasrags/AnotherMUD/internal/stats"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// ErrQuit is returned by Dispatch when the actor's quit verb fires.
// The session loop unwinds cleanly on this — it is a signal, not a
// failure.
var ErrQuit = errors.New("command: quit")

// actorRoomID returns the actor's current room id, or "" when roomless
// (mid-transition / test actors). Used by the §6 unknown-verb log.
func actorRoomID(a Actor) world.RoomID {
	if r := a.Room(); r != nil {
		return r.ID
	}
	return ""
}

// Actor is the per-session view a command handler needs. The session
// layer implements this; command does not own player state.
type Actor interface {
	ID() string
	Room() *world.Room
	SetRoom(*world.Room)
	// Name returns the player's display name (mixed case as the player
	// chose at character creation). Used by handlers that emit
	// observable presence ("Alice heads north.").
	Name() string
	// PlayerID returns the stable player identifier used by the
	// session manager's indices. Empty for test actors that don't
	// participate in broadcast.
	PlayerID() string
	// Write sends a line of output to the actor. The implementation
	// is responsible for any line-ending conventions and for expanding
	// any pack-authored color markup (see internal/ansi).
	Write(ctx context.Context, msg string) error
	// ColorEnabled reports whether the actor currently wants ANSI
	// color in their output. Used by the `color` command to read
	// state for the no-arg form.
	ColorEnabled() bool
	// SetColorEnabled toggles color for this actor.
	SetColorEnabled(bool)

	// Inventory returns the entity ids of items currently held by this
	// actor, in the order they were picked up. Fresh slice — safe to
	// mutate.
	Inventory() []entities.EntityID
	// AddToInventory appends id to the actor's contents. Mutating the
	// holder marks the underlying save dirty.
	AddToInventory(entities.EntityID)
	// RemoveFromInventory removes id; returns true if it was present.
	RemoveFromInventory(entities.EntityID) bool

	// Equipment returns slot key → entity id for currently-equipped
	// items. Fresh map — safe to mutate.
	Equipment() map[string]entities.EntityID
	// Equip atomically moves id from inventory to equipment at
	// slotKey and applies mods to the holder's stat block under
	// EquipmentSourceKey(id). Returns false if id was not in
	// inventory (TOCTOU loss to a concurrent drop). Auto-swap is the
	// handler's responsibility — perform the unequip side first, then
	// call Equip on the now-empty slot.
	Equip(slotKey string, id entities.EntityID, mods []stats.Modifier) bool
	// Unequip atomically removes the item at slotKey, returns it to
	// inventory, and reverses its stat modifiers. Returns the removed
	// entity id and true on success.
	Unequip(slotKey string) (entities.EntityID, bool)

	// MarkContentsDirty re-syncs the actor's persisted inventory tree
	// from runtime state and flips the save dirty bit. Called by
	// handlers that mutate the Contents substrate without touching
	// the actor's top-level inventory list (M5.9b put: the sword
	// leaves inventory in one step, then lands in a container in a
	// second step that the actor doesn't see, so the save tree built
	// during the first step has an empty container; this method
	// re-runs the build after the second step).
	//
	// Implementations that do not persist (test stubs) should make
	// this a no-op.
	MarkContentsDirty()
}

// ProgressionHolder is the M8.2 surface a handler needs to read and
// mutate the actor's progression-track state. Separated from Actor
// so tests / test actors that don't exercise XP can satisfy Actor
// without a no-op stub forest. The xp verb (and future class
// path-grant subscribers) type-asserts to this when wiring is
// needed.
type ProgressionHolder interface {
	GrantXP(ctx context.Context, mgr *progression.Manager, track, source string, amount int64) progression.GrantResult
	DeductXP(ctx context.Context, mgr *progression.Manager, track string, amount int64) progression.DeductResult
	TrackInfo(mgr *progression.Manager, track string) (progression.TrackInfo, bool)
}

// Broadcaster is the small surface a handler needs to address other
// players in the world. The session manager satisfies it. Handlers
// MUST tolerate a nil Broadcaster (e.g. unit tests) and skip the
// broadcast in that case.
type Broadcaster interface {
	// SendToRoom delivers text to every player whose current room
	// matches roomID, excluding any player whose id appears in
	// excludePlayerIDs. The implementation is responsible for any
	// per-recipient formatting (color, prompts).
	SendToRoom(ctx context.Context, roomID world.RoomID, text string, excludePlayerIDs ...string)
}

// Locator finds an actor by display name within a room. The session
// manager satisfies this via a small adapter so the give handler (and
// future targeted verbs: tell, follow) can resolve a name argument
// without each handler needing to know about session.Manager.
//
// Returns nil if no live actor matches. Name match is case-insensitive
// on Name(). Handlers MUST tolerate a nil Locator (tests that don't
// exercise target lookup pass a zero-value env).
//
// PlayersInRoom (M17.2d₄) enumerates every live player actor in roomID
// so the §5 entity/player/visible resolvers can keyword-match players
// the same way they match mobs. BuildResolveContext excludes the actor
// itself from the candidates. The order is unspecified; callers that
// need determinism rely on the resolver's match rules, not iteration
// order. Returns nil/empty for an unknown room or a name-only Locator
// that predates the method (the resolvers then surface mobs only).
type Locator interface {
	FindInRoom(roomID world.RoomID, name string) Actor
	PlayersInRoom(roomID world.RoomID) []Actor
}

// Env bundles the per-server singletons a handler may need beyond the
// actor and the world. Carrying them in a struct lets future additions
// (registries, services) land without re-widening Dispatch.
//
// All fields are optional. Handlers MUST tolerate nils — unit tests
// for command groups that don't touch items routinely pass a zero-value
// env.
type Env struct {
	World       *world.World
	Broadcaster Broadcaster
	Items       *entities.Store
	Placement   *entities.Placement
	// Contents is the container↔item index (M5.9b). Consumed by the
	// put handler to move items between actor inventory and
	// containers and to read container fullness. May be nil in tests
	// that don't exercise containers.
	Contents *entities.Contents
	// Slots is the equipment-slot registry, consumed by the equip
	// command handler. Templates are deliberately NOT carried here —
	// they're only needed at login time (respawnInventory /
	// respawnEquipment) and live on session.Config.
	Slots *slot.Registry
	// Bus is the engine event bus. Handlers publish observable
	// events after successful mutations. May be nil in tests that
	// don't subscribe to anything — handlers MUST nil-guard.
	Bus *eventbus.Bus
	// Properties is the engine-wide property registry (M14). The admin
	// `set property` handler (M19.4h) reads it to validate a property
	// exists, is admin-settable, and to coerce the value to its declared
	// type. May be nil in tests that don't exercise property writes.
	Properties *property.Registry
	// Rarity / Essence are the M20 item-decoration registries. Item
	// display resolves an item's reserved rarity/essence property key
	// through them to render the decoration markup. nil disables
	// decoration rendering (items show their bare names).
	Rarity  *decoration.RarityRegistry
	Essence *decoration.EssenceRegistry
	// Stacking is the M21 inventory stack-grouping service. Item listings
	// group identical items into "name (xN)" stacks through it. nil
	// degrades to one line per item.
	Stacking *stacking.Service
	// Locator resolves another actor by name + room. Consumed by the
	// give command handler (and future targeted verbs). May be nil
	// in tests; handlers MUST nil-guard.
	Locator Locator
	// Roster is the world-wide online-player snapshot the `who` verb reads
	// (who §2–§4). The session Manager satisfies it. nil disables `who`.
	Roster Roster
	// BadInput records unknown player verbs (commands-and-dispatch §6). The
	// dispatcher writes to it on every unknown verb; the `badinput` admin
	// verb reads its snapshot. nil disables tracking (Record is a no-op).
	BadInput *BadInputTracker
	// Disposition is the room-entry hook the disposition evaluator
	// exposes (spec mobs-ai-spawning §4). May be nil in tests and
	// in headless boot paths. Handlers MUST nil-guard.
	Disposition DispositionHook
	// Combat is the engage/disengage manager (spec combat §2). The
	// kill verb routes here; future combat verbs (flee, wimpy)
	// follow. May be nil in tests that don't exercise combat verbs.
	Combat *combat.Manager

	// Flee is the verb-driven flee primitive (M7.6d). The function
	// runs the §5.2 attempt against the configured Mover / Rooms /
	// Bus / Cooldowns set up at the composition root and returns the
	// outcome so the verb can render a precise message. nil in
	// tests that don't exercise the flee verb.
	Flee func(ctx context.Context, c combat.CombatantID) combat.FleeOutcome

	// ReloadScripts is the M17.3 script hot-reload trigger. It
	// re-discovers pack Lua from disk and swaps the scripting runtime
	// without a restart, returning the number of scripts loaded. Wired
	// at the composition root (pack.DiscoverScripts → Runtime.Reload);
	// nil disables the reload verb.
	ReloadScripts func(ctx context.Context) (int, error)

	// Progression is the M8.2 XP/level service. The xp verb routes
	// through it; future verbs (score, train, practice) will too.
	// nil in tests that don't exercise progression verbs.
	Progression *progression.Manager
	// Training is the M8.6 training service. The train + practice
	// verbs route through it. nil in tests that don't exercise
	// training.
	Training *progression.TrainingManager

	// Abilities is the M9.1 ability registry (spec
	// abilities-and-effects §2). The `abilities` verb reads it for
	// display names + classification; the enqueue verbs (cast +
	// skill-named) look an ability up before queueing. nil in tests
	// that don't exercise ability verbs.
	Abilities *progression.AbilityRegistry
	// Proficiency is the M9.1 per-entity proficiency manager. The
	// `abilities` verb reads the actor's learned set + caps from it.
	// nil in tests.
	Proficiency *progression.ProficiencyManager
	// ActionQueue is the M9.3 per-entity action queue (spec §4.1).
	// The enqueue verbs Push onto it; the combat ability phase
	// drains it. nil in tests.
	ActionQueue *progression.ActionQueueManager

	// Help is the M10.5 help-topic service. The help verb queries it.
	// nil in tests that don't exercise help.
	Help *help.Service

	// Quests is the M10.7 quest service. The accept/abandon/quests verbs
	// route through it. nil in tests that don't exercise quests.
	Quests *quest.Service

	// Currency is the M11.1 economy currency service (spec
	// economy-survival §2). The `gold` verb reads through it and the
	// get/give auto-convert hook credits through it. nil in tests
	// that don't exercise currency; handlers MUST nil-guard.
	Currency *economy.CurrencyService

	// Shop is the M11.2 shop service (spec §3). The buy/sell/value/
	// list verbs route through it after locating a shop-tagged NPC in
	// the room. nil in tests that don't exercise shops; handlers MUST
	// nil-guard.
	Shop *economy.ShopService

	// Rest is the M11.4 rest service (spec §5). The rest/sleep/wake
	// verbs drive transitions through it. nil in tests that don't
	// exercise rest; handlers MUST nil-guard.
	Rest *economy.RestService

	// Consumable is the M11.5 consumable service (spec §6). The
	// eat/drink/use verbs route through it. nil in tests that don't
	// exercise consumables; handlers MUST nil-guard.
	Consumable *economy.ConsumableService

	// Notifications is the M13.1 notification manager. The tell /
	// reply verbs publish through it. nil-safe: with no manager
	// wired the tell verbs print "Tells are not enabled."
	Notifications *notifications.Manager

	// TellResolver maps a player name to a recipient route (online
	// actor or offline save). The tell / reply verbs consult it to
	// choose between immediate delivery and persisted enqueue.
	// nil-safe alongside Notifications.
	TellResolver TellResolver

	// RoleTargetResolver maps a player name to a live RoleController for
	// the grant / revoke verbs (roles-and-permissions §4). nil disables
	// role administration. GrantingRole is the role an actor must hold to
	// grant or revoke (config, §8; defaults to `admin` when empty).
	RoleTargetResolver RoleTargetResolver
	GrantingRole       string
	// AdminRole is the role an admin-marked command requires at dispatch
	// (admin-verbs §2/§8). The Dispatch gate reads it; defaults to `admin`
	// when empty.
	AdminRole string

	// Announcer is the all-sessions broadcast seam the `announce` admin
	// verb uses (admin-verbs §5). The session Manager satisfies it. nil
	// disables `announce` (the handler reports it's not enabled).
	Announcer Announcer

	// PlayerRoom resolves an online player's current room by name,
	// world-wide (admin-verbs §3 world-scoped resolution). The `teleport`
	// verb uses it for the teleport-to-player form. nil disables that form
	// (teleport-to-room still works).
	PlayerRoom PlayerRoomResolver

	// ChatRegistry is the M13.6 channel catalog. Read by chat list /
	// chat history and by the dynamically-registered per-channel
	// verbs (ooc / admin / pack channels). nil-safe.
	ChatRegistry *chat.Registry

	// ChatSubscribers returns the online-subscriber set for a channel
	// (entity id → canonical name). v1 returns every online player
	// for every channel.
	ChatSubscribers ChatSubscribers

	// ChatScrollbacks returns the per-channel ring buffer. The
	// publish path appends after fan-out; chat history reads tails.
	ChatScrollbacks ChatScrollbacks

	// Clock is the engine time source. Handlers that need to stamp
	// timestamps (e.g., chat scrollback PublishedAt) MUST use it
	// rather than time.Now(); see ROADMAP foundation F3. nil-safe;
	// handlers MUST nil-guard and fall back to a sensible default
	// for test fixtures.
	Clock clock.Clock
	// Ambience is the M15.4b₂b per-room weather-ambience source.
	// Implementations return the current state's "ongoing" message
	// for the passed room, or "" when the room is not weather-
	// eligible or no ongoing line is configured. RenderRoom
	// invokes it on every look; nil leaves the room render
	// weather-free (test paths, link-dead recovery before the
	// service is wired). Spec world-rooms-movement §6.6.
	Ambience func(*world.Room) string

	// NowTick returns the current game tick, used by the loot verb to
	// evaluate a corpse's ownership window against its creation tick
	// (loot-and-corpses §4). nil degrades the window check to "open"
	// (tests + headless paths). Wired to tick.Loop.TickCount.
	NowTick func() uint64
	// CorpseOwnershipWindow is how many ticks a corpse stays reserved
	// for its owner set after creation (loot-and-corpses §4/§9). Zero
	// means no reservation (open immediately).
	CorpseOwnershipWindow uint64
}

// TellResolver maps a player name to a recipient route. Returns
// online actors when the recipient has a live session, or an
// offline route (entity id + canonical name) when only a save file
// exists. Implementations must match names case-insensitively
// (spec chat-channels-and-tells §10: exact match, case-insensitive).
type TellResolver interface {
	// ResolveOnline returns the live actor named `name` if one is
	// currently logged in.
	ResolveOnline(name string) (Actor, bool)
	// ResolveOffline returns (entityID, canonicalName, true) when a
	// player save by that name exists but no session is live. The
	// canonicalName is what notifications.Manager uses to find the
	// recipient's notifications.yaml.
	ResolveOffline(ctx context.Context, name string) (string, string, bool)
}

// DispositionHook is the seam movement and login flows call when a
// player arrives in a new room. The immediate variant runs in
// aggro-only mode before the room description renders; the deferred
// variant runs full evaluation after the description so non-hostile
// reactions don't appear above it (spec §4 + §5.4).
//
// Method signatures take primitives (string, []string) instead of an
// ai-package type so the command package doesn't import ai. The
// production implementation is an adapter that constructs an
// ai.PlayerView internally.
type DispositionHook interface {
	OnPlayerEnteredImmediate(ctx context.Context, playerID, playerName string, tags []string, room world.RoomID)
	OnPlayerEnteredDeferred(ctx context.Context, playerID, playerName string, tags []string, room world.RoomID)
}

// Context carries the per-invocation arguments passed to a Handler.
type Context struct {
	Actor       Actor
	World       *world.World
	Broadcaster Broadcaster                 // may be nil in tests
	Items       *entities.Store             // may be nil in tests
	Placement   *entities.Placement         // may be nil in tests
	Contents    *entities.Contents          // may be nil in tests
	Slots       *slot.Registry              // may be nil in tests
	Bus         *eventbus.Bus               // may be nil in tests
	Properties  *property.Registry          // may be nil in tests (M19.4h set property)
	Rarity      *decoration.RarityRegistry  // may be nil in tests (M20 decorations)
	Essence     *decoration.EssenceRegistry // may be nil in tests (M20 decorations)
	Stacking    *stacking.Service           // may be nil in tests (M21 stacking)
	Locator     Locator                     // may be nil in tests
	Roster      Roster                      // may be nil in tests (who verb)
	BadInput    *BadInputTracker            // may be nil in tests (§6 tracker)
	Disposition DispositionHook             // may be nil in tests
	Combat      *combat.Manager             // may be nil in tests
	// Flee is the M7.6 verb-driven §5.2 flee primitive closure. nil
	// in tests that don't exercise the flee verb.
	Flee func(ctx context.Context, c combat.CombatantID) combat.FleeOutcome
	// ReloadScripts is the M17.3 script hot-reload trigger (re-discover
	// pack Lua → swap the scripting runtime). nil disables the reload
	// verb; tests that don't exercise reload leave it unset.
	ReloadScripts func(ctx context.Context) (int, error)
	// Progression is the M8.2 XP/level service. nil in tests.
	Progression *progression.Manager
	// Training is the M8.6 training service. nil in tests.
	Training *progression.TrainingManager
	// Abilities / Proficiency / ActionQueue are the M9.6 ability-verb
	// seam. nil in tests that don't exercise ability verbs.
	Abilities   *progression.AbilityRegistry
	Proficiency *progression.ProficiencyManager
	ActionQueue *progression.ActionQueueManager
	// Help is the M10.5 help-topic service. nil in tests.
	Help *help.Service
	// Quests is the M10.7 quest service. nil in tests.
	Quests *quest.Service
	// Currency is the M11.1 economy currency service. nil in tests.
	Currency *economy.CurrencyService
	// Shop is the M11.2 shop service. nil in tests.
	Shop *economy.ShopService
	// Rest is the M11.4 rest service. nil in tests.
	Rest *economy.RestService
	// Consumable is the M11.5 consumable service. nil in tests.
	Consumable *economy.ConsumableService
	// Notifications is the M13.1 notification manager. tell/reply
	// publish through it. nil in tests that don't exercise tells.
	Notifications *notifications.Manager
	// TellResolver maps a player name to a recipient route. nil in
	// tests that don't exercise tells; tell verbs surface
	// "Tells are not enabled." when either this or Notifications is
	// nil.
	TellResolver TellResolver
	// RoleTargetResolver + GrantingRole are the M19.2 grant/revoke seam.
	// nil resolver disables role administration; GrantingRole defaults to
	// `admin` when empty (roles-and-permissions §4/§8).
	RoleTargetResolver RoleTargetResolver
	GrantingRole       string
	// Announcer is the all-sessions broadcast seam the `announce` admin
	// verb uses (admin-verbs §5). nil disables `announce`.
	Announcer Announcer
	// PlayerRoom resolves an online player's room by name world-wide for
	// the `teleport` verb's teleport-to-player form (admin-verbs §3). nil
	// disables that form.
	PlayerRoom PlayerRoomResolver
	// ChatRegistry / ChatSubscribers / ChatScrollbacks are the M13.6
	// channel seams. nil in tests that don't exercise chat verbs.
	ChatRegistry    *chat.Registry
	ChatSubscribers ChatSubscribers
	ChatScrollbacks ChatScrollbacks
	// Clock is the engine time source (foundation F3). nil in
	// tests that don't stamp timestamps.
	Clock clock.Clock
	// Ambience is the M15.4b₂b per-room weather-ambience source.
	// Mirrors Env.Ambience; copied from Env at dispatch time so
	// handlers reach for it as `c.Ambience` instead of having to
	// chase Env. nil-safe (RenderRoom skips when nil or when the
	// callback returns "").
	Ambience func(*world.Room) string
	// NowTick / CorpseOwnershipWindow are the M22.3 loot-window seam
	// (loot-and-corpses §4). Copied from Env at dispatch. NowTick nil →
	// the loot verb treats every corpse as open.
	NowTick               func() uint64
	CorpseOwnershipWindow uint64
	Raw                   string   // raw input line, trimmed
	Verb                  string   // resolved verb (lowercase)
	Args                  []string // tokens after the verb (space-split)

	// Resolved holds the §5 typed-argument values, keyed by each
	// declared ArgDefinition.Name, for commands that declared Args.
	// Dispatch populates it after a successful resolve and before
	// calling the handler; it is nil for handlers not yet migrated
	// onto the arg-typing pipeline. Migrated handlers read their
	// arguments from here (type-asserting to ItemRef / EntityRef /
	// DoorRef / string / int / the bulk []ItemRef as declared)
	// instead of hand-parsing Args.
	Resolved map[string]any

	// ArgResolver is the dispatcher's §5 resolver registry, exposed
	// to handlers that resolve arguments as a SERVICE rather than via
	// declared Args (Option B). The self-referencing combat verbs use
	// it: they run their own self-check first (the entity arg excludes
	// self), then resolve through the shared findCombatantInRoom
	// helper. Dispatch always sets it; helpers fall back to a fresh
	// registry when a test builds a Context directly.
	ArgResolver *ArgResolverRegistry

	// registry back-references the dispatching command Registry so the
	// `complete` debug verb (tab-completion §9) can run the completion
	// query, which needs the verb set. Set by Dispatch; nil for Contexts
	// built directly in tests (the query is then unavailable — tests
	// exercise it via (*Registry).Complete directly).
	registry *Registry
}

// Publish is the nil-safe shortcut every handler should use to emit
// an event. Centralizing the nil-guard once means a future handler
// that forgets `if c.Bus != nil` cannot silently introduce a
// nil-deref when called from a test fixture with a zero-value Env.
func (c *Context) Publish(ctx context.Context, e eventbus.Event) {
	if c.Bus == nil {
		return
	}
	c.Bus.Publish(ctx, e)
}

// Handler is the function invoked for a matched command.
type Handler func(ctx context.Context, c *Context) error

// defaultCommandCategory is the bucket a command's generated help topic
// lands in when its registration leaves Category unset (spec
// commands-and-dispatch §8).
const defaultCommandCategory = "commands"

// Command is a full command registration (spec commands-and-dispatch
// §2.1). Beyond the keyword + handler that Register takes, it carries the
// optional metadata listing and help-generation UIs need: aliases that
// route to the same handler, a category, a one-line brief, and synthesized
// syntax lines. A registration that supplies any of this metadata becomes
// discoverable via Commands() (and thus help generation); a bare
// keyword+handler does not.
type Command struct {
	Keyword  string
	Aliases  []string
	Category string // defaults to "commands" when any metadata is present
	Brief    string
	Syntax   []string
	Keywords []string
	Handler  Handler

	// Admin marks the command as administrative (admin-verbs §2): the
	// dispatcher refuses it — with the SAME "Huh?" an unknown verb produces,
	// so the verb is not enumerable — unless the actor holds the configured
	// admin role (Env.AdminRole). Admin commands are also hidden from help
	// for non-admins. The check runs once, at dispatch, before the handler.
	Admin bool

	// Args declares the command's typed arguments (commands-and-
	// dispatch §5). When non-empty, Dispatch resolves them against the
	// actor's scope BEFORE calling the handler (Option A): on success
	// the resolved values land in Context.Resolved keyed by each
	// ArgDefinition.Name; on a resolution failure the dispatcher writes
	// the error to the actor and the handler never runs. When empty,
	// Dispatch skips resolution entirely and the handler reads the raw
	// Context.Args tokens as before — this is what lets handlers
	// migrate onto the pipeline one at a time.
	Args []ArgDefinition

	// HandParsed marks a command that declares Args for completion and
	// help synthesis (commands-and-dispatch §5/§8, tab-completion §4) but
	// parses them ITSELF in the handler — the dispatcher must NOT
	// auto-resolve them. Used by verbs whose argument scope can't be
	// expressed by the single-scope auto-resolve pipeline: `get` (item
	// scope flips on the `from` preposition) and `kill` (self-check must
	// run before resolving, and the entity arg excludes self). The
	// handler keeps reading raw Args; completion gets the type info for
	// free. When false (the default), declared Args are auto-resolved as
	// before.
	HandParsed bool
}

// CommandInfo is the read-only view of a registered command's metadata,
// returned by Commands() for listing and help-generation UIs. Slice fields
// are fresh copies — safe to mutate.
type CommandInfo struct {
	Keyword  string
	Aliases  []string
	Category string
	Brief    string
	Syntax   []string
	Keywords []string
	// Admin is true for an administrative command (admin-verbs §2). Help
	// listings hide these from actors who don't hold the admin role.
	Admin bool
	// Args is the command's typed-argument declaration (§5), surfaced so
	// the help generator can synthesize a syntax line from it (§8). Empty
	// for untyped commands.
	Args []ArgDefinition
}

// cmdMeta is the stored metadata for a primary command registration. It is
// non-nil only on the primary entry of a RegisterCommand call that carried
// metadata; bare Register and alias entries leave it nil so they're
// excluded from listings.
type cmdMeta struct {
	category string
	brief    string
	syntax   []string
	keywords []string
	aliases  []string
	admin    bool
	// args is the command's typed-argument declaration, retained so the
	// help generator can synthesize a syntax line from it (§8). Stored on
	// the primary registration only; aliases inherit via the primary.
	args []ArgDefinition
}

type registration struct {
	keyword string
	order   int
	handler Handler
	// alias marks an entry that routes to a primary's handler under an
	// alternate keyword; aliases never appear in Commands().
	alias bool
	// meta is non-nil only on a primary registration that carried
	// metadata. It is the source for Commands() / help generation.
	meta *cmdMeta
	// args is the command's declared typed-argument list (§5). Empty
	// for handlers not yet migrated onto the arg-typing pipeline;
	// Dispatch resolves it before the handler runs when non-empty (and
	// handParsed is false). Carried on aliases too so an alias resolves
	// (and completes) identically to its primary.
	args []ArgDefinition
	// handParsed suppresses auto-resolution of args at dispatch — the
	// handler parses them itself (see Command.HandParsed). Completion
	// still reads args. Carried on aliases alongside args.
	handParsed bool
	// admin gates the command on the admin role at dispatch (admin-verbs
	// §2). Carried on every registration (primary AND alias) so an alias
	// of an admin command is gated too.
	admin bool
}

// Registry holds the command keyword → handler bindings.
//
// All public methods are safe for concurrent use, but in M1 the
// expectation is "register at boot, read during play".
type Registry struct {
	mu      sync.RWMutex
	byKey   map[string]registration
	order   int
	ordered []string // keywords in registration order, for prefix scans

	// argResolvers is the §5 arg-typing resolver registry the
	// dispatcher consults for commands that declare Args. Seeded with
	// the engine-baseline resolvers; packs extend it via ArgResolvers.
	argResolvers *ArgResolverRegistry
}

// New returns an empty Registry seeded with the engine-baseline arg
// resolvers (keyword/text/number/inventory/…/door).
func New() *Registry {
	return &Registry{
		byKey:        make(map[string]registration),
		argResolvers: NewArgResolverRegistry(),
	}
}

// ArgResolvers exposes the dispatcher's arg-type resolver registry so
// the composition root can register pack-authored arg types (§5.3)
// before play begins. Never nil for a Registry built via New().
func (r *Registry) ArgResolvers() *ArgResolverRegistry {
	return r.argResolvers
}

// Register binds keyword to h with no listing metadata. Keywords are
// stored lowercased. Duplicate keywords return an error. Commands
// registered this way are routable but invisible to Commands() / help
// generation — use RegisterCommand to make a command discoverable.
func (r *Registry) Register(keyword string, h Handler) error {
	return r.RegisterCommand(Command{Keyword: keyword, Handler: h})
}

// RegisterCommand binds c.Keyword (and each alias) to c.Handler. Keywords
// and aliases are stored lowercased; an exact match on any of them resolves
// to the handler. If c carries any metadata (category, brief, syntax,
// keywords, or aliases) the primary keyword becomes discoverable via
// Commands(). A duplicate primary keyword or alias returns an error, and
// alias collisions are detected before any mutation so a partial command is
// never left registered.
func (r *Registry) RegisterCommand(c Command) error {
	if c.Keyword == "" {
		return errors.New("command.RegisterCommand: empty keyword")
	}
	if c.Handler == nil {
		return errors.New("command.RegisterCommand: nil handler")
	}

	var meta *cmdMeta
	if c.Category != "" || c.Brief != "" || len(c.Syntax) > 0 || len(c.Keywords) > 0 || len(c.Aliases) > 0 {
		cat := c.Category
		if cat == "" {
			cat = defaultCommandCategory
		}
		meta = &cmdMeta{
			category: cat,
			brief:    c.Brief,
			syntax:   append([]string(nil), c.Syntax...),
			keywords: append([]string(nil), c.Keywords...),
			aliases:  append([]string(nil), c.Aliases...),
			admin:    c.Admin,
			args:     append([]ArgDefinition(nil), c.Args...),
		}
	}

	k := strings.ToLower(c.Keyword)
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byKey[k]; exists {
		return fmt.Errorf("command.RegisterCommand: duplicate keyword %q", k)
	}
	// Pre-validate aliases so a mid-list collision can't leave the
	// primary registered without its aliases.
	lowered := make([]string, 0, len(c.Aliases))
	for _, a := range c.Aliases {
		la := strings.ToLower(a)
		if la == "" {
			return fmt.Errorf("command.RegisterCommand: empty alias for %q", k)
		}
		if la == k {
			return fmt.Errorf("command.RegisterCommand: alias equals keyword %q", k)
		}
		if _, exists := r.byKey[la]; exists {
			return fmt.Errorf("command.RegisterCommand: duplicate alias %q", la)
		}
		lowered = append(lowered, la)
	}

	r.order++
	r.byKey[k] = registration{
		keyword:    k,
		order:      r.order,
		handler:    c.Handler,
		meta:       meta,
		args:       append([]ArgDefinition(nil), c.Args...),
		handParsed: c.HandParsed,
		admin:      c.Admin,
	}
	r.ordered = append(r.ordered, k)
	for _, la := range lowered {
		r.order++
		// Aliases carry the primary's args + handParsed so dispatch
		// resolution and completion behave identically whether the player
		// typed the primary keyword or an alias (e.g. `shut` == `close`,
		// `take` == `get`). Meta stays nil so aliases remain out of help
		// listings.
		r.byKey[la] = registration{
			keyword:    la,
			order:      r.order,
			handler:    c.Handler,
			alias:      true,
			args:       append([]ArgDefinition(nil), c.Args...),
			handParsed: c.HandParsed,
			admin:      c.Admin,
		}
		r.ordered = append(r.ordered, la)
	}
	return nil
}

// Commands returns the metadata for every discoverable primary command
// (those registered via RegisterCommand with metadata), sorted by keyword.
// Aliases and bare Register entries are excluded. Used by listing UIs and
// help generation.
func (r *Registry) Commands() []CommandInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []CommandInfo
	for _, k := range r.ordered {
		reg := r.byKey[k]
		if reg.alias || reg.meta == nil {
			continue
		}
		out = append(out, CommandInfo{
			Keyword:  reg.keyword,
			Aliases:  append([]string(nil), reg.meta.aliases...),
			Category: reg.meta.category,
			Brief:    reg.meta.brief,
			Syntax:   append([]string(nil), reg.meta.syntax...),
			Keywords: append([]string(nil), reg.meta.keywords...),
			Admin:    reg.meta.admin,
			Args:     append([]ArgDefinition(nil), reg.meta.args...),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Keyword < out[j].Keyword })
	return out
}

// Resolve returns the handler that the verb routes to, or nil if no
// match. Exact match wins; on no exact match, the keyword with the
// smallest registration-order index whose name has verb as a prefix
// wins (spec §2.3). Thin wrapper over resolveRegistration so the
// routing rule lives in one place.
func (r *Registry) Resolve(verb string) Handler {
	reg, ok := r.resolveRegistration(verb)
	if !ok {
		return nil
	}
	return reg.handler
}

// resolveRegistration is the §2.3 routing used by Dispatch: it returns
// the full matched registration (handler + declared Args), not just
// the handler, so the dispatcher can pre-resolve typed arguments.
// Resolution order matches Resolve exactly — exact match wins, else
// the lowest registration-order prefix match.
func (r *Registry) resolveRegistration(verb string) (registration, bool) {
	v := strings.ToLower(verb)
	if v == "" {
		return registration{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if reg, ok := r.byKey[v]; ok {
		return reg, true
	}
	var matches []registration
	for _, k := range r.ordered {
		if strings.HasPrefix(k, v) {
			matches = append(matches, r.byKey[k])
		}
	}
	if len(matches) == 0 {
		return registration{}, false
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].order < matches[j].order })
	return matches[0], true
}

// Dispatch parses a raw input line and routes it. Empty / whitespace
// input is a no-op (spec §3.1 step 1). Unknown verbs send "Huh?" to
// the actor and return nil (the bad-input tracker lands later).
//
// env carries the per-server singletons handlers may need (world,
// broadcaster, item store, placement). Any field may be nil; handlers
// MUST guard before dereferencing.
func (r *Registry) Dispatch(ctx context.Context, env Env, actor Actor, raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	fields := strings.Fields(trimmed)
	verb := fields[0]
	args := fields[1:]

	reg, ok := r.resolveRegistration(verb)
	if !ok {
		// Bad-input tracking (§6): record + log the unknown verb. This is the
		// player route only (mobs dispatch elsewhere), so the tracker never
		// sees a mob verb. The admin-gate "Huh?" below is a KNOWN verb being
		// refused and is deliberately not recorded here.
		env.BadInput.Record(verb)
		logging.From(ctx).Debug("unknown verb",
			slog.String("event", "command.unknown"),
			slog.String("verb", strings.ToLower(verb)),
			slog.String("raw", raw),
			slog.String("player", actor.Name()),
			slog.String("room_id", string(actorRoomID(actor))))
		return actor.Write(ctx, "Huh?")
	}

	// Admin gate (admin-verbs §2): an admin-marked command is refused —
	// with the IDENTICAL "Huh?" an unknown verb produces, so a non-admin
	// cannot tell the verb exists — unless the actor holds the admin role.
	// Checked once here, before the Context is built and the handler runs.
	if reg.admin {
		adminRole := env.AdminRole
		if adminRole == "" {
			adminRole = defaultAdminRole
		}
		holder, ok := actor.(RoleHolder)
		if !ok || !holder.HasRole(adminRole) {
			return actor.Write(ctx, "Huh?")
		}
	}

	c := &Context{
		Actor:                 actor,
		World:                 env.World,
		Broadcaster:           env.Broadcaster,
		Items:                 env.Items,
		Placement:             env.Placement,
		Contents:              env.Contents,
		Slots:                 env.Slots,
		Bus:                   env.Bus,
		Properties:            env.Properties,
		Rarity:                env.Rarity,
		Essence:               env.Essence,
		Stacking:              env.Stacking,
		Locator:               env.Locator,
		Roster:                env.Roster,
		BadInput:              env.BadInput,
		Disposition:           env.Disposition,
		Combat:                env.Combat,
		Flee:                  env.Flee,
		ReloadScripts:         env.ReloadScripts,
		Progression:           env.Progression,
		Training:              env.Training,
		Abilities:             env.Abilities,
		Proficiency:           env.Proficiency,
		ActionQueue:           env.ActionQueue,
		Help:                  env.Help,
		Quests:                env.Quests,
		Currency:              env.Currency,
		Shop:                  env.Shop,
		Rest:                  env.Rest,
		Consumable:            env.Consumable,
		Notifications:         env.Notifications,
		TellResolver:          env.TellResolver,
		RoleTargetResolver:    env.RoleTargetResolver,
		GrantingRole:          env.GrantingRole,
		Announcer:             env.Announcer,
		PlayerRoom:            env.PlayerRoom,
		ChatRegistry:          env.ChatRegistry,
		ChatSubscribers:       env.ChatSubscribers,
		ChatScrollbacks:       env.ChatScrollbacks,
		Clock:                 env.Clock,
		Ambience:              env.Ambience,
		NowTick:               env.NowTick,
		CorpseOwnershipWindow: env.CorpseOwnershipWindow,
		Raw:                   trimmed,
		Verb:                  strings.ToLower(verb),
		Args:                  args,
		ArgResolver:           r.argResolvers,
		registry:              r,
	}

	// §5 arg-typing (Option A): when the command declares typed args,
	// resolve them against the actor's scope before the handler runs.
	// A resolution failure is terminal for this input — the dispatcher
	// writes the player-facing error and the handler never executes.
	// Commands with no declared Args — or HandParsed commands that
	// declare Args only for completion/help — skip this entirely and read
	// the raw c.Args tokens themselves (the incremental-migration path).
	if len(reg.args) > 0 && !reg.handParsed {
		resolved, warnings, _, err := r.argResolvers.ResolveArgsWithContext(
			reg.args, args, c.BuildResolveContext())
		for _, w := range warnings {
			logging.From(ctx).Debug("argres warning", "verb", c.Verb, "warning", w)
		}
		if err != nil {
			return actor.Write(ctx, err.Error())
		}
		c.Resolved = resolved
	}

	return reg.handler(ctx, c)
}
