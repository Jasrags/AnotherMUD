// Package session bridges the connection layer (internal/conn) to the
// command dispatcher (internal/command), the world (internal/world), and
// the M3 login + persistence layer.
//
// In M3 a session is still one connection ↔ one character, but logged-
// in characters are tracked in a Manager so autosave and shutdown can
// iterate them. The connection/session split proper lands in M4 per
// docs/specs/session-lifecycle.md.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/action"
	"github.com/Jasrags/AnotherMUD/internal/ansi"
	"github.com/Jasrags/AnotherMUD/internal/auction"
	"github.com/Jasrags/AnotherMUD/internal/biome"
	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/chat"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/crafting"
	"github.com/Jasrags/AnotherMUD/internal/decoration"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/faction"
	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/gathering"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/grade"
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/login"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/pool"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/property"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/queststore"
	"github.com/Jasrags/AnotherMUD/internal/rangedflavor"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/render"
	"github.com/Jasrags/AnotherMUD/internal/reputation"
	"github.com/Jasrags/AnotherMUD/internal/size"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/srckey"
	"github.com/Jasrags/AnotherMUD/internal/stacking"
	"github.com/Jasrags/AnotherMUD/internal/stats"
	"github.com/Jasrags/AnotherMUD/internal/trade"
	"github.com/Jasrags/AnotherMUD/internal/wizard"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Config is the per-server wiring the session loop needs.
type Config struct {
	World    *world.World
	Commands *command.Registry
	Players  *player.Store
	Login    login.Config

	// Items is the runtime entity store. Item instantiation, get/drop,
	// and inventory restoration all go through it. May be nil only in
	// tests that don't exercise inventory.
	Items *entities.Store
	// Placement is the room↔item index. Used by the session layer to
	// spawn inventory items into the world without a room and by
	// get/drop handlers to move items between rooms and contents.
	Placement *entities.Placement
	// Contents is the container↔item index (M5.9b). Used by
	// respawnInventory to restore container nesting and by the put
	// handler to move items between actor inventory and containers.
	// May be nil only in tests that don't exercise containers.
	Contents *entities.Contents
	// Templates is the item-template registry used at login time to
	// respawn persisted inventory entries into fresh instances.
	Templates *item.Templates
	// Slots is the equipment-slot registry. Required by the equip
	// command handler to validate slot names and look up capacities.
	// May be nil in tests that don't exercise equipment.
	Slots *slot.Registry
	// Bus is the engine event bus, passed through command.Env to
	// inventory/equipment handlers so they can publish observable
	// events after successful mutations. May be nil in tests.
	Bus *eventbus.Bus
	// Properties is the engine property registry (M14), passed through
	// command.Env so the admin `set property` handler (M19.4h) can
	// validate + type-coerce a write. May be nil in tests.
	Properties *property.Registry
	// Rarity / Essence are the M20 item-decoration registries, passed
	// through command.Env so item display can resolve an item's
	// rarity/essence key to its decoration markup. May be nil in tests.
	Rarity  *decoration.RarityRegistry
	Essence *decoration.EssenceRegistry
	// Stacking is the M21 inventory stack-grouping service, passed through
	// command.Env so item listings group identical items. May be nil.
	Stacking *stacking.Service

	// Disposition is the room-entry hook surface (spec
	// mobs-ai-spawning §4). Passed through command.Env and called
	// directly from initial-login + link-dead-reconnect render
	// paths. May be nil in tests that don't exercise disposition.
	Disposition command.DispositionHook

	// Combat is the engage/disengage manager (spec combat §2),
	// passed through to command.Env so the kill verb (and future
	// flee / wimpy) can reach it. May be nil in tests that don't
	// exercise combat verbs.
	Combat *combat.Manager

	// CombatLocator resolves a CombatantID back to a live
	// Combatant — needed by the M16.4d Char.Combat flusher to
	// look up the target's name + Vitals snapshot. Production
	// wires the same combatLocator the combat package uses
	// (composition root in cmd/anothermud). nil-safe: when
	// unset, FlushGmcpCombat skips the target-resolved fields
	// and only ships the in_combat flag.
	CombatLocator combat.Locator

	// Flee is the verb-driven flee primitive (M7.6). The function
	// closure captures the production FleeConfig built at the
	// composition root; command.Context.Flee receives the same shape.
	// nil in tests that don't exercise the flee verb.
	Flee func(ctx context.Context, c combat.CombatantID) combat.FleeOutcome

	// ResolveAttack is the ranged-combat §3 one-shot attack primitive used by
	// the throw verb; passed through to command.Env. nil in tests that don't
	// exercise throw.
	ResolveAttack func(ctx context.Context, attacker, target combat.CombatantID, room world.RoomID) bool

	// ReloadScripts is the M17.3 script hot-reload trigger, passed
	// through to command.Env so the `reload` verb can re-discover pack
	// Lua and swap the scripting runtime. nil disables the verb.
	ReloadScripts func(ctx context.Context) (int, error)

	// Progression is the M8.2 XP/level service. nil in tests that
	// don't exercise progression; in production the composition root
	// builds it from the pack-loaded track registry and a bus-backed
	// EventSink.
	Progression *progression.Manager

	// Training is the M8.6 training service (spec progression.md
	// §7). The composition root builds it against the race
	// registry + a TrainerSource that scans the entity store for
	// `skill_trainer`-tagged mobs in the actor's room. nil-safe;
	// the `train` and `practice` verbs print "training not
	// enabled" when nil.
	Training *progression.TrainingManager

	// Races is the M8.3 race registry. Consulted at login to
	// resolve the player's race id into RacialFlags + starting
	// alignment. nil-safe: a missing registry means racial flags
	// are never applied to players (the engine falls back to
	// "racially blank" — equivalent to a player with no
	// declared race).
	Races *progression.RaceRegistry

	// Alignment is the M8.5 alignment manager. Consulted at
	// login to seed initial alignment for fresh characters
	// (race + class StartingAlignment summed) and to sync the
	// bucket tag on every login. nil-safe: a missing manager
	// leaves alignment at the persisted value with no tag
	// (disposition rules that match on alignment won't fire).
	Alignment *progression.AlignmentManager

	// Faction is the S8 faction/standing manager (faction.md). Consulted at
	// login to re-sync each touched faction's rank tag from the restored
	// standing bag, and held by gameplay paths (on-kill, quests, the standing
	// command) that Shift standing. nil-safe: a missing manager leaves
	// standing at the persisted value with no rank tags (disposition rules
	// that match on faction won't fire).
	Faction *faction.Manager

	// Reputation is the single-axis renown manager (reputation.md). Consulted at
	// login to re-sync the tier tag from the restored score, and held by the
	// earn paths (R3+) that Shift renown. nil-safe: a missing manager leaves the
	// score at the persisted value with no tier tag.
	Reputation *reputation.Manager

	// Classes is the M8.4 class registry. Consulted at login so
	// the actor's classID is validated against loaded content, and
	// passed through to ClassPathProcessor + ApplyStatGrowth on
	// level-up subscriptions wired in cmd/anothermud. nil-safe:
	// missing registry means class-side effects never fire.
	Classes *progression.ClassRegistry

	// Feats is the EPIC S4 feat registry. Held so the actor can resolve its
	// known_feats into conferred bonuses (Phase 3 — the saves consumer reads
	// it in connActor.Saves). nil-safe: a missing registry means feats confer
	// nothing.
	Feats *feat.Registry

	// Languages is the tongue registry (languages.md §2). Held so the actor
	// resolves its known-language ids into display names for `score` + the
	// `languages` listing. nil-safe: a missing registry renders ids verbatim.
	Languages *progression.LanguageRegistry

	// Backgrounds is the creation-origin registry (backgrounds §2). Held so the
	// actor can derive its background's weapon restrictions at login (the equip
	// gate — backgrounds.md §Restrictions). nil-safe: a missing registry means
	// no restriction is enforced.
	Backgrounds *progression.BackgroundRegistry

	// AttributeSets is the content-declared base attribute-set registry (SR-M1 —
	// shadowrun-mvp.md Appendix A), and WorldAttributeSets maps a world
	// namespace → the set id it selects. Held so the actor constructor seeds a
	// brand-new/returning character from ITS WORLD'S attribute set instead of
	// the fixed six — the fix for the "carries both sets" merge bug. nil-safe:
	// a missing registry falls back to progression.DefaultPlayerBase.
	AttributeSets      *progression.AttributeSetRegistry
	WorldAttributeSets map[string]string

	// Pools is the content-declared resource-pool registry (shadowrun-mvp
	// SR-M3a). Held so the actor constructor seeds a character's pool.Set from
	// the active world's player-seed pool decls (mana/movement in core, plus a
	// world's own monitors) instead of the hardcoded pair. nil-safe: a missing
	// registry falls back to the hardcoded mana/movement seed (seedResourcePools).
	Pools *pool.Registry

	// Proficiency is the M9.1 per-entity ability proficiency
	// manager (spec abilities-and-effects §3). The session-load
	// path restores the actor's persisted Abilities snapshot into
	// it; logout drops the in-memory state. nil-safe: when nil,
	// abilities are neither restored nor persisted (the actor
	// behaves as if they have learned nothing).
	Proficiency *progression.ProficiencyManager

	// Abilities is the M9.1 ability registry. Consulted by the
	// proficiency manager for DefaultCap and ability display
	// names; passed through the session config so future verbs
	// (M9.3+ abilities verb, M9.6 learn/forget admin) can read it
	// without re-plumbing. nil-safe.
	Abilities *progression.AbilityRegistry

	// Recipes is the crafting-recipe registry (crafting-and-cooking
	// §3). Consulted by the crafting/learn paths; passed through so
	// command handlers can read it without re-plumbing. nil-safe.
	Recipes *recipe.Registry

	// Known is the per-character known-recipe manager
	// (crafting-and-cooking §7, §9). The session-load path restores the
	// actor's persisted KnownRecipes list into it; logout drops the
	// in-memory state; Persist snapshots it back. nil-safe: when nil,
	// recipes are neither restored nor persisted (mirrors Proficiency).
	Known *recipe.KnownManager

	// Craft is the crafting service (quality roll + atomic consume/produce,
	// crafting-and-cooking §3, §5). The `craft` verb routes through it via
	// commandEnv. nil-safe.
	Craft *crafting.Service

	// Gathering / Biomes / ForageTables are the gathering seam
	// (gathering.md §2): the `forage` verb resolves the room's biome →
	// forage table and rolls it. Threaded into command.Env via commandEnv.
	// nil-safe.
	Gathering    *gathering.Service
	Biomes       *biome.Registry
	ForageTables *gathering.ForageRegistry
	// Grades is the masterwork quality-grade registry (masterwork §3),
	// threaded into command.Env via commandEnv. nil-safe.
	Grades *grade.Registry

	// Effects is the M9.2 active-effect manager (spec
	// abilities-and-effects §5). Resolves targets via a closure
	// over the session Manager so the package boundary stays
	// clean. The session-load path does no per-actor restore
	// today — active-effect state is ephemeral by spec §5.5 —
	// but logout calls Effects.Drop to release in-memory state.
	// nil-safe: when nil, the EffectTarget surface on connActor
	// is never reached.
	Effects *progression.EffectManager

	// EffectTemplates resolves an effect template by id for the afflict
	// condition verb (conditions §5). *effect.Registry satisfies the
	// command.EffectTemplateSource interface. nil disables afflict.
	EffectTemplates command.EffectTemplateSource

	// SkillRoller is the d20 source for skill checks (skills §3 — the `pick`
	// verb). Must be concurrency-safe (used off the command goroutine).
	SkillRoller progression.Roller
	// RecognitionDifficulty is the look-at-player renown recognition difficulty
	// (reputation.md §6), passed through to the command Env.
	RecognitionDifficulty int

	// ActionQueue is the M9.3 per-entity action queue (spec
	// abilities-and-effects §4.1). The M9.4 ability phase pops from
	// it each pulse; logout calls Drop to release queued entries.
	// nil-safe: when nil, logout skips the queue drop. No enqueue
	// path exists until the M9.6 verb surface, so today this stays
	// empty in production.
	ActionQueue *progression.ActionQueueManager

	// PulseDelay is the M9.3 per-entity ability-cooldown tracker
	// (spec §4.5 step 3). The resolver records cooldowns into it;
	// logout calls Drop. nil-safe like ActionQueue.
	PulseDelay *progression.PulseDelayTracker

	// Casts is the WoT S2 per-entity in-flight timed-cast tracker (the
	// channel interrupt game). The ability phase records a weave's warmup
	// into it; logout calls Drop so a half-woven weave does not survive a
	// reconnect (it is simply lost, like a mid-pulse cancellation). nil-safe
	// like ActionQueue — non-channeling boots leave it nil.
	Casts *progression.CastTracker

	// Actions is the per-actor timed-action / busy-state tracker
	// (action-economy.md). The dispatcher gates IsAction commands on it and the
	// don/doff path begins occupations on it; the action-complete tick sweep
	// (CompleteReadyActions) finishes due actions by replaying their command.
	// Logout calls Drop. nil disables timed actions (every action is instant).
	Actions *action.Tracker
	// DonTicks is the occupation length (engine ticks) for donning/doffing slow
	// armor (action-economy.md §7.2). 0 → the command-package default. Sourced
	// from ANOTHERMUD_DON_TICKS.
	DonTicks int

	// DefaultRace is the race id assigned to legacy saves with no
	// `race` field, and to fresh characters that haven't been
	// through a M12 character-creation flow yet. Empty means the
	// engine does not seed a default — players retain their
	// (empty) saved race and no flags apply. Production sets this
	// to "human" via ANOTHERMUD_DEFAULT_RACE.
	DefaultRace string

	// RoleSeed is the operator-configured role bootstrap
	// (roles-and-permissions §5): a map from lowercased character name to
	// the role names that character is granted at load. An out-of-band
	// privilege source that breaks the grant chicken-and-egg so an
	// operator can name a known character as admin. Applied additively and
	// idempotently by applyRoles (re-ensured every login); nil disables
	// seeding. Not content — a pack cannot populate it. (The session also
	// auto-grants admin to the first account's first character — see the
	// bootstrap block by applyRoles — so a fresh deployment has a working
	// admin even without RoleSeed configured.)
	RoleSeed map[string][]string

	// StartID is the fallback starting room when a character's saved
	// location is not present in the loaded world (e.g. a room was
	// removed from content between restarts).
	StartID world.RoomID

	// ColorEnabled is the per-session default for ANSI color output.
	ColorEnabled bool

	// Render is the M10.2 themed color renderer (internal/render),
	// built and compiled once at boot against the pack-loaded theme.
	// connActor.Write runs every outbound line through it. Shared
	// read-only across all sessions. nil-safe: when nil, Write falls
	// back to the minimal M2 ansi.Render so tests need not wire it.
	Render *render.ColorRenderer

	// Help is the M10.5 help-topic service, passed through command.Env
	// so the help verb can query it. nil-safe: the verb reports
	// "help not available" when nil.
	Help *help.Service

	// Quests is the M10.7 quest service. The login path loads the
	// player's persisted state into it; teardown drops the in-memory
	// state. nil-safe (tests that don't exercise quests).
	Quests *quest.Service
	// QuestStore is the M10.8 quest persistence store. Login calls Load
	// (which also caches the name for Save's path); teardown calls
	// Forget. Deviation from spec §6.3: load is a direct synchronous
	// call here rather than a bus event, consistent with how Effects /
	// Proficiency are wired and because the load must complete before
	// the player issues commands (the spec's §11 flags the event-driven
	// load as racy). nil-safe.
	QuestStore *queststore.Store

	// Notifications is the M13.1 per-entity notification manager
	// (spec notifications.md). Bound at the post-Add moment so the
	// active-phase drain delivers any tells / channel posts that
	// arrived while the player was offline. Unregister fires from
	// every "session gone" path (fullTeardown, linkdead reap,
	// takeover) so the queue persists and in-memory state
	// releases. nil-safe.
	Notifications *notifications.Manager

	// TellResolver is the M13.5 player-name resolver consumed by
	// the tell / reply verbs. Wired by the composition root from
	// session.Manager (online) + player.Store (offline). nil-safe;
	// when nil the tell verbs print "Tells are not enabled."
	TellResolver command.TellResolver

	// RoleTargets resolves an online player name to a RoleController for
	// the M19.2 grant/revoke verbs (roles-and-permissions §4). GrantingRole
	// is the role an actor must hold to grant/revoke (§8, defaults to
	// `admin` when empty). nil RoleTargets disables role administration.
	RoleTargets  command.RoleTargetResolver
	GrantingRole string
	// AdminRole is the role an admin-marked command requires at dispatch
	// (M19.3 — admin-verbs §2/§8). Defaults to `admin` when empty.
	AdminRole string
	// DefaultXPTrack is the engine's primary progression track, threaded
	// to command.Env so the `xp` verb and quest rewards share one source
	// (the content track, e.g. "adventurer"). Empty falls back to
	// command.DefaultXPTrack in the handler.
	DefaultXPTrack string

	// ChannelMap is the active ruleset's stat→combat-channel derivation
	// (the channel layer). Threaded to connActor.Stats() so HitMod/AC
	// derive through it. nil ⇒ direct stat reads (the baseline behavior).
	// NOTE: unrelated to ChatRegistry below — "channel" here is a derived
	// combat input, not a chat channel.
	ChannelMap *channel.Mapping

	// UnarmedSubdual makes an unarmed player's strikes nonlethal — fists knock a
	// foe OUT instead of killing (subdual-damage §6). The composition root sets it
	// from ANOTHERMUD_UNARMED_SUBDUAL (default true — the faithful d20/WoT
	// behavior). A wielded weapon ignores this (its own `subdual` flag decides);
	// mob natural weapons are unaffected (this is a player-unarmed concern).
	UnarmedSubdual bool

	// ChatRegistry is the M13.6 channel catalog. Threaded through
	// to command.Env for chat verbs. nil-safe.
	ChatRegistry *chat.Registry

	// ChatSubscribers returns the online-subscriber set for a
	// channel. v1: every online player is subscribed.
	ChatSubscribers command.ChatSubscribers

	// ChatScrollbacks returns the per-channel ring buffer for the
	// chat history verb and the publish-time append.
	ChatScrollbacks command.ChatScrollbacks

	// Currency is the M11.1 economy currency service (spec
	// economy-survival §2). Passed through command.Env so the `gold`
	// verb and the get/give auto-convert hook can credit/read the
	// actor's balance. nil-safe: the gold verb reports zero and
	// auto-convert is a no-op (currency items just enter inventory).
	Currency *economy.CurrencyService

	// Mounts is the mount lifecycle service (mounts.md §2/§3). Passed
	// through command.Env so the buymount/stable/unstable verbs can
	// materialize and dematerialize owned mounts. nil-safe: the verbs
	// report "no stable here" when unwired.
	Mounts command.MountService

	// Hirelings is the hireling lifecycle service (hireable-mobs.md §2/§3).
	// Passed through command.Env so the hire/dismiss verbs can materialize and
	// dematerialize owned hirelings. HirelingCap is the simultaneous cap (§3.3).
	Hirelings   command.HirelingService
	HirelingCap int

	// Spawn is the admin builder-spawn service (command SpawnService). Passed
	// through command.Env so the `spawn` verb can mint items/mobs into the world.
	Spawn command.SpawnService

	// RangedFlavor resolves ranged-weapon moment text by ranged_style
	// (rangedflavor). Passed through command.Env for the shoot/load verbs.
	RangedFlavor *rangedflavor.Registry

	// Trades is the direct-trade session manager (direct-trade.md).
	// Passed through command.Env so the trade/offer/confirm/decline verbs
	// route through it, and used by the teardown hooks (disconnect /
	// link-death / room change) to cancel an actor's open trade. nil-safe:
	// the verbs report trading is unavailable when unwired.
	Trades *trade.Manager

	// Auction is the auction-house manager (auction-house.md). Passed
	// through command.Env so the auction/browse/buyout/collect verbs route
	// through it. nil-safe: the verbs report the auction house is unavailable
	// when unwired. No teardown hook — listings persist independent of a
	// session.
	Auction *auction.Manager

	// Shop is the M11.2 shop service (spec §3). Passed through
	// command.Env so the buy/sell/value/list verbs can reach it.
	// nil-safe: the verbs report "no shop here" when unwired.
	Shop *economy.ShopService

	// Sustenance is the M11.3 sustenance service (spec §4). Used at
	// login to seed a fresh character's pool to full and by the
	// sustenance-drain world-tick subscriber (via Manager.DrainSustenance).
	// nil-safe: no seed and no drain when unwired (the test default).
	Sustenance *economy.SustenanceService

	// Rest is the M11.4 rest service (spec §5). Passed through
	// command.Env so the rest/sleep/wake verbs can drive transitions.
	// The combat-wake subscriber lives at the composition root, not
	// here. nil-safe: the verbs report they can't be used when unwired.
	Rest *economy.RestService

	// Consumable is the M11.5 consumable service (spec §6). Passed
	// through command.Env so the eat/drink/use verbs can consume items.
	// nil-safe: the verbs report they can't be used when unwired.
	Consumable *economy.ConsumableService

	// Ambience is the M15.4b₂b per-room weather-ambience source —
	// the closure built at composition over weather.Service.Ambience.
	// Threaded into command.Env so handlers reach RenderRoom with it,
	// AND consumed directly by the login/link-dead renderers that
	// build their own room renders outside the command dispatcher.
	// nil-safe: a nil callback leaves the room render weather-free.
	Ambience func(*world.Room) string

	// WeatherState returns an area's current weather state — the build
	// verb's wet-weather gate (crafting-and-cooking §4). Threaded into
	// command.Env. nil-safe (no weather gate).
	WeatherState func(world.AreaID) string

	// Light is the light-and-darkness resolver (light §2), threaded
	// into command.Env (and, in Phase 5, the login/link-dead renderers).
	// nil disables light gating — the world renders as if always lit.
	Light *light.Resolver

	// CreationFlow is the M12.3 interactive character-creation wizard
	// (spec character-creation §2/§3). When set, a new player runs it
	// after login to choose race/class before commit; nil takes the §2
	// "no flow → immediate commit" path (the M12.2 behavior). Built from
	// the race/class registries via NewCreationFlow at the composition
	// root.
	CreationFlow *wizard.Flow

	// Manager tracks logged-in sessions for autosave + shutdown sweeps.
	// Required.
	Manager *Manager

	// Clock is the time source for time-dependent session machinery
	// (flood protection refill, idle timeouts). Defaults to
	// clock.RealClock when nil so existing tests don't have to wire it.
	Clock clock.Clock

	// ChainCap bounds how many commands one input line may expand to via
	// chaining/repeat (commands-and-dispatch §4.1). Zero falls back to
	// command.DefaultChainCap inside ParseInput.
	ChainCap int

	// BadInput is the unknown-verb tracker (commands-and-dispatch §6).
	// Threaded into command.Env so the dispatcher records misfires and the
	// `badinput` admin verb can read them. nil disables tracking.
	BadInput *command.BadInputTracker

	// NowTick returns the current game tick; threaded into command.Env
	// so the loot verb can evaluate a corpse's ownership window
	// (loot-and-corpses §4). Wired to tick.Loop.TickCount at the
	// composition root; nil degrades the window check to "open".
	NowTick func() uint64
	// CorpseOwnershipWindow is how many ticks a corpse stays reserved
	// for its killer after creation (loot-and-corpses §4/§9). Zero =
	// no reservation.
	CorpseOwnershipWindow uint64

	// DefaultMoveCost is the flat movement-point cost of a step when the
	// destination biome configures none (world-rooms-movement §3.3).
	// Sourced from ANOTHERMUD_MOVE_COST (default 1).
	DefaultMoveCost int

	// Flood is the per-session rate-limit policy. Zero value disables
	// flood protection (the test default). Production wires
	// DefaultFloodConfig.
	Flood FloodConfig

	// LinkDead is the link-dead survival policy (M4.4). When Enabled
	// is false, every disconnect immediately runs full teardown (the
	// M3 behavior). When true, an involuntary connection drop parks
	// the session for TimeoutSeconds so a returning login can
	// reattach. Zero value (Enabled=false) is the test default.
	LinkDead LinkDeadConfig
}

// Handler returns a server.Handler-compatible function that drives one
// connection through login and into the game loop.
func Handler(cfg Config) func(ctx context.Context, c conn.Connection) error {
	// Wire the timed-action sweep once, here where the in-package commandEnv is
	// reachable (action-economy.md §3). nil-safe; no-op when Actions is unset.
	if cfg.Manager != nil {
		cfg.Manager.enableActionSweep(cfg)
	}
	return func(ctx context.Context, c conn.Connection) error {
		return run(ctx, c, cfg)
	}
}

// resolveAttributeSet resolves the content-declared base attribute set a
// character's world seeds from + renders `score` from (SR-M1 — shadowrun-mvp.md
// Appendix A). It maps worldID → the set id the world selects (its manifest
// `attribute_set:`, default the engine `classic`) and returns the registered
// AttributeSet, or nil when the registry is absent or the resolved set is
// unregistered (a boot with no attribute content). Pure + nil-safe.
func resolveAttributeSet(sets *progression.AttributeSetRegistry, selection map[string]string, worldID string) *progression.AttributeSet {
	if sets == nil {
		return nil
	}
	setID := ""
	if selection != nil {
		setID = selection[worldID]
	}
	if setID == "" {
		setID = progression.ClassicAttributeSetID
	}
	set, _ := sets.Get(setID)
	return set
}

// seedBaseFromSetOrDefault builds the character's base attribute seed from a
// resolved attribute set (attribute defaults + engine-vital keys), or falls
// back to progression.DefaultPlayerBase when the set is nil (a boot with no
// attribute content still seeds the classic six).
func seedBaseFromSetOrDefault(set *progression.AttributeSet) map[progression.StatType]int {
	if set != nil {
		return progression.SeedBaseFromSet(set)
	}
	return progression.DefaultPlayerBase()
}

// seedBaseFor resolves a world's attribute set and builds its base seed.
//
// This is the fix for the "carries both sets" merge bug: because RestoreBase
// MERGES the persisted snapshot over the constructor seed, seeding a foreign
// world's keys would leave them behind. Seeding the character's OWN world set
// means the persisted keys overlay the same keys — no leftovers.
func seedBaseFor(sets *progression.AttributeSetRegistry, selection map[string]string, worldID string) map[progression.StatType]int {
	return seedBaseFromSetOrDefault(resolveAttributeSet(sets, selection, worldID))
}

func run(ctx context.Context, c conn.Connection, cfg Config) error {
	loaded, err := login.Run(ctx, c, cfg.Login)
	if err != nil {
		if errors.Is(err, login.ErrAborted) || errors.Is(err, login.ErrQuit) {
			return nil
		}
		// ErrPasswordCap / ErrEmailCap are terminal but not actionable
		// for the server — log + close.
		logging.From(ctx).Info("login ended", slog.Any("err", err))
		return nil
	}

	ctx = logging.With(ctx,
		slog.String("player", loaded.Player.Name),
		slog.String("account_id", loaded.Account.ID))

	clk := cfg.Clock
	if clk == nil {
		clk = clock.RealClock{}
	}

	// Reconnect check: if a session for this player already exists,
	// branch on its phase. LinkDead → reattach this new connection.
	// Playing → prompt for takeover per session-lifecycle §8.
	if existing, ok := cfg.Manager.GetByPlayerID(loaded.Player.ID); ok {
		if existing.isLinkDead() {
			return reconnect(ctx, c, cfg, existing, clk)
		}
		if !promptTakeoverConfirm(ctx, c) {
			_ = writeLine(ctx, c, renderTakeoverDeclined())
			logging.From(ctx).Info("login: takeover declined",
				slog.String("player", loaded.Player.Name))
			return nil
		}
		liveSave, _, ok := performTakeover(ctx, cfg, existing)
		if !ok {
			// Lost the takeover claim to a concurrent login.
			_ = writeLine(ctx, c, renderTakeoverRaced())
			logging.From(ctx).Info("login: takeover lost race",
				slog.String("player", loaded.Player.Name))
			return nil
		}
		// Override the stale-from-disk save loaded by login.Run with the
		// live in-memory record from the displaced session. Without this
		// any post-autosave movement / mutation on the old actor would
		// be silently dropped (session-lifecycle §8: "same entity").
		// resolveStartRoom below picks up the live Location automatically.
		if liveSave != nil {
			loaded.Player = liveSave
		}
	}

	// M12.3: a new player runs the interactive creation wizard (spec
	// §3-§7) BEFORE the runtime actor is built, so the chosen race/class
	// land on loaded.Player and the downstream applyRace/applyClass +
	// alignment seed + commit consume them unchanged. A disconnect here
	// persists nothing (§8 — the actor isn't built or in the Manager).
	// A nil CreationFlow takes the §2 immediate-commit path (no-op).
	if loaded.New {
		if err := runCreation(ctx, c, cfg, loaded); err != nil {
			// The only errors are connection failures (disconnect mid-
			// creation). Nothing was persisted; close quietly.
			logging.From(ctx).Info("creation: aborted before commit",
				slog.String("player", loaded.Player.Name),
				slog.Any("err", err))
			return nil
		}
	}

	start, err := resolveStartRoom(cfg, loaded.Player.Location)
	if err != nil {
		return fmt.Errorf("session: resolve start room: %w", err)
	}

	floodCfg := cfg.Flood
	// Resolve the character's world attribute set once (SR-M1): it seeds the
	// StatBlock AND is held for `score` to render the declared attributes.
	attrSet := resolveAttributeSet(cfg.AttributeSets, cfg.WorldAttributeSets, loaded.Player.WorldID)
	a := &connActor{
		id:            c.ID(),
		conn:          c,
		renderer:      cfg.Render,
		playerID:      loaded.Player.ID,
		accountID:     loaded.Account.ID,
		room:          start,
		colorEnabled:  cfg.ColorEnabled,
		colorTier:     readColorTier(c),
		save:          loaded.Player,
		players:       cfg.Players,
		faction:       cfg.Faction,
		reputation:    cfg.Reputation,
		prof:          cfg.Proficiency,
		known:         cfg.Known,
		combat:        cfg.Combat,
		combatLocator: cfg.CombatLocator,
		effects:       cfg.Effects,
		progression:   cfg.Progression,
		items:         cfg.Items,
		contents:      cfg.Contents,
		placement:     cfg.Placement,
		trades:        cfg.Trades,
		light:         cfg.Light,
		equipment:     make(map[string]entities.EntityID),
		footprints:    make(map[entities.EntityID][]string),
		statBlock:     progression.NewWithBase(seedBaseFromSetOrDefault(attrSet)),
		attrSet:       attrSet,
		progress:      progression.NewProgressionState(),
		// M7.5: vitals restore from the persisted save when present;
		// absent block (fresh character, migrated-from-v4 save) spawns
		// at full HP via NewVitals. The race/class/level inputs that
		// would derive real numbers for max HP here are M8.3/M8.4.
		// NOTE: this seeds Vitals BEFORE the hp_max OnMaxChange listener is
		// wired (~L769); RestoreBase (~L972) then fires that listener, so the
		// StatBlock's hp_max reconciles the ceiling afterward. A save that
		// already encodes the post-bonus maxHP is a round-trip no-op; a future
		// migration emitting a maxHP below the stat-block value would clamp
		// current here first, then SetMax raises it back — order matters.
		vitals:         restorePlayerVitals(loaded.Player.Vitals),
		pools:          pool.NewSet(),
		poolDecls:      playerSeedPoolDecls(cfg.Pools),
		channelMap:     cfg.ChannelMap,
		unarmedSubdual: cfg.UnarmedSubdual,
		flood:          newFloodGate(floodCfg, clk),
		gmcpFlood:      newFloodGate(gmcpFloodConfig(floodCfg), clk),
		floodCfg:       &floodCfg,
		clk:            clk,
		lastInputAt:    clk.Now(),
	}

	// M14.1: bind Vitals.SetMax to StatBlock hp_max changes so an
	// effect or level-up that raises hp_max actually moves the
	// player's max-HP ceiling, and a drop in hp_max clamps current
	// HP down. The listener fires outside the StatBlock lock; it
	// only takes Vitals.mu briefly.
	//
	// Registered BEFORE RestoreBase / RestoreModifiers below. This
	// makes StatBlock the canonical source of hp_max on login: any
	// mismatch between persisted Vitals.maxHP and the freshly-
	// restored StatBlock pulls Vitals into line with StatBlock. The
	// typical case is that they agree (both were last persisted in
	// sync); the exceptional case (effect-boosted Vitals.maxHP
	// persisted before the effect was stripped, etc.) heals on
	// login rather than carrying stale debt forward.
	a.statBlock.OnMaxChange(progression.StatHPMax, func(_, newMax int) {
		a.vitals.SetMax(newMax)
	})

	// M8.3: resolve the actor's race id. Save's race wins; an
	// empty saved race (legacy v7 or fresh character) falls back
	// to cfg.DefaultRace. If the resolved id isn't registered, the
	// actor stays raceless (raceID="", no flags) — see applyRace.
	applyRace(a, &cfg, loaded.Player.Race)
	// Keep save in sync with the resolved race so the next Persist
	// commits the assigned default. Only sync when the resolution
	// produced a real id — if applyRace returned with a.raceID=""
	// because the saved race isn't currently registered (content
	// removed between restarts), we PRESERVE the original id on the
	// save so re-adding the race later reattaches the character.
	// Closes m8-4 "applyClass fail-soft erases saved id" item +
	// the analogous m8-3 race-side concern in one symmetric fix.
	if a.raceID != "" && loaded.Player.Race != a.raceID {
		a.mu.Lock()
		a.save.Race = a.raceID
		a.markDirtyLocked()
		a.mu.Unlock()
	}

	// M8.4: resolve and apply class. The class id is informational
	// at session start (subscribers read it off the actor when a
	// level.up event lands); the apply step just validates against
	// the registry and snapshots the trains_available pool.
	//
	// Same fail-soft preservation as race above: an unresolved class
	// id (a.classID="") leaves the save's class field untouched so
	// re-adding the class later reattaches the character.
	applyClass(a, &cfg, loaded.Player.Class)
	applyBackground(a, loaded.Player.Background, cfg.Backgrounds)
	a.mu.Lock()
	a.feats = cfg.Feats         // EPIC S4: feat registry for known_feats → bonuses
	a.languages = cfg.Languages // languages.md: registry for known-language display
	a.trainsAvailable = loaded.Player.TrainsAvailable
	a.featCredits = loaded.Player.FeatCredits
	// Re-sync the save's class list if applyClass dropped a removed-content
	// id (the resolved list differs from what was loaded). Re-adding the
	// class later reattaches the character.
	if len(a.classIDs) > 0 && !slices.Equal(a.classIDs, loaded.Player.Class) {
		a.save.Class = append([]string(nil), a.classIDs...)
		a.markDirtyLocked()
	}
	a.mu.Unlock()

	// M19.1: restore the role set from the save, then apply the config
	// seed (roles-and-permissions §5/§6). A seeded role marks the save
	// dirty so the bootstrap admin persists on first login.
	applyRoles(a, &cfg, loaded.Player.Roles)

	// Bootstrap admin: the very first character created in a fresh
	// deployment is granted the admin role automatically, so the operator
	// who stands up the game has a working administrator without
	// configuring ANOTHERMUD_ROLE_SEED out of band. The signal is "no
	// character exists yet" (the player store is empty) on a fresh
	// character (loaded.New) — the to-be-created character is not written
	// until commitCreation, so an empty store means this is genuinely the
	// first character. It fires exactly once ever: the second character
	// sees the first already on disk. Unlike RoleSeed this is a one-time
	// grant — it persists in the save via Grant and is never re-ensured,
	// so it stays revocable in-game later. cfg.AdminRole defaults to
	// "admin" (the dispatch default) when unset.
	if loaded.New && cfg.Players != nil && cfg.Players.IsEmpty() {
		bootstrapRole := cfg.AdminRole
		if bootstrapRole == "" {
			bootstrapRole = "admin"
		}
		a.Grant(bootstrapRole)
	}

	// M8.5: restore persisted alignment + sync bucket tag.
	// AlignmentManager.Bucket is idempotent and sets the tag
	// regardless of whether the integer changed, so a returning
	// character always lands with a current tag. For a fresh
	// character (loaded.New), seed initial alignment from race +
	// class StartingAlignment (spec §3.1, §4.1 "presentation
	// fields"). Set is the admin path — silent, no events, no
	// history. AlignmentManager may be nil in tests.
	a.mu.Lock()
	a.alignment = loaded.Player.Alignment
	// faction.md §8: restore the per-character standing bag. Rank tags are
	// derived, re-synced from this bag below (outside the lock — the manager's
	// Rank re-enters a.mu via Standing/SetRankTag). At this point in run() the
	// actor is single-threaded (not yet registered with the manager or
	// accepting input), so reading a.factionStanding outside the lock for the
	// sync loop is safe.
	if len(loaded.Player.FactionStanding) > 0 {
		a.factionStanding = make(map[string]int, len(loaded.Player.FactionStanding))
		maps.Copy(a.factionStanding, loaded.Player.FactionStanding)
	}
	// reputation.md §10: restore the single renown score. The tier tag is
	// derived, re-synced from this score below (outside the lock — the manager's
	// Tier re-enters a.mu via Renown/SetTierTag). Absent on a pre-v32 save → 0
	// (Unknown), the correct default.
	a.renown = loaded.Player.Reputation
	// M11.1: restore persisted gold balance (spec §2.1). No manager
	// involvement at login — gold has no bucket/tag derivation like
	// alignment; the raw integer is the whole state.
	a.gold = loaded.Player.Gold
	// M11.3: restore persisted sustenance pool (spec §4.1). For a
	// returning character this is the saved value (legacy v12 saves were
	// migrated to 100); a fresh character carries 0 here and is seeded
	// to full below.
	a.sustenance = loaded.Player.Sustenance
	a.mu.Unlock()
	if cfg.Alignment != nil {
		if loaded.New {
			seed := 0
			if cfg.Races != nil {
				if r, ok := cfg.Races.Get(a.raceID); ok {
					seed += r.StartingAlignment
				}
			}
			if cfg.Classes != nil {
				// Sum every class's starting alignment (one class today).
				for _, cid := range a.ClassIDs() {
					if c, ok := cfg.Classes.Get(cid); ok {
						seed += c.StartingAlignment
					}
				}
			}
			if seed != 0 {
				cfg.Alignment.Set(ctx, a, seed, "character-created")
			} else {
				// Even at zero, install the neutral bucket tag so
				// rule matchers see a consistent tag set from
				// first login forward.
				_ = cfg.Alignment.Bucket(a)
			}
		} else {
			_ = cfg.Alignment.Bucket(a)
		}
	}

	// faction.md §8: re-mirror each touched faction's rank tag from the
	// restored standing bag (derived state, not persisted). Outside the actor
	// lock — Rank calls back into Standing/SetRankTag, which take a.mu. Skips
	// any standing for a faction no longer in content (fail-soft).
	if cfg.Faction != nil {
		for fid := range a.factionStanding {
			if def, ok := cfg.Faction.Registry().Get(fid); ok {
				_ = cfg.Faction.Rank(a, def)
			}
		}
	}

	// reputation.md §3/§10: install the renown tier tag from the restored score
	// (derived state, not persisted). For a fresh or pre-v32 character this is
	// renown:unknown, so rule matchers see a consistent renown tag from first
	// login. Outside the actor lock — Tier calls back into Renown/SetTierTag,
	// which take a.mu. (A class/background starting renown is an R3 seed.)
	if cfg.Reputation != nil {
		_ = cfg.Reputation.Tier(a)
	}

	// M11.3: seed a fresh character's sustenance pool to full (spec
	// §4.1 — "starts at 100 on character creation"). Done inline rather
	// than via a character.created bus subscriber because the actor is
	// not yet registered with the Manager at the publish below (Add
	// happens later in this function), so a subscriber resolving the
	// actor by id would miss it. Mirrors the alignment seed above. The
	// service write-through marks the save dirty so the seeded value is
	// persisted on the first autosave. nil in tests → the field stays
	// at its restored value.
	if loaded.New && cfg.Sustenance != nil {
		cfg.Sustenance.Set(a, economy.MaxSustenance)
	}

	// M12.2: character.created now publishes from the completion pipeline
	// (after commit + placement, §6.4 step 6), not here — see the block
	// after Manager.Add below. Publishing pre-commit would grant level-1
	// abilities to a character a last-chance name conflict then discards.

	// Seed the lock-free wimpy threshold from the persisted save.
	// Done after struct construction because atomic.Int32 has no
	// natural composite-literal initializer; this is the canonical
	// "Store the initial value before any reader can race the
	// goroutine that owns the actor" pattern.
	a.wimpyThreshold.Store(int32(clampWimpy(loaded.Player.WimpyThreshold)))
	// M22.4: seed the autoloot preference from the persisted save.
	a.autoloot.Store(loaded.Player.Autoloot)
	// feats Bucket C: seed the Power Attack stance from the persisted save.
	a.powerAttack.Store(loaded.Player.PowerAttackActive)
	// grouping.md §9: seed the auto-assist preference from the persisted save.
	a.autoAssist.Store(loaded.Player.AutoAssist)

	// M15.3: hydrate the recall room id from the persisted save.
	// Empty = no recall point bound (the documented default for
	// fresh / migrated characters). The verb path validates the
	// id against the world at recall time per recall.md §4 — no
	// load-time validation here.
	a.recall = loaded.Player.Recall

	// Install the persisted base attributes (M8.1 v6) before modifiers
	// so a level-up bump sitting on disk is in place before any equip-
	// driven modifier overlays on top.
	if len(loaded.Player.StatsBase) > 0 {
		a.statBlock.RestoreBase(loaded.Player.StatsBase)
	}
	// Install the persisted modifier set FIRST relative to respawn.
	// respawnEquipment will then rebind each entry's source key from
	// the saved entity id to the fresh one as it spawns the matching
	// ItemInstance. Order matters: the block must hold the old source
	// keys at the moment respawn runs so RebindSource has something
	// to find.
	a.statBlock.RestoreModifiers(loaded.Player.Stats)

	// EPIC S4 Phase 3b: install the stat-shaped feat bonuses (today the
	// Toughness hp_max bonus) from the loaded known_feats — AFTER RestoreModifiers
	// so the feat modifier sits on top of the fully-restored block (and replaces
	// any stale feat: entry a legacy save round-tripped). The OnMaxChange→vitals
	// binding (wired far above) then moves the ceiling. No-op without such feats.
	a.applyFeatGrants()

	// Seed the generalized resource pools (mana, movement) full from the
	// now-finalized stat maxes and bind each ceiling to its max stat. Done
	// after the stat block is fully restored so the maxes are final.
	a.seedResourcePools()

	// M8.2: install persisted progression state. Empty snapshot is
	// a no-op (uninitialized tracks lazy-init on first interaction
	// per spec §5.3).
	if len(loaded.Player.Progression) > 0 {
		a.progress.Restore(loaded.Player.Progression)
	}

	// M9.1: install persisted ability proficiency + cap maps
	// (spec abilities-and-effects §3.1). Empty snapshot is a
	// no-op — the ProficiencyManager's Restore short-circuits so
	// fresh characters don't inflate manager state. Manager is
	// nil-safe to keep test wiring minimal.
	if cfg.Proficiency != nil {
		cfg.Proficiency.Restore(loaded.Player.ID, loaded.Player.Abilities)
	}

	// Crafting Phase 0/1: install persisted known recipes
	// (crafting-and-cooking §9). Restore drops any id whose recipe is no
	// longer in content — a removed recipe loads cleanly, never an error.
	// nil-safe.
	if cfg.Known != nil {
		cfg.Known.Restore(loaded.Player.ID, loaded.Player.KnownRecipes)
	}

	// Respawn persisted inventory into live ItemInstances. Done before
	// the actor enters the manager / starts taking input so a takeover
	// or autosave can't observe a partial inventory.
	if cfg.Items != nil && cfg.Templates != nil {
		respawnInventory(ctx, a, cfg.Items, cfg.Contents, cfg.Templates, loaded.Player.Inventory)
		respawnEquipment(ctx, a, cfg.Items, cfg.Templates, cfg.Slots, loaded.Player.Equipment)
	}

	// Keep the save's location in sync with the room we actually placed
	// the player in. Mark dirty so the corrected location is flushed
	// even if the player idles and disconnects before issuing any
	// movement command (covers the saved-room-removed fallback case).
	if a.save.Location != string(start.ID) {
		a.mu.Lock()
		a.save.Location = string(start.ID)
		a.markDirtyLocked()
		a.mu.Unlock()
	}

	// M10.8: load the player's persisted quest state BEFORE Add makes the
	// actor visible to the autosave tick and the quest watcher — so an
	// autosave or a same-tick watcher advance can't observe empty quest
	// state (mirrors how inventory/effects are restored pre-Add).
	// QuestStore.Load also caches the id→name mapping (so a later Save
	// resolves its path even when the player has no quests file yet).
	// nil-safe: Quests and QuestStore are wired together (both set or
	// both nil) at the composition root.
	if cfg.QuestStore != nil {
		if state, ok := cfg.QuestStore.Load(ctx, a.PlayerID(), a.Name()); ok && cfg.Quests != nil {
			cfg.Quests.LoadState(a.PlayerID(), state)
		}
	}

	// M12.2: character-creation completion pipeline (spec §6.4). A new
	// character's entity is fully assembled above (race/class/alignment/
	// sustenance seeded, location synced) but has NOT touched disk yet —
	// so a disconnect before this point leaves nothing persisted (§8).
	// commitCreation persists it under the last-chance name-conflict
	// guard. Returning players skip this entirely (already on disk).
	//
	// The interactive wizard that would run BETWEEN assembly and commit,
	// plus restart-on-validation (§7) and input routing (§4), lands in
	// M12.3; M12.2 takes the §2 "no flow registered → immediate commit"
	// path, so the Creating phase is synchronous here.
	if loaded.New {
		a.mu.Lock()
		a.phase = phaseCreating
		a.mu.Unlock()
		if err := commitCreation(ctx, cfg, a); err != nil {
			if errors.Is(err, ErrNameConflict) {
				_ = a.Write(ctx, "Sorry — that name was just taken. Please reconnect and choose another.")
				logging.From(ctx).Info("creation: last-chance name conflict",
					slog.String("name", a.Name()))
				return nil // server closes the conn; actor never entered the Manager
			}
			return fmt.Errorf("session: %w", err)
		}
		a.mu.Lock()
		a.phase = phasePlaying
		a.mu.Unlock()
		_ = a.Write(ctx, fmt.Sprintf("Welcome, %s.", a.Name()))
	}

	cfg.Manager.Add(a)

	// M16.6a: log the negotiated color tier alongside the welcome
	// line so operators can see what the renderer is about to use.
	// Spec §7.2 — tier is derived from TTYPE + IsMudClient at
	// telnet negotiation completion; websocket conns always
	// report TrueColor.
	logging.From(ctx).Debug("session color tier",
		slog.String("event", "session.color_tier"),
		slog.String("tier", a.colorTier.String()))

	// M13.1c: bind to the notifications manager and drain any
	// persisted backlog (offline tells / channel posts that arrived
	// while away). Done post-Add so the welcome line and in-room
	// arrival broadcast settle before the drain text appears.
	notifRegister(ctx, cfg, a)

	// hireable-mobs.md §9: re-materialize the actor's owned hirelings into their
	// room — a persisted hire contract puts the help back at the owner's side on
	// login. Post-Add so the actor is placed and reachable first.
	rematerializeHirelings(ctx, cfg, a)

	// M12.2: publish character.created AFTER commit + placement (§6.4
	// step 6) so the class-path processor's level-1 grant runs only for a
	// character that actually committed, and so the actor is already in
	// the Manager when the notifier resolves it. Bus may be nil in tests.
	if loaded.New && cfg.Bus != nil {
		cfg.Bus.Publish(ctx, eventbus.CharacterCreated{
			EntityID: loaded.Player.ID,
			ClassID:  a.ClassID(), // primary; the subscriber walks ClassIDs() live
		})
	}

	// Announce arrival to the start room (excluding self) so anyone
	// already there sees the new player materialize.
	cfg.Manager.SendToRoom(ctx, start.ID,
		fmt.Sprintf("%s has arrived.", a.Name()), a.PlayerID())

	// Publish player.moved for the login spawn; From is empty since
	// the player wasn't previously in any room (spec §5.2 — empty
	// source room is a no-op for state clearing). Then run the
	// immediate hook before the room description.
	if cfg.Bus != nil {
		cfg.Bus.Publish(ctx, eventbus.PlayerMoved{
			PlayerID: a.PlayerID(),
			To:       start.ID,
		})
	}
	if cfg.Disposition != nil && a.PlayerID() != "" {
		cfg.Disposition.OnPlayerEnteredImmediate(ctx, a.PlayerID(), a.Name(), nil, start.ID)
	}

	// Seed the area-transition tracker with the spawn area so the first
	// crossing narrates a real "from" and the spawn render itself shows
	// no zone-line (player-maps §4, spawn-suppression rule). A brand-new
	// character entering a never-seen home area still earns the once-ever
	// first-entry banner (B1), prepended to the spawn view.
	a.SetLastAreaSeen(start.AreaID)
	var firstEntry string
	if !a.HasSeenArea(start.AreaID) {
		a.MarkAreaSeen(start.AreaID)
		firstEntry = command.FirstEntryBanner(command.MapAreaName(cfg.World, start.AreaID))
	}
	startLvl := command.EffectiveLight(cfg.Light, start, a, cfg.Items, cfg.Placement)
	spawnView := command.RenderRoom(start, cfg.Placement, cfg.Items, questMarkerFor(cfg.Quests, a.PlayerID()), cfg.Ambience, nil, startLvl, exitVisibleFor(a, cfg.AdminRole), otherPlayerNames(cfg.Manager, start.ID, a.PlayerID())...)
	spawnView = command.AppendMinimap(spawnView, start, a, cfg.World)
	spawnView = command.AppendRoomData(spawnView, start, a, cfg.AdminRole)
	if firstEntry != "" {
		spawnView = firstEntry + "\n" + spawnView
	}
	if err := a.Write(ctx, spawnView); err != nil {
		// Initial render failed: the connection is unusable. Full
		// teardown immediately — no point parking link-dead.
		fullTeardown(ctx, cfg, a)
		return fmt.Errorf("first render: %w", err)
	}
	// M16.4b: emit Room.Info for the login spawn. Construction
	// assigned a.room directly (not via SetRoom) so the SetRoom
	// hook didn't fire; emit explicitly here so a Mudlet client
	// that activated GMCP during login has its mapper panel
	// populated for the starting room.
	a.sendGmcpRoomInfo(ctx, start)

	if cfg.Disposition != nil && a.PlayerID() != "" {
		cfg.Disposition.OnPlayerEnteredDeferred(ctx, a.PlayerID(), a.Name(), nil, start.ID)
	}

	exit := pump(ctx, c, cfg, a, clk)
	dispatchTeardown(ctx, cfg, a, exit, clk)
	return nil
}

// pumpExit is the reason the read loop unwound, used by the teardown
// dispatcher to choose between link-dead and full teardown.
type pumpExit int

const (
	// exitClientQuit: dispatcher returned ErrQuit (the player typed
	// "quit"). Always full teardown — the player meant to leave.
	exitClientQuit pumpExit = iota
	// exitConnGone: Read returned EOF / ErrClosed — the transport
	// went away. Eligible for link-dead if the actor's disconnecting
	// latch is not set (i.e. the server didn't initiate the close).
	exitConnGone
	// exitCtxCancel: context was cancelled — server is shutting down.
	// Always full teardown so SaveAll can commit and indices clear.
	exitCtxCancel
	// exitForced: server-initiated disconnect (flood threshold). The
	// actor's disconnecting latch is set before the loop returns;
	// teardown is unconditionally full.
	exitForced
)

// pump is the per-connection read → dispatch loop. Extracted from run
// so the reconnect path can re-enter it against the new connection.
// Returns the reason for exit; teardown is dispatched by the caller.
func pump(ctx context.Context, c conn.Connection, cfg Config, a *connActor, clk clock.Clock) pumpExit {
	// Install the inbound (client→server) GMCP handler before the read
	// loop — GMCP frames are processed inside c.Read, so the handler must
	// be set first. No-op for a transport without GMCP. Covers both pump
	// call sites (initial play + link-dead reattach).
	installGmcpInbound(c, a, cfg)
	installCharMode(ctx, c, a, cfg)
	for {
		line, err := c.Read(ctx)
		if err != nil {
			switch {
			case errors.Is(err, io.EOF), errors.Is(err, conn.ErrClosed):
				return exitConnGone
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return exitCtxCancel
			case errors.Is(err, conn.ErrLineTooLong):
				_ = a.Write(ctx, "Input too long; truncated.")
				continue
			default:
				logging.From(ctx).Warn("read error", slog.Any("err", err))
				return exitConnGone
			}
		}

		logging.From(ctx).Debug("input received", slog.String("line", sanitizeForLog(line)))

		// Refresh the idle bookkeeping before the flood gate so even
		// dropped input still counts as activity (a flooding client is
		// not "idle"; the flood gate alone decides how it's punished).
		a.noteInput(clk.Now())

		// Flood gate runs before dispatch. The warn write happens after
		// the gate returns so no caller can accidentally re-enter the
		// gate from inside the Write path.
		decision, warn := a.flood.Check()
		if warn {
			_ = a.Write(ctx, "Slow down.")
		}
		switch decision {
		case floodAllow:
			// proceed
		case floodDrop:
			continue
		case floodDisconnect:
			_ = a.Write(ctx, "Disconnected: command flooding.")
			logging.From(ctx).Warn("session: disconnect on flood threshold",
				slog.String("player", a.PlayerName()))
			a.mu.Lock()
			a.disconnecting = true
			a.mu.Unlock()
			return exitForced
		}

		// Parse the line into one or more commands (chaining `;` + repeat
		// `3n`, commands-and-dispatch §4) and dispatch each in order. The
		// chain cap bounds expansion; the flood gate above already counted
		// this submission once. Per-tick pacing of the expanded commands is
		// out of scope (§4.4) — they run synchronously, like any line.
		env := commandEnv(cfg)
		for _, segment := range command.ParseInput(line, cfg.ChainCap) {
			if err := cfg.Commands.Dispatch(ctx, env, a, segment); err != nil {
				if errors.Is(err, command.ErrQuit) {
					return exitClientQuit
				}
				logging.From(ctx).Warn("command handler error", slog.Any("err", err))
			}
		}
	}
}

// dispatchTeardown picks the right cleanup path for the actor based on
// the exit reason, the disconnecting latch, and the link-dead policy.
//
// Routing:
//   - exitClientQuit / exitCtxCancel / exitForced → full teardown.
//   - exitConnGone with disconnecting set (idle sweep closed the
//     connection from underneath us) → full teardown.
//   - exitConnGone with LinkDead disabled → full teardown.
//   - exitConnGone with LinkDead enabled and not disconnecting → park
//     in LinkDead phase, broadcast "lost their connection", keep the
//     actor in every index except byConn so a reconnect can find it.
func dispatchTeardown(ctx context.Context, cfg Config, a *connActor, exit pumpExit, clk clock.Clock) {
	a.mu.Lock()
	disc := a.disconnecting
	taken := a.takenOver
	a.mu.Unlock()

	// §6.1 stale-event guard: a taken-over session's pump has unwound
	// after the new session displaced it. Every index entry the old
	// actor held has already been reassigned or cleared by
	// performTakeover. Running fullTeardown here would re-broadcast
	// "X has left" and delete byPlayerID / byRoom entries that now
	// belong to the replacement session.
	if taken {
		logging.From(ctx).Debug("teardown: skipped (taken over)",
			slog.String("player", a.PlayerName()))
		return
	}

	if exit == exitConnGone && !disc && cfg.LinkDead.Enabled {
		enterLinkDeadTeardown(ctx, cfg, a, clk)
		return
	}
	fullTeardown(ctx, cfg, a)
}

// fullTeardown runs the M3 unwind in the canonical order
// "broadcast → remove → persist". This matches LinkDeadCleanup so a
// future refactor that consolidates the two paths cannot accidentally
// transpose the order and introduce a use-after-remove. Safe on an
// actor still in any phase; the manager's Remove is itself idempotent.
//
// After persist, any item entities the actor was carrying are
// untracked so the entity store does not grow without bound across
// reconnects. They will be respawned on the next login from the
// freshly-saved Inventory list.
func fullTeardown(ctx context.Context, cfg Config, a *connActor) {
	// Cancel any open trade FIRST so staged items/coin return to this actor
	// before Persist flushes the save (direct-trade.md §6). Without this, a
	// disconnect mid-trade would persist a save missing the staged value.
	if cfg.Trades != nil {
		cfg.Trades.CancelFor(ctx, a.PlayerID(), fmt.Sprintf("%s disconnected", a.Name()))
	}
	room := a.Room()
	if room != nil {
		cfg.Manager.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s has left.", a.Name()), a.PlayerID())
	}
	cfg.Manager.Remove(a)
	if err := a.Persist(ctx); err != nil {
		logging.From(ctx).Warn("save on disconnect failed", slog.Any("err", err))
	}
	untrackInventory(ctx, cfg.Items, cfg.Contents, a)

	// M9.1: drop in-memory proficiency state so the manager's
	// working set tracks currently-connected players only. Persist
	// has already flushed the snapshot to disk so this is purely a
	// memory release.
	if cfg.Proficiency != nil {
		cfg.Proficiency.Drop(a.PlayerID())
	}

	// Crafting: drop in-memory known-recipe state on logout. Persist has
	// already flushed the snapshot; this is a memory release (mirrors the
	// proficiency drop above).
	if cfg.Known != nil {
		cfg.Known.Drop(a.PlayerID())
	}

	// M9.2: drop any active effects on logout. Spec §5.5 marks
	// effect-list state as ephemeral (not persisted), so this is
	// the canonical lifecycle endpoint — no Persist, no event.
	// The stat-block snapshot has already been written by Persist
	// above, which captured the effect-driven modifiers under
	// their EffectSourceKey entries; whether those round-trip
	// across login is a M9.4-era decision (the spec calls it an
	// open question).
	if cfg.Effects != nil {
		cfg.Effects.Drop(a.PlayerID())
	}

	// M9.4b: release queued actions + recorded pulse-delays so the
	// managers' working sets track connected players only. Both are
	// ephemeral (queued actions are mid-pulse intent; cooldowns are
	// in-memory per spec §2.2) — no persist, no event.
	if cfg.ActionQueue != nil {
		cfg.ActionQueue.Drop(a.PlayerID())
	}
	if cfg.PulseDelay != nil {
		cfg.PulseDelay.Drop(a.PlayerID())
	}
	if cfg.Casts != nil {
		cfg.Casts.Drop(a.PlayerID())
	}
	// action-economy.md §6: a timed action in flight at logout is simply lost
	// (it reserved nothing, so the world is untouched).
	if cfg.Actions != nil {
		cfg.Actions.Drop(a.PlayerID())
	}
	// follow.md §4: logout clears both sides of every follow edge this actor was
	// in (their leader, and everyone following them), notifying the survivors.
	if cfg.Manager != nil {
		cfg.Manager.dropFollow(ctx, a.PlayerID(), a.Name())
		// grouping.md §3: logout removes the actor from their party (a leader
		// leaving disbands it), notifying the survivors.
		cfg.Manager.dropParty(ctx, a.PlayerID(), a.Name())
	}

	// M10.8: drop in-memory quest state + the persistence name cache so
	// the working sets track connected players only. Save has already
	// flushed every mutation to disk, so this is a pure memory release.
	if cfg.Quests != nil {
		cfg.Quests.DropState(a.PlayerID())
	}
	if cfg.QuestStore != nil {
		cfg.QuestStore.Forget(a.PlayerID())
	}
	// Note: notification-queue unregister fires from session.Manager.Remove
	// above (M13.1c), so takeover and linkdead-reap pick it up via the
	// same central path.
}

// respawnInventory creates fresh ItemInstances for each persisted
// InventoryEntry and attaches them to the actor. Containers recurse
// into their Contents: each child is spawned and pushed into the
// freshly-minted container via contents.Put. Missing templates are
// logged and skipped — the player loses that item (and anything
// nested inside it, if it was a container) on this load.
//
// Whenever any entry is dropped (template gone, spawn failure, or a
// non-container template with non-empty Contents), the in-memory
// save is rebuilt from survivors and dirty is flipped so the next
// Persist trims dead references.
func respawnInventory(ctx context.Context, a *connActor, store *entities.Store, contents *entities.Contents, tpls *item.Templates, saved []player.InventoryEntry) {
	if len(saved) == 0 {
		return
	}
	survivors, dropped := spawnEntries(ctx, a, store, contents, tpls, saved, "")
	for _, id := range survivors.ids {
		a.mu.Lock()
		a.inventory = append(a.inventory, id)
		a.mu.Unlock()
	}
	if dropped {
		a.mu.Lock()
		a.save.Inventory = survivors.entries
		a.markDirtyLocked()
		a.mu.Unlock()
	}
}

// spawnedSlice carries the parallel result of a respawn pass: the
// top-level entity ids (so the caller can attach them to the
// holder), and the post-prune save entries (so the caller can trim
// the on-disk record when something didn't survive). Kept as a
// dedicated type rather than two returns so the recursive call site
// reads cleanly.
type spawnedSlice struct {
	ids     []entities.EntityID
	entries []player.InventoryEntry
}

// spawnEntries is the recursive worker behind respawnInventory.
// parentID is the container entity id to put children into; empty
// means "top-level inventory, no put". Returns the spawned entity
// ids in source order and the surviving entries; the dropped flag
// tells the caller whether any entry was pruned (so a Persist can
// trim).
//
// A non-container template carrying Contents in the save is a sign
// of either a content edit (the template's type changed since save
// time) or a corrupted save. Drop the Contents quietly and keep the
// item — losing one container is better than refusing to load.
func spawnEntries(ctx context.Context, a *connActor, store *entities.Store, contents *entities.Contents, tpls *item.Templates, saved []player.InventoryEntry, parentID entities.EntityID) (spawnedSlice, bool) {
	out := spawnedSlice{
		ids:     make([]entities.EntityID, 0, len(saved)),
		entries: make([]player.InventoryEntry, 0, len(saved)),
	}
	dropped := false
	for _, entry := range saved {
		tpl, err := tpls.Get(item.TemplateID(entry.Template))
		if err != nil {
			logging.From(ctx).Warn("inventory: dropping unknown template",
				slog.String("template_id", entry.Template),
				slog.String("player", a.PlayerName()),
				slog.Any("err", err))
			dropped = true
			continue
		}
		inst, err := store.Spawn(tpl)
		if err != nil {
			logging.From(ctx).Warn("inventory: spawn failed",
				slog.String("template_id", entry.Template),
				slog.Any("err", err))
			dropped = true
			continue
		}
		if parentID != "" && contents != nil {
			contents.Put(parentID, inst.ID())
		}
		// Restore a persisted magazine load onto the fresh instance (no-op for a
		// non-magazine weapon or a nil count).
		if entry.Loaded != nil {
			inst.SetMagazineLoaded(*entry.Loaded)
		}

		survivor := player.InventoryEntry{Template: entry.Template, Loaded: entry.Loaded}
		if len(entry.Contents) > 0 {
			if tpl.Type != "container" || contents == nil {
				// A non-container template carrying nested contents in
				// the save can't be honored at runtime. Keep the
				// outer item, drop the children.
				logging.From(ctx).Warn("inventory: dropping nested contents on non-container",
					slog.String("template_id", entry.Template),
					slog.String("template_type", tpl.Type))
				dropped = true
			} else {
				child, childDropped := spawnEntries(ctx, a, store, contents, tpls, entry.Contents, inst.ID())
				if len(child.entries) > 0 {
					survivor.Contents = child.entries
				}
				if childDropped {
					dropped = true
				}
			}
		}
		out.ids = append(out.ids, inst.ID())
		out.entries = append(out.entries, survivor)
	}
	return out, dropped
}

// respawnEquipment creates fresh ItemInstances for each persisted
// equipment entry and reattaches the persisted stat-block source keys
// from the saved entity ids to the freshly-minted ones (§3.5). Slot
// keys with unknown templates are dropped; the modifier set under
// their old source key stays orphaned in the block until the next
// Persist trims via syncStatsToSaveLocked (a.stats has no entry for
// the dropped slot — the orphan is in a.save.Stats, not a.stats).
// To keep the runtime block clean we also drop the matching modifier
// set when a template lookup fails.
//
// Entries with an empty Entity id (legacy v2-migrated saves, §3.5
// open question) install the item but skip the rebind — no source
// key exists to migrate, so the modifier set is effectively absent
// for that slot until the player re-equips.
func respawnEquipment(ctx context.Context, a *connActor, store *entities.Store, tpls *item.Templates, slots *slot.Registry, saved map[string]player.EquippedItem) {
	if len(saved) == 0 {
		return
	}
	// Iterate slot keys in deterministic order so logs (and any future
	// dependent-modifier semantics) don't churn across restarts.
	keys := make([]string, 0, len(saved))
	for k := range saved {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	type respawned struct {
		slot       string // the persisted target/canonical slot key
		newID      entities.EntityID
		companions []string // re-derived footprint beyond the target (§3.8)
	}
	survivors := make([]respawned, 0, len(keys))
	dropped := make([]string, 0)

	for _, slotKey := range keys {
		entry := saved[slotKey]
		tpl, err := tpls.Get(item.TemplateID(entry.Template))
		if err != nil {
			logging.From(ctx).Warn("equipment: dropping unknown template",
				slog.String("slot_key", slotKey),
				slog.String("template_id", entry.Template),
				slog.String("player", a.PlayerName()),
				slog.Any("err", err))
			dropped = append(dropped, slotKey)
			continue
		}
		inst, err := store.Spawn(tpl)
		if err != nil {
			logging.From(ctx).Warn("equipment: spawn failed",
				slog.String("slot_key", slotKey),
				slog.String("template_id", entry.Template),
				slog.Any("err", err))
			dropped = append(dropped, slotKey)
			continue
		}
		// Restore a persisted magazine load onto the fresh instance so a
		// reloaded wielded firearm keeps its rounds across relog (no-op for a
		// non-magazine weapon or a nil count).
		if entry.Loaded != nil {
			inst.SetMagazineLoaded(*entry.Loaded)
		}
		// Restore an inserted ammunition holder on a holder-fed weapon
		// (ammo-and-reloading §9) so a loaded firearm stays loaded across relog.
		if entry.Holder != nil {
			inst.SetInsertedHolder(entry.Holder.Template, entry.Holder.Loaded)
		}
		// Companion slots are re-derived from the (re-spawned) template, not
		// persisted (§3.8) — so a spanning item's full footprint is rebuilt
		// on reload from the saved target key alone.
		survivors = append(survivors, respawned{slot: slotKey, newID: inst.ID(), companions: inst.CompanionSlots()})

		if entry.Entity == "" {
			// Migrated-from-v2 entry: no old source key to rebind. The
			// modifier set is absent for this slot until re-equip.
			continue
		}
		oldSrc := entities.EquipmentSourceKey(entities.EntityID(entry.Entity))
		newSrc := entities.EquipmentSourceKey(inst.ID())
		if !a.statBlock.RebindSource(oldSrc, newSrc) {
			// Either the persisted stat block had no entry for the old
			// id (item was equipped but contributed no modifiers — fine)
			// or the new source key collided with an existing entry
			// (programming error — log and move on; the slot is still
			// equipped, just without modifier reattachment).
			logging.From(ctx).Debug("equipment: source-key rebind no-op",
				slog.String("slot_key", slotKey),
				slog.String("old", string(oldSrc)),
				slog.String("new", string(newSrc)))
		}
	}

	a.mu.Lock()
	// Re-expand each survivor's footprint: the persisted target key plus
	// companion-slot keys re-derived from the template (§3.8). Occupancy
	// accumulates across survivors (processed in sorted target-key order)
	// so companions pack into free indices deterministically.
	occ := make(map[string]bool, len(survivors))
	for _, r := range survivors {
		fp := []string{r.slot}
		occ[r.slot] = true
		if slots != nil {
			for _, comp := range r.companions {
				k, err := slots.FreeKey(comp, occ)
				if err != nil {
					// Companion names are validated at content load
					// (validateItemSlots), so this is unreachable for loaded
					// content; skip defensively rather than panic.
					continue
				}
				fp = append(fp, k)
				occ[k] = true
			}
		}
		for _, k := range fp {
			a.equipment[k] = r.newID
		}
		a.footprints[r.newID] = fp
	}
	// §4.5: derive the wielded-weapon snapshot from the restored set so a
	// returning player who logged out wielding a sword swings it again.
	a.recomputeWeaponLocked()
	if len(dropped) > 0 {
		// On-disk Equipment is now ahead of runtime; flip dirty so the
		// next persist trims dead slot entries (and any orphaned stat
		// modifier sets under their old source keys still sitting in
		// a.save.Stats — syncStatsToSaveLocked rewrites from the live
		// block, which doesn't contain them).
		a.syncEquipmentToSaveLocked()
		a.syncStatsToSaveLocked()
		a.markDirtyLocked()
	}
	a.mu.Unlock()
}

// untrackInventory removes every item entity in the actor's inventory
// and equipment from the runtime store. Called from teardown paths
// after the save has been written so the next login can respawn from
// the template ids. Safe to call with a nil store (tests) or empty
// containers.
//
// Container children (M5.9b) are swept recursively via Contents: a
// carried sack with three items inside has all four entities
// untracked. Contents entries are also cleared so the index doesn't
// retain phantom mappings to untracked ids. A nil Contents skips
// the recursion (tests that don't exercise containers).
func untrackInventory(ctx context.Context, store *entities.Store, contents *entities.Contents, a *connActor) {
	if store == nil {
		return
	}
	// Dedupe by id: a spanning item appears under several equipment keys
	// but is one entity, so untrack it once.
	seen := make(map[entities.EntityID]bool)
	for _, id := range a.Equipment() {
		if seen[id] {
			continue
		}
		seen[id] = true
		untrackTree(ctx, store, contents, id)
	}
	for _, id := range a.Inventory() {
		untrackTree(ctx, store, contents, id)
	}
}

// untrackTree untracks id and, if it's a container with Contents
// entries, recurses into its children. Each child is also cleared
// from the Contents index so the post-teardown state is clean.
// Logs at Debug on failure: untracking an already-gone entity is
// not a bug worth a Warn.
func untrackTree(ctx context.Context, store *entities.Store, contents *entities.Contents, id entities.EntityID) {
	if contents != nil {
		for _, childID := range contents.In(id) {
			untrackTree(ctx, store, contents, childID)
		}
		contents.Take(id)
	}
	if err := store.Untrack(id); err != nil {
		logging.From(ctx).Debug("inventory: untrack on teardown",
			slog.String("entity_id", string(id)),
			slog.Any("err", err))
	}
}

// enterLinkDeadTeardown parks the actor in LinkDead phase per spec §7.2:
// drop only the connection-id index, keep entity / name / account /
// room indices intact, broadcast lost-connection. The cleanup tick
// handler reaps if no reconnect arrives within the timeout.
func enterLinkDeadTeardown(ctx context.Context, cfg Config, a *connActor, clk clock.Clock) {
	if !a.enterLinkDead(clk.Now()) {
		// Lost a race; another path already advanced the phase. Fall
		// back to full teardown so we don't leak a dangling actor.
		fullTeardown(ctx, cfg, a)
		return
	}
	cfg.Manager.RemoveConnectionOnly(a)

	// A link-dead player can't hold a trade hostage while the partner waits
	// for a maybe-reconnect — end it now with full restore (direct-trade.md
	// §6). Returned value lands in the parked actor's inventory and persists
	// whenever it is next saved/reaped.
	if cfg.Trades != nil {
		cfg.Trades.CancelFor(ctx, a.PlayerID(), fmt.Sprintf("%s lost their connection", a.Name()))
	}

	room := a.Room()
	if room != nil {
		cfg.Manager.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s has lost their connection.", a.Name()), a.PlayerID())
	}
	logging.From(ctx).Info("session: link-dead",
		slog.String("player", a.PlayerName()),
		slog.Int("timeout_seconds", cfg.LinkDead.TimeoutSeconds))
}

// reconnect attaches a freshly-authenticated connection to an existing
// link-dead session and resumes the read loop. Per spec §7.4: swap
// the connection, re-install the byConn mapping, send "Reconnected.",
// re-render the room, then drop into pump.
func reconnect(ctx context.Context, c conn.Connection, cfg Config, a *connActor, clk clock.Clock) error {
	if !a.reattach(c, clk.Now()) {
		// Cleanup beat us to it; the link-dead session has been
		// reaped. Treat this connection as a fresh login fallback
		// would be too disruptive at this layer — close the new
		// connection with a friendly message and let the client try
		// again. (Vanishingly rare in practice.)
		_ = writeLine(ctx, c, "Your previous session has already ended; please reconnect.")
		return nil
	}
	if err := cfg.Manager.ReRegisterConnectionForSession(a, c.ID()); err != nil {
		logging.From(ctx).Warn("reconnect: re-register failed", slog.Any("err", err))
		// The actor is now in an inconsistent state (phase=Playing
		// but no byConn entry). Force full teardown to recover.
		a.mu.Lock()
		a.disconnecting = true
		a.mu.Unlock()
		fullTeardown(ctx, cfg, a)
		return nil
	}

	logging.From(ctx).Info("session: reconnected",
		slog.String("player", a.PlayerName()))

	if err := a.Write(ctx, renderReconnect()); err != nil {
		logging.From(ctx).Warn("reconnect: banner write failed", slog.Any("err", err))
	}
	// Treat reconnect as a fresh room entry for the disposition
	// system (spec §5.2 — the player effectively just walked back
	// in). Publish player.moved with From=To so any prior per-room
	// state for this player is cleared, then run both hooks around
	// the re-render. Capture the room once: a.Room() can in
	// principle change between calls (reconnect runs before pump,
	// so the window is near-zero today, but the snapshot keeps the
	// immediate/deferred pair consistent regardless).
	room := a.Room()
	if room != nil {
		if cfg.Bus != nil {
			cfg.Bus.Publish(ctx, eventbus.PlayerMoved{
				PlayerID: a.PlayerID(),
				From:     room.ID,
				To:       room.ID,
			})
		}
		if cfg.Disposition != nil && a.PlayerID() != "" {
			cfg.Disposition.OnPlayerEnteredImmediate(ctx, a.PlayerID(), a.Name(), nil, room.ID)
		}
	}
	if rendered := renderRoomForReconnect(a, cfg); rendered != "" {
		_ = a.Write(ctx, rendered)
	}
	// M16.4b: emit a Room.Info frame so the new peer's mapper
	// panel has a baseline. Goes through sendGmcpRoomInfo's
	// nil/no-GMCP guards so a non-GMCP client just sees no
	// effect.
	a.sendGmcpRoomInfo(ctx, room)
	if room != nil && cfg.Disposition != nil && a.PlayerID() != "" {
		cfg.Disposition.OnPlayerEnteredDeferred(ctx, a.PlayerID(), a.Name(), nil, room.ID)
	}

	exit := pump(ctx, c, cfg, a, clk)
	dispatchTeardown(ctx, cfg, a, exit, clk)
	return nil
}

// managerLocator adapts *Manager to command.Locator. The adapter
// exists for one reason only: Manager.FindInRoom returns *connActor
// (an unexported type), and assigning a typed-nil *connActor into a
// command.Actor interface yields a non-nil interface — a classic Go
// pitfall. Routing through this adapter lets us return an untyped
// nil interface when the lookup misses.
type managerLocator struct{ m *Manager }

func (ml managerLocator) FindInRoom(roomID world.RoomID, name string) command.Actor {
	a := ml.m.FindInRoom(roomID, name)
	if a == nil {
		return nil
	}
	return a
}

// PlayersInRoom satisfies command.Locator (M17.2d₄): every live player
// actor in roomID, as command.Actor, for the §5 entity/player/visible
// resolvers. The connActor → command.Actor widening drops nothing the
// resolvers need (Name + PlayerID). BuildResolveContext filters out the
// requesting actor itself.
func (ml managerLocator) PlayersInRoom(roomID world.RoomID) []command.Actor {
	actors := ml.m.roomConnActors(roomID)
	out := make([]command.Actor, 0, len(actors))
	for _, a := range actors {
		out = append(out, a)
	}
	return out
}

// otherPlayerNames returns the names of players in roomID other than
// selfID, for the room render's "You see here:" line. nil-safe on a nil
// Manager (test/headless paths).
func otherPlayerNames(m *Manager, roomID world.RoomID, selfID string) []string {
	if m == nil {
		return nil
	}
	var out []string
	for _, p := range m.PlayersInRoom(roomID) {
		if p.ID == selfID {
			continue
		}
		if p.Name != "" {
			out = append(out, p.Name)
		}
	}
	return out
}

// writeLine is a tiny helper for raw-conn writes that don't need the
// actor's color rendering (used before an actor exists, e.g. the
// "already online" rejection).
func writeLine(ctx context.Context, c conn.Connection, s string) error {
	_, err := c.Write(ctx, []byte(s+"\r\n"))
	return err
}

func resolveStartRoom(cfg Config, savedLoc string) (*world.Room, error) {
	if savedLoc != "" {
		if r, err := cfg.World.Room(world.RoomID(savedLoc)); err == nil {
			return r, nil
		}
	}
	return cfg.World.Room(cfg.StartID)
}

// connActor adapts a conn.Connection to the command.Actor interface and
// carries the player save record so SetRoom can mark it dirty.
//
// The manager back-reference is set by Manager.Add and used by SetRoom
// to keep the per-room broadcast index synchronized. Lock order is
// Manager → actor: the broadcast path (which takes actor locks via
// Write) snapshots its recipient list under the Manager lock and then
// releases before writing. SetRoom releases the actor lock before
// calling moveRoom for symmetry.
//
// playerID / accountID are immutable shadows of the save fields,
// cached at construction so the manager can read them lock-free
// during snapshot iteration. The Save record itself is only mutated
// for Location today.
type connActor struct {
	id   string
	conn conn.Connection

	// renderer is the M10.2 themed color renderer, captured at actor
	// construction from cfg.Render. Write runs every outbound line
	// through it (RenderAnsi when color is enabled, RenderPlain
	// otherwise). Nil-safe: when nil (tests that don't wire it) Write
	// falls back to the minimal M2 ansi.Render. Read-only after
	// construction, so it needs no lock.
	renderer *render.ColorRenderer

	playerID  string
	accountID string

	players *player.Store

	// prof is the M9.1 ProficiencyManager reference captured at
	// actor construction. Persist snapshots the actor's
	// abilities into save before write; logout drops in-memory
	// state. Nil-safe (mirrors the cfg.Proficiency nil-safety):
	// when nil, abilities neither persist nor flush.
	prof *progression.ProficiencyManager

	// known is the per-character known-recipe manager reference
	// (crafting-and-cooking §7, §9), captured at construction. Persist
	// snapshots the actor's known recipes into save before write; logout
	// drops in-memory state. Nil-safe (mirrors prof): when nil, recipes
	// neither persist nor flush.
	known *recipe.KnownManager

	// combat is the engage/disengage manager reference (M9.4b),
	// captured so the ResolutionSource seam can answer InCombat /
	// CurrentTarget for the ability-resolution phase. Nil-safe:
	// when nil (tests that don't wire combat) both report
	// not-in-combat / no-target.
	combat *combat.Manager

	// combatLocator resolves a CombatantID to a live Combatant for
	// the M16.4d Char.Combat flusher (needs the target's name +
	// Vitals). Wired from Config.CombatLocator at construction.
	// Read-only after construction; safe lock-free. nil-safe: the
	// flusher only ships the in_combat flag when this is nil.
	combatLocator combat.Locator

	// effects is the M9.2 active-effect manager reference, captured
	// here so the M16.4e Char.Effects flusher can snapshot active
	// effects without holding cfg in scope. Wired from
	// Config.Effects at construction. Read-only after construction;
	// safe lock-free. Nil-safe: the flusher returns early when nil.
	effects *progression.EffectManager

	// progression is the M8.2 XP/level manager reference, captured
	// here so the M16.4f Char.Experience flusher can enumerate
	// registered tracks and call GetTrackInfo without holding cfg
	// in scope. Wired from Config.Progression at construction.
	// Read-only after construction; safe lock-free. Nil-safe: the
	// flusher returns early when nil.
	progression *progression.Manager

	// race is the resolved *progression.Race (M9.4b), captured at
	// applyRace so the ResolutionSource seam can supply it to
	// AdjustCost for race-adjusted ability costs (spec §4.7). Nil
	// when the actor is raceless or the race isn't registered;
	// AdjustCost handles the nil case.
	//
	// Write-before-publish: set in applyRace (during construction,
	// before cfg.Manager.Add makes the actor reachable) and never
	// reassigned. Race() reads it lock-free from the tick goroutine;
	// the happens-before edge is Manager.mu, which both Add (writer
	// side) and the driver's mgr.GetByPlayerID lookup (reader side)
	// acquire. Same publish discipline as raceID / racialTags.
	race *progression.Race

	// class is the resolved PRIMARY *progression.Class (the first class id),
	// captured at applyClass for the generated player description (look
	// appearance lens). Read lock-free under the write-before-publish
	// discipline as race: set during construction before cfg.Manager.Add
	// makes the actor reachable. SetClass re-resolves it under a.mu on a
	// quest class-swap (the one reassignment path). Nil when classless or
	// the class isn't registered.
	class *progression.Class

	// classes is the class registry, captured at applyClass so weapon
	// proficiency (weapon-identity §3) can resolve the actor's CURRENT
	// class by classID at check time — this sidesteps the SetClass
	// staleness of the lock-free a.class pointer (SetClass updates
	// classID but never reassigns a.class). Set-once at construction;
	// read under a.mu. Nil for test/headless actors that skip applyClass.
	classes *progression.ClassRegistry

	// feats is the EPIC S4 feat registry (Config.Feats), used to resolve the
	// actor's known_feats into conferred bonuses. Set-once at construction;
	// read under a.mu. Nil for test/headless actors.
	feats *feat.Registry

	// languages is the tongue registry (Config.Languages), resolving the
	// actor's known-language ids to display names for `score` + the `languages`
	// listing. Set-once at construction; read under a.mu. Nil for test/headless
	// actors (ids render verbatim).
	languages *progression.LanguageRegistry

	// grades is the masterwork quality-grade registry (Config.Grades),
	// captured at applyClass so NonProficientArmorCheckPenalty can reduce a
	// worn piece's check penalty by its grade live — the same grade reduction
	// the equip path applies to the `armor_check` stat (masterwork §3). Read
	// under a.mu. Nil for test/headless actors → no reduction.
	grades *grade.Registry

	// lastAbility is the spec §4.5 step 2 "last ability used"
	// property, recorded by the resolver on every resolution.
	// In-memory only today (not persisted) — it's a transient
	// combat-feedback property, not durable character state.
	// Guarded by a.mu.
	lastAbility string

	// items is the runtime entity store reference, captured at actor
	// construction so syncInventoryToSaveLocked can resolve template
	// ids without holding cfg in scope. Never reassigned after Add.
	items *entities.Store
	// contents is the container↔item index reference, captured at
	// actor construction so syncInventoryToSaveLocked can walk
	// container trees and untrackInventory can sweep child entries.
	// Nil only in tests that don't exercise containers.
	contents *entities.Contents

	// placement + light are captured at construction so sendGmcpRoomInfo
	// can compute this viewer's effective light level for the Room.Info
	// `light` field (light-and-darkness §8) without threading cfg
	// through the SetRoom seam. Both nil-safe: a nil light resolver
	// omits the field (the room reads as fully lit).
	placement *entities.Placement
	light     *light.Resolver

	// inventory holds the runtime entity ids the actor is carrying.
	// Mutations go through AddToInventory / RemoveFromInventory and
	// flip the dirty bit so autosave commits the new contents.
	//
	// Invariant: an item id is in EXACTLY ONE of {inventory, equipment,
	// Placement room buckets} at any time. The get/drop/equip/unequip
	// paths pair their mutations so this never has to be reconciled
	// after the fact. Teardown only untracks ids from inventory +
	// equipment; Placement entries persist (items left in rooms stay).
	inventory []entities.EntityID
	// equipment maps slot key → equipped entity id. Slot keys are the
	// strings produced by slot.BuildKey: bare name for cap-1 slots,
	// "name:index" for multi-cap. A spanning item (two-handed weapon)
	// appears under several keys, all mapping to the same id.
	equipment map[string]entities.EntityID
	// footprints maps an equipped item id to all slot keys it occupies,
	// the target (canonical/save) key first (inventory-equipment-items
	// §3.3). A non-spanning item has a single-key footprint; a spanning
	// item appears under several equipment keys but exactly one footprints
	// entry — so modifiers apply once, the save writes one entry (the
	// target key, §3.8), and unequip frees every key at once (§3.5 step 2).
	// Maintained in lockstep with equipment under a.mu.
	footprints map[entities.EntityID][]string
	// weapon is the cached wielded-weapon snapshot fed into
	// combat.Stats (combat §4.5). Recomputed under a.mu whenever
	// equipment changes (equip / unequip / login respawn) and read
	// lock-free by Stats() on the combat tick goroutine — an
	// atomic.Pointer keeps that read off a.mu so combat never blocks on
	// a session-side equip. nil means "no weapon → unarmed default".
	weapon atomic.Pointer[weaponInfo]
	// offWeapon is the cached off-hand weapon snapshot for a dual-wielding
	// actor (two-weapon-fighting §2) — the weapon in the `offhand` slot when it
	// is a DISTINCT weapon from the main hand (not a spanning two-hander, whose
	// id appears under both slot keys). Recomputed under a.mu alongside `weapon`
	// and read lock-free by Stats(); nil means no off-hand weapon. Stats() grants
	// the off-hand attack only when this resolves the LIGHT wield mode (§2.2).
	offWeapon atomic.Pointer[weaponInfo]
	// armorResist caches the actor's aggregated per-damage-type resistance
	// from worn armor (armor-depth §4), recomputed on equip/unequip/login
	// (recomputeWeaponLocked, lock-held) and read LOCK-FREE in Stats() —
	// the same atomic.Pointer discipline as `weapon`, so the combat hot
	// path never enumerates the live equipment map under a.mu. nil = no
	// resistances. The stored map is never mutated after Store (copy-on-
	// recompute), so readers may share it without copying.
	armorResist atomic.Pointer[armorResistances]
	// armorDexCap is the most restrictive (lowest) max-Dex cap across worn
	// armor (armor-depth §3), recomputed on equip/unequip/login alongside
	// armorResist and read LOCK-FREE in Stats() to derive the synthetic
	// `dex_ac` channel input — same atomic discipline as `weapon`. nil = no
	// cap (the full Dex modifier counts toward AC, the d20 unarmored case).
	armorDexCap atomic.Pointer[int]
	// armorTiers caches the distinct non-empty armor tiers currently worn
	// (armor-depth §5), recomputed alongside armorDexCap and read by
	// IsArmorProficient (off a.mu, on the combat goroutine via HitModAdjust).
	// nil = no tiered armor worn. The stored slice is never mutated after
	// Store (copy-on-recompute), so readers may share it.
	armorTiers atomic.Pointer[[]string]
	// wornReputation caches the summed signed reputation delta of worn/visible
	// gear (special-weapons §8 / reputation.md §7 — masterwork +1, Trolloc
	// scythesword −2). Recomputed alongside armorResist over the same distinct-id
	// equipment pass, read lock-free by EffectiveRenown / RenownTier. 0 = no gear
	// reputation. The renown sibling of armorResist.
	wornReputation atomic.Int64
	// wornArmorBonus caches the summed AC contribution of worn armor (the defender
	// armor rating the whip anti-armor gate reads — subdual-damage §6). Recomputed
	// alongside armorResist over the same distinct-id equipment pass, read lock-free
	// by Stats() into combat.Stats.ArmorRating. 0 = unarmored. Intrinsic natural
	// armor is not folded in (v1 = worn armor only).
	wornArmorBonus atomic.Int64
	// featWeaponBonus caches the per-weapon-category feat hit/crit bonuses
	// (EPIC S4 Phase 3c), recomputed on feat change (applyFeatGrants) and read
	// LOCK-FREE in Stats() — same atomic.Pointer discipline as `weapon`, so the
	// combat hot path never blocks on a.mu. nil = no such feats.
	featWeaponBonus atomic.Pointer[featWeaponBonuses]
	// statBlock is the actor's progression-layer stat block (M8.1 —
	// docs/specs/progression.md §2). Holds base attributes (the six
	// classics + vital maxima + the combat-derived hit_mod / ac slots
	// today) plus the holder's sourced modifier set. Equipment
	// modifiers apply under EquipmentSourceKey(item.ID()); on save
	// the base and modifier snapshots are mirrored into a.save; on
	// login they're restored before respawnEquipment rewires source
	// keys onto fresh entity ids.
	//
	// statBlock subsumes the M5.6 stats.Block: the modifier-set
	// surface lives inside StatBlock and the persisted shape
	// (stats.Snapshot) round-trips unchanged. The pointer is
	// established at construction and never reassigned for the life
	// of the actor; StatBlock carries its own internal RWMutex so
	// combat tick reads of Stats() do not serialize on a.mu.
	statBlock *progression.StatBlock

	// attrSet is the character's resolved base attribute set (SR-M1 — the same
	// set that seeded statBlock), held so `score` renders the world's declared
	// attributes in order rather than a hardcoded six. nil when no attribute
	// content resolved (a degenerate boot); the score sheet falls back to the
	// classic six then. Immutable after construction.
	attrSet *progression.AttributeSet

	// vitals is the actor's mutable HP state (M7.1). The pointer is
	// established at login and never reassigned; combat applies damage
	// and heals through the pointer under its own lock. Persistence
	// landed with M7.5 (player.Save.Vitals).
	vitals *combat.Vitals

	// pools holds the actor's generalized resource pools (mana, movement)
	// alongside HP. HP stays in vitals (the alongside route); these route
	// through pool.Set so DeductMana/DeductMovement are real and a future
	// channeling pool (the One Power) is just another entry. Seeded full
	// from the stat maxes in the constructor; nil in bare test-built
	// actors (the accessors are nil-safe).
	pools *pool.Set

	// poolDecls is the active world's player-seed pool declarations (SR-M3a),
	// resolved once from Config.Pools at construction and consumed by
	// seedResourcePools to build `pools`. Empty/nil (a bare test actor or a boot
	// with no pool content) makes seedResourcePools fall back to the hardcoded
	// mana/movement pair — the same nil-content fallback SR-M1 uses for the
	// attribute set. Kind-sorted (registry order) so seeding is deterministic.
	poolDecls []*pool.Decl

	// channelMap is the active ruleset's stat→combat-channel derivation,
	// set from Config.ChannelMap at construction (nil in bare test-built
	// actors). When set, Stats() derives HitMod/AC through it; nil reads
	// the stat keys directly. The baseline mapping reproduces those reads,
	// so both paths yield identical numbers. Read lock-free by Stats() on
	// the tick goroutine (immutable after construction).
	channelMap *channel.Mapping

	// unarmedSubdual makes an UNARMED player's strikes nonlethal (subdual-damage
	// §6 — fists knock out rather than kill, the faithful d20/WoT default). Set
	// from Config.UnarmedSubdual at construction; false in bare test-built actors
	// (so existing unarmed combat tests stay lethal). Read lock-free by Stats() on
	// the tick goroutine. Only the unarmed branch (no wielded weapon) reads it — a
	// wielded weapon's own `subdual` flag governs when armed.
	unarmedSubdual bool

	// raceID is the canonical race id the actor was constructed
	// with (M8.3). Established at login from save (or the
	// configured default for legacy v7 saves / fresh characters)
	// and never reassigned for the life of the actor — race-change
	// at runtime is a M10+ admin verb. Lowercased on assignment.
	raceID string

	// racialTags is the snapshot of racial-flag strings the race
	// definition contributed at construction. Stored alongside
	// raceID so Tags() can surface them without re-resolving the
	// registry on every read. Set once at construction.
	racialTags []string

	// classIDs is the actor's resolved class id list (wot-character-model
	// D1 — multi-track-as-multiclass). One class today (single element); the
	// list lets a future second class-track be additive without another save
	// migration. Established at login from save; empty means classless (the
	// path processor and stat-growth subscriber short-circuit). Lowercased on
	// assignment for case-insensitive registry lookups. The primary (first)
	// class is what single-value readers (ClassID/Class, score, GMCP, quest
	// gate) surface; composing readers (Saves, IsWeaponProficient) walk all.
	classIDs []string

	// backgroundID is the actor's creation origin id (backgrounds §5). Set at
	// login from the save; empty = background-less. The starting package the
	// background granted (skills/items/gold) lives in the proficiency/
	// inventory/gold state, not here — this is the label for display.
	backgroundID string

	// weaponRestrictions are the weapon-category ids the actor's background
	// forbids wielding (backgrounds.md §Restrictions — the Aiel sword taboo),
	// DERIVED from the background registry at login (not persisted — the
	// background id is the single source of truth). weaponRestrictionMessage is
	// the in-character refusal. Both set-once before the actor is published, so
	// the equip gate reads them lock-free (like backgroundID/raceID).
	weaponRestrictions       []string
	weaponRestrictionMessage string

	// trainsAvailable is the actor's training pool (spec §4.6
	// step 4 + §7.1). M8.4 credits via StatGrowthSubscriber on
	// every level-up; the M8.6 train verb is the only consumer.
	// Guarded by a.mu since the credit happens off the bus
	// dispatch and Persist also reads it.
	trainsAvailable int

	// featCredits is the actor's banked-but-unspent feat slots (EPIC S4
	// Phase 2 — docs/proposals/wot-feats.md §2.2). Credited 1 at character
	// creation + 1 per 3 character levels (off the bus dispatch, same as
	// trainsAvailable); the feat verb (Phase 4) is the only consumer.
	// Guarded by a.mu since CreditFeats and Persist both touch it.
	featCredits int

	// alignment is the actor's integer alignment property
	// (M8.5 — spec progression.md §6.1). Written through
	// AlignmentManager.Set / Shift; read by AI disposition
	// matching and by Persist via syncAlignmentToSaveLocked.
	// Guarded by a.mu — same actor-lock discipline as
	// trainsAvailable.
	alignment int

	// alignmentTag is the spec §6.2 bucket tag mirrored onto
	// the actor (one of TagAlignmentEvil/Neutral/Good or empty
	// for an actor the alignment manager has not touched yet).
	// Tags() appends it to racialTags so the AI evaluator's
	// PlayerView carries it for has_tag matchers.
	alignmentTag string

	// faction is the S8 faction manager (faction.md), retained so consumers
	// like the ability faction gate (MeetsFactionStanding) can resolve the
	// actor's EFFECTIVE standing (starting-substituted) through the registry.
	// nil when faction isn't wired → those gates fail open.
	faction *faction.Manager

	// factionStanding is the actor's per-faction standing bag (faction.md
	// §3.1): faction id → signed standing. Loaded from save.FactionStanding,
	// written through the faction.Entity adapter (SetStanding) which mirrors
	// into a.save.FactionStanding. Guarded by a.mu.
	factionStanding map[string]int
	// factionRankTags mirrors the current rank tag per touched faction
	// (faction.md §3.3): faction id → "faction:<id>:<rank>". Derived from
	// standing on login and on every Shift; Tags()/HasTag fold the values in
	// so disposition rules can match on them. Guarded by a.mu.
	factionRankTags map[string]string

	// reputation is the single-axis renown manager (reputation.md), retained so
	// future consumers (a score line, the recognition check, a disposition
	// reaction) can resolve renown/tier through it. nil when unwired.
	reputation *reputation.Manager
	// renown is the actor's single renown score (reputation.md §10): fame +,
	// infamy −, Unknown 0. Restored from save.Reputation on login, written
	// through SetRenown (which mirrors into the save). Guarded by a.mu.
	renown int
	// reputationTierTag is the current renown tier tag ("renown:<slug>"),
	// derived from renown on login and every Shift; Tags()/HasTag fold it in.
	// Derived state — never persisted (re-synced on login). Guarded by a.mu.
	reputationTierTag string

	// gold is the actor's integer currency balance (M11.1 — spec
	// economy-survival §2.1). Written through economy.CurrencyService
	// (AddGold / SetGold) via the SetGold adapter below; read by the
	// `gold` verb and the auto-convert hook. Guarded by a.mu — same
	// actor-lock discipline as alignment; SetGold mirrors into
	// a.save.Gold under the lock so Persist needs no separate sync
	// step (the value is always already in the save when the dirty
	// bit is set).
	gold int

	// sustenance is the actor's hunger pool in [0, 100] (M11.3 — spec
	// economy-survival §4.1). Written through economy.SustenanceService
	// (Set / Add / Drain) via the SetSustenance adapter below; read by
	// the drain world-tick subscriber and (M11.5) the regen heartbeat.
	// Guarded by a.mu — same write-through-to-save discipline as gold.
	sustenance int

	// lastHungerReminderTick is the engine tick at which the most
	// recent hunger reminder was sent to this player. The drain
	// subscriber consults it to throttle reminders to at most one per
	// SustenanceConfig.ReminderIntervalTicks (spec §4.4) so a hungry
	// player isn't nudged on every drain tick. Zero means "never
	// reminded" — the first reminder fires immediately. Guarded by a.mu.
	lastHungerReminderTick uint64

	// restState / restTargetID / sleepStartTick are the M11.4 rest
	// machine's transient fields (spec economy-survival §5.1/§5.2).
	// Written through economy.RestService via the RestEntity adapter
	// below; read by the M11.5 regen heartbeat. ALL THREE ARE TRANSIENT
	// — they are never synced to the save (the setters do not mark
	// dirty), so a disconnect while resting/sleeping restores as awake
	// (the zero-value "" normalizes to awake). Guarded by a.mu.
	restState      string
	restTargetID   string
	sleepStartTick uint64

	// craftPending / hasCraft are the B3 timed-craft occupation state
	// (crafting-and-cooking §3, "time — how long the craft occupies the
	// player"). hasCraft true means a craft is in flight; the craft-complete
	// tick finishes it when the engine tick reaches craftPending.ReadyAt.
	// TRANSIENT like the rest fields — never synced to the save, so a craft
	// in flight at logout/crash is simply lost (the lazy-completion model
	// reserves no inputs, so nothing is lost with it). Movement (SetRoom)
	// and combat (the engagement sink) cancel an in-flight craft; the
	// crafting service drives these via the crafting.CraftBusy adapter
	// below. Guarded by a.mu.
	craftPending crafting.PendingCraft
	hasCraft     bool

	// forageReadyAt is the engine tick the per-character forage cooldown
	// elapses (gathering.md §5). TRANSIENT like craftPending/rest — never
	// synced to the save, so a relog clears the cooldown (acceptable; it's
	// anti-spam, not durable state). Guarded by a.mu.
	forageReadyAt uint64

	// loadedWeapon is the entity id of the currently-loaded reload-gated
	// projectile (a crossbow), or "" when nothing is chambered
	// (action-economy.md §7.1). A shot is allowed only when this matches the
	// wielded weapon, so swapping weapons drops the loaded state. TRANSIENT —
	// never persisted; a relog leaves you unloaded. The in-flight RELOAD is the
	// action.Tracker's busy state, not a field here. Guarded by a.mu.
	loadedWeapon entities.EntityID

	// progress is the actor's progression-track state (M8.2 —
	// docs/specs/progression.md §5.2). Holds per-track (level, xp)
	// maps; mutated through progression.Manager operations and
	// persisted in player.Save.Progression. The pointer is set at
	// construction and never reassigned; ProgressionState carries
	// its own internal lock so reads from a tick goroutine don't
	// serialize on a.mu.
	progress *progression.ProgressionState

	// wimpyThreshold is the §5.1 HP%-threshold property. 0 disables
	// wimpy. Set by the `wimpy <pct>` verb; read by combat's wimpy
	// phase via the WimpyHolder interface (defined on combat package
	// side; this connActor satisfies it). Persistence lives with the
	// rest of the save shape — see player.Save.WimpyThreshold.
	//
	// Stored as atomic.Int32 so the read path (tick goroutine, every
	// combat round) does NOT take a.mu. Persist holds a.mu across the
	// autosave file write; routing every wimpy read through that lock
	// would stall the combat tick for the duration of disk I/O. The
	// write path still acquires a.mu because it also mutates a.save
	// — but the field-level read stays lock-free.
	wimpyThreshold atomic.Int32

	// autoloot is the loot-and-corpses §6 per-character preference (off
	// by default). Read by the autoloot corpse.created subscriber on the
	// tick goroutine; stored as atomic.Bool so that read stays lock-free
	// (same rationale as wimpyThreshold). The write path takes a.mu to
	// also mutate a.save. Persistence: player.Save.Autoloot.
	autoloot atomic.Bool

	// powerAttack is the Power Attack combat stance (feats Bucket C — a melee
	// accuracy-for-power trade). Toggled by `powerattack on|off`; read by
	// Stats() every combat round, so stored as atomic.Bool to keep that read
	// lock-free (same rationale as wimpyThreshold). The write path takes a.mu to
	// also mutate a.save. Persistence: player.Save.PowerAttackActive (v27).
	powerAttack atomic.Bool

	// autoAssist is the grouping.md §9 auto-assist preference (off by
	// default). Read by the combat sink's OnEngagement hook on the tick
	// goroutine to decide whether to pull this member into a party-mate's
	// fight; stored as atomic.Bool so that read stays lock-free (same
	// rationale as wimpyThreshold). The write path takes a.mu to also mutate
	// a.save. Persistence: player.Save.AutoAssist.
	autoAssist atomic.Bool

	mu           sync.Mutex
	room         *world.Room
	colorEnabled bool

	// Concealment state (visibility.md §3.1, §7) — ephemeral, NEVER
	// persisted (cleared on logout/restart, like active effects). hidden
	// marks the actor as concealed via the `hide` verb; concealScore is the
	// snapshot difficulty an observer's perception contest must beat
	// (§4.2); concealInstance identifies this concealment establishment for
	// observers' sticky detection memory (§4.1) — it changes on each
	// re-hide so a remembered pierce keys off the right thing. All guarded
	// by a.mu.
	//
	// WRITER INVARIANT: every mutation (Hide/Reveal — via the hide/unhide
	// verbs, move-drops-hide, and S3b reveal-on-action) happens on THIS
	// actor's owning connection goroutine (command dispatch + the
	// synchronous player.moved subscriber). Cross-goroutine access is
	// READ-ONLY (the S3b visibility filter reading IsHidden/score/instance
	// from the tick goroutine), and every getter takes a.mu, so reads never
	// tear. The verb pre-checks (IsHidden then Hide/Reveal) are therefore a
	// benign message-only TOCTOU, not a data race — keep it that way: do NOT
	// add a tick-goroutine writer without making the check-and-act atomic.
	hidden          bool
	concealScore    int
	concealInstance uint64

	// Sneak concealment (visibility.md §3.2) — the MOVING counterpart to
	// hide: unlike hidden (which the move-drops-hide subscriber clears on a
	// room change), sneaking SURVIVES room changes and instead filters the
	// per-observer enter/leave movement lines (§3.2). sneaking marks the
	// actor as moving quietly; sneakScore is the snapshot difficulty an
	// occupant's perception contest must beat to see the movement line;
	// sneakInstance identifies this sneak establishment. Same writer
	// invariant + a.mu guard as the hide fields above (mutated only on this
	// actor's own goroutine: the sneak verb, the reveal-on-action hook;
	// NEVER persisted, cleared on logout/restart).
	sneaking      bool
	sneakScore    int
	sneakInstance uint64

	// Admin invisibility / "wizinvis" (visibility.md §3.4) — a flag-gated
	// (no contest, no score) concealment toggled by the `wizinvis` admin verb:
	// the actor is hidden from the room render, target resolution, and `who`
	// for any observer of LOWER admin rank, and — unlike hide/sneak — it does
	// NOT break on a revealing action (§4.5). Ephemeral, never persisted; same
	// a.mu guard + own-goroutine writer invariant as the hide/sneak fields.
	adminInvisible bool

	// contested is this actor's sticky detection memory AS AN OBSERVER
	// (visibility.md §4.1): instance id → contest outcome (true = pierced,
	// false = failed to pierce). Presence means "already contested this
	// instance this room" — the result is sticky for BOTH outcomes, so
	// CanSee never re-rolls (in either direction) until the concealment
	// re-establishes or the observer leaves. Invalidated (cleared) when the
	// observer changes rooms (SetRoom). Lazily allocated; guarded by a.mu.
	// Ephemeral, never persisted.
	contested map[uint64]bool

	// liveMounts tracks which of this character's owned mounts (mounts.md §2.2)
	// currently have a live creature in the world — entity id → mount template
	// id. Durable ownership lives on the save (save.Mounts); this is the
	// transient overlay distinguishing a stabled mount (record only) from a
	// retrieved one (record + a live MobInstance). Lazily allocated, guarded by
	// a.mu, never persisted: on logout every live mount is dematerialized and
	// the character returns to all-stabled (§9, §10). The mount-riding methods
	// live in mount.go.
	liveMounts map[entities.EntityID]string

	// liveHirelings tracks which of this character's owned hirelings
	// (hireable-mobs.md §2) currently have a creature materialized in the world
	// (entity id → template + order stance). Transient session overlay over the
	// durable save.Hirelings list; drained + dematerialized on logout (§9). The
	// stance (§8) is transient too — reset to follow on re-materialize. Guarded by a.mu.
	liveHirelings map[entities.EntityID]liveHireling

	// mountedOn is the entity id of the mount this character is currently
	// riding (mounts.md §4.3), or empty when on foot. Transient — the live
	// ride relationship is never persisted (§10); a logout/restart resolves
	// the rider to on-foot. Guarded by a.mu. While set, SetRoom relocates the
	// mount with the rider (co-located travel, §4.3, §5).
	mountedOn entities.EntityID

	// discoveredExits is this actor's per-room hidden-exit discovery memory
	// (hidden-exits §3.4): the directions whose hidden exit this character has
	// found via `search`. Direction-keyed (not concealment-instance-keyed like
	// contested) because exits are room-scoped edges, not entities — and the
	// whole set is invalidated on a room change, so a direction uniquely
	// identifies the exit within the current room. Lazily allocated; guarded
	// by a.mu. Ephemeral, never persisted (PD-3).
	discoveredExits map[world.Direction]bool
	// colorTier is the per-session capability ceiling captured from
	// the conn at construction (M16.6a). Sources:
	//   - telnet: derived from TTYPE + IsMudClient per spec §7.2.
	//   - websocket: always TrueColor per §6.5.
	//   - conn that doesn't implement the accessor (test fakes):
	//     render.ColorTierBasic — preserves the M0-era ANSI-16
	//     behavior so legacy fakes don't suddenly emit no-color.
	// Read-only after construction; safe lock-free. M16.6b will
	// wire tier-aware ANSI emission; M16.6a only captures and logs
	// the tier.
	colorTier render.ColorTier
	// Prompt-refresh state machine (session-lifecycle §2.5, M10.3b).
	// All three are guarded by a.mu.
	//   promptDisplayed    — the most recent send left a prompt at the
	//                        bottom of the screen.
	//   receivedInput      — input has arrived since that prompt.
	//   needsPromptRefresh — content was sent since the last prompt;
	//                        the end-of-tick flush should re-render.
	promptDisplayed    bool
	receivedInput      bool
	needsPromptRefresh bool
	save               *player.Save
	dirty              bool
	// lastAreaSeen is the area id of the room this actor was most
	// recently shown — the "from" of the area-transition zone-line
	// (player-maps §4, command.AreaTracker). Session-scoped and in
	// memory only: a restart re-narrates the first crossing, which is
	// harmless. Seeded at login spawn so the first move narrates a
	// real "from". Guarded by mu.
	lastAreaSeen world.AreaID
	// lastTellPartner is the display name of the most recent
	// counterparty in a tell conversation (set on both publish and
	// receive). The `reply` verb reads it. v1 in-memory only: a
	// server restart clears it. Guarded by mu. (Spec
	// chat-channels-and-tells §7.1.)
	lastTellPartner string
	// recall is the saved recall room id (recall.md §6, M15.3). Empty
	// = no recall point set. Guarded by mu — read/write only by the
	// verb path, never from the tick loop, so an atomic isn't worth
	// it. Hydrated from save.Recall at construction; SetRecall
	// updates both this field and save.Recall under the lock.
	recall string
	// roles is the actor's authorization role set (roles-and-permissions
	// §2). Keys are normalized (lowercased/trimmed) role names; a present
	// key means the role is held. Built at construction by applyRoles from
	// the saved roles + the config seed, and mutated by grant/revoke. A
	// SEPARATE namespace from gameplay tags (racialTags/alignmentTag) —
	// the two never cross. Guarded by a.mu (mutated at runtime, unlike the
	// set-once racial tags). nil/empty = unprivileged.
	roles map[string]struct{}
	// visited is the in-memory fog-of-war set (player-maps §3): the room
	// ids this character has entered, lazily built from save.VisitedRooms
	// on first use and kept in sync by markVisitedLocked. The persisted
	// authority is save.VisitedRooms; this is the O(1) membership index
	// over it. Guarded by a.mu; nil until first seeded.
	visited map[string]struct{}
	// seenAreas is the in-memory index over save.SeenAreas (player-maps
	// §4): the area ids this character has ever entered, gating the
	// once-ever first-entry banner. Lazily built on first use and kept
	// in sync by MarkAreaSeen. Guarded by a.mu; nil until first seeded.
	seenAreas map[world.AreaID]struct{}
	// gmcpLastVitals is the M16.4a poll-and-diff shadow for the
	// Char.Vitals package — the most recent snapshot the manager
	// emitted to the peer. The gmcp-vitals-flush tick handler
	// recomputes the current snapshot every tick and only sends
	// when it differs from this shadow. Guarded by gmcpVitalsMu
	// because the flush goroutine reads/writes it concurrently
	// with the actor's own mutators (write paths don't dirty-flag
	// — see the Manager.FlushGmcpVitals doc for why poll-and-diff
	// is preferred over instrumentation at every Vitals mutator).
	gmcpVitalsMu        sync.Mutex
	gmcpLastVitals      gmcp.CharVitals
	gmcpLastVitalsValid bool // false = never sent / reset; next flush emits unconditionally

	// gmcpItems* are the M16.4c per-location shadows for
	// Char.Items.List. Tracked per-location (inv / wear) so a
	// pure-inventory change skips the wear frame and vice versa.
	// Both slices are kept sorted by item id (entityIDsToCharItems)
	// so the diff compare is stable. The single valid flag covers
	// both — reset on link-dead reattach gives the new peer a
	// baseline frame for each location.
	gmcpItemsMu        sync.Mutex
	gmcpItemsLastInv   []gmcp.CharItem
	gmcpItemsLastWear  []gmcp.CharItem
	gmcpItemsLastValid bool

	// gmcpCombat* are the M16.4d shadow for Char.Combat. Single
	// snapshot per actor since each player has at most one primary
	// target. Reset on link-dead reattach gives the new peer a
	// baseline frame for the combat HUD.
	gmcpCombatMu        sync.Mutex
	gmcpLastCombat      gmcp.CharCombat
	gmcpLastCombatValid bool

	// gmcpEffects* are the M16.4e shadow for Char.Effects. Full
	// list per actor (sorted by id by EffectManager.Effects). The
	// diff compare runs over the slice; equality requires same
	// length and same per-row id/remaining/permanent/source/flag
	// tuple. Reset on link-dead reattach gives the new peer a
	// baseline frame for the effects panel.
	gmcpEffectsMu        sync.Mutex
	gmcpLastEffects      []gmcp.CharEffect
	gmcpLastEffectsValid bool

	// gmcpExperience* are the M16.4f shadow for Char.Experience.
	// Per-track list, ordered by TrackRegistry.All (sorted by name).
	// Equality requires same per-row track/level/xp/xpnext/at_max/
	// overflow tuple. Reset on link-dead reattach gives the new
	// peer a baseline frame for the XP-bar panel.
	gmcpExperienceMu        sync.Mutex
	gmcpLastExperience      []gmcp.CharExperienceTrack
	gmcpLastExperienceValid bool

	// gmcpCharStatus* are the M16.4h shadow set covering the
	// boot-identity packages: Char.Login + Char.StatusVars are
	// emit-once-then-watch (re-emit on link-dead reattach);
	// Char.Status is poll-and-diff like the rest. The single mutex
	// guards all three valid flags + the last Status snapshot so
	// the flusher can run a one-shot login/vars-emit branch in the
	// same critical section as the diff compare.
	gmcpCharStatusMu    sync.Mutex
	gmcpLoginSent       bool
	gmcpStatusVarsSent  bool
	gmcpLastStatus      gmcp.CharStatus
	gmcpLastStatusValid bool
	// recentTells is a session-scoped ring of recently-received tell
	// lines for the `tells` verb (a brief review of what scrolled past).
	// In-memory only. Capped by tellsSessionHistoryCap. Guarded by mu.
	recentTells []string
	// saveGen is incremented on every mutation that flips dirty. Persist
	// captures the value at snapshot time and only clears dirty if the
	// counter hasn't advanced — guards against a concurrent equip /
	// inventory mutation getting lost because the in-flight Save wrote
	// stale state and then cleared dirty on completion. Comparing
	// individual fields (the M3-era approach, which compared only
	// Location) doesn't scale as the save shape grows.
	saveGen uint64
	manager *Manager
	// trades is the direct-trade session manager (direct-trade.md), set once
	// at construction and never mutated, so it is read without a.mu. nil
	// disables trading. SetRoom / teardown use it to cancel an open trade on
	// separation / disconnect. The cancel is invoked WITHOUT holding a.mu
	// (lock order: trade.Manager.mu → actor.mu).
	trades        *trade.Manager
	flood         *floodGate
	gmcpFlood     *floodGate   // inbound-GMCP rate limit, separate from the command gate
	floodCfg      *FloodConfig // retained so reattach() can rebuild a fresh bucket
	clk           clock.Clock  // retained for the same reason
	lastInputAt   time.Time
	idleWarned    bool
	disconnecting bool         // set when teardown (idle, flood, etc.) is in flight
	phase         sessionPhase // playing | linkDead | tearing (M4.4)
	linkDeadAt    time.Time    // when the actor entered linkDead phase
	takenOver     bool         // §6.1/§8 stale-event guard: stale teardown is a no-op
}

func (a *connActor) ID() string { return a.id }

func (a *connActor) Name() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return ""
	}
	return a.save.Name
}

// PlayerID is the immutable account-scoped identity of the character.
// Read lock-free from the shadow field so manager snapshots don't have
// to take the actor mutex.
func (a *connActor) PlayerID() string { return a.playerID }

func (a *connActor) Room() *world.Room {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.room
}

func (a *connActor) SetRoom(r *world.Room) {
	a.mu.Lock()
	var oldID world.RoomID
	if a.room != nil {
		oldID = a.room.ID
	}
	a.room = r
	if a.save != nil {
		a.save.Location = string(r.ID)
		a.markDirtyLocked()
	}
	// Fog-of-war exploration hook (player-maps §3): mark the destination
	// visited. SetRoom is the single room-change chokepoint, so this
	// covers every arrival — movement, recall, teleport, login spawn,
	// link-dead reattach — without each call site opting in (PD-5).
	a.markVisitedLocked(string(r.ID))
	// Movement breaks an in-flight timed craft (crafting-and-cooking §3):
	// you can't carry a half-forged blade between rooms. SetRoom is the
	// single room-change chokepoint, so this covers move/recall/teleport/
	// flee without each site opting in. Only on an actual change — a
	// same-room SetRoom (link-dead reattach, re-render) leaves the craft
	// running. Captured under the lock; the notice writes after release.
	craftBroke := false
	if oldID != r.ID && a.hasCraft {
		a.craftPending = crafting.PendingCraft{}
		a.hasCraft = false
		craftBroke = true
	}
	// Leaving a room invalidates this observer's sticky detection memory
	// (visibility.md §4.1): the rogue you spotted last room must be
	// re-contested here. The mover's OWN hide is dropped by the player.moved
	// subscriber (move-drops-hide), not here.
	if oldID != r.ID {
		a.clearDetectionLocked()
	}
	// Co-located mounted travel (mounts.md §4.3, §5): while ridden, the mount
	// relocates WITH its rider through whatever moved them — travel, recall,
	// teleport, flee — so the never-strand rule holds across every relocation
	// path, not just the walk verb. The mount is a world entity in placement,
	// so moving its placement entry is the whole of co-location. Captured under
	// the lock; the placement write (its own lock) runs after release.
	ridden := a.mountedOn
	placement := a.placement
	mgr := a.manager
	a.mu.Unlock()

	// Leaving the room ends any open direct trade — you can't trade with
	// someone you walked away from (direct-trade.md §6). Fired AFTER the
	// a.mu release (lock order: trade.Manager.mu → actor.mu) and only on an
	// actual room change. CancelFor is idempotent / a no-op when not trading.
	if a.trades != nil && oldID != r.ID {
		a.trades.CancelFor(context.Background(), a.playerID, fmt.Sprintf("%s left", a.Name()))
	}

	if craftBroke {
		_ = a.Write(context.Background(), "You set your work aside and move on.")
	}

	if ridden != "" && placement != nil && oldID != r.ID {
		placement.Place(ridden, r.ID)
	}

	if mgr != nil && oldID != r.ID {
		mgr.moveRoom(a, a.playerID, oldID, r.ID)
	}
	// M16.4b: emit a Room.Info GMCP frame on every transition so
	// Mudlet's mapper module sees the move. context.Background()
	// because SetRoom is called from many sites that don't thread
	// a ctx (the actual GMCP write is a quick fire-and-forget
	// through the conn's own write mutex). No-op when GMCP isn't
	// negotiated or the conn doesn't speak it.
	a.sendGmcpRoomInfo(context.Background(), r)
}

// nextConcealInstance hands out a process-unique id for each concealment
// establishment (visibility.md §4.1) so an observer's sticky detection
// memory keys off the right thing — a re-hide is a new instance and forces
// a fresh perception contest. Atomic: hide can fire from any goroutine.
var nextConcealInstance atomic.Uint64

// Hide marks the actor concealed with the given perception-contest
// difficulty (visibility.md §3.1), allocating a fresh concealment instance
// and returning it. Ephemeral state — never persisted. Re-hiding overwrites
// the score and bumps the instance, invalidating observers' prior pierces.
func (a *connActor) Hide(score int) uint64 {
	inst := nextConcealInstance.Add(1)
	a.mu.Lock()
	a.hidden = true
	a.concealScore = score
	a.concealInstance = inst
	a.mu.Unlock()
	return inst
}

// Reveal clears the actor's hide concealment, returning whether it had been
// hidden (so the caller can decide whether to emit entity.revealed and a
// message). Idempotent.
func (a *connActor) Reveal() bool {
	a.mu.Lock()
	was := a.hidden
	a.hidden = false
	a.concealScore = 0
	a.concealInstance = 0
	a.mu.Unlock()
	return was
}

// Feat skill-bonus axes wired into the live perception/stealth check sites
// (feats Phase 1). The WoT detection/concealment feats fold onto the engine's
// two composite skill axes: perception (Alertness, Sharp-Eyed) raises the
// observer's contest, stealth (Stealthy) raises the hider's concealment. These
// are bonus-channel keys for FeatSkillBonus, distinct from proficiency abilities.
const (
	skillPerception = "perception"
	skillStealth    = "stealth"
)

// Skill-ability ids (skills §2 — EPIC S3) whose use-based proficiency folds
// into the visibility perception contest (§4.2). Distinct from the feat-bonus
// axes above: these key the ProficiencyManager, those key FeatSkillBonus. The
// perception SKILL id intentionally equals the perception FEAT axis string —
// both name awareness, but they are read from different systems and their
// contributions stack additively in PerceptionBonus.
const (
	skillAbilityHide         = "hide"
	skillAbilityMoveSilently = "move-silently"
	// Aliases the perception feat axis so the two systems' shared "perception"
	// string is enforced by the compiler, not by convention — renaming one
	// without the other can't silently break the feat+proficiency stacking.
	skillAbilityPerception = skillPerception
)

// skillProficiency reads the actor's current 1–100 proficiency in a skill
// ability (0 when untrained or no proficiency manager is wired). The
// ProficiencyManager carries its own lock, and playerID is set once at
// construction, so this takes no actor lock.
func (a *connActor) skillProficiency(abilityID string) int {
	if a.prof == nil {
		return 0
	}
	prof, _ := a.prof.Proficiency(a.playerID, abilityID)
	return prof
}

// HideScore computes the would-be concealment difficulty for a hide
// attempt (visibility.md §3.1 / §8: proficiency + governing stat + mods).
// v1 is a base plus the actor's DEX modifier — stealthy/agile characters
// hide better; the proficiency + situational (cover/light) terms are a
// later tuning pass (§8). Exposed so the `hide` verb sets a score it cannot
// compute itself (it has no stat access through the Actor interface).
func (a *connActor) HideScore() int {
	const baseHideDC = 10
	a.mu.Lock()
	sb := a.statBlock
	a.mu.Unlock()
	if sb == nil {
		return baseHideDC // no stats wired (defensive; the player path always has them)
	}
	// FeatSkillBonus takes a.mu itself; safe now that the read above unlocked.
	// Hide proficiency (skills §2) folds into the concealment difficulty via
	// SkillBonus (= proficiency term + the Dex modifier); at proficiency 0 this
	// equals the bare Dex modifier — the pre-skill behavior. The Stealthy feat
	// axis stays additive on top.
	prof := a.skillProficiency(skillAbilityHide)
	return baseHideDC + progression.SkillBonus(prof, sb.Effective(progression.StatDEX), progression.DefaultSkillConfig()) + a.FeatSkillBonus(skillStealth)
}

// IsHidden reports whether the actor is currently hide-concealed
// (visibility.md §3.1). Used by the visibility filter's target side.
func (a *connActor) IsHidden() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.hidden
}

// ConcealmentScore returns the snapshot perception-contest difficulty of
// the actor's current hide (visibility.md §4.2); zero when not hidden.
func (a *connActor) ConcealmentScore() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.concealScore
}

// HiddenInstance returns the id of the actor's current concealment
// establishment (visibility.md §4.1); zero when not hidden.
func (a *connActor) HiddenInstance() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.concealInstance
}

// --- Sneak (visibility.md §3.2): moving concealment.

// Sneak marks the actor as moving quietly with the given per-observer
// contest difficulty, allocating a fresh concealment instance and returning
// it. Reuses nextConcealInstance so a sneak and a hide can never share an
// instance id (the observer's sticky-detection map keys both). Ephemeral —
// never persisted. Re-sneaking overwrites the score and bumps the instance.
func (a *connActor) Sneak(score int) uint64 {
	inst := nextConcealInstance.Add(1)
	a.mu.Lock()
	a.sneaking = true
	a.sneakScore = score
	a.sneakInstance = inst
	a.mu.Unlock()
	return inst
}

// Unsneak clears the actor's sneak concealment, returning whether it had been
// sneaking. Idempotent. Unlike Reveal (hide), this is NOT called by the
// move-drops-hide subscriber — sneak survives a room change (§3.2).
func (a *connActor) Unsneak() bool {
	a.mu.Lock()
	was := a.sneaking
	a.sneaking = false
	a.sneakScore = 0
	a.sneakInstance = 0
	a.mu.Unlock()
	return was
}

// IsSneaking reports whether the actor is currently moving concealed
// (visibility.md §3.2). Read by the movement-broadcast filter.
func (a *connActor) IsSneaking() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sneaking
}

// SneakDifficulty computes the would-be sneak score for a new sneak attempt
// (visibility.md §3.2 / §8: sneak proficiency + governing stat + mods). v1
// mirrors HideScore — a base plus the actor's DEX modifier — with the
// proficiency + situational terms deferred to a later tuning pass (§8).
func (a *connActor) SneakDifficulty() int {
	const baseSneakDC = 10
	a.mu.Lock()
	sb := a.statBlock
	a.mu.Unlock()
	if sb == nil {
		return baseSneakDC
	}
	// Move Silently proficiency folds in via SkillBonus (proficiency 0 = the
	// bare Dex modifier, the pre-skill behavior); the Stealthy feat axis stays
	// additive on top.
	prof := a.skillProficiency(skillAbilityMoveSilently)
	return baseSneakDC + progression.SkillBonus(prof, sb.Effective(progression.StatDEX), progression.DefaultSkillConfig()) + a.FeatSkillBonus(skillStealth)
}

// SneakConcealmentScore returns the snapshot difficulty an occupant's
// perception contest must beat to see this mover's movement line (§3.2);
// zero when not sneaking.
func (a *connActor) SneakConcealmentScore() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sneakScore
}

// --- Admin invisibility (visibility.md §3.4): flag-gated "wizinvis".

// SetAdminInvisible sets the actor's admin-invisibility and returns the
// PREVIOUS state. Ephemeral — never persisted.
func (a *connActor) SetAdminInvisible(v bool) bool {
	a.mu.Lock()
	prev := a.adminInvisible
	a.adminInvisible = v
	a.mu.Unlock()
	return prev
}

// ToggleAdminInvisible flips admin-invisibility under a SINGLE lock and
// returns the NEW state, so the `wizinvis` verb's check-and-act is atomic
// (no read-then-write window) and it knows which event to emit.
func (a *connActor) ToggleAdminInvisible() bool {
	a.mu.Lock()
	a.adminInvisible = !a.adminInvisible
	now := a.adminInvisible
	a.mu.Unlock()
	return now
}

// IsAdminInvisible reports whether the actor is walking invisibly (§3.4).
// Read by the visibility filter's target side and the `who` roster.
func (a *connActor) IsAdminInvisible() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.adminInvisible
}

// --- Observer side (visibility.md §4): perception + sticky detection memory.

// PerceptionBonus is the actor's bonus in the §4.2 perception contest that
// pierces another's hide/sneak. v1 keys it to WIS (awareness); the awareness
// proficiency + situational terms are a later tuning pass (§8). Defensive
// against a nil stat block (mirrors HideScore).
func (a *connActor) PerceptionBonus() int {
	a.mu.Lock()
	sb := a.statBlock
	a.mu.Unlock()
	if sb == nil {
		return 0
	}
	// Perception proficiency (collapsing Spot/Listen/Search) folds in via
	// SkillBonus (proficiency 0 = the bare Wis modifier, the pre-skill
	// behavior); the Alertness/Sharp-Eyed feat axis stays additive on top.
	prof := a.skillProficiency(skillAbilityPerception)
	return progression.SkillBonus(prof, sb.Effective(progression.StatWIS), progression.DefaultSkillConfig()) + a.FeatSkillBonus(skillPerception)
}

// ContestOutcome reports this observer's remembered result against a
// concealment instance (sticky memory, §4.1): won is the outcome, done is
// whether a contest was already resolved against this instance. The result
// is sticky for BOTH win and loss — a single contest per room-entry (the
// memory clears on room change), so a hidden target the observer failed to
// spot stays unseen until it re-establishes or the observer leaves, and one
// it spotted stays seen. This prevents per-look re-rolling in either
// direction.
func (a *connActor) ContestOutcome(instance uint64) (won, done bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	won, done = a.contested[instance]
	return won, done
}

// RecordContest remembers the outcome of a perception contest (win or loss)
// so subsequent checks for this instance skip the re-roll (§4.1). Lazily
// allocates the set.
func (a *connActor) RecordContest(instance uint64, won bool) {
	a.mu.Lock()
	if a.contested == nil {
		a.contested = make(map[uint64]bool)
	}
	a.contested[instance] = won
	a.mu.Unlock()
}

// clearDetectionLocked drops this observer's sticky contest memory AND its
// hidden-exit discovery memory — called on a room change (visibility §4.1 /
// hidden-exits §3.4: you lose track of what you spotted when you leave, and
// must re-search on return). Caller holds a.mu.
func (a *connActor) clearDetectionLocked() {
	a.contested = nil
	a.discoveredExits = nil
}

// exitVisibleFor builds the hidden-exits §5.1 render filter for a freshly-
// shown actor (spawn + link-dead reattach): a hidden exit is listed only if
// this character may see it. Mirrors command.canSeeExit so the first room view
// agrees with the live command-render path — an admin or a detect_hidden
// carrier sees hidden exits "without searching" (§3.3), and an ordinary
// character sees only what it has discovered (empty on these cold paths, since
// discovery clears on every room change). Effect-granted detect_hidden is not
// resolved here (connActor.HasTag covers racial/alignment tags only); that
// refinement is picked up on the next `look` through canSeeExit.
func exitVisibleFor(a *connActor, adminRole string) func(world.Direction, world.Exit) bool {
	return func(d world.Direction, e world.Exit) bool {
		if !e.Hidden {
			return true
		}
		if adminRole != "" && a.HasRole(adminRole) {
			return true
		}
		if a.HasTag(command.DetectHiddenFlag) {
			return true
		}
		return a.IsExitDiscovered(d)
	}
}

// IsExitDiscovered reports whether this character has found the hidden exit in
// the given direction in the current room (hidden-exits §3). A non-hidden exit
// is the caller's concern; this only tracks discovery memory.
func (a *connActor) IsExitDiscovered(dir world.Direction) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.discoveredExits[dir]
}

// DiscoverExit records a hidden exit as found, returning true if it was newly
// discovered (so the caller emits the discovery message + event exactly once).
// Lazily allocates the set. Cleared on room change by clearDetectionLocked.
func (a *connActor) DiscoverExit(dir world.Direction) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.discoveredExits == nil {
		a.discoveredExits = make(map[world.Direction]bool)
	}
	if a.discoveredExits[dir] {
		return false
	}
	a.discoveredExits[dir] = true
	return true
}

// ColorTier returns the actor's color-capability tier captured
// from the conn at construction (M16.6a). Stable for the life of
// the session; M16.6b will dispatch renderer paths off this.
func (a *connActor) ColorTier() render.ColorTier { return a.colorTier }

// TerminalWidth reports the conn's current window width in columns (RFC
// 1073 NAWS), read live so a mid-session resize is honored on the next
// render. Returns 0 when the conn doesn't report a width (websocket,
// test fakes, or a client that refused NAWS); side-by-side renderers
// treat 0 as "unknown" and fall back to a fixed column width.
func (a *connActor) TerminalWidth() int {
	if src, ok := a.conn.(terminalWidthSource); ok {
		return src.TerminalWidth()
	}
	return 0
}

func (a *connActor) ColorEnabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.colorEnabled
}

func (a *connActor) SetColorEnabled(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.colorEnabled = v
}

// tellsSessionHistoryCap bounds connActor.recentTells (spec
// chat-channels-and-tells §10). When the ring is full, the oldest
// line evicts.
const tellsSessionHistoryCap = 50

// LastTellPartner returns the display name of the most recent
// tell counterparty for this actor, or "" if none.
func (a *connActor) LastTellPartner() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastTellPartner
}

// SetLastTellPartner updates the reply slot. Empty strings are
// silently ignored so a delivery that produced no sender name
// can't blank an existing slot.
func (a *connActor) SetLastTellPartner(name string) {
	if name == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastTellPartner = name
}

// RecentTells returns a fresh copy of the in-session tell history
// (oldest first). Safe for the caller to mutate.
func (a *connActor) RecentTells() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.recentTells) == 0 {
		return nil
	}
	out := make([]string, len(a.recentTells))
	copy(out, a.recentTells)
	return out
}

// AppendRecentTell pushes a line onto the session history ring.
// Drops the oldest entry once the cap is reached.
func (a *connActor) AppendRecentTell(line string) {
	if line == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.recentTells = append(a.recentTells, line)
	if len(a.recentTells) > tellsSessionHistoryCap {
		// drop oldest
		a.recentTells = a.recentTells[len(a.recentTells)-tellsSessionHistoryCap:]
	}
}

// Write expands any color markup in msg per the actor's color
// preference and writes the rendered text plus CRLF.
func (a *connActor) Write(ctx context.Context, msg string) error {
	// Content-send half of the prompt-refresh state machine
	// (session-lifecycle §3.5). If a prompt is sitting at the bottom of
	// the screen and the player hasn't typed since, break the line first
	// so the new content doesn't run into the prompt. Then mark that a
	// prompt refresh is owed at end of tick.
	a.mu.Lock()
	breakPrompt := a.promptDisplayed && !a.receivedInput
	a.promptDisplayed = false
	a.receivedInput = false
	a.needsPromptRefresh = true
	a.mu.Unlock()

	rendered := a.render(msg)
	payload := rendered + "\r\n"
	if breakPrompt {
		payload = "\r\n" + payload
	}
	_, err := a.conn.Write(ctx, []byte(payload))
	return err
}

// render applies the themed color renderer when wired, choosing
// RenderAnsi vs RenderPlain by the session's color flag (the spec's
// SupportsAnsi role, §5). Falls back to the minimal M2 ansi.Render
// when no renderer is configured (tests).
//
// M16.6b: when color is enabled the per-tier dispatch
// (RenderAnsiForTier) emits ANSI-16 / 256-color / truecolor SGR
// based on the actor's captured capability. ColorEnabled is the
// admin/preference override that still wins — even a TrueColor-
// capable client with color disabled by the user gets plain text.
func (a *connActor) render(msg string) string {
	if a.renderer == nil {
		return ansi.Render(msg, a.ColorEnabled())
	}
	if a.ColorEnabled() {
		return a.renderer.RenderAnsiForTier(msg, a.colorTier)
	}
	return a.renderer.RenderPlain(msg)
}

// Persist writes the current player save through the store, but only
// when something has changed since the last write. Safe to call from
// any goroutine.
//
// The dirty flag is only cleared if no mutation occurred between
// snapshot and completion (saveGen unchanged) — otherwise an
// interleaved SetRoom / Equip / drop would be silently dropped: the
// mutation sets dirty=true while our Save is in flight, and we'd
// then clear that flag after writing the older snapshot.
//
// The snapshot deep-copies slice- and map-backed save fields so the
// YAML encoder running on snapshot cannot race a concurrent
// syncXxxToSaveLocked rewrite of a.save.
func (a *connActor) Persist(ctx context.Context) error {
	a.mu.Lock()
	// Sync vitals into save BEFORE the dirty check so HP changes
	// from the combat tick (which never go through markDirtyLocked
	// — combat doesn't know about session) participate in autosave.
	if a.syncVitalsToSaveLocked() {
		a.markDirtyLocked()
	}
	// Sync pools (mana/movement) before the dirty check like vitals: a
	// channeling/movement spend on the combat tick never goes through
	// markDirtyLocked, so it must be pulled in here to participate in autosave.
	if a.syncPoolsToSaveLocked() {
		a.markDirtyLocked()
	}
	if a.syncAbilitiesToSaveLocked() {
		a.markDirtyLocked()
	}
	if a.syncRecipesToSaveLocked() {
		a.markDirtyLocked()
	}
	if !a.dirty || a.save == nil || a.players == nil {
		a.mu.Unlock()
		return nil
	}
	snapshot := snapshotSave(a.save)
	gen := a.saveGen
	a.mu.Unlock()

	if err := a.players.Save(ctx, &snapshot); err != nil {
		return err
	}
	a.mu.Lock()
	if a.saveGen == gen {
		a.dirty = false
	}
	a.mu.Unlock()
	return nil
}

// cloneInventoryEntries deep-copies an InventoryEntry tree so a
// concurrent syncInventoryToSaveLocked rewrite of a.save.Inventory
// can't race the YAML encode that runs after the actor lock is
// released. The recursion stops naturally at leaves (empty
// Contents).
func cloneInventoryEntries(in []player.InventoryEntry) []player.InventoryEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]player.InventoryEntry, len(in))
	for i, e := range in {
		out[i] = player.InventoryEntry{
			Template: e.Template,
			Contents: cloneInventoryEntries(e.Contents),
			Loaded:   e.Loaded,
		}
	}
	return out
}

// clampWimpy normalizes a persisted wimpy threshold into [0, 100].
// Anything outside the range maps to 0 (disabled) — defensively
// permissive so a hand-edited save with a nonsense value loads
// cleanly into a known-good disabled state.
func clampWimpy(pct int) int {
	if pct < 0 || pct > 100 {
		return 0
	}
	return pct
}

// restorePlayerVitals returns a fresh *combat.Vitals for a logging-in
// player. A nil persisted block (legacy v4 save, brand-new character)
// spawns at full HP with the hardcoded player max. A persisted block
// rehydrates via NewVitalsAt, which clamps to sane ranges if the saved
// values are out of bounds (HP > MaxHP, MaxHP < 1).
func restorePlayerVitals(v *player.VitalsState) *combat.Vitals {
	if v == nil {
		return combat.NewVitals(combat.DefaultPlayerMaxHP)
	}
	maxHP := v.MaxHP
	if maxHP < 1 {
		maxHP = combat.DefaultPlayerMaxHP
	}
	// Safety floor: a player whose save records HP <= 0 (killed in
	// combat, then disconnected before any recovery path ran) would
	// otherwise log back in dead, unable to act, with no way out
	// short of operator intervention. Combat spec §6.4 says the
	// player-death subscriber owns recovery (corpse, respawn, gear
	// loss); until that lands (tracked in m7-5-deferred-fixes.md #1)
	// clamp HP up to 1 on restore so login is at least playable.
	// Removing this floor is safe the moment a real §6.4 subscriber
	// guarantees no save ever serializes HP <= 0.
	hp := v.HP
	if hp <= 0 {
		hp = 1
	}
	return combat.NewVitalsAt(hp, maxHP)
}

// syncVitalsToSaveLocked rewrites a.save.Vitals from a.vitals if the
// live HP differs from what's currently in the save. Returns true if
// the save record actually changed; callers use the result to decide
// whether to bump the dirty flag.
//
// Caller MUST hold a.mu. Lives next to the other syncXxxToSaveLocked
// helpers (inventory, equipment, stats) so the persist path has a
// single read-vitals → write-save touchpoint.
func (a *connActor) syncVitalsToSaveLocked() bool {
	if a.save == nil || a.vitals == nil {
		return false
	}
	cur, max := a.vitals.Snapshot()
	if a.save.Vitals != nil && a.save.Vitals.HP == cur && a.save.Vitals.MaxHP == max {
		return false
	}
	a.save.Vitals = &player.VitalsState{HP: cur, MaxHP: max}
	return true
}

// syncPoolsToSaveLocked rewrites a.save.Pools from the live pool.Set if a
// pool's persisted current would change. Only NON-FULL pools (current <
// max) are written — a full or zero-max pool is omitted so the login path
// reseeds it full, keeping non-channeler saves clean (no `pools:` key).
// Returns true if the save record actually changed. Caller MUST hold a.mu;
// lives beside syncVitalsToSaveLocked (HP is in Vitals, the other pools
// here) so the persist path has one read-pool → write-save touchpoint.
//
// Lock order: this takes a.mu then pool.Set.mu (via Snapshot). That order
// is deadlock-free because the only goroutine touching pools concurrently
// (the regen tick via regenPool, the prompt via resourceSnapshot) acquires
// pool.Set.mu WITHOUT ever holding a.mu — so the two never form a cycle.
func (a *connActor) syncPoolsToSaveLocked() bool {
	if a.save == nil || a.pools == nil {
		return false
	}
	var desired pool.Snapshot
	for _, e := range a.pools.Snapshot() {
		if e.Current < e.Max {
			desired = append(desired, e)
		}
	}
	if poolSnapshotsEqual(a.save.Pools, desired) {
		return false
	}
	a.save.Pools = desired
	return true
}

// poolSnapshotsEqual reports whether two pool snapshots carry the same
// entries in the same order. Both come from pool.Set.Snapshot (sorted by
// kind) or the filtered copy above (which preserves that order), so a
// positional compare is sufficient — no need to sort or build a map.
func poolSnapshotsEqual(a, b pool.Snapshot) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// snapshotSave produces an isolated copy of save suitable for a YAML
// encode that may run after the actor lock is released. Strings/ints
// copy by value; slices and maps need explicit duplication.
func snapshotSave(save *player.Save) player.Save {
	out := *save
	if save.Inventory != nil {
		out.Inventory = cloneInventoryEntries(save.Inventory)
	}
	if save.Equipment != nil {
		dup := make(map[string]player.EquippedItem, len(save.Equipment))
		maps.Copy(dup, save.Equipment)
		out.Equipment = dup
	}
	if save.Stats != nil {
		dup := make(stats.Snapshot, len(save.Stats))
		for i, e := range save.Stats {
			entryDup := e
			if e.Modifiers != nil {
				mods := make([]stats.Modifier, len(e.Modifiers))
				copy(mods, e.Modifiers)
				entryDup.Modifiers = mods
			}
			dup[i] = entryDup
		}
		out.Stats = dup
	}
	if save.Vitals != nil {
		v := *save.Vitals
		out.Vitals = &v
	}
	if save.Pools != nil {
		// pool.Entry is a flat value struct, so a shallow element copy is a
		// full deep copy — duplicated so the persist path can rewrite
		// a.save.Pools without racing the YAML encoder on this snapshot.
		dup := make(pool.Snapshot, len(save.Pools))
		copy(dup, save.Pools)
		out.Pools = dup
	}
	if save.StatsBase != nil {
		// BaseEntry is a flat value struct (no pointer fields), so a shallow
		// element copy is a full deep copy. syncStatsToSaveLocked reassigns
		// a.save.StatsBase wholesale today (no in-place append), so this is
		// belt-and-suspenders — but it matches every sibling field above so a
		// future in-place mutation can't tear the async YAML encode.
		dup := make(progression.BaseSnapshot, len(save.StatsBase))
		copy(dup, save.StatsBase)
		out.StatsBase = dup
	}
	if save.KnownFeats != nil {
		// KnownFeat is a flat value struct, so a shallow element copy is a full
		// deep copy. Snapshotted here (EPIC S4 Phase 2) so the Phase-4 spend
		// path can append to a.save.KnownFeats without racing the YAML encoder.
		dup := make([]player.KnownFeat, len(save.KnownFeats))
		copy(dup, save.KnownFeats)
		out.KnownFeats = dup
	}
	if save.KnownLanguages != nil {
		// String slice — a shallow copy is a full deep copy. Snapshotted (like
		// KnownFeats) so the LearnLanguage append path can't race the encoder.
		dup := make([]string, len(save.KnownLanguages))
		copy(dup, save.KnownLanguages)
		out.KnownLanguages = dup
	}
	return out
}

// markDirtyLocked flips dirty and advances the save generation
// counter. Caller MUST hold a.mu. Centralized so every mutation path
// stays in step with Persist's "did anything change between snapshot
// and completion?" check; a bare `dirty = true` would set the flag
// but leave saveGen untouched and Persist would silently clear it.
func (a *connActor) markDirtyLocked() {
	a.dirty = true
	a.saveGen++
}

// Inventory returns a snapshot of the actor's currently-held item
// entity ids in pickup order. Safe to mutate.
func (a *connActor) Inventory() []entities.EntityID {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]entities.EntityID, len(a.inventory))
	copy(out, a.inventory)
	return out
}

// AddToInventory appends id to contents and marks the save dirty so
// autosave commits the new inventory list. The save's persisted
// Inventory field is updated in lockstep so a concurrent Persist
// snapshot is always self-consistent.
func (a *connActor) AddToInventory(id entities.EntityID) {
	a.mu.Lock()
	a.inventory = append(a.inventory, id)
	a.syncInventoryToSaveLocked()
	a.markDirtyLocked()
	a.mu.Unlock()
}

// RemoveFromInventory removes id from contents. Returns true on hit.
// Marks the save dirty when something was actually removed.
func (a *connActor) RemoveFromInventory(id entities.EntityID) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, e := range a.inventory {
		if e == id {
			a.inventory = append(a.inventory[:i], a.inventory[i+1:]...)
			a.syncInventoryToSaveLocked()
			a.markDirtyLocked()
			return true
		}
	}
	return false
}

// AmmoConsumer is the host seam the combat AmmoFor hook uses to spend a
// projectile's ammunition (ranged-combat §3). The combat package never
// touches inventory; the binary type-asserts a Combatant to this interface
// and calls ConsumeAmmo once per projectile swing. A combatant that does not
// satisfy it (a mob) fires freely — mob ranged AI is deferred (§9).
type AmmoConsumer interface {
	ConsumeAmmo(kind string) (gradeKey string, ok bool)
}

// ConsumeAmmo spends one matching ammunition unit (ranged-combat §3) and
// returns its quality-grade key (masterwork §3; "" when ungraded) plus
// whether anything was consumed. Inventory is N separate instances (stacking
// is display-only), so consuming one unit = removing the first matching
// instance, the same removal the eat/drink/use consumables use. A blank kind
// never matches. Runs under a.mu — safe to call from the combat tick
// goroutine, like the equip/unequip mutations.
func (a *connActor) ConsumeAmmo(kind string) (string, bool) {
	if kind == "" {
		return "", false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.items == nil {
		return "", false
	}
	// A firearm draws from the weapon, not a loose round from inventory. Two
	// feed models (ammo-and-reloading): a HOLDER-FED weapon fires from the
	// holder inserted in it; an INTERNALLY-FED magazine weapon (SR-M3e) fires
	// from its own loaded count. An empty feed is a dry click. Rounds are an
	// abstract count here, so no per-round ammo grade rides the shot (that
	// lands with the clip in SR-M3f-2); the weapon's own grade is applied
	// elsewhere in combat. A save re-sync captures the new count since Persist
	// does not re-sync equipment (combat changes bypass it, like vitals).
	if wid, ok := a.equipment[mainHandSlot]; ok {
		if e, ok := a.items.GetByID(wid); ok {
			if w, isItem := e.(*entities.ItemInstance); isItem {
				if w.AcceptsHolder() != "" {
					_, loaded, has := w.InsertedHolder()
					if !has || loaded <= 0 {
						return "", false // no holder / empty holder — dry
					}
					w.SetInsertedHolderLoaded(loaded - 1)
					a.syncEquipmentToSaveLocked()
					a.markDirtyLocked()
					return "", true
				}
				if w.Magazine() > 0 {
					loaded := w.MagazineLoaded()
					if loaded <= 0 {
						return "", false // empty magazine — dry
					}
					w.SetMagazineLoaded(loaded - 1)
					a.syncEquipmentToSaveLocked()
					a.markDirtyLocked()
					return "", true
				}
			}
		}
	}
	for i, id := range a.inventory {
		e, ok := a.items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		// Skip holders — they carry ammo_kind (the round they hold), but are not
		// themselves loose rounds (ammo-and-reloading §2).
		if !ok || it.HolderFits() != "" || it.AmmoKind() != kind {
			continue
		}
		gradeKey := it.Grade()
		a.inventory = append(a.inventory[:i], a.inventory[i+1:]...)
		a.syncInventoryToSaveLocked()
		a.markDirtyLocked()
		return gradeKey, true
	}
	return "", false
}

// ReloadWieldedMagazine tops up the wielded magazine weapon's loaded rounds from
// carried ammo of the weapon's ammo kind. It returns the loaded count before and
// after, the magazine capacity, and whether the wielded weapon is magazine-fed
// at all (false → the caller reports there's nothing to reload). A short supply
// tops up what it can (a partial reload); consumed rounds leave inventory.
func (a *connActor) ReloadWieldedMagazine() (before, after, capacity int, isMagazine bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.items == nil {
		return 0, 0, 0, false
	}
	wid, ok := a.equipment[mainHandSlot]
	if !ok {
		return 0, 0, 0, false
	}
	e, ok := a.items.GetByID(wid)
	if !ok {
		return 0, 0, 0, false
	}
	w, ok := e.(*entities.ItemInstance)
	if !ok || w.Magazine() <= 0 {
		return 0, 0, 0, false
	}
	capacity = w.Magazine()
	before = w.MagazineLoaded()
	need := capacity - before
	if need <= 0 {
		return before, before, capacity, true // already full
	}
	got := a.pullAmmoLocked(w.AmmoKind(), need)
	after = before + got
	if got > 0 {
		w.SetMagazineLoaded(after)
		// Persist the new magazine count with the equipped weapon (Persist does
		// not re-sync equipment). pullAmmoLocked already re-synced inventory.
		a.syncEquipmentToSaveLocked()
		a.markDirtyLocked()
	}
	return before, after, capacity, true
}

// pullAmmoLocked removes up to `max` inventory items whose ammo kind matches
// `kind`, returning how many it took. The caller holds a.mu. Ammo stacks are
// separate entity ids (display-grouped), so this removes one id per round.
func (a *connActor) pullAmmoLocked(kind string, max int) int {
	if kind == "" || max <= 0 || a.items == nil {
		return 0
	}
	got := 0
	for got < max {
		removed := false
		for i, id := range a.inventory {
			e, ok := a.items.GetByID(id)
			if !ok {
				continue
			}
			it, ok := e.(*entities.ItemInstance)
			// A holder declares ammo_kind (the round it HOLDS), not that it IS a
			// round — never consume a holder as loose ammo (ammo-and-reloading §2).
			if !ok || it.HolderFits() != "" || it.AmmoKind() != kind {
				continue
			}
			a.inventory = append(a.inventory[:i], a.inventory[i+1:]...)
			got++
			removed = true
			break
		}
		if !removed {
			break
		}
	}
	if got > 0 {
		a.syncInventoryToSaveLocked()
	}
	return got
}

// magazineLoadedForSave returns the loaded-round count to persist for entity
// `id`, or nil when there's nothing to store — the item isn't a magazine weapon,
// or its magazine is empty (an empty/untouched magazine respawns lazy-empty, so
// storing 0 would only bloat the save). Caller holds a.mu.
func (a *connActor) magazineLoadedForSave(id entities.EntityID) *int {
	if a.items == nil {
		return nil
	}
	e, ok := a.items.GetByID(id)
	if !ok {
		return nil
	}
	w, ok := e.(*entities.ItemInstance)
	if !ok || w.Magazine() <= 0 {
		return nil
	}
	if n := w.MagazineLoaded(); n > 0 {
		return &n
	}
	return nil
}

// insertedHolderForSave returns the holder-inserted-in-a-weapon record to persist
// for entity `id`, or nil when it isn't a holder-fed weapon or has no holder
// inserted (ammo-and-reloading §9). Caller holds a.mu.
func (a *connActor) insertedHolderForSave(id entities.EntityID) *player.EquippedHolder {
	if a.items == nil {
		return nil
	}
	e, ok := a.items.GetByID(id)
	if !ok {
		return nil
	}
	w, ok := e.(*entities.ItemInstance)
	if !ok || w.AcceptsHolder() == "" {
		return nil
	}
	tpl, loaded, has := w.InsertedHolder()
	if !has {
		return nil
	}
	return &player.EquippedHolder{Template: tpl, Loaded: loaded}
}

// InsertHolder loads the fullest compatible loaded holder from inventory into the
// wielded holder-fed weapon (ammo-and-reloading §5). It records the holder's
// state on the weapon and consumes the holder item, returning any displaced
// holder's template + remaining rounds so the caller can eject it into the room.
// outcome: "ok" | "not-holder-fed" (wielded weapon takes no holder) | "no-holder"
// (no compatible, loaded holder carried).
func (a *connActor) InsertHolder() (outcome, weapon string, loaded, capacity int, ejectedTpl string, ejectedLoaded int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.items == nil {
		return "not-holder-fed", "", 0, 0, "", 0
	}
	wid, ok := a.equipment[mainHandSlot]
	if !ok {
		return "not-holder-fed", "", 0, 0, "", 0
	}
	ge, ok := a.items.GetByID(wid)
	if !ok {
		return "not-holder-fed", "", 0, 0, "", 0
	}
	gun, ok := ge.(*entities.ItemInstance)
	if !ok || gun.AcceptsHolder() == "" {
		return "not-holder-fed", "", 0, 0, "", 0
	}
	family := gun.AcceptsHolder()
	weapon = gun.Name()
	// Pick the fullest compatible loaded holder carried.
	var bestID entities.EntityID
	var best *entities.ItemInstance
	bestLoaded := 0
	for _, id := range a.inventory {
		e, ok := a.items.GetByID(id)
		if !ok {
			continue
		}
		h, ok := e.(*entities.ItemInstance)
		if !ok || h.HolderFits() != family {
			continue
		}
		if n := h.MagazineLoaded(); n > bestLoaded {
			bestLoaded, bestID, best = n, id, h
		}
	}
	if best == nil || bestLoaded <= 0 {
		return "no-holder", weapon, 0, 0, "", 0
	}
	// Capture the currently-inserted holder (if any) for ejection.
	if tpl, l, has := gun.InsertedHolder(); has {
		ejectedTpl, ejectedLoaded = tpl, l
	}
	// Insert the new holder: record its state on the gun, consume the item.
	loaded = bestLoaded
	capacity = best.Magazine()
	gun.SetInsertedHolder(string(best.TemplateID()), loaded)
	for i, id := range a.inventory {
		if id == bestID {
			a.inventory = append(a.inventory[:i], a.inventory[i+1:]...)
			break
		}
	}
	a.syncInventoryToSaveLocked()
	a.syncEquipmentToSaveLocked()
	a.markDirtyLocked()
	return "ok", weapon, loaded, capacity, ejectedTpl, ejectedLoaded
}

// FillHolder tops up an ammunition holder (an inventory item, resolved by the
// caller) from carried loose rounds of the holder's kind (ammo-and-reloading §4).
// Returns the load before/after, the capacity, and whether the target is a
// holder at all (false → the caller reports it can't be reloaded).
func (a *connActor) FillHolder(holderID entities.EntityID) (before, after, capacity int, ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.items == nil {
		return 0, 0, 0, false
	}
	e, found := a.items.GetByID(holderID)
	if !found {
		return 0, 0, 0, false
	}
	h, isItem := e.(*entities.ItemInstance)
	if !isItem || h.HolderFits() == "" || h.Magazine() <= 0 {
		return 0, 0, 0, false
	}
	// Only fill a holder the actor actually carries (ownership discipline, like
	// InsertHolder) — never drain this actor's ammo into an item it doesn't own.
	inInventory := false
	for _, id := range a.inventory {
		if id == holderID {
			inInventory = true
			break
		}
	}
	if !inInventory {
		return 0, 0, 0, false
	}
	capacity = h.Magazine()
	before = h.MagazineLoaded()
	need := capacity - before
	if need <= 0 {
		return before, before, capacity, true
	}
	got := a.pullAmmoLocked(h.AmmoKind(), need)
	after = before + got
	if got > 0 {
		h.SetMagazineLoaded(after)
		// Re-sync inventory AFTER updating the holder's count: pullAmmoLocked
		// synced with the pre-fill count, and Persist does not re-sync
		// equipment/inventory, so without this the filled count is lost on the
		// next save (the loose holder reverts to its pre-fill load).
		a.syncInventoryToSaveLocked()
		a.markDirtyLocked()
	}
	return before, after, capacity, true
}

// Equipment returns a snapshot of the actor's currently-equipped items
// keyed by slot key. Fresh map — safe to mutate.
func (a *connActor) Equipment() map[string]entities.EntityID {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]entities.EntityID, len(a.equipment))
	maps.Copy(out, a.equipment)
	return out
}

// AngrealPower returns the highest One Power amplification rating among the
// actor's EQUIPPED angreal/sa'angreal devices whose gender gate matches the
// supplied channeler gender (wot-the-one-power.md S2). A cross-gender device is
// inert (skipped); a device merely carried in inventory does not count (it must
// be held/equipped). Returns 0 when the actor holds no matching device — the
// caller treats 0 as "no amplification". Only the single strongest device
// applies (v1 does not stack); the per-point multiplier math lives in the
// composition root beside the affinity rule, keeping this method setting-free.
func (a *connActor) AngrealPower(gender string) int {
	g := strings.ToLower(strings.TrimSpace(gender))
	if g == "" {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.items == nil {
		return 0
	}
	best := 0
	for _, id := range a.equipment {
		e, ok := a.items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		power, devGender, ok := it.Angreal()
		if !ok || devGender != g {
			continue
		}
		if power > best {
			best = power
		}
	}
	return best
}

// weaponInfo is the immutable wielded-weapon snapshot stored in
// connActor.weapon and copied into combat.Stats each round. Held behind
// an atomic.Pointer so the tick-goroutine read in Stats() never touches
// a.mu.
type weaponInfo struct {
	dice combat.DiceExpr
	name string
	// category / tier are the wielded weapon's identity labels
	// (weapon-identity §2), captured alongside the dice so the to-hit
	// hook can decide proficiency without re-fetching the item. Empty
	// tier means "untiered" (treated as the lowest tier, §3).
	category string
	tier     string
	// critThreatLow / critMultiplier are the weapon's §4 critical params
	// carried into combat.Stats. Zero ⇒ the resolver applies its defaults.
	critThreatLow  int
	critMultiplier int
	// damageTypes are the weapon's damage type(s) (weapon-identity §2),
	// carried into combat.Stats so the defender's per-type resistance can
	// be selected (armor-depth §4). nil ⇒ untyped.
	damageTypes []string
	// targetPool is the pool.Kind (lowercased string) this weapon's damage
	// fills (shadowrun-mvp SR-M3b); "" ⇒ the hp path. Carried into
	// combat.Stats.TargetPool so a stun baton routes to the Stun monitor.
	targetPool string
	// wieldMode is the size-relative grip this weapon resolves to for THIS
	// wielder (size-and-wielding §3), derived in recomputeWeaponLocked from
	// the wielder's size and the weapon's size. Drives the two-handed melee
	// Strength bonus (§4.2). Recomputed whenever equipment or (via re-login)
	// race changes; ephemeral, never persisted.
	wieldMode size.WieldMode
	// Ranged metadata (ranged-combat §2/§4). rangedClass empty ⇒ melee;
	// ammoKind is what a projectile fires; rangeIncrement is the §5.3 falloff
	// unit (inert until Slice B); strRating caps a Strength-rated bow's
	// positive Strength bonus (nil ⇒ default no-positive-Strength rule). Read
	// by Stats() to apply the Strength rule and carry the class into combat.
	rangedClass    string
	ammoKind       string
	rangedStyle    string
	rangeIncrement int
	reloadTicks    int
	magazine       int
	acceptsHolder  string
	strRating      *int
	// reach is the weapon's reach rating (special-weapons §3) — a numeric
	// cross-ruleset stat; WoT reads `> 0` as "strikes at the near band". Read by
	// Stats() into combat.Stats.Reach.
	reach int
	// set reports the `set` special tag (special-weapons §4 — set vs a charge).
	// Read by Stats() into combat.Stats.Set so a braced polearm answers a charge
	// with a bonus blow. false for an ordinary weapon.
	set bool
	// subdual reports the weapon is nonlethal (subdual-damage §2 — sap/whip).
	// Read by Stats() into combat.Stats.Subdual so a finishing blow knocks the
	// foe out instead of killing. false for an ordinary (lethal) weapon.
	subdual bool
	// ineffectiveVsArmor reports the weapon is a whip (the `whip` special tag,
	// subdual-damage §6): it cannot bite through armor. Read by Stats() into
	// combat.Stats.IneffectiveVsArmor. false for an ordinary weapon.
	ineffectiveVsArmor bool
	// doubleDamage is a DOUBLE weapon's SECOND-end dice (special-weapons §7 —
	// quarterstaff/ashandarei). When set and the weapon is wielded with no
	// distinct off-hand item, Stats() grants an off-hand strike from this end (the
	// weapon used as two weapons). Zero for an ordinary weapon.
	doubleDamage combat.DiceExpr
	// tripBonus / disarmBonus are the wielded weapon's maneuver DC bonuses
	// (special-weapons §4/§5) — read by the composition root's save-DC hook so a
	// trip/disarm weapon raises the maneuver's save DC. 0 for an ordinary weapon.
	tripBonus   int
	disarmBonus int
}

// armorResistances is the atomic snapshot of an actor's aggregated
// per-damage-type damage reduction from worn armor (armor-depth §4),
// summed across distinct equipped items. Immutable after Store.
type armorResistances struct {
	byType map[string]int
}

// mainHandSlot / offHandSlot are the equipment-map keys for the primary and
// off hand. The main weapon is read from the wield slot specifically (not
// "first weapon by sorted key"), because once the off hand can hold a weapon
// (two-weapon-fighting §2) "offhand" sorts before "wield" and would otherwise
// be mistaken for the main weapon.
const (
	// Alias the canonical engine slot keys so the player and mob (entities) weapon
	// resolvers share one definition (slot/baseline.go) rather than re-spelling
	// the literals.
	mainHandSlot = slot.WieldSlot
	offHandSlot  = slot.OffHandSlot
)

// buildWeaponInfoLocked builds the cached weaponInfo for an equipped item id,
// or nil when the id is empty/unknown or the item declares no weapon dice. The
// caller MUST hold a.mu. Shared by recomputeWeaponLocked for both the main
// (wield) and off-hand weapon picks (combat §4.5, two-weapon-fighting §2).
func (a *connActor) buildWeaponInfoLocked(id entities.EntityID) *weaponInfo {
	if id == "" || a.items == nil {
		return nil
	}
	e, ok := a.items.GetByID(id)
	if !ok {
		return nil
	}
	it, ok := e.(*entities.ItemInstance)
	if !ok {
		return nil
	}
	dice, ok := it.WeaponDamage()
	if !ok {
		return nil
	}
	return &weaponInfo{
		dice:           dice,
		name:           it.Name(),
		category:       it.WeaponCategory(),
		tier:           it.ProficiencyTier(),
		critThreatLow:  it.CritThreatLow(),
		critMultiplier: it.CritMultiplier(),
		damageTypes:    it.DamageTypes(),
		targetPool:     it.TargetPool(),
		// size-and-wielding §3: resolve the grip for THIS wielder.
		wieldMode:          size.Mode(it.WeaponSize(), a.sizeLocked()),
		rangedClass:        it.RangedClass(),
		ammoKind:           it.AmmoKind(),
		rangedStyle:        it.RangedStyle(),
		rangeIncrement:     it.RangeIncrement(),
		reloadTicks:        it.ReloadTicks(),
		magazine:           it.Magazine(),
		acceptsHolder:      it.AcceptsHolder(),
		strRating:          it.StrRating(),
		reach:              it.Reach(),
		set:                it.HasSpecial(item.SpecialSet),
		subdual:            it.Subdual(),
		ineffectiveVsArmor: it.HasSpecial(item.SpecialWhip),
		doubleDamage:       doubleDamageOf(it),
		tripBonus:          it.TripBonus(),
		disarmBonus:        it.DisarmBonus(),
	}
}

// doubleDamageOf returns a weapon's double-weapon second-end dice, or the zero
// DiceExpr when it is not a double weapon (special-weapons §7).
func doubleDamageOf(it *entities.ItemInstance) combat.DiceExpr {
	dd, _ := it.DoubleDamage()
	return dd
}

// recomputeWeaponLocked refreshes the cached wielded-weapon snapshots from the
// current equipment set (combat §4.5, two-weapon-fighting §2). The caller MUST
// hold a.mu. The MAIN weapon is the item in the wield slot; the OFF-HAND weapon
// is the item in the off-hand slot when it is a DISTINCT id (a spanning
// two-hander shares one id across both slot keys, so it is the main weapon, not
// a second one). It also scans every equipped slot to sum per-type armor
// resistances and the most restrictive Dex cap / worn tiers (armor-depth §3/§4).
// Stores nil weapon(s) when nothing relevant is wielded, so Stats() falls back
// to the unarmed default. A nil item store (tests) yields nil.
func (a *connActor) recomputeWeaponLocked() {
	if a.items == nil || len(a.equipment) == 0 {
		a.weapon.Store(nil)
		a.offWeapon.Store(nil)
		a.armorResist.Store(nil)
		a.armorDexCap.Store(nil)
		a.armorTiers.Store(nil)
		a.wornReputation.Store(0)
		a.wornArmorBonus.Store(0)
		return
	}
	keys := make([]string, 0, len(a.equipment))
	for k := range a.equipment {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// One pass over the equipped set: pick the wielded weapon (the first
	// distinct item by sorted key that declares dice) and sum per-type
	// armor resistances across every distinct item (armor-depth §4). A
	// spanning item (two-hander, multi-slot armor) appears under several
	// keys mapping to the same id, so dedup by id keeps the weapon pick
	// stable and the resistance sum from double-counting one piece.
	seen := make(map[entities.EntityID]bool, len(keys))
	var resist map[string]int
	// Armor-depth §3/§5: the most restrictive (lowest) max-Dex cap across worn
	// armor, and the distinct tiers worn. minCap nil ⇒ no piece caps Dex.
	var minCap *int
	var tiers []string
	var wornRep int   // special-weapons §8: summed visible-gear reputation
	var wornArmor int // subdual-damage §6: summed worn armor AC contribution
	for _, k := range keys {
		id := a.equipment[k]
		if seen[id] {
			continue
		}
		e, ok := a.items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		seen[id] = true
		wornRep += it.Reputation()   // special-weapons §8: visible-gear renown delta
		wornArmor += it.ArmorBonus() // subdual-damage §6: worn armor rating (whip gate)
		for dt, amt := range it.Resistances() {
			if resist == nil {
				resist = make(map[string]int)
			}
			resist[dt] += amt
		}
		if mdx := it.ArmorMaxDex(); mdx != nil && (minCap == nil || *mdx < *minCap) {
			minCap = mdx // ArmorMaxDex returns a fresh copy — safe to retain
		}
		if t := it.ArmorTier(); t != "" {
			already := slices.Contains(tiers, t)
			if !already {
				tiers = append(tiers, t)
			}
		}
	}
	// Main weapon = the wield slot; off-hand weapon = the off-hand slot when it
	// holds a DISTINCT weapon (two-weapon-fighting §2). A spanning two-hander
	// has the same id under both keys, so the off-hand pick is suppressed for it.
	mainID := a.equipment[mainHandSlot]
	a.weapon.Store(a.buildWeaponInfoLocked(mainID))
	if offID := a.equipment[offHandSlot]; offID != "" && offID != mainID {
		a.offWeapon.Store(a.buildWeaponInfoLocked(offID))
	} else {
		a.offWeapon.Store(nil)
	}
	if resist != nil {
		a.armorResist.Store(&armorResistances{byType: resist})
	} else {
		a.armorResist.Store(nil)
	}
	a.armorDexCap.Store(minCap)
	if len(tiers) > 0 {
		a.armorTiers.Store(&tiers)
	} else {
		a.armorTiers.Store(nil)
	}
	a.wornReputation.Store(int64(wornRep))
	a.wornArmorBonus.Store(int64(wornArmor))
}

// IsWeaponProficient reports whether the actor may wield their current
// weapon without the non-proficient to-hit penalty (weapon-identity §3).
// The class is resolved LIVE by classID (not via the lock-free a.class
// pointer), so a SetClass change is honored without a relogin. An unarmed
// actor, an untiered weapon, and a lowest-tier weapon are always
// proficient. Read on the combat tick goroutine; combat cadence makes the
// per-swing class lookup (a registry RLock + a tiny tier/category scan)
// negligible, so no cached result is kept.
// WieldedTripBonus / WieldedDisarmBonus return the maneuver DC bonus of the
// actor's currently-wielded main weapon (special-weapons §4/§5); 0 unarmed or
// with an ordinary weapon. Read lock-free off the weapon atomic (the combat-tick
// goroutine's save-DC hook reads them), like IsWeaponProficient.
func (a *connActor) WieldedTripBonus() int {
	if w := a.weapon.Load(); w != nil {
		return w.tripBonus
	}
	return 0
}

func (a *connActor) WieldedDisarmBonus() int {
	if w := a.weapon.Load(); w != nil {
		return w.disarmBonus
	}
	return 0
}

func (a *connActor) IsWeaponProficient() bool {
	w := a.weapon.Load()
	if w == nil {
		return true // unarmed → lowest tier → always proficient
	}
	// Snapshot the feat bonus (atomic) BEFORE taking a.mu, matching how
	// hasCleave/hasPowerAttack are read — so the class set and the feat-granted
	// categories come from a deterministic ordering rather than a mix straddling
	// an unlock (a concurrent applyFeatGrants then yields a coherent before/after).
	fb := a.featWeaponBonus.Load()
	a.mu.Lock()
	// Union the proficiency grants across every class — proficient if ANY
	// class grants the weapon's tier or category (weapon-identity §3,
	// multiclass). One class today, so this is a single class's grants.
	var tiers, cats []string
	if a.classes != nil {
		for _, cid := range a.classIDs {
			if cls, ok := a.classes.Get(cid); ok {
				tiers = append(tiers, cls.ProficiencyTiers...)
				cats = append(cats, cls.ProficiencyCategories...)
			}
		}
	}
	a.mu.Unlock()
	// Feat-granted categories (Militia — feats Bucket B) union with the class
	// set: a weapon whose category a feat grants is proficient even if no class
	// grants it. Empty when no such feat is held → behavior unchanged.
	if fb != nil && len(fb.weaponProficiencyCategories) > 0 {
		cats = append(cats, fb.weaponProficiencyCategories...)
	}
	return item.Proficient(tiers, cats, w.tier, w.category)
}

// cappedDexAC is the synthetic `dex_ac` channel input (armor-depth §3): the
// wearer's Dex modifier contribution to AC, capped by the most restrictive
// worn armor's max-Dex. With no cap (unarmored, or all-uncapped armor) the
// full Dex modifier applies — the d20 unarmored case. Read live each round by
// the defense-channel lookup, so a Dex buff moves AC immediately; only the cap
// snapshot (armorDexCap) moves on equip. Uses the same trunc-toward-zero
// modifier as the WoT defense formula's `trunc((dex-10)/2)`, so the full-Dex
// term and this cap share arithmetic.
func (a *connActor) cappedDexAC() int {
	dexMod := progression.AbilityModifier(a.statBlock.Effective(progression.StatDEX))
	cap := a.armorDexCap.Load()
	if cap == nil || dexMod <= *cap {
		return dexMod
	}
	return *cap
}

// IsArmorProficient reports whether every worn tiered armor is one the actor's
// class(es) grant (armor-depth §5). An unarmored actor and untiered armor are
// always proficient. Mirrors IsWeaponProficient: the class is resolved LIVE by
// id and grants are unioned across a multiclass character. Read on the combat
// goroutine (HitModAdjust) and by the equip cue.
func (a *connActor) IsArmorProficient() bool {
	worn := a.armorTiers.Load()
	if worn == nil || len(*worn) == 0 {
		return true
	}
	a.mu.Lock()
	var granted []string
	if a.classes != nil {
		for _, cid := range a.classIDs {
			if cls, ok := a.classes.Get(cid); ok {
				granted = append(granted, cls.ArmorProficiencyTiers...)
			}
		}
	}
	a.mu.Unlock()
	for _, t := range *worn {
		if !item.ArmorProficient(granted, t) {
			return false
		}
	}
	return true
}

// NonProficientArmorCheckPenalty sums the grade-reduced check penalty of ONLY
// the worn pieces whose tier the actor's class(es) do not grant (armor-depth
// §5). This is the magnitude the §5 non-proficient consequence extends to
// attack rolls — distinct from the summed `armor_check` stat (the §6 skill
// penalty the lockpick path reads, which counts every worn piece). Attributing
// per-piece keeps a mixed loadout
// (a proficient shield + non-proficient body) from over-penalizing to-hit with
// the proficient pieces' penalty. Untiered and proficient pieces contribute
// nothing; 0 when fully proficient or unarmored. The grade reduction mirrors
// the equip-time computation in internal/command/equipment.go (masterwork §3) —
// the two paths must stay in sync; a nil Grades registry skips it. A class
// change (multiclass / SetClass) is honored without a re-equip — the same
// live-proficiency discipline as IsArmorProficient / IsWeaponProficient.
//
// Locking mirrors IsArmorProficient's NARROW window: snapshot the granted
// tiers + worn ids + registry pointers under a.mu, release, then read the item
// store OUTSIDE the lock. This keeps the combat-goroutine hot path (HitModAdjust)
// from holding a.mu across item-store reads — no a.mu→store lock ordering.
func (a *connActor) NonProficientArmorCheckPenalty() int {
	a.mu.Lock()
	if a.items == nil || len(a.equipment) == 0 {
		a.mu.Unlock()
		return 0
	}
	var granted []string
	if a.classes != nil {
		for _, cid := range a.classIDs {
			if cls, ok := a.classes.Get(cid); ok {
				granted = append(granted, cls.ArmorProficiencyTiers...)
			}
		}
	}
	ids := make([]entities.EntityID, 0, len(a.equipment))
	for _, id := range a.equipment {
		ids = append(ids, id)
	}
	items, grades := a.items, a.grades
	a.mu.Unlock()

	// Dedup by id: a spanning piece (multi-slot armor) appears under several
	// keys mapping to one id, so counting distinct ids avoids double-charging it.
	seen := make(map[entities.EntityID]bool, len(ids))
	total := 0
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		e, ok := items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		tier := it.ArmorTier()
		if tier == "" || item.ArmorProficient(granted, tier) {
			continue
		}
		penalty := it.ArmorCheckPenalty()
		if penalty <= 0 {
			continue
		}
		if grades != nil {
			if g, ok := grades.Get(it.Grade()); ok {
				penalty -= g.ArmorCheckImprove
			}
		}
		if penalty > 0 {
			total += penalty
		}
	}
	return total
}

// Saves derives the actor's three saving throws (saves §2): the class-
// granted base bonus (strong/weak progression read at the class's bound-
// track level) plus the d20 modifier of each governing ability read off the
// live stat block. The class is resolved live by id (like IsWeaponProficient)
// so a SetClass change is honored without a relogin; a classless actor gets
// modifier-only saves. Uses the engine-default save curves
// (progression.DefaultSaveConfig) — magnitudes become env-tunable when a
// consumer needs them. Read by the score sheet and the massive-damage
// Fortitude consumer (saves §4).
func (a *connActor) Saves() progression.Saves {
	a.mu.Lock()
	var classes []*progression.Class
	if a.classes != nil {
		for _, cid := range a.classIDs {
			if cls, ok := a.classes.Get(cid); ok {
				classes = append(classes, cls)
			}
		}
	}
	// EPIC S4 §3: snapshot the feat registry + held feats under the same lock,
	// so the conferred per-axis save bonuses (feat.GrantSaveBonus) add on top of
	// the class-base + ability-mod derivation below.
	featReg := a.feats
	var held []feat.Taken
	if a.save != nil && len(a.save.KnownFeats) > 0 {
		held = make([]feat.Taken, 0, len(a.save.KnownFeats))
		for _, kf := range a.save.KnownFeats {
			held = append(held, feat.Taken{FeatID: kf.FeatID, Param: kf.Param, Count: kf.Count})
		}
	}
	a.mu.Unlock()

	// One ClassSaveInput per class; ClassBaseSaves takes the best base bonus
	// per axis across them (saves §2 best-per-axis multiclass). a.Level
	// handles its own locking; resolve it outside a.mu to keep ordering flat.
	var inputs []progression.ClassSaveInput
	for _, cls := range classes {
		inputs = append(inputs, progression.ClassSaveInput{Class: cls, Level: a.Level(cls.BoundTrack)})
	}
	base := progression.ClassBaseSaves(inputs, progression.DefaultSaveConfig())
	derived := progression.DeriveSaves(base, a.statBlock.Effective)
	if featReg != nil && len(held) > 0 {
		if b := feat.ComputeBonuses(held, featReg); len(b.Saves) > 0 {
			axes := make(map[progression.SaveType]int, len(b.Saves))
			for axis, n := range b.Saves {
				axes[progression.SaveType(axis)] = n
			}
			derived = derived.Plus(axes)
		}
	}
	return derived
}

// Equip is the atomic equip-side mutation invoked by the equip command
// handler: removes id from inventory, installs it under every key in
// footprint (footprint[0] is the target/canonical key; the rest are
// companion-slot keys for a spanning item — §3.4 step 8), applies its
// modifiers ONCE to the holder's stat block under EquipmentSourceKey(id),
// and marks the save dirty. Returns false if id is not in inventory (the
// handler treats this as a TOCTOU loss to a concurrent drop).
//
// Auto-swap (§3.4 step 6) and the cancellable veto (step 7) are the
// handler's responsibility — it resolves the footprint and displaces any
// occupants BEFORE calling Equip, so Equip assumes the footprint keys are
// free. Equip is the leaf mutation.
func (a *connActor) Equip(footprint []string, id entities.EntityID, mods []stats.Modifier) bool {
	if len(footprint) == 0 {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Re-verify every footprint key is free under the lock. The handler
	// resolved the footprint against an unlocked Equipment() snapshot and
	// displaced any occupants before calling Equip; this guard makes the
	// mutation self-consistent so an occupied key is never silently
	// overwritten (which would orphan its occupant's footprint). Command
	// dispatch is serialized per session today, so this cannot currently
	// race — it is defensive against a future parallel dispatch and a
	// caller that skipped the displacement step.
	for _, k := range footprint {
		if _, taken := a.equipment[k]; taken {
			return false
		}
	}
	// Verify id is in inventory and remove it atomically with the
	// equipment insertion.
	for i, e := range a.inventory {
		if e == id {
			a.inventory = append(a.inventory[:i], a.inventory[i+1:]...)
			keys := append([]string(nil), footprint...)
			for _, k := range keys {
				a.equipment[k] = id
			}
			a.footprints[id] = keys
			a.statBlock.AddModifiers(entities.EquipmentSourceKey(id), mods)
			a.recomputeWeaponLocked() // §4.5: equipping a weapon arms the actor
			a.syncInventoryToSaveLocked()
			a.syncEquipmentToSaveLocked()
			a.syncStatsToSaveLocked()
			a.markDirtyLocked()
			return true
		}
	}
	return false
}

// Unequip is the atomic unequip-side mutation: removes the item occupying
// slotKey — freeing its ENTIRE footprint (every key a spanning item holds,
// §3.5 step 2), not just slotKey — returns it to inventory, reverses its
// stat modifiers by source key, and marks dirty. Returns the entity id and
// true on success; (empty, false) if the slot is unoccupied.
func (a *connActor) Unequip(slotKey string) (entities.EntityID, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	id, ok := a.equipment[slotKey]
	if !ok {
		return "", false
	}
	keys := a.footprints[id]
	if len(keys) == 0 {
		// Defensive: an item should always have a footprint entry, but if
		// the index is somehow missing fall back to the single key.
		keys = []string{slotKey}
	}
	for _, k := range keys {
		delete(a.equipment, k)
	}
	delete(a.footprints, id)
	a.inventory = append(a.inventory, id)
	a.statBlock.RemoveBySource(entities.EquipmentSourceKey(id))
	a.recomputeWeaponLocked() // §4.5: re-derive the weapon after disarming
	a.syncInventoryToSaveLocked()
	a.syncEquipmentToSaveLocked()
	a.syncStatsToSaveLocked()
	a.markDirtyLocked()
	return id, true
}

// StatsHas reports whether the holder's stat block carries any
// modifiers under src. Test-facing helper; production code goes
// through the equip/unequip mutations.
func (a *connActor) StatsHas(src entities.SourceKey) bool {
	return a.statBlock.HasSource(src)
}

// GrantXP grants amount XP on track via the supplied
// progression.Manager and marks the actor dirty so the next
// Persist commits the new state. Wraps Manager.GrantExperience —
// callers go through this rather than reaching into a.progress
// directly so the dirty bit is consistently flipped on every
// mutation path (admin xp verb today, future combat-driven and
// quest-driven sources later).
//
// Returns the structured result for renderers ("you reach level
// 2!"). A nil manager is a no-op (returns TrackUnknown=true);
// tests that don't wire Progression don't need to special-case.
func (a *connActor) GrantXP(ctx context.Context, mgr *progression.Manager, track, source string, amount int64) progression.GrantResult {
	if mgr == nil {
		return progression.GrantResult{Track: track, TrackUnknown: true}
	}
	res := mgr.GrantExperience(ctx, a.progress, a.CombatantIDString(), track, amount, source)
	if res.XPAdded > 0 || res.NewLevel != res.OldLevel {
		a.mu.Lock()
		a.syncProgressionToSaveLocked()
		a.markDirtyLocked()
		a.mu.Unlock()
	}
	return res
}

// DeductXP removes amount XP on track via the supplied Manager
// and marks dirty if anything was lost. Wraps
// Manager.DeductExperience.
func (a *connActor) DeductXP(ctx context.Context, mgr *progression.Manager, track string, amount int64) progression.DeductResult {
	if mgr == nil {
		return progression.DeductResult{Track: track, TrackUnknown: true}
	}
	res := mgr.DeductExperience(ctx, a.progress, a.CombatantIDString(), track, amount)
	if res.XPLost > 0 {
		a.mu.Lock()
		a.syncProgressionToSaveLocked()
		a.markDirtyLocked()
		a.mu.Unlock()
	}
	return res
}

// TrackInfo returns the structured per-track view (level, XP,
// XpToNext, etc.) for renderers. Returns (zero, false) when track
// is unknown.
func (a *connActor) TrackInfo(mgr *progression.Manager, track string) (progression.TrackInfo, bool) {
	if mgr == nil {
		return progression.TrackInfo{}, false
	}
	return mgr.GetTrackInfo(a.progress, track)
}

// CombatantIDString returns the string form of CombatantID, used
// as the entity-id payload in progression event emissions. Mirrors
// the form combat already uses for player events.
func (a *connActor) CombatantIDString() string {
	return string(a.CombatantID())
}

// RaceID returns the actor's resolved race id. Empty means
// raceless (no flags applied, no cast-cost modifier). Set once at
// construction; never mutates for the life of the actor.
func (a *connActor) RaceID() string { return a.raceID }

// Size returns the actor's size category (size-and-wielding §3.1), resolved
// from its race (the baseline when raceless or the race declares none). No
// per-character size choice exists. Acquires a.mu.
func (a *connActor) Size() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sizeLocked()
}

// sizeLocked is Size without taking a.mu — for callers that already hold it
// (recomputeWeaponLocked). Reads the resolved race id and the immutable race
// registry; falls back to the baseline size when raceless or the race carries
// no size.
func (a *connActor) sizeLocked() string {
	if a.race != nil && a.race.Size != "" {
		return a.race.Size
	}
	return size.Baseline
}

// Gender returns the actor's gender ("male"/"female", lowercase), chosen at
// creation and persisted on the save (v22+). Empty means unset (a pre-v22
// character, or a pack whose creation flow omits the gender step). Read under
// the lock because it lives on the shared save struct. The WoT affinity layer
// reads this to derive a channeler's saidin/saidar element strengths.
func (a *connActor) Gender() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return ""
	}
	return a.save.Gender
}

// Madness returns the actor's accumulated saidin taint (WoT S2 Phase 4+);
// 0 means clean. Lives on the save (like Gender), read under the lock.
func (a *connActor) Madness() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return 0
	}
	return a.save.Madness
}

// ChannelingGift returns the actor's WoT channeling origin chosen at creation
// ("spark"/"learn"/"none"); empty means unset (a non-WoT character, or a save
// pre-v28). Lives on the save (like Gender/Madness), read under the lock. Read
// by `score` and available to future S2 hooks.
func (a *connActor) ChannelingGift() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return ""
	}
	return a.save.ChannelingGift
}

// LearnLanguage adds a language id to the actor's known set (languages.md §3),
// idempotent and case-insensitive — a tongue already known is a no-op. The id
// is stored as given (the granter passes the qualified id the registry uses).
// Marks the save dirty on a real change so autosave persists it. The granter
// calls this for a background's home language; an empty id is ignored.
func (a *connActor) LearnLanguage(id string) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return
	}
	if slices.Contains(a.save.KnownLanguages, id) {
		return // already known — idempotent
	}
	a.save.KnownLanguages = append(a.save.KnownLanguages, id)
	a.markDirtyLocked()
}

// KnownLanguages returns the display names of every language the actor knows,
// id-sorted for a stable listing (languages.md §4). Resolves each id through the
// language registry; an id with no registered language renders by its id rather
// than vanishing or erroring. Read by `score` and the `languages` verb via a
// duck-typed interface assertion, mirroring ChannelingGift.
func (a *connActor) KnownLanguages() []string {
	a.mu.Lock()
	var ids []string
	if a.save != nil {
		ids = append([]string(nil), a.save.KnownLanguages...)
	}
	reg := a.languages
	a.mu.Unlock()
	if len(ids) == 0 {
		return nil
	}
	sort.Strings(ids)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if reg != nil {
			out = append(out, reg.DisplayName(id))
			continue
		}
		out = append(out, id)
	}
	return out
}

// HasFeat reports whether the actor has taken the given feat (case-insensitive),
// reading the persisted KnownFeats under the lock. A lightweight query for hot
// paths (the madness accrual / manifestation seam) that don't need the full
// buildFeatCharView eligibility snapshot.
func (a *connActor) HasFeat(featID string) bool {
	id := strings.ToLower(strings.TrimSpace(featID))
	if id == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return false
	}
	for _, kf := range a.save.KnownFeats {
		if strings.ToLower(strings.TrimSpace(kf.FeatID)) == id {
			return true
		}
	}
	return false
}

// AddMadness adjusts the actor's saidin taint by delta — positive accrues (a
// man weaving), negative cures (Heal the Mind) — floored at 0, and returns the
// resulting value. Marks the save dirty so autosave persists the change. WoT S2
// Phase 4+. The MALE-only rule lives at the call site (the composition root),
// not here: this is a neutral counter mutator.
func (a *connActor) AddMadness(delta int) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return 0
	}
	next := max(a.save.Madness+delta, 0)
	if next == a.save.Madness {
		// No change (e.g. curing an already-clean target, or decaying at 0) —
		// don't dirty the save and force a spurious autosave write.
		return next
	}
	a.save.Madness = next
	a.markDirtyLocked()
	return next
}

// Tags returns the actor's session-side tag set — today just the
// racial flags from the race definition (M8.3). Returns a fresh
// slice so callers cannot alias the backing storage. Surfaces to
// the AI disposition evaluator via session.Manager.PlayersInRoom
// and dispositionHook so `has_tag` rules can match on racial
// flags. Future per-actor tags (admin role, party affiliation,
// curse effects) will join this list as their consumers arrive.
func (a *connActor) Tags() []string {
	a.mu.Lock()
	at := a.alignmentTag
	rt := a.reputationTierTag
	ftags := make([]string, 0, len(a.factionRankTags))
	for _, t := range a.factionRankTags {
		ftags = append(ftags, t)
	}
	var admin []string
	if a.save != nil {
		admin = append(admin, a.save.AdminTags...)
	}
	a.mu.Unlock()
	if len(a.racialTags) == 0 && at == "" && rt == "" && len(ftags) == 0 && len(admin) == 0 {
		return nil
	}
	out := make([]string, 0, len(a.racialTags)+2+len(ftags)+len(admin))
	out = append(out, a.racialTags...)
	if at != "" {
		out = append(out, at)
	}
	if rt != "" {
		out = append(out, rt)
	}
	out = append(out, ftags...)
	// Admin-applied free-form tags (admin-verbs §4 `set tag`) — the one
	// hand-authored, persisted category, kept in its own save bag so the
	// managers above retain sole ownership of their derived tags.
	out = append(out, admin...)
	return out
}

// AddTag records a free-form admin gameplay tag on the character (admin-verbs
// §4 `set tag add`). Idempotent — a tag already present (whether admin-applied
// or manager-derived, e.g. a racial flag) is left as-is so Tags() never
// double-lists it. Persisted in the save's AdminTags bag; marks the save dirty
// so autosave commits it and it survives relog. Reports whether it changed.
// The manager-owned namespaces are refused at the command layer (set.go), not
// here — this is a neutral bag mutator.
func (a *connActor) AddTag(tag string) bool {
	if tag == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return false
	}
	if slices.Contains(a.save.AdminTags, tag) {
		return false
	}
	// Also skip a tag a manager already contributes (racial flag), so the
	// admin bag never duplicates a derived tag in Tags().
	if a.hasDerivedTagLocked(tag) {
		return false
	}
	a.save.AdminTags = append(a.save.AdminTags, tag)
	a.markDirtyLocked()
	return true
}

// RemoveTag drops a free-form admin gameplay tag (admin-verbs §4 `set tag
// remove`). Only the AdminTags bag is touched — a manager-derived tag cannot
// be removed here (those clear through their managers), so removing one is a
// no-op. Marks the save dirty when the set changes. Reports whether it changed.
func (a *connActor) RemoveTag(tag string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return false
	}
	// Filter in place: the write position (len of out) is always ≤ the read
	// index, so no element is overwritten before it is read. Reuses the
	// backing array — same idiom as MobInstance.RemoveTag / SetAlignmentTag.
	out := a.save.AdminTags[:0]
	removed := false
	for _, t := range a.save.AdminTags {
		if t == tag {
			removed = true
			continue
		}
		out = append(out, t)
	}
	a.save.AdminTags = out
	if removed {
		a.markDirtyLocked()
	}
	return removed
}

// hasDerivedTagLocked reports whether a manager-derived tag (racial flag,
// alignment bucket, faction rank, reputation tier) currently equals tag.
// Caller holds a.mu. Used by AddTag to keep the admin bag from duplicating a
// tag a manager already contributes to Tags().
func (a *connActor) hasDerivedTagLocked(tag string) bool {
	if tag == a.alignmentTag || tag == a.reputationTierTag {
		return true
	}
	if slices.Contains(a.racialTags, tag) {
		return true
	}
	for _, t := range a.factionRankTags {
		if t == tag {
			return true
		}
	}
	return false
}

// applyRace resolves the actor's race id from save (taking
// cfg.DefaultRace as the fallback for an empty save value) and
// snapshots the race's RacialFlags onto the actor for the life of
// the session. Called once during construction in run(); the
// resolved id is also written back to a.save.Race so the next
// Persist commits the default-substitution. If the resolved id is
// not in the registry (race removed between restarts) the actor
// stays raceless with an empty raceID — fail-soft mirrors the mob
// spawn behavior.
func applyRace(a *connActor, cfg *Config, saved string) {
	candidate := strings.ToLower(strings.TrimSpace(saved))
	if candidate == "" {
		candidate = strings.ToLower(strings.TrimSpace(cfg.DefaultRace))
	}
	if candidate == "" || cfg.Races == nil {
		return
	}
	race, ok := cfg.Races.Get(candidate)
	if !ok {
		// Race id present on save but not in the registry —
		// content removed it. Keep raceID empty so no stale flags
		// apply. The next save still records the requested id so
		// content authors who readd the race see their players
		// reconnect with it.
		return
	}
	a.raceID = race.ID
	a.race = race
	if len(race.RacialFlags) > 0 {
		a.racialTags = make([]string, len(race.RacialFlags))
		copy(a.racialTags, race.RacialFlags)
	}
}

// ClassID returns the actor's current class id. Empty means classless
// (no path / no stat growth on level-up). Normally set once at
// construction, but a quest class-unlock reward may rewrite it via
// SetClass (M10.10) — readers must not cache it as immutable.
func (a *connActor) ClassID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.classIDs) == 0 {
		return ""
	}
	return a.classIDs[0] // primary class
}

// ClassIDs returns a copy of the actor's full class id list (wot-character-
// model D1). One element today; composing readers (the level-up subscriber,
// alignment seed) walk it so a future second class-track works without code
// changes at the call sites.
func (a *connActor) ClassIDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.classIDs...)
}

// --- quest.Player (M10.10) ---
//
// connActor satisfies quest.Player so the accept verb and the quest
// reward dispatcher can read prereq inputs and apply class/race unlocks.
// EntityID is the player id; the methods below cover level, class, and
// the unlock setters.

// Class returns the actor's class id (quest.Player). Alias of ClassID.
func (a *connActor) Class() string { return a.ClassID() }

// Level returns the actor's level on a progression track (quest.Player),
// defaulting to 1 when the track is uninitialized (spec quests.md §3.2).
func (a *connActor) Level(track string) int {
	lvl := max(a.progress.Level(track), 1)
	return lvl
}

// SetClass applies a quest class-unlock (quest.Player, §5.4). Unlike
// the construction-time classID (normally immutable), a quest reward may
// rewrite it; the new id is mirrored into the save and the dirty bit is
// flipped so it persists.
// SetClass REPLACES the actor's class with a single class id (the quest
// class-swap; v1 single-class — wot-character-model D1). A future true
// multiclass grant (add a second class-track) gets its own AddClass method;
// the seam — the classIDs list — already supports it.
func (a *connActor) SetClass(classID string) {
	a.mu.Lock()
	a.classIDs = []string{classID}
	// Re-resolve the primary class pointer so the generated look description
	// reflects the swap without a relogin (it would otherwise show the old
	// class). nil when the id no longer resolves (removed content).
	a.class = nil
	if a.classes != nil {
		if cls, ok := a.classes.Get(classID); ok {
			a.class = cls
		}
	}
	if a.save != nil {
		a.save.Class = []string{classID}
	}
	a.markDirtyLocked()
	a.mu.Unlock()
}

// SetRace applies a quest race-unlock (quest.Player, §5.4).
func (a *connActor) SetRace(raceID string) {
	a.mu.Lock()
	a.raceID = raceID
	if a.save != nil {
		a.save.Race = raceID
	}
	a.markDirtyLocked()
	a.mu.Unlock()
}

// TrainsAvailable returns the actor's current training pool. Read
// by the M8.6 train verb; surfaced on score panels later.
func (a *connActor) TrainsAvailable() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.trainsAvailable
}

// SpendTrain decrements the actor's training pool by one and
// marks the save dirty. Returns false when the pool is already
// zero (the train verb pre-checks but a concurrent grant/spend
// could TOCTOU; the false branch keeps the manager honest).
// M8.6 — progression.md §7.4 step 6.
func (a *connActor) SpendTrain() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.trainsAvailable <= 0 {
		return false
	}
	a.trainsAvailable--
	if a.save != nil {
		a.save.TrainsAvailable = a.trainsAvailable
	}
	a.markDirtyLocked()
	return true
}

// HasRoomTag reports whether the actor's current room carries tag.
// Used by the M8.6 training manager for the §7.4 step 2 safe-room gate
// (tag "safe"). Now backed by world.Room.Tags (cluster 2): a host can
// enable TrainingConfig.RequireSafeRoomForStats and tag rooms `safe`.
// Reads a.room under a.mu (SetRoom mutates it from the move path).
func (a *connActor) HasRoomTag(tag string) bool {
	a.mu.Lock()
	room := a.room
	a.mu.Unlock()
	return room != nil && room.HasTag(tag)
}

// CreditTrains adds n to trainsAvailable and marks the actor
// dirty so the next Persist commits the new pool. Used by the
// M8.4 stat-growth subscriber on every bound-track level-up
// (spec §4.6 step 4). Negative values are clamped at zero — the
// train verb is the only path that subtracts and it handles
// underflow itself.
func (a *connActor) CreditTrains(_ context.Context, _ string, n int) {
	if n == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if n < 0 && -n > a.trainsAvailable {
		a.trainsAvailable = 0
	} else {
		a.trainsAvailable += n
	}
	if a.save != nil {
		a.save.TrainsAvailable = a.trainsAvailable
	}
	a.markDirtyLocked()
}

// FeatCredits returns the actor's banked-but-unspent feat slots (EPIC S4
// Phase 2 — docs/proposals/wot-feats.md §2.2). Read by the feat verb (Phase 4)
// and surfaced on score later.
func (a *connActor) FeatCredits() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.featCredits
}

// CreditFeats adds n feat slots to the actor's banked pool and marks the save
// dirty (EPIC S4 Phase 2). Mirrors CreditTrains: called off the bus dispatch
// from the character-created + level-up subscribers (1 at creation, 1 per 3
// character levels). n<=0 is a no-op; the feat verb (Phase 4) owns the
// spend-side decrement + underflow.
func (a *connActor) CreditFeats(n int) {
	if n <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.featCredits += n
	if a.save != nil {
		a.save.FeatCredits = a.featCredits
	}
	a.markDirtyLocked()
}

// Alignment returns the actor's current alignment integer
// (M8.5 — spec §6.1). Reads under a.mu so the value is consistent
// with concurrent Shift writes.
func (a *connActor) Alignment() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.alignment
}

// MeetsFactionStanding reports whether the actor's effective standing with
// factionID is at least min (faction.md §6 — the ability faction gate seam,
// progression.ValidationEntity). Resolves through the faction manager so an
// untouched faction reads at its starting standing; fails open (true) when
// faction isn't wired or the faction isn't in content, so a content typo never
// locks an ability. Does not take a.mu — the manager's Get reads the standing
// bag through the faction.Entity adapter, which locks a.mu itself.
func (a *connActor) MeetsFactionStanding(factionID string, min int) bool {
	if a.faction == nil {
		return true
	}
	def, ok := a.faction.Registry().Get(factionID)
	if !ok {
		return true
	}
	return a.faction.MeetsStanding(a, def, min)
}

// SetAlignment writes the alignment integer. Used by the
// AlignmentEntity adapter under the manager's per-entity lock.
// Marks the save dirty so the new value rides to disk on the
// next Persist.
func (a *connActor) SetAlignment(value int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.alignment = value
	if a.save != nil {
		a.save.Alignment = value
	}
	a.markDirtyLocked()
}

// SetAlignmentTag mirrors the bucket tag onto the actor (spec
// §6.2). Empty tag clears the slot. AI disposition matching
// reads through Tags(), which appends the alignment tag to the
// racial-flag list.
func (a *connActor) SetAlignmentTag(tag string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.alignmentTag = tag
}

// AlignmentTag returns the actor's current bucket tag (one of
// progression.TagAlignment{Evil,Neutral,Good} or empty for an
// untouched actor). Read by the session manager to populate
// PlayerInfo.Bucket without needing a reference to the
// AlignmentManager.
func (a *connActor) AlignmentTag() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.alignmentTag
}

// --- faction.Entity adapter (faction.md §4) ----------------------------
//
// connActor satisfies faction.Entity so the FactionManager can read/write
// per-faction standing and mirror rank tags. Mirrors the AlignmentEntity
// adapter above; the same concurrency contract applies (the manager holds no
// per-entity lock across these callbacks).

// Standing returns the actor's stored standing with factionID and whether an
// entry is present (faction.md §3.1; absent → the manager substitutes the
// faction's starting standing).
func (a *connActor) Standing(factionID string) (int, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, ok := a.factionStanding[factionID]
	return v, ok
}

// SetStanding writes the actor's standing with factionID (the manager passes a
// value already clamped to the faction's bounds), mirrors it into the save bag,
// and marks the actor dirty so it rides to disk on the next Persist.
func (a *connActor) SetStanding(factionID string, value int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.factionStanding == nil {
		a.factionStanding = make(map[string]int)
	}
	a.factionStanding[factionID] = value
	if a.save != nil {
		if a.save.FactionStanding == nil {
			a.save.FactionStanding = make(map[string]int)
		}
		a.save.FactionStanding[factionID] = value
	}
	a.markDirtyLocked()
}

// SetRankTag installs the rank tag for factionID (faction.md §3.3). The map is
// keyed by faction id, so a new tag inherently replaces the prior one for that
// faction only; an empty tag clears it. Rank tags are derived state — never
// persisted (re-synced from standing on login).
func (a *connActor) SetRankTag(factionID, rankTag string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.factionRankTags == nil {
		a.factionRankTags = make(map[string]string)
	}
	if rankTag == "" {
		delete(a.factionRankTags, factionID)
		return
	}
	a.factionRankTags[factionID] = rankTag
}

// Renown returns the actor's current renown score (reputation.md §10),
// satisfying reputation.Entity. Reads under a.mu for consistency with concurrent
// SetRenown writes.
func (a *connActor) Renown() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.renown
}

// SetRenown writes the actor's renown (the manager passes a value already
// clamped to the configured bounds), mirrors it into the save, and marks the
// actor dirty so it rides to disk on the next Persist.
func (a *connActor) SetRenown(value int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.renown = value
	if a.save != nil {
		a.save.Reputation = value
	}
	a.markDirtyLocked()
}

// SetTierTag installs the renown tier tag (reputation.md §3). A new tag replaces
// the prior (exactly one renown tier tag); an empty tag clears it. Derived state
// — never persisted (re-synced from the score on login).
func (a *connActor) SetTierTag(tierTag string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reputationTierTag = tierTag
}

// RenownTier returns the actor's current renown tier display name (reputation.md
// §3), resolved from EFFECTIVE renown (base + the Fame feat bonus, §7) via the
// manager's ladder, so the score sheet's tier and number agree. "" when
// reputation isn't wired. RenownBonus reads its own atomic cache, so it is
// resolved outside a.mu (don't nest the locks).
func (a *connActor) RenownTier() string {
	a.mu.Lock()
	mgr := a.reputation
	base := a.renown
	a.mu.Unlock()
	if mgr == nil {
		return ""
	}
	return mgr.Config().TierOf(base + a.RenownBonus() + a.WornReputation())
}

// Gold returns the actor's current balance (M11.1 — spec §2.1).
// Reads under a.mu so the value is consistent with concurrent
// AddGold / SetGold writes. Satisfies economy.Entity.
func (a *connActor) Gold() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.gold
}

// SetGold writes the balance. Used by the economy.CurrencyService
// adapter; the service always passes a value already floored at
// zero. Mirrors the value into a.save.Gold and marks the save
// dirty so the new balance rides to disk on the next Persist —
// same write-through discipline as SetAlignment. Satisfies
// economy.Entity.
func (a *connActor) SetGold(value int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.gold = value
	if a.save != nil {
		a.save.Gold = value
	}
	a.markDirtyLocked()
}

// Sustenance returns the actor's current hunger pool (M11.3 — spec
// §4.1). Reads under a.mu so the value is consistent with concurrent
// Drain / consume writes. Satisfies economy.SustenanceEntity.
func (a *connActor) Sustenance() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sustenance
}

// SetSustenance writes the pool. Used by the economy.SustenanceService
// adapter; the service always passes a value already clamped to
// [0, MaxSustenance]. Mirrors into a.save.Sustenance and marks dirty so
// the value rides to disk on the next Persist — same write-through
// discipline as SetGold. Satisfies economy.SustenanceEntity.
func (a *connActor) SetSustenance(value int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sustenance = value
	if a.save != nil {
		a.save.Sustenance = value
	}
	a.markDirtyLocked()
}

// RestState returns the actor's current rest state ("" == awake). Reads
// under a.mu. Satisfies economy.RestEntity.
func (a *connActor) RestState() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.restState
}

// SetRestState writes the transient rest state. Used by the
// economy.RestService adapter. Does NOT mark the save dirty — rest
// state never persists (spec §5.1). Satisfies economy.RestEntity.
func (a *connActor) SetRestState(state string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.restState = state
}

// SetRestTarget sets (or clears, with "") the furniture id being rested
// on (spec §5.2). Transient; no save write-through. Satisfies
// economy.RestEntity.
func (a *connActor) SetRestTarget(furnitureID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.restTargetID = furnitureID
}

// SetSleepStart records the tick sleeping began (spec §5.2). Transient;
// no save write-through. Satisfies economy.RestEntity.
func (a *connActor) SetSleepStart(tick uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sleepStartTick = tick
}

// PendingCraft returns the actor's in-flight timed craft and whether one is
// active. Reads under a.mu. Satisfies crafting.CraftBusy.
func (a *connActor) PendingCraft() (crafting.PendingCraft, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.craftPending, a.hasCraft
}

// SetPendingCraft records a started timed craft. Returns false (refusing
// the new craft) when one is already in flight. Transient; no save
// write-through — a craft never persists. Satisfies crafting.CraftBusy.
func (a *connActor) SetPendingCraft(p crafting.PendingCraft) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.hasCraft {
		return false
	}
	a.craftPending = p
	a.hasCraft = true
	return true
}

// ClearPendingCraft drops any in-flight craft, returning what was cleared
// and whether one was active. Satisfies crafting.CraftBusy.
func (a *connActor) ClearPendingCraft() (crafting.PendingCraft, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p, had := a.craftPending, a.hasCraft
	a.craftPending = crafting.PendingCraft{}
	a.hasCraft = false
	return p, had
}

// ForageReadyAt / SetForageReadyAt hold the transient per-character forage
// cooldown (gathering.md §5). Guarded by a.mu; never persisted. Satisfies
// gathering.Gatherer.
func (a *connActor) ForageReadyAt() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.forageReadyAt
}

func (a *connActor) SetForageReadyAt(tick uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.forageReadyAt = tick
}

// IsWeaponLoaded reports whether the wielded reload-gated weapon is chambered
// (action-economy.md §7.1). The loaded state is keyed to a specific weapon id,
// so it reads false after a weapon swap. The `load` verb gates its already-loaded
// check on this; the FIRE paths use TakeLoadedShot (an atomic check-and-clear).
func (a *connActor) IsWeaponLoaded() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loadedWeapon != "" && a.equipment[mainHandSlot] == a.loadedWeapon
}

// TakeLoadedShot atomically consumes the chambered shot for a fire: it returns
// true (clearing the loaded state) only when the wielded weapon is the one that
// was loaded, and false otherwise. One lock acquisition, so the check and the
// discharge cannot interleave with a concurrent weapon swap (action-economy.md
// §7.1) — the fire paths (round loop + shoot) call this instead of a separate
// IsWeaponLoaded + ClearWeaponLoaded pair.
func (a *connActor) TakeLoadedShot() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loadedWeapon == "" || a.equipment[mainHandSlot] != a.loadedWeapon {
		return false
	}
	a.loadedWeapon = ""
	return true
}

// SetWeaponLoaded chambers the currently-wielded weapon (the load action's
// completion). Returns false when nothing is wielded — the load then fails
// cleanly (the player unwielded mid-load). The bolt is consumed by the load
// handler, not here.
func (a *connActor) SetWeaponLoaded() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := a.equipment[mainHandSlot]
	if id == "" {
		return false
	}
	a.loadedWeapon = id
	return true
}

// ClearWeaponLoaded drops the chambered state. Used only to back out a load
// whose bolt couldn't be spent (the ammo-out undo); the fire paths discharge
// atomically via TakeLoadedShot. No-op when nothing is loaded.
func (a *connActor) ClearWeaponLoaded() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.loadedWeapon = ""
}

// CancelCraft drops any in-flight craft and, if there was one, writes an
// interrupt notice to the actor. Returns whether a craft was cancelled. The
// combat-engagement sink calls this so being drawn into a fight breaks a
// craft (crafting-and-cooking §3, mirroring the rest combat-wake).
func (a *connActor) CancelCraft(ctx context.Context) bool {
	if _, had := a.ClearPendingCraft(); !had {
		return false
	}
	_ = a.Write(ctx, "Your concentration breaks and you abandon your work.")
	return true
}

// shouldRemindHunger reports whether a hunger reminder may be sent to
// this player at tick now, recording the tick when it returns true.
// Throttles to at most one reminder per interval ticks (spec §4.4). A
// zero lastHungerReminderTick (never reminded) always fires. Mutates
// under a.mu so the drain tick goroutine's read-and-set is atomic.
func (a *connActor) shouldRemindHunger(now, interval uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lastHungerReminderTick != 0 && now-a.lastHungerReminderTick < interval {
		return false
	}
	a.lastHungerReminderTick = now
	return true
}

// HasTag reports whether the actor carries tag. Used by the
// AlignmentEntity adapter to detect the admin role bypass
// (spec §6.4 Shift step 2). Scans racial flags + alignment tag
// in a single locked section. racialTags is write-once at
// construction today, but a future verb that mutates it would
// race a tags reader without the lock — taking a.mu here keeps
// the surface consistent with Tags() and future-proofs the
// admin-tag addition (M10+ role system).
func (a *connActor) HasTag(tag string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if slices.Contains(a.racialTags, tag) {
		return true
	}
	if a.alignmentTag == tag {
		return true
	}
	if a.reputationTierTag == tag {
		return true
	}
	for _, t := range a.factionRankTags {
		if t == tag {
			return true
		}
	}
	return false
}

// StatBlock returns the actor's progression-layer stat block. The
// returned pointer is the live block — callers MUST treat it as
// read-mostly and use StatBlock's own internal lock for any
// mutation (AdjustBase, AddModifiers, etc.). Used by the M8.4
// stat-growth subscriber to apply level-up growth dice without
// having to thread a.mu through.
func (a *connActor) StatBlock() *progression.StatBlock { return a.statBlock }

// EntityID implements progression.EffectTarget (spec
// abilities-and-effects §5). The id under which the
// EffectManager keys active effects matches PlayerID; the alias
// exists so connActor satisfies both the TrainingEntity and
// EffectTarget interfaces directly without a wrapper adapter.
func (a *connActor) EntityID() string { return a.playerID }

// AddModifiers implements progression.EffectTarget by
// delegating to the actor's stat block. The EffectManager calls
// this when applying an active effect's stat deltas under
// EffectSourceKey(effectID).
func (a *connActor) AddModifiers(src entities.SourceKey, mods []stats.Modifier) {
	a.statBlock.AddModifiers(src, mods)
	a.mu.Lock()
	a.markDirtyLocked()
	a.mu.Unlock()
}

// RemoveBySource implements progression.EffectTarget by
// delegating to the actor's stat block. Returns whether anything
// was removed so the EffectManager can distinguish "effect was
// flag-only" from "stat reversal happened" for diagnostics.
func (a *connActor) RemoveBySource(src entities.SourceKey) bool {
	removed := a.statBlock.RemoveBySource(src)
	if removed {
		a.mu.Lock()
		a.markDirtyLocked()
		a.mu.Unlock()
	}
	return removed
}

// ProgressionState returns the actor's per-track (level, xp)
// state. Same contract as StatBlock — the state has its own lock.
func (a *connActor) ProgressionState() *progression.ProgressionState { return a.progress }

// --- progression.ResolutionSource / ValidationEntity (M9.4b) -------
//
// connActor satisfies progression.ResolutionSource so the ability-
// resolution phase can validate (§4.3) and resolve (§4.5) a player's
// queued abilities. ResolutionSource embeds ValidationEntity, so this
// block supplies both surfaces. EntityID / Alignment are already
// defined above (shared with EffectTarget / AlignmentEntity).

// IsResting reports the §4.3 step-1 rest-state gate. No rest/sleep
// state exists for players yet (it lands with economy-survival,
// M11), so players are never resting — abilities are always
// rest-permitted today.
func (a *connActor) IsResting() bool { return false }

// EquippedTags returns the tag list of the item equipped in slot
// (spec §4.3 step 4). Second return is false when the slot is empty
// or the item can't be resolved in the store. ItemInstance.Tags()
// already returns a fresh copy, so the result is caller-owned.
//
// a.mu is held across the store lookup so the slot→id read and the
// item resolution are atomic against a concurrent Unequip on the
// connection goroutine. Without this, the ability phase (tick
// goroutine) could read a slot id, have Unequip return that item to
// inventory, and then still resolve its tags — validating a
// slot-required ability against gear the actor no longer wears.
// Lock ordering is a.mu → Store's internal RWMutex; no Store path
// re-enters connActor under its lock, so this can't deadlock.
func (a *connActor) EquippedTags(slot string) ([]string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	id, ok := a.equipment[slot]
	if !ok || a.items == nil {
		return nil, false
	}
	ent, ok := a.items.GetByID(id)
	if !ok {
		return nil, false
	}
	it, ok := ent.(*entities.ItemInstance)
	if !ok {
		return nil, false
	}
	return it.Tags(), true
}

// InCombat reports whether the actor is engaged (spec §4.3 steps
// 5-6). Reads the combat manager keyed by the actor's CombatantID.
// Nil-safe: an actor built without a combat manager is never in
// combat.
func (a *connActor) InCombat() bool {
	if a.combat == nil {
		return false
	}
	return a.combat.InCombat(a.CombatantID())
}

// CurrentTarget returns the actor's primary combat target as a bare
// entity id (spec §4.4 step 2). Combat tracks targets as prefixed
// CombatantIDs ("mob:wolf-3"); the resolver / effect manager key on
// the bare entity id, so we strip the prefix here. Second return is
// false when the actor has no combat target.
func (a *connActor) CurrentTarget() (string, bool) {
	if a.combat == nil {
		return "", false
	}
	t, ok := a.combat.PrimaryTargetOf(a.CombatantID())
	if !ok {
		return "", false
	}
	return combat.EntityIDOf(t), true
}

// poolKindMana / poolKindMovement name the actor's resource pools. The
// string values match progression.ResourceMana / ResourceMovement so the
// resolver's ResourceFor → pool routing stays consistent.
const (
	poolKindMana     = pool.Kind("mana")
	poolKindMovement = pool.Kind("movement")
)

// resourcePool returns the actor's pool of the given kind. Nil-safe for
// bare test-built actors that never seeded a pool.Set.
func (a *connActor) resourcePool(kind pool.Kind) (*pool.Pool, bool) {
	if a.pools == nil {
		return nil, false
	}
	return a.pools.Get(kind)
}

// seedResourcePools creates the mana + movement pools full from the
// finalized stat maxes and binds each ceiling to its max stat via
// OnMaxChange — the same pattern that ties Vitals to hp_max. Called once
// from the constructor AFTER the stat block is restored. Today both maxes
// default to 0, so the pools spawn empty and cost-bearing abilities still
// fizzle until content grants a max (or a channeling pool is added): this
// makes the substrate real, it does not change live numbers.
//
// Each pool is seeded FULL from its stat-derived max (binding OnMaxChange
// so the ceiling tracks the stat), then any persisted current from the
// save is applied on top via SetCurrent — so a character who logged out
// mid-drain returns drained, while a full or unseeded pool defaults full.
// The save persists only `current` (the max is re-derived here), so a
// rebalanced max stat never needs a migration (player save §Pools, v21).
func (a *connActor) seedResourcePools() {
	if a.pools == nil {
		a.pools = pool.NewSet()
	}
	// Binds come from the active world's player-seed pool decls (SR-M3a step 3);
	// fall back to the hardcoded mana/movement pair only when no pool content
	// resolved (a bare test actor or a degenerate boot) — the same nil-content
	// fallback discipline SR-M1 uses for the attribute set. Because the core
	// pack now declares mana/movement identically (the step-2 regression gate),
	// the real boot takes the data-driven branch and produces the same pools.
	type poolBind struct {
		kind    pool.Kind
		channel progression.StatType
		formula string
		rules   pool.Rules
	}
	var binds []poolBind
	if len(a.poolDecls) > 0 {
		for _, d := range a.poolDecls {
			binds = append(binds, poolBind{d.Kind, progression.StatType(d.MaxChannel), d.MaxFormula, d.Rules})
		}
	} else {
		binds = []poolBind{
			{poolKindMana, progression.StatResourceMax, "", pool.Rules{Floor: 0}},
			{poolKindMovement, progression.StatMovementMax, "", pool.Rules{Floor: 0}},
		}
	}
	for _, b := range binds {
		entities.SeedPoolInto(a.pools, a.statBlock, b.kind, b.channel, b.formula, b.rules)
	}
	// Apply persisted currents AFTER seeding full + binding maxes. SetCurrent
	// clamps to [floor, live max], so a stale persisted value (max shrank
	// between sessions) is pulled into range rather than trusted blindly.
	if a.save != nil {
		for _, e := range a.save.Pools {
			if p, ok := a.pools.Get(e.Kind); ok {
				p.SetCurrent(e.Current)
			}
		}
	}
}

// playerSeedPoolDecls returns the player-seeded pool decls from the registry in
// deterministic (kind-sorted) order. A nil registry yields nil, which makes
// seedResourcePools fall back to the hardcoded mana/movement pair (a bare test
// actor / a boot with no pool content).
func playerSeedPoolDecls(reg *pool.Registry) []*pool.Decl {
	if reg == nil {
		return nil
	}
	return reg.PlayerSeed()
}

// Movement returns the actor's current movement pool for the §4.7 skill
// resource check — the live pool current (0 when no pool is seeded).
func (a *connActor) Movement() int {
	if p, ok := a.resourcePool(poolKindMovement); ok {
		return p.Current()
	}
	return 0
}

// Mana returns the actor's current mana pool for the §4.7 spell resource
// check — the live pool current (0 when no pool is seeded).
func (a *connActor) Mana() int {
	if p, ok := a.resourcePool(poolKindMana); ok {
		return p.Current()
	}
	return 0
}

// ManaMax returns the actor's mana pool ceiling (0 when no pool is
// seeded). Distinct from Mana(): with real pools the prompt must show
// current/max separately, not current/current.
func (a *connActor) ManaMax() int {
	if p, ok := a.resourcePool(poolKindMana); ok {
		return p.Max()
	}
	return 0
}

// MovementMax returns the actor's movement pool ceiling (0 when no pool
// is seeded). See ManaMax for why this is distinct from Movement().
func (a *connActor) MovementMax() int {
	if p, ok := a.resourcePool(poolKindMovement); ok {
		return p.Max()
	}
	return 0
}

// resourceSnapshot returns the (current, max) of the named pool atomically
// — the TOCTOU-safe way for a caller (the prompt) needing both, vs two
// separate Current()/Max() reads a concurrent regen tick could tear into a
// transient current > max. (0, 0) for an unseeded pool.
func (a *connActor) resourceSnapshot(kind pool.Kind) (cur, max int) {
	if p, ok := a.resourcePool(kind); ok {
		return p.Snapshot()
	}
	return 0, 0
}

// Race returns the actor's resolved race for §4.7 cost adjustment.
// nil when raceless; AdjustCost handles the nil case.
func (a *connActor) Race() *progression.Race { return a.race }

// DeductMovement is the §4.5 step-1 movement spend: subtract from the
// movement pool, flooring at zero (validation already proved sufficiency).
// No-op when no movement pool is seeded.
func (a *connActor) DeductMovement(amount int) {
	if p, ok := a.resourcePool(poolKindMovement); ok {
		p.Deduct(amount)
	}
}

// DeductMana is the §4.5 step-1 mana spend: subtract from the mana pool,
// flooring at zero. No-op when no mana pool is seeded.
func (a *connActor) DeductMana(amount int) {
	if p, ok := a.resourcePool(poolKindMana); ok {
		p.Deduct(amount)
	}
}

// ApplyStartingStats adds each entry in m to the actor's base stats
// (AdjustBase) and persists the change — the level-1 endowment a class
// grants at creation (a channeler's resource_max One Power pool). It MUST
// sync the base into the save itself: it runs from the character.created
// subscriber, AFTER commitCreation already wrote the character and cleared
// the dirty bit, and the general Persist path does NOT re-sync base stats
// (only the equip/train/level-up mutators do, e.g. Equip). Without this the
// endowment would live only in memory and vanish on relogin (the pool would
// reseed to 0). Mirrors the Equip pattern: mutate, syncStats, markDirty.
func (a *connActor) ApplyStartingStats(m map[progression.StatType]int) {
	if len(m) == 0 {
		return
	}
	// AdjustBase takes the stat block's own lock (not a.mu); the OnMaxChange
	// listener wired in seedResourcePools fires here, moving a resource_max
	// bump straight onto the live pool ceiling.
	for stat, amount := range m {
		a.statBlock.AdjustBase(stat, amount)
	}
	a.mu.Lock()
	a.syncStatsToSaveLocked()
	a.markDirtyLocked()
	a.mu.Unlock()
}

// FillResourcePools sets every resource pool's current to its max. Called
// once at character creation, after a class's StartingStats endows the pool
// maxes (the channeler's One Power): SetMax raised the ceiling via
// OnMaxChange but left current at 0, so a fresh pool would otherwise begin
// empty. Relogin doesn't need this — RestoreBase sets the max before
// seedResourcePools, which then seeds current full (or from the save).
func (a *connActor) FillResourcePools() {
	if a.pools != nil {
		a.pools.Fill()
	}
}

// FillVitals tops current HP up to its max. Called once at character creation
// after StartingStats/StatBonuses may have raised hp_max (a metatype's Physical-
// monitor bonus — sr-m3c-deferred-fixes): the OnMaxChange→SetMax wiring moved the
// Vitals ceiling, but SetMax leaves current alone (a raise never auto-heals), so
// a fresh character with an hp_max bonus would otherwise spawn below full. Heal
// caps at max, so a character with no hp_max bonus is already full and this is a
// no-op. Relogin doesn't need it — restorePlayerVitals carries the saved current
// AND maxHP, and the RestoreBase→OnMaxChange pass reconciles the ceiling, so a
// returning character is never re-topped from this path.
func (a *connActor) FillVitals() {
	if a.vitals != nil {
		a.vitals.Heal(a.vitals.Max())
	}
}

// regenPool restores amount to the named pool, capped at its max. The
// owner-driven regen step (the pool itself has no clock); called by the
// RegenTick heartbeat. No-op for a non-positive amount or an unseeded /
// zero-max pool (pool.Restore caps at max, so a full pool absorbs nothing).
func (a *connActor) regenPool(kind pool.Kind, amount int) {
	if amount <= 0 {
		return
	}
	if p, ok := a.resourcePool(kind); ok {
		p.Restore(amount)
	}
}

// SetLastAbility records the §4.5 step-2 "last ability used"
// property. In-memory only (transient combat feedback, not durable
// save state).
func (a *connActor) SetLastAbility(abilityID string) {
	a.mu.Lock()
	a.lastAbility = abilityID
	a.mu.Unlock()
}

// LastAbility returns the most recently resolved ability id (or "").
// Exposed for the M9.6 UI surface + tests.
func (a *connActor) LastAbility() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastAbility
}

// StatValue returns the actor's effective value for stat, used by
// the §3.5 proficiency-gain stat factor. Pass-through to the stat
// block's effective (base + modifiers) read.
func (a *connActor) StatValue(stat progression.StatType) int {
	return a.statBlock.Effective(stat)
}

// AttributeSet returns the character's resolved base attribute set (SR-M1),
// so `score` renders the world's declared attributes in order. nil when no
// attribute content resolved; the score sheet falls back to the classic six.
func (a *connActor) AttributeSet() *progression.AttributeSet {
	return a.attrSet
}

// applyClass resolves the actor's class id list from save (wot-character-
// model D1). An empty save stays classless (no default class; character-
// creation owns initial selection). Each id that doesn't resolve in the
// registry is dropped (removed-content), so the surviving classIDs may be
// shorter than the saved list. Replaces (never accumulates) — mirrors
// applyRace's fail-soft policy.
func applyClass(a *connActor, cfg *Config, saved []string) {
	// Capture the registry unconditionally so SetClass-driven class
	// changes (and classless characters who gain a class later) can still
	// resolve weapon proficiency by id (weapon-identity §3).
	a.classes = cfg.Classes
	a.grades = cfg.Grades       // class-independent, but captured here alongside classes
	a.classIDs = a.classIDs[:0] // replace, never accumulate (mirrors applyRace)
	a.class = nil
	if cfg.Classes == nil {
		return
	}
	// Validate each saved id against the registry; an unresolved id
	// (removed content) is dropped — same fail-soft policy as applyRace. The
	// surviving list (lowercased, registry-canonical) becomes classIDs; the
	// first resolved class is captured on a.class for the look description.
	for _, raw := range saved {
		candidate := strings.ToLower(strings.TrimSpace(raw))
		if candidate == "" {
			continue
		}
		cls, ok := cfg.Classes.Get(candidate)
		if !ok {
			continue
		}
		a.classIDs = append(a.classIDs, cls.ID)
		if a.class == nil {
			a.class = cls // primary class — generated player description (look).
		}
	}
}

// applyBackground records the actor's background id from save (backgrounds
// §5) and derives its weapon restrictions from the registry (backgrounds.md
// §Restrictions). Lowercased for case-insensitive registry lookups; an empty id
// leaves the actor background-less. reg may be nil (no restriction enforced).
// Set-once before the actor is published — the equip gate reads the restriction
// fields lock-free, like backgroundID.
func applyBackground(a *connActor, saved string, reg *progression.BackgroundRegistry) {
	a.backgroundID = strings.ToLower(strings.TrimSpace(saved))
	a.weaponRestrictions = nil
	a.weaponRestrictionMessage = ""
	if reg == nil || a.backgroundID == "" {
		return
	}
	if bg, ok := reg.Get(a.backgroundID); ok {
		a.weaponRestrictions = bg.WeaponRestrictions // Register-lowercased
		a.weaponRestrictionMessage = bg.WeaponRestrictionMessage
	}
}

// BackgroundID returns the actor's creation origin id, or "" when
// background-less (backgrounds §5). Set once at construction (applyBackground,
// before the actor is published) and never mutates — read lock-free, like
// RaceID.
func (a *connActor) BackgroundID() string { return a.backgroundID }

// WeaponRestrictionRefusal returns an in-character refusal message if the
// actor's background forbids wielding the given weapon category (backgrounds.md
// §Restrictions — the Aiel sword taboo), else "". An empty category (a
// non-weapon) is never refused. Reads the set-once restriction fields lock-free
// (like BackgroundID); the equip gate calls it before any mutation.
func (a *connActor) WeaponRestrictionRefusal(category string) string {
	category = strings.ToLower(strings.TrimSpace(category))
	if category == "" || len(a.weaponRestrictions) == 0 {
		return ""
	}
	for _, restricted := range a.weaponRestrictions {
		if restricted != category {
			continue
		}
		if a.weaponRestrictionMessage != "" {
			return a.weaponRestrictionMessage
		}
		return "You will not wield such a weapon — it is against the ways you were raised."
	}
	return ""
}

// BackgroundChoices returns the pick-one background-chooser selections persisted
// on the save (v29): the chosen feat id (from the background's FeatOptions) and
// the chosen equipment-package index. Read once by the character.created grant.
// Lives on the save (like Gender), read under the lock.
func (a *connActor) BackgroundChoices() (feat string, equipmentIndex int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return "", 0
	}
	return a.save.BackgroundFeat, a.save.BackgroundEquipmentChoice
}

// MarkContentsDirty re-runs syncInventoryToSaveLocked so the save
// tree picks up Contents mutations the actor itself didn't perform
// (M5.9b put: the handler removes from inventory, then writes into
// a container via the Contents substrate; the inventory-remove
// re-sync ran before the Contents.Put so it captured an empty
// container in the save tree). Re-syncing here closes the gap and
// flips dirty so the corrected tree reaches disk on next Persist.
//
// Cheap: the tree walker is a recursive Contents.In() scan over
// items the actor already carries.
func (a *connActor) MarkContentsDirty() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.syncInventoryToSaveLocked()
	a.markDirtyLocked()
}

// syncEquipmentToSaveLocked mirrors the runtime equipment map into the
// save's Equipment field, capturing both the template id (so respawn
// can locate the template) and the current entity id (so respawn can
// rebind the stats block's source key from old to new). Caller MUST
// hold a.mu.
func (a *connActor) syncEquipmentToSaveLocked() {
	if a.save == nil {
		return
	}
	if len(a.footprints) == 0 {
		a.save.Equipment = nil
		return
	}
	// One entry per equipped item, keyed by its TARGET slot key (§3.8).
	// A spanning item's companion keys are NOT persisted — respawn
	// re-derives them from the template's companion slots on reload, so
	// the save never duplicates a spanning item across its footprint.
	out := make(map[string]player.EquippedItem, len(a.footprints))
	for id, keys := range a.footprints {
		if len(keys) == 0 {
			continue
		}
		tpl, ok := a.lookupTemplateID(id)
		if !ok {
			// Untracked entity — drop from save. Matches the silent
			// drop policy in syncInventoryToSaveLocked.
			continue
		}
		out[keys[0]] = player.EquippedItem{Template: tpl, Entity: string(id), Loaded: a.magazineLoadedForSave(id), Holder: a.insertedHolderForSave(id)}
	}
	a.save.Equipment = out
}

// syncStatsToSaveLocked rewrites a.save.Stats from the live block.
// Snapshot returns a fresh slice each call so the Persist path's
// shallow *a.save copy doesn't share backing storage.
//
// Effect-driven modifiers (source "effect:…") are filtered OUT of the
// persisted snapshot: active effects are ephemeral (spec
// abilities-and-effects §5.5 — the effect list is dropped at logout,
// not saved), so a buff active when Persist runs must not round-trip
// into a permanent bonus on reload. Equipment modifiers persist as
// before; respawnEquipment rebinds their source keys at login.
func (a *connActor) syncStatsToSaveLocked() {
	if a.save == nil {
		return
	}
	snap := a.statBlock.ModifiersSnapshot()
	persisted := make(stats.Snapshot, 0, len(snap))
	for _, e := range snap {
		// Effect AND feat modifiers are derived (effects from active effects,
		// feats from known_feats) and reinstalled at load, so neither is
		// persisted — round-tripping a derived value risks baking it in.
		if progression.IsEffectSource(e.Source) || srckey.IsFeat(e.Source) {
			continue
		}
		persisted = append(persisted, e)
	}
	a.save.Stats = persisted
	a.save.StatsBase = a.statBlock.BaseSnapshot()
}

// syncProgressionToSaveLocked rewrites a.save.Progression from the
// live state. Called from the Persist path before the dirty check
// so XP grants from the combat tick or admin commands between
// autosaves round-trip through disk. Caller MUST hold a.mu.
func (a *connActor) syncProgressionToSaveLocked() {
	if a.save == nil || a.progress == nil {
		return
	}
	a.save.Progression = a.progress.Snapshot()
}

// syncRecipesToSaveLocked rewrites a.save.KnownRecipes from the live
// KnownManager snapshot and returns true when it differs from the
// previously-persisted set. Mirrors syncAbilitiesToSaveLocked: the
// learn/craft paths mutate the manager directly (not through the actor),
// so this runs unconditionally at Persist before the dirty check and
// returns a delta signal. Caller MUST hold a.mu.
//
// Same Drop/autosave race guard as abilities: an autosave firing after
// logout's Drop would see an empty snapshot; with no forget-recipe verb a
// populated→empty transition cannot legitimately happen, so the
// empty-over-populated case is treated as a race and skipped to avoid
// clobbering the persisted set with nothing.
func (a *connActor) syncRecipesToSaveLocked() bool {
	if a.save == nil || a.known == nil {
		return false
	}
	snap := a.known.Snapshot(a.playerID)
	if len(snap) == 0 && len(a.save.KnownRecipes) > 0 {
		return false
	}
	if stringSetEqual(a.save.KnownRecipes, snap) {
		return false
	}
	a.save.KnownRecipes = snap
	return true
}

// stringSetEqual reports whether two known-recipe slices are equal.
//
// Precondition: both inputs come from KnownManager.Snapshot, which always
// returns a sorted, deduplicated slice (and a.save.KnownRecipes is only
// ever assigned from a prior Snapshot). Under that invariant a plain
// element-wise compare is exact set equality — and unlike a set-membership
// check it cannot give a false "equal" when an input carries a duplicate
// (which would silently skip a needed write). A hand-edited unsorted save
// would at worst trigger one harmless normalizing rewrite. nil and empty
// compare equal so a fresh load matches an unmodified snapshot.
func stringSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// syncAbilitiesToSaveLocked rewrites a.save.Abilities from the
// live ProficiencyManager snapshot and returns true when the
// snapshot differs from the previously-persisted one.
//
// M9.1 wires the proficiency manager outside the actor (the
// TrainingManager mutates it directly via the AbilityProficiency
// seam), so practice-driven cap raises don't flip the actor's
// dirty bit at the mutation site. This sync runs unconditionally
// at Persist before the dirty check, returning a delta signal so
// autosave still commits training results. Caller MUST hold a.mu.
//
// Returns false (no change) when the manager is unwired or the
// snapshot matches what's already on the save — autosave then
// short-circuits unless some other path flipped dirty.
//
// **Drop/autosave race guard.** `fullTeardown` calls Persist
// then Drop on the actor's goroutine, but an autosave tick may
// hold an actor reference from before Manager.Remove and call
// Persist after Drop has cleared the manager's state. In that
// case Snapshot returns an empty AbilitySnapshot. If we accepted
// the empty as truth, the diff would flip dirty and overwrite
// the player's persisted abilities with nothing. Until a
// Forget-style admin verb exists (M9.6+) a legitimate transition
// from "has entries" to "has none" cannot happen, so we treat
// the empty-snap-over-populated-save case as a Drop race and
// skip. The deferred memory tracks this; a real eviction story
// lands when the verb surface needs it.
func (a *connActor) syncAbilitiesToSaveLocked() bool {
	if a.save == nil || a.prof == nil {
		return false
	}
	snap := a.prof.Snapshot(a.playerID)
	if len(snap.Proficiency) == 0 && len(snap.Cap) == 0 &&
		(len(a.save.Abilities.Proficiency) > 0 || len(a.save.Abilities.Cap) > 0) {
		// Drop race or manager-not-populated-yet — preserve the
		// persisted save until a real mutation re-fills the
		// manager.
		return false
	}
	if abilitySnapshotEqual(a.save.Abilities, snap) {
		return false
	}
	a.save.Abilities = snap
	return true
}

// abilitySnapshotEqual reports map-equality for two AbilitySnapshot
// values. Empty maps and nil maps compare equal so a fresh load
// (snapshot nil) matches an unmodified-since-load Restore (snapshot
// nil) without re-marking the actor dirty.
func abilitySnapshotEqual(a, b progression.AbilitySnapshot) bool {
	return intMapEqual(a.Proficiency, b.Proficiency) && intMapEqual(a.Cap, b.Cap)
}

func intMapEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// syncInventoryToSaveLocked mirrors the actor's runtime inventory
// into the save's persisted Inventory field, recursing into
// container Contents so nested items round-trip across restart
// (M5.9b). Caller MUST hold a.mu.
//
// Entities the store no longer knows about are dropped silently
// (same policy as the leaf-only v3 implementation): the runtime
// inventory may still reference them, but persistence can only
// record what's resolvable.
func (a *connActor) syncInventoryToSaveLocked() {
	if a.save == nil {
		return
	}
	if len(a.inventory) == 0 {
		a.save.Inventory = nil
		return
	}
	a.save.Inventory = a.buildSaveEntriesLocked(a.inventory)
}

// buildSaveEntriesLocked walks a slice of entity ids and emits the
// matching InventoryEntry list, recursing into a.contents for any
// container-typed entity. Caller MUST hold a.mu. Returns nil for an
// all-dropped slice so the caller can decide whether to write `nil`
// (top-level Inventory) or omit the Contents field on a leaf entry
// (via omitempty).
//
// Lock-order note: this function acquires a.contents.mu (via
// Contents.In) while a.mu is held. That is the canonical order
// documented on entities.Contents — actor.mu → contents.mu. New
// callers of Contents that mutate actor state in response MUST NOT
// hold contents.mu while taking actor.mu; doing so would deadlock
// against a concurrent autosave Persist on that actor.
func (a *connActor) buildSaveEntriesLocked(ids []entities.EntityID) []player.InventoryEntry {
	if len(ids) == 0 {
		return nil
	}
	out := make([]player.InventoryEntry, 0, len(ids))
	for _, id := range ids {
		tpl, ok := a.lookupTemplateID(id)
		if !ok {
			continue
		}
		entry := player.InventoryEntry{Template: tpl, Loaded: a.magazineLoadedForSave(id)}
		if a.contents != nil && a.isContainerLocked(id) {
			if child := a.buildSaveEntriesLocked(a.contents.In(id)); len(child) > 0 {
				entry.Contents = child
			}
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// isContainerLocked reports whether id resolves to a container-typed
// item in the entity store. Used by the inventory tree builder to
// decide whether to recurse into Contents. Cheap: the store does a
// single map lookup. Returning false on store/type mismatch is
// correct — a non-container can never legally appear in the
// Contents index (the put handler enforces this).
func (a *connActor) isContainerLocked(id entities.EntityID) bool {
	if a.items == nil {
		return false
	}
	e, ok := a.items.GetByID(id)
	if !ok {
		return false
	}
	it, ok := e.(*entities.ItemInstance)
	if !ok {
		return false
	}
	return it.Type() == "container"
}

// lookupTemplateID resolves an entity id back to its template id by
// asking the entity store. Returns ok=false if the store is unwired or
// the entity is unknown / not an item. The store reference is set when
// connActor is constructed (see session.run); it is never reassigned,
// so reading it without holding a.mu is safe.
func (a *connActor) lookupTemplateID(id entities.EntityID) (string, bool) {
	if a.items == nil {
		return "", false
	}
	e, ok := a.items.GetByID(id)
	if !ok {
		return "", false
	}
	it, ok := e.(*entities.ItemInstance)
	if !ok {
		return "", false
	}
	return string(it.TemplateID()), true
}

// CombatantID returns the combat-side identity of this actor. The
// PlayerPrefix keeps the namespace disjoint from mob ids (see
// combat.CombatantID). PlayerID is account-scoped and stable across
// reconnects so a fight started against this player survives a
// link-dead reattach.
func (a *connActor) CombatantID() combat.CombatantID {
	return combat.NewPlayerCombatantID(a.playerID)
}

// WimpyThreshold returns the actor's configured wimpy HP-percent
// threshold ([0,100]; 0 disables). Satisfies combat.WimpyHolder.
// Lock-free: the underlying atomic.Int32 makes this safe to call
// from the tick goroutine without touching a.mu, which Persist
// holds across autosave file I/O.
func (a *connActor) WimpyThreshold() int {
	return int(a.wimpyThreshold.Load())
}

// SetWimpyThreshold updates the wimpy property and marks the save
// dirty so the new value persists on the next autosave. Clamps to
// [0, 100]; values outside are silently coerced (the verb handler
// already validates input, this is defense-in-depth).
func (a *connActor) SetWimpyThreshold(pct int) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if int(a.wimpyThreshold.Load()) == pct {
		return
	}
	a.wimpyThreshold.Store(int32(pct))
	if a.save != nil {
		a.save.WimpyThreshold = pct
		a.markDirtyLocked()
	}
}

// PowerAttackActive reports whether the Power Attack stance is on (feats
// Bucket C). Lock-free read off the atomic; read by Stats() every combat round.
func (a *connActor) PowerAttackActive() bool {
	return a.powerAttack.Load()
}

// HasPowerAttackFeat reports whether the actor holds the power-attack ability
// (granted by the Power Attack feat). Read from the feat-bonus cache so it
// recomputes on any feat change. The `powerattack on` verb gates on this so a
// character cannot enter a stance they have no feat for.
func (a *connActor) HasPowerAttackFeat() bool {
	fb := a.featWeaponBonus.Load()
	return fb != nil && fb.hasPowerAttack
}

// HasCleave reports whether the actor holds the Cleave and/or Great Cleave
// feats (feats Bucket C), read from the feat-bonus cache so it recomputes on any
// feat change. The combat CleaveFor hook calls it to drive the on-kill bonus
// swing; great implies cleave (Great Cleave is the uncapped form).
func (a *connActor) HasCleave() (cleave, great bool) {
	fb := a.featWeaponBonus.Load()
	if fb == nil {
		return false, false
	}
	return fb.hasCleave || fb.hasGreatCleave, fb.hasGreatCleave
}

// RenownBonus returns the additive effective-renown bonus from held feats (Fame
// — reputation.md §7), read lock-free from the feat-bonus cache. 0 when none.
func (a *connActor) RenownBonus() int {
	fb := a.featWeaponBonus.Load()
	if fb == nil {
		return 0
	}
	return fb.renownBonus
}

// Infamous reports whether a held feat flags the actor as infamous (Infamy —
// reputation.md §7): reactions resolve as feared regardless of the score sign.
// Read lock-free from the feat-bonus cache.
func (a *connActor) Infamous() bool {
	fb := a.featWeaponBonus.Load()
	return fb != nil && fb.infamous
}

// HasLowProfile reports whether a held feat scales down renown gains (Low
// Profile — reputation.md §7). The reputation.shift.check subscriber reads it.
func (a *connActor) HasLowProfile() bool {
	fb := a.featWeaponBonus.Load()
	return fb != nil && fb.lowProfile
}

// WornReputation returns the summed signed reputation delta of worn/visible gear
// (special-weapons §8 — masterwork +1, Trolloc scythesword −2), read lock-free
// from the equip-time cache. 0 when no gear confers reputation.
func (a *connActor) WornReputation() int {
	return int(a.wornReputation.Load())
}

// EffectiveRenown returns the actor's renown score folded with the Fame feat
// bonus (reputation.md §7) and the worn-gear reputation (special-weapons §8) —
// the "how known am I" value the score sheet shows and recognition checks (R4)
// will read. The stored base score (Renown) remains the source of truth the
// manager shifts; Fame and worn signifiers are additive overlays.
func (a *connActor) EffectiveRenown() int {
	return a.Renown() + a.RenownBonus() + a.WornReputation()
}

// SetPowerAttack toggles the Power Attack stance and marks the save dirty so
// the new value persists on the next autosave. Mirrors SetWimpyThreshold: the
// atomic write keeps the combat-round read lock-free, while a.mu guards the
// coupled a.save mutation. No-ops (and skips the dirty flip) when unchanged.
func (a *connActor) SetPowerAttack(on bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.powerAttack.Load() == on {
		return
	}
	a.powerAttack.Store(on)
	if a.save != nil {
		a.save.PowerAttackActive = on
		a.markDirtyLocked()
	}
}

// Autoloot reports the actor's autoloot preference (loot-and-corpses
// §6). Lock-free read off the atomic; safe from the tick goroutine.
func (a *connActor) Autoloot() bool {
	return a.autoloot.Load()
}

// SetAutoloot updates the autoloot preference and marks the save dirty
// so it persists on the next autosave. No-op when unchanged.
func (a *connActor) SetAutoloot(on bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.autoloot.Load() == on {
		return
	}
	a.autoloot.Store(on)
	if a.save != nil {
		a.save.Autoloot = on
		a.markDirtyLocked()
	}
}

// AutoAssistEnabled reports the actor's auto-assist preference (grouping.md
// §9). Lock-free read off the atomic; safe from the tick goroutine (the
// combat sink reads it inside OnEngagement).
func (a *connActor) AutoAssistEnabled() bool {
	return a.autoAssist.Load()
}

// SetAutoAssist updates the auto-assist preference and marks the save dirty
// so it persists on the next autosave. No-op when unchanged.
func (a *connActor) SetAutoAssist(on bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.autoAssist.Load() == on {
		return
	}
	a.autoAssist.Store(on)
	if a.save != nil {
		a.save.AutoAssist = on
		a.markDirtyLocked()
	}
}

// Recall returns the actor's persisted recall room id (recall.md §6).
// Empty string means no recall point is bound. Satisfies the
// command-package recallController contract.
func (a *connActor) Recall() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.recall
}

// SetRecall binds the actor's recall to roomID and marks the save
// dirty so the new address persists on the next autosave. Empty
// roomID is rejected silently so a misuse can't quietly erase a
// bound recall (the verb path never passes empty — defense in
// depth).
func (a *connActor) SetRecall(roomID string) {
	if roomID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.recall == roomID {
		return
	}
	a.recall = roomID
	if a.save != nil {
		a.save.Recall = roomID
		a.markDirtyLocked()
	}
}

// PromptTemplate implements command.promptController: the player's
// stored prompt template, or "" when unset (the renderer then uses the
// default — ui-rendering-help §7.1 / §7.4).
func (a *connActor) PromptTemplate() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return ""
	}
	return a.save.PromptTemplate
}

// SetPromptTemplate implements command.promptController: store template
// ("" clears it → default), mark the save dirty so it persists, and flag
// a prompt refresh so the new prompt renders on the next flush
// (ui-rendering-help §7.3 / §7.4). A no-op when the value is unchanged.
func (a *connActor) SetPromptTemplate(template string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil || a.save.PromptTemplate == template {
		return
	}
	a.save.PromptTemplate = template
	a.markDirtyLocked()
	a.needsPromptRefresh = true
}

// ShowRoomData reports the admin/builder room-metadata `look` preference
// (persisted). Satisfies command.RoomDataViewer. False when the save is
// absent (test actors) — the look block also gates on the admin role, so
// this is purely the user's display toggle.
func (a *connActor) ShowRoomData() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return false
	}
	return a.save.ShowRoomData
}

// SetShowRoomData stores the room-metadata toggle and marks the save
// dirty so it persists across logins. A no-op when unchanged or when the
// save is absent.
func (a *connActor) SetShowRoomData(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil || a.save.ShowRoomData == v {
		return
	}
	a.save.ShowRoomData = v
	a.markDirtyLocked()
}

// Vitals returns the actor's mutable HP state. The pointer is set at
// construction time in run() and is never reassigned for the life of
// the connActor, so reading it without taking a.mu is safe (the
// pointer itself; the Vitals struct carries its own internal lock).
func (a *connActor) Vitals() *combat.Vitals { return a.vitals }

// Pools is the actor's full resource-pool set — the combat.Combatant seam
// a typed attack's TargetPool routes through (shadowrun-mvp SR-M2). hp
// lives in Vitals, not here, so the canonical damage path never reads this;
// only a non-empty TargetPool (a Shadowrun stun weapon) does. Like Vitals,
// the pointer is set at construction and never reassigned, and pool.Set
// carries its own internal lock, so reading it without a.mu is safe. May be
// nil for a bare test-built actor that never seeded a set.
func (a *connActor) Pools() *pool.Set { return a.pools }

// gmcpSender is the subset of telnet.Conn the GMCP vitals flusher
// needs. Defined here at the use site (small-interface convention)
// so the session package doesn't import internal/conn/telnet for
// the GMCP-aware connections — non-telnet transports (test fakes,
// future WebSocket) opt in by satisfying the interface.
type gmcpSender interface {
	GmcpActive() bool
	SendGmcp(ctx context.Context, pkg string, payload []byte) error
}

// colorTierSource is the conn-side accessor the session layer
// reads to learn each conn's color-capability tier (M16.6a).
// Defined here at the use site so the session package doesn't
// import internal/conn/telnet directly. Non-implementing conns
// (test fakes) default to render.ColorTierBasic — see the
// readColorTier helper.
type colorTierSource interface {
	ColorTier() render.ColorTier
}

// readColorTier returns the conn's reported color tier, falling
// back to render.ColorTierBasic for conns that don't implement
// the accessor. The fallback preserves the M0-era ANSI-16 default
// so test fakes don't trip onto the no-color path until they
// explicitly opt in.
func readColorTier(c conn.Connection) render.ColorTier {
	if src, ok := c.(colorTierSource); ok {
		return src.ColorTier()
	}
	return render.ColorTierBasic
}

// terminalWidthSource is the conn-side accessor the session layer reads
// to learn each conn's reported window width in columns (RFC 1073
// NAWS). telnet.Conn satisfies it; conns that don't (websocket, test
// fakes) report 0, and the renderer falls back to its default width.
type terminalWidthSource interface {
	TerminalWidth() int
}

// flushGmcpVitals snapshots the actor's current vitals + sustenance
// and emits a Char.Vitals frame to the peer when the payload has
// changed since the last emission (the M16.4a poll-and-diff
// pattern from PD-3). Silent no-op when:
//
//   - the underlying conn doesn't speak GMCP (test fakes, future
//     non-GMCP transports);
//   - GMCP hasn't been negotiated (peer never sent DO GMCP);
//   - the payload matches the last-sent shadow exactly.
//
// Safe to call concurrently with the actor's own mutators: the
// payload snapshot reads cheap thread-safe accessors (Vitals
// carries its own lock, Sustenance reads under a.mu), and the
// shadow compare-and-swap is guarded by gmcpVitalsMu so two
// flusher invocations can't race a partial update.
func (a *connActor) flushGmcpVitals(ctx context.Context) {
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}
	hp, hpMax := a.vitals.Snapshot()
	payload := gmcp.CharVitals{
		HP:         hp,
		MaxHP:      hpMax,
		Sustenance: a.Sustenance(),
	}

	a.gmcpVitalsMu.Lock()
	// Skip the send when we've sent the same payload before. The
	// valid flag distinguishes "never sent" (always emit) from
	// "sent and matches" (skip) — without it, a fresh-reset
	// shadow at zero would silently swallow a legitimate
	// HP=0/MaxHP=0 snapshot (player dead at link-dead reconnect).
	if a.gmcpLastVitalsValid && a.gmcpLastVitals == payload {
		a.gmcpVitalsMu.Unlock()
		return
	}
	a.gmcpLastVitals = payload
	a.gmcpLastVitalsValid = true
	a.gmcpVitalsMu.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		// Marshal never fails on this struct shape; defensive only.
		return
	}
	// SendGmcp drops silently when the peer's Core.Supports set
	// excludes the package, so we don't pre-check SupportsPackage.
	// A real I/O error means the wire's broken; log at debug so
	// operators can spot a chronic write failure but don't promote
	// it (the next tick sees GmcpActive=false and skips).
	if err := sender.SendGmcp(ctx, gmcp.PackageCharVitals, data); err != nil {
		logging.From(ctx).Debug("gmcp vitals send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// resetGmcpVitalsShadow marks the last-sent shadow invalid so the
// next flushGmcpVitals call emits unconditionally. Called on
// link-dead reattach (and any future conn rebind path) so the
// new peer — which has its own fresh Core.Supports set and
// panel state — gets a baseline Char.Vitals frame even when
// nothing has changed on the engine side since the previous
// peer's last frame. Flipping only the valid flag (not the
// payload bytes) avoids the "shadow == zero vitals" collision
// that would otherwise drop a HP=0/MaxHP=0 reattach.
func (a *connActor) resetGmcpVitalsShadow() {
	a.gmcpVitalsMu.Lock()
	a.gmcpLastVitalsValid = false
	a.gmcpVitalsMu.Unlock()
}

// Stats returns the actor's combat stat block derived from the
// progression-layer StatBlock (M8.1). HitMod, AC, and STR are read
// through the StatBlock's effective values — base attribute +
// sum-of-modifiers — so equipment-driven modifiers now flow into
// auto-attack and consider without a separate sync step.
//
// Damage and WeaponName are filled from the wielded-weapon snapshot
// (combat §4.5): the actor.weapon atomic pointer, refreshed on
// equip/unequip/login. Unset (no weapon) falls through to the engine's
// unarmed defaults via EffectiveDamage / EffectiveWeaponName.
//
// LOCK NOTE: StatBlock carries its own RWMutex and a.weapon is an
// atomic.Pointer, so this method does not take a.mu — both reads are
// safe to call concurrently with session-side equip / unequip
// mutations on the combat tick goroutine.
func (a *connActor) Stats() combat.Stats {
	str := a.statBlock.Effective(progression.StatSTR)
	hitMod := a.statBlock.Effective(progression.StatHitMod)
	ac := a.statBlock.Effective(progression.StatAC)
	// Damage scaling falls back to STRBonus when no mapping is wired (bare
	// test actors); production always has the baseline mapping, which maps
	// damage_bonus to the same trunc((str-10)/2). Mitigation is 0 unmapped.
	damageBonus := combat.STRBonus(str)
	mitigation := 0
	if a.channelMap != nil {
		// The lookup resolves a channel-formula variable to its value. Most are
		// real stats; `dex_ac` is a SYNTHETIC input (armor-depth §3) — the
		// wearer's Dex contribution to AC, capped by worn armor — computed live
		// here rather than stored, so a Dex change is reflected immediately and
		// only the cap snapshot moves on equip. A ruleset that wants Dex in AC
		// (the WoT d20 mapping: `defense: ac + dex_ac`) references it; the
		// fantasy baseline (`defense: ac`) never asks, so this stays inert there.
		lookup := func(name string) int {
			if name == channel.InputDexAC {
				return a.cappedDexAC()
			}
			if name == channel.InputArmor {
				// Shadowrun soak input (`mitigation: body + armor`): the summed worn-
				// armour rating, the same lock-free snapshot combat.Stats.ArmorRating
				// reads. 0 unarmoured, so a mapping that never asks (the fantasy
				// baseline) is unaffected.
				return int(a.wornArmorBonus.Load())
			}
			return a.statBlock.Effective(progression.StatType(name))
		}
		hitMod = a.channelMap.Value(channel.Attack, lookup)
		ac = a.channelMap.Value(channel.Defense, lookup)
		damageBonus = a.channelMap.Value(channel.DamageBonus, lookup)
		mitigation = a.channelMap.Value(channel.Mitigation, lookup)
	}
	// baseHitMod is the attribute/channel-derived attack modifier BEFORE any
	// per-weapon feat bonus (Weapon Focus, added below for the main weapon). The
	// off-hand swing (two-weapon-fighting §4.1) derives its hit from this base so
	// it does not inherit the main weapon's feat bonus.
	baseHitMod := hitMod
	s := combat.Stats{
		HitMod:      hitMod,
		AC:          ac,
		STR:         str,
		DamageBonus: damageBonus,
		Mitigation:  mitigation,
	}
	if ar := a.armorResist.Load(); ar != nil {
		// Copy out of the shared snapshot: combat.Stats is a self-contained
		// per-round value (see its doc), so it must not alias a cached map.
		s.Resistances = make(map[string]int, len(ar.byType))
		maps.Copy(s.Resistances, ar.byType)
	}
	// subdual-damage §6: the defender armor rating (worn armor AC sum) the whip
	// anti-armor gate reads when THIS actor is the target. 0 when unarmored.
	s.ArmorRating = int(a.wornArmorBonus.Load())
	// Single consistent snapshot of the feat-bonus cache for this whole Stats()
	// call: it feeds BOTH the per-weapon-category Weapon Focus / Improved
	// Critical path and the two-weapon penalty reductions below. Loading it once
	// avoids observing two cache generations if a concurrent feat grant Stores a
	// new pointer mid-call (the same snapshot discipline as armorResist).
	fb := a.featWeaponBonus.Load()
	w := a.weapon.Load()
	if w == nil {
		// subdual-damage §6: an UNARMED player strikes nonlethally when the host
		// enables it (the faithful d20/WoT default) — fists knock out, they do not
		// kill. A wielded weapon (the w != nil branch) carries its own lethality.
		s.Subdual = a.unarmedSubdual
	}
	if w != nil {
		s.Damage = w.dice
		s.WeaponName = w.name
		s.WeaponDamageTypes = append([]string(nil), w.damageTypes...) // copy: don't alias the cached weaponInfo slice
		s.TargetPool = pool.Kind(w.targetPool)                        // shadowrun-mvp SR-M3b: route damage to the weapon's monitor
		s.CritThreatLow = w.critThreatLow
		s.CritMultiplier = w.critMultiplier
		// Ranged metadata + the §4 Strength rule. The class/ammo/increment
		// are carried for the round loop's ammo + (Slice B) band logic; the
		// Strength rule re-derives DamageBonus for a projectile (no positive
		// bonus, or capped at a rating) while thrown keeps the full melee bonus.
		s.RangedClass = w.rangedClass
		s.AmmoKind = w.ammoKind
		s.RangedStyle = w.rangedStyle
		s.RangeIncrement = w.rangeIncrement
		s.ReloadTicks = w.reloadTicks
		s.Magazine = w.magazine
		s.AcceptsHolder = w.acceptsHolder
		s.Reach = w.reach                           // special-weapons §3: strikes at the `near` band too
		s.Set = w.set                               // special-weapons §4: braced bonus blow vs a charge
		s.Subdual = w.subdual                       // subdual-damage §2: a nonlethal finish knocks out
		s.IneffectiveVsArmor = w.ineffectiveVsArmor // subdual-damage §6: a whip can't bite armor
		s.DamageBonus = item.RangedDamageBonus(w.rangedClass, w.strRating, s.DamageBonus)
		// size-and-wielding §4.2: a two-handed MELEE wield multiplies the
		// Strength contribution to damage by the two-handed factor. Add only the
		// EXTRA Strength (TwoHandedStrBonus) on top of the 1× already in
		// DamageBonus, so grade / flat bonuses stay at 1×. Ranged weapons are
		// excluded — their Strength rule is the ranged concern handled above. A
		// DOUBLE weapon (special-weapons §7) is also excluded: it is used as two
		// weapons (1× main + ½× off below), not as a single two-handed weapon, so
		// the main end takes the ordinary 1× Strength, not the 1.5× two-hander bonus.
		if w.wieldMode == size.TwoHanded && w.rangedClass == "" && w.doubleDamage.IsZero() {
			s.DamageBonus += size.TwoHandedStrBonus(combat.STRBonus(str), size.DefaultTwoHandedStrFactor)
		}
		// EPIC S4 Phase 3c: per-weapon-category feat bonuses (Weapon Focus
		// to-hit, Improved Critical threat widen), read lock-free from the
		// cache. Category match is case-insensitive (feat params are lowercased).
		if fb != nil && w.category != "" {
			cat := strings.ToLower(w.category)
			s.HitMod += fb.hit[cat]
			// Weapon Specialization (feats Bucket B): per-category melee damage,
			// the damage sibling of Weapon Focus's to-hit. Ranged weapons are
			// excluded — it specializes melee technique (d20 Special feat).
			if w.rangedClass == "" {
				s.DamageBonus += fb.dmg[cat]
			}
			if widen := fb.crit[cat]; widen > 0 {
				low := s.CritThreatLow
				if low <= 0 {
					low = 20 // an unthreatening weapon threatens only on a natural 20
				}
				low -= widen
				if low < 2 {
					low = 2
				}
				s.CritThreatLow = low
			}
		}
	}
	// two-weapon-fighting §3/§4: a valid off-hand weapon grants one extra
	// off-hand attack. It requires a MELEE main weapon (the off-hand strike is a
	// melee concern, §3) and an off-hand weapon that resolves the LIGHT wield
	// mode for this wielder (size-and-wielding §2.2/§4.3 — the off-hand
	// eligibility F reserved). Open to all: the full two-weapon penalty applies
	// to both hands now; the feats (slice 2) will reduce it. The penalty on the
	// main hand is folded into s.HitMod here; the off-hand profile carries the
	// larger off-hand penalty and the reduced (½×) Strength damage (§4.2).
	if w != nil && w.rangedClass == "" {
		// The off-hand strike has two sources (two-weapon-fighting §3,
		// special-weapons §7): a distinct LIGHT off-hand weapon (dual-wielding),
		// OR — when no second weapon is wielded — the SAME item's second end if it
		// is a DOUBLE weapon (a quarterstaff/ashandarei used as two weapons). Both
		// resolve to a light off-hand strike: identical penalties, ½× Strength, and
		// feat reductions. The double-weapon end carries the weapon's own crit/type.
		var (
			offDice    combat.DiceExpr
			offName    string
			offTypes   []string
			offCritLow int
			offCritMul int
			haveOff    bool
		)
		offSubdual := false     // subdual-damage §2: the off-hand end's lethality
		offIneffective := false // subdual-damage §6: the off-hand end's whip-ness
		if off := a.offWeapon.Load(); off != nil && off.wieldMode == size.Light {
			offDice, offName, offTypes = off.dice, off.name, off.damageTypes
			offCritLow, offCritMul = off.critThreatLow, off.critMultiplier
			offSubdual = off.subdual
			offIneffective = off.ineffectiveVsArmor
			haveOff = true
		} else if !w.doubleDamage.IsZero() {
			offDice, offName, offTypes = w.doubleDamage, w.name, w.damageTypes
			offCritLow, offCritMul = w.critThreatLow, w.critMultiplier
			offSubdual = w.subdual                // a double weapon's second end shares the weapon's lethality
			offIneffective = w.ineffectiveVsArmor // ...and its whip-ness (no double weapon is a whip today)
			haveOff = true
		}
		if haveOff {
			// Slice 2: the two-weapon feats SUBTRACT from the baseline penalties
			// (two-weapon-fighting §4.1). Two-Weapon Fighting reduces both hands;
			// Ambidexterity removes the off-hand-specific extra. Reductions ride
			// the same lock-free feat cache as Weapon Focus; penalties clamp at
			// zero so a feat never turns the penalty into a bonus.
			mainPenalty := combat.DefaultTwoWeaponMainPenalty
			offPenalty := combat.DefaultTwoWeaponOffHandPenalty
			offHandAttacks := 1 // baseline: one off-hand strike (§3)
			if fb != nil {
				mainPenalty -= fb.twoWeaponHitReduce
				offPenalty -= fb.twoWeaponHitReduce + fb.offHandHitReduce
				// Improved Two-Weapon Fighting grants extra off-hand strikes (§3.1).
				offHandAttacks += fb.offHandExtraAttacks
			}
			if mainPenalty < 0 {
				mainPenalty = 0
			}
			if offPenalty < 0 {
				offPenalty = 0
			}
			s.HitMod -= mainPenalty
			strBonus := combat.STRBonus(str)
			// TODO(SR-M3b): OffHandProfile carries no TargetPool — an off-hand stun
			// weapon routes to hp, not the Stun monitor. Fine for the main-hand-
			// forward MVP; revisit before authoring dual-wield SR content.
			s.OffHand = &combat.OffHandProfile{
				Damage:            offDice,
				WeaponName:        offName,
				WeaponDamageTypes: append([]string(nil), offTypes...),
				CritThreatLow:     offCritLow,
				CritMultiplier:    offCritMul,
				HitMod:            baseHitMod - offPenalty,
				// ½× Strength on the off hand: the full damage bonus with only the
				// Strength term reduced (flat bonuses stay 1×), mirroring the
				// two-handed 1.5× rule (size-and-wielding §4.2).
				DamageBonus: damageBonus + size.StrBonusDelta(strBonus, size.DefaultOffHandStrFactor),
				// Improved Two-Weapon Fighting raises the off-hand strike count (§3.1).
				Attacks: offHandAttacks,
				// subdual-damage §2: a nonlethal off-hand end knocks out on a finish.
				Subdual: offSubdual,
				// subdual-damage §6: the off-hand end's own whip-ness (anti-armor).
				IneffectiveVsArmor: offIneffective,
			}
		}
	}
	// Power Attack stance (feats Bucket C): a melee accuracy-for-power trade.
	// Applies only with the stance on, a held power-attack feat, and a MELEE
	// weapon (d20 Power Attack is melee-only — a ranged or unarmed wield is
	// excluded). -trade to-hit / +trade damage; a two-handed melee wield doubles
	// the damage half (size-and-wielding §4.2). Main-hand only for now — the
	// off-hand interaction is a fidelity follow-up. Folded in last so the penalty
	// rides on top of the per-weapon feat hit bonus already applied above.
	if a.powerAttack.Load() && fb != nil && fb.hasPowerAttack && w != nil && w.rangedClass == "" {
		trade := combat.DefaultPowerAttackTrade
		s.HitMod -= trade
		dmg := trade
		if w.wieldMode == size.TwoHanded {
			dmg *= 2
		}
		s.DamageBonus += dmg
	}
	return s
}

// PlayerName returns the loaded character's name, used by the autosave
// loop's structured logs. Returns "" for an actor that never finished
// login (shouldn't happen after Manager.Add, but defensive).
func (a *connActor) PlayerName() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return ""
	}
	return a.save.Name
}

// sanitizeForLog delegates to logging.Sanitize (kept as a local alias so the
// many call sites and the existing test read unchanged).
func sanitizeForLog(s string) string {
	return logging.Sanitize(s)
}
