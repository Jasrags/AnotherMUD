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
}

// Context carries the per-invocation arguments passed to a Handler.
type Context struct {
	Actor Actor
	World *world.World
	Raw   string   // raw input line, trimmed
	Verb  string   // resolved verb (lowercase)
	Args  []string // tokens after the verb (space-split)
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
func (r *Registry) Dispatch(ctx context.Context, w *world.World, actor Actor, raw string) error {
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
		Actor: actor,
		World: w,
		Raw:   trimmed,
		Verb:  strings.ToLower(verb),
		Args:  args,
	})
}
