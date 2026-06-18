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
	"github.com/Jasrags/AnotherMUD/internal/grade"
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/property"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stacking"
	"github.com/Jasrags/AnotherMUD/internal/trade"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

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
	// ResolveAttack is the ranged-combat §3 one-shot attack primitive (the
	// thrown-weapon throw): it resolves a single swing from attacker against
	// target in room, returning whether the target survived. Wired in main.go
	// from combat.Manager.ResolveSingleAttack + the combat roller; nil in tests
	// that don't exercise throw.
	ResolveAttack func(ctx context.Context, attacker, target combat.CombatantID, room world.RoomID) bool
	// ReloadScripts is the M17.3 script hot-reload trigger (re-discover
	// pack Lua → swap the scripting runtime). nil disables the reload
	// verb; tests that don't exercise reload leave it unset.
	ReloadScripts func(ctx context.Context) (int, error)
	// Progression is the M8.2 XP/level service. nil in tests.
	Progression *progression.Manager
	// Effects is the effect manager (conditions §5): the afflict/cure/stand
	// verbs apply + remove condition effects through it. nil disables them.
	Effects *progression.EffectManager
	// EffectTemplates resolves an effect template by id — the afflict verb
	// maps a condition name to its effect (conditions §5). nil disables
	// afflict. effect.Registry satisfies it.
	EffectTemplates EffectTemplateSource
	// SkillRoller is the d20 source for skill checks (skills §3 — the `pick`
	// verb). Must be safe to call from the command goroutine (the production
	// wiring uses a concurrency-safe source). nil disables skill-check verbs.
	SkillRoller progression.Roller
	// Training is the M8.6 training service. nil in tests.
	Training *progression.TrainingManager
	// Abilities / Proficiency / ActionQueue are the M9.6 ability-verb
	// seam. nil in tests that don't exercise ability verbs.
	Abilities   *progression.AbilityRegistry
	Proficiency *progression.ProficiencyManager
	ActionQueue *progression.ActionQueueManager
	// Recipes / Known are the crafting seam (crafting-and-cooking §3,
	// §7). Recipes is the recipe registry; Known is the per-character
	// known-recipe manager. The `learn` verb reads both. nil in tests
	// that don't exercise crafting.
	Recipes *recipe.Registry
	Known   *recipe.KnownManager
	// Craft is the crafting service (quality roll + atomic consume/produce,
	// crafting-and-cooking §3, §5). The `craft` verb routes through it.
	// nil in tests that don't exercise crafting.
	Craft *crafting.Service
	// Gathering / Biomes / ForageTables are the gathering seam (gathering.md
	// §2). The `forage` verb resolves the room's biome (Biomes) → its forage
	// table id (ForageTables) and rolls it through Gathering. All nil in
	// tests that don't exercise gathering.
	Gathering    *gathering.Service
	Biomes       *biome.Registry
	ForageTables *gathering.ForageRegistry
	// Grades is the masterwork quality-grade registry (masterwork §3),
	// copied from Env at dispatch. The equip path reads a graded item's
	// bonus through it; nil → ungraded (no grade bonus).
	Grades *grade.Registry
	// Help is the M10.5 help-topic service. nil in tests.
	Help *help.Service
	// Quests is the M10.7 quest service. nil in tests.
	Quests *quest.Service
	// Currency is the M11.1 economy currency service. nil in tests.
	Currency *economy.CurrencyService
	// Mounts is the mount lifecycle service (mounts.md §2/§3) — materialize /
	// dematerialize / name an owned mount. The stable verbs route through it.
	// nil disables mount verbs (tests / headless).
	Mounts MountService
	// Trades is the direct-trade session manager (direct-trade.md). The
	// trade/offer/confirm/decline verbs route through it. nil disables
	// trading (tests / headless); handlers MUST nil-guard.
	Trades *trade.Manager
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
	// AdminRole is the role an admin-marked command requires (copied from
	// Env.AdminRole, defaulted to `admin`). The dispatcher gates admin
	// verbs before the handler runs; this field lets a non-admin handler
	// (e.g. `look`) make a defensive in-handler admin check for an
	// admin-only display block — see AppendRoomData, which falls back to
	// the default when this is empty so a direct-constructed test Context
	// behaves like dispatch.
	AdminRole string
	// DefaultXPTrack is the engine's primary XP track (copied from Env).
	// The `xp` verb grants on it when no track argument is given; empty
	// falls back to "adventurer" in the handler.
	DefaultXPTrack string
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
	// WeatherState returns the current weather state for an area (e.g.
	// "clear"/"rain"), copied from Env. The build verb reads it to refuse
	// lighting a campfire in wet weather (crafting-and-cooking §4). nil →
	// no weather gate (tests/headless).
	WeatherState func(world.AreaID) string
	// Light is the light-and-darkness resolver (light §2). The
	// light/extinguish verbs read its config (auto-light policy); the
	// render/combat/movement paths use it to gate on effective light.
	// nil disables light gating (tests, and any path that has not
	// wired it).
	Light *light.Resolver
	// NowTick / CorpseOwnershipWindow are the M22.3 loot-window seam
	// (loot-and-corpses §4). Copied from Env at dispatch. NowTick nil →
	// the loot verb treats every corpse as open.
	NowTick               func() uint64
	CorpseOwnershipWindow uint64
	// DefaultMoveCost is the flat movement-point cost of a step when the
	// destination biome sets none (world-rooms-movement §3.3). Copied from
	// Env at dispatch; sourced from ANOTHERMUD_MOVE_COST (default 1). Zero
	// in bare fixtures, where the gate falls back to fallbackMoveCost.
	DefaultMoveCost int
	Raw             string   // raw input line, trimmed
	Verb            string   // resolved verb (lowercase)
	Args            []string // tokens after the verb (space-split)

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

// PublishCancellable is the nil-safe shortcut for emitting a cancellable
// pre-event and learning whether a listener vetoed. With no bus wired (a
// test fixture's zero-value Env) nothing can veto, so it returns false —
// the operation proceeds, matching Publish's nil-safe no-op.
func (c *Context) PublishCancellable(ctx context.Context, e eventbus.CancellableEvent) bool {
	if c.Bus == nil {
		return false
	}
	return c.Bus.PublishCancellable(ctx, e)
}
