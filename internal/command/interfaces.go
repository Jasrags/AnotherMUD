package command

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/biome"
	"github.com/Jasrags/AnotherMUD/internal/chat"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/crafting"
	"github.com/Jasrags/AnotherMUD/internal/decoration"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/gathering"
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/property"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stacking"
	"github.com/Jasrags/AnotherMUD/internal/stats"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

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
	// Equip atomically moves id from inventory to equipment, installing
	// it under every key in footprint (footprint[0] is the target /
	// canonical slot key; the rest are companion-slot keys for a spanning
	// item — inventory-equipment-items §3.4 step 8). Applies mods ONCE to
	// the holder's stat block under EquipmentSourceKey(id). Returns false
	// if id was not in inventory (TOCTOU loss to a concurrent drop) OR if
	// any footprint key is already occupied (the handler must displace
	// occupants first). Auto-swap and the cancellable veto are the
	// handler's responsibility: it resolves the footprint and displaces
	// occupants before calling Equip.
	Equip(footprint []string, id entities.EntityID, mods []stats.Modifier) bool
	// Unequip atomically removes the item occupying slotKey — freeing its
	// ENTIRE footprint (every key a spanning item holds), not just slotKey
	// — returns it to inventory, and reverses its stat modifiers. Returns
	// the removed entity id and true on success.
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

	// Recipes / Known are the crafting seam (crafting-and-cooking §3,
	// §7). The `learn` verb reads the recipe registry + the
	// per-character known-recipe manager. nil in tests that don't
	// exercise crafting.
	Recipes *recipe.Registry
	Known   *recipe.KnownManager
	// Craft is the crafting service (quality roll + atomic consume/produce).
	// The `craft` verb routes through it. nil in tests.
	Craft *crafting.Service
	// Gathering / Biomes / ForageTables are the gathering seam (gathering.md
	// §2): the `forage` verb resolves the room's biome → forage table and
	// rolls it. nil in tests that don't exercise gathering.
	Gathering    *gathering.Service
	Biomes       *biome.Registry
	ForageTables *gathering.ForageRegistry

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

	// DefaultXPTrack is the engine's primary progression track — the
	// content-specific track name (e.g. "adventurer") bound at the
	// composition root. The admin `xp` verb grants on it when no track is
	// given, and it is the single source quest rewards also bind to (so
	// the two paths can't drift). Empty falls back to "adventurer" in the
	// handler for test Contexts built directly.
	DefaultXPTrack string

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

	// WeatherState returns the current weather state for an area (e.g.
	// "clear"/"rain"). The build verb reads it to refuse a campfire in wet
	// weather (crafting-and-cooking §4). Wired to weather.Service.
	// CurrentWeather; nil → no weather gate.
	WeatherState func(world.AreaID) string

	// Light is the light-and-darkness resolver (light §2), copied to
	// each Context at dispatch. nil disables light gating.
	Light *light.Resolver

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
	// Hostile is a read-only query (no event, no cache write) reporting
	// whether mob m would react hostilely toward the given player — used
	// to redden hostile mobs in the room render. The production adapter
	// constructs an ai.PlayerView and calls Evaluator.ReactionFor.
	Hostile(m *entities.MobInstance, playerID, playerName string, tags []string) bool
}
