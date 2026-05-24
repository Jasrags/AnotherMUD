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
	"sort"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
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
	// Slots is the equipment-slot registry, consumed by the equip
	// command handler. Templates are deliberately NOT carried here —
	// they're only needed at login time (respawnInventory /
	// respawnEquipment) and live on session.Config.
	Slots *slot.Registry
	// Bus is the engine event bus. Handlers publish observable
	// events after successful mutations. May be nil in tests that
	// don't subscribe to anything — handlers MUST nil-guard.
	Bus *eventbus.Bus
}

// Context carries the per-invocation arguments passed to a Handler.
type Context struct {
	Actor       Actor
	World       *world.World
	Broadcaster Broadcaster         // may be nil in tests
	Items       *entities.Store     // may be nil in tests
	Placement   *entities.Placement // may be nil in tests
	Slots       *slot.Registry      // may be nil in tests
	Bus         *eventbus.Bus       // may be nil in tests
	Raw         string              // raw input line, trimmed
	Verb        string              // resolved verb (lowercase)
	Args        []string            // tokens after the verb (space-split)
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

type registration struct {
	keyword string
	order   int
	handler Handler
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

// Register binds keyword to h. Keywords are stored lowercased.
// Duplicate keywords return an error.
func (r *Registry) Register(keyword string, h Handler) error {
	if keyword == "" {
		return errors.New("command.Register: empty keyword")
	}
	if h == nil {
		return errors.New("command.Register: nil handler")
	}
	k := strings.ToLower(keyword)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byKey[k]; exists {
		return errors.New("command.Register: duplicate keyword " + k)
	}
	r.order++
	r.byKey[k] = registration{keyword: k, order: r.order, handler: h}
	r.ordered = append(r.ordered, k)
	return nil
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
		Slots:       env.Slots,
		Bus:         env.Bus,
		Raw:         trimmed,
		Verb:        strings.ToLower(verb),
		Args:        args,
	})
}
