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
	"sort"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/chat"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stats"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// ErrQuit is returned by Dispatch when the actor's quit verb fires.
// The session loop unwinds cleanly on this — it is a signal, not a
// failure.
var ErrQuit = errors.New("command: quit")

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
type Locator interface {
	FindInRoom(roomID world.RoomID, name string) Actor
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
	// Locator resolves another actor by name + room. Consumed by the
	// give command handler (and future targeted verbs). May be nil
	// in tests; handlers MUST nil-guard.
	Locator Locator
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
	Broadcaster Broadcaster         // may be nil in tests
	Items       *entities.Store     // may be nil in tests
	Placement   *entities.Placement // may be nil in tests
	Contents    *entities.Contents  // may be nil in tests
	Slots       *slot.Registry      // may be nil in tests
	Bus         *eventbus.Bus       // may be nil in tests
	Locator     Locator             // may be nil in tests
	Disposition DispositionHook     // may be nil in tests
	Combat      *combat.Manager     // may be nil in tests
	// Flee is the M7.6 verb-driven §5.2 flee primitive closure. nil
	// in tests that don't exercise the flee verb.
	Flee func(ctx context.Context, c combat.CombatantID) combat.FleeOutcome
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
	Ambience        func(*world.Room) string
	Raw             string   // raw input line, trimmed
	Verb            string   // resolved verb (lowercase)
	Args            []string // tokens after the verb (space-split)
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
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{byKey: make(map[string]registration)}
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
	r.byKey[k] = registration{keyword: k, order: r.order, handler: c.Handler, meta: meta}
	r.ordered = append(r.ordered, k)
	for _, la := range lowered {
		r.order++
		r.byKey[la] = registration{keyword: la, order: r.order, handler: c.Handler, alias: true}
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
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Keyword < out[j].Keyword })
	return out
}

// Resolve returns the handler that the verb routes to, or nil if no
// match. Exact match wins; on no exact match, the keyword with the
// smallest registration-order index whose name has verb as a prefix
// wins (spec §2.3).
func (r *Registry) Resolve(verb string) Handler {
	v := strings.ToLower(verb)
	if v == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if reg, ok := r.byKey[v]; ok {
		return reg.handler
	}
	// Prefix scan: collect candidates, pick the lowest order.
	var matches []registration
	for _, k := range r.ordered {
		if strings.HasPrefix(k, v) {
			matches = append(matches, r.byKey[k])
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].order < matches[j].order })
	return matches[0].handler
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

	h := r.Resolve(verb)
	if h == nil {
		return actor.Write(ctx, "Huh?")
	}
	return h(ctx, &Context{
		Actor:       actor,
		World:       env.World,
		Broadcaster: env.Broadcaster,
		Items:       env.Items,
		Placement:   env.Placement,
		Contents:    env.Contents,
		Slots:       env.Slots,
		Bus:         env.Bus,
		Locator:     env.Locator,
		Disposition: env.Disposition,
		Combat:      env.Combat,
		Flee:        env.Flee,
		Progression: env.Progression,
		Training:    env.Training,
		Abilities:   env.Abilities,
		Proficiency: env.Proficiency,
		ActionQueue: env.ActionQueue,
		Help:        env.Help,
		Quests:      env.Quests,
		Currency:    env.Currency,
		Shop:        env.Shop,
		Rest:          env.Rest,
		Consumable:    env.Consumable,
		Notifications:   env.Notifications,
		TellResolver:    env.TellResolver,
		ChatRegistry:    env.ChatRegistry,
		ChatSubscribers: env.ChatSubscribers,
		ChatScrollbacks: env.ChatScrollbacks,
		Clock:           env.Clock,
		Ambience:        env.Ambience,
		Raw:             trimmed,
		Verb:            strings.ToLower(verb),
		Args:            args,
	})
}
