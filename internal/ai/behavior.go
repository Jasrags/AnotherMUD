// Package ai owns the mob behavior dispatch surface (spec
// mobs-ai-spawning §4): the behavior registry, the per-tick
// dispatcher, and the built-in behaviors.
//
// The package is deliberately small and substrate-only in M6.4:
// `stationary` (no-op) and `wander` (random adjacent-room move on a
// fixed interval). Disposition reactions (§5), mob commands (§6.1),
// abilities (§6.2), and the active-vs-inactive area filter (§4.1)
// land with later M6 slices.
package ai

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Broadcaster is the message-delivery seam the wander behavior uses
// to announce departure/arrival to other players in the affected
// rooms. session.Manager satisfies it. The interface lives in ai (not
// in command) so the package isn't forced to pull command + session
// in just to call SendToRoom.
type Broadcaster interface {
	SendToRoom(ctx context.Context, roomID world.RoomID, text string, excludePlayerIDs ...string)
}

// CombatGate is the read-only surface the dispatcher consults to skip
// behavior dispatch for mobs currently in combat (spec combat §3 +
// §4.1 implies the round loop owns combatants; the AI tick MUST NOT
// race a wander against an active fight). combat.Manager satisfies
// it via InCombat. Defined here rather than depended on as a concrete
// *combat.Manager so ai keeps its narrow dep surface — the same
// pattern Broadcaster uses for session.Manager.
type CombatGate interface {
	InCombat(id combat.CombatantID) bool
}

// Deps bundles every external dependency a behavior may need. A
// single struct is cleaner than threading 5+ args through every
// Behavior signature and keeps the registry boundary stable as new
// behaviors arrive.
//
// Broadcaster may be nil (tests / headless setups); behaviors MUST
// tolerate that and skip the announcement. Rand may also be nil; in
// that case Dispatcher.Tick supplies a process-wide default before
// invoking the behavior.
type Deps struct {
	World       *world.World
	Placement   *entities.Placement
	Store       *entities.Store
	Bus         *eventbus.Bus
	Broadcaster Broadcaster
	Clock       clock.Clock
	Rand        *rand.Rand

	// Evaluator is optional; when present, behaviors that move a
	// mob into a new room (today: wander) call OnMobEntered so the
	// arriving mob is evaluated against every player already in
	// the destination (spec mobs-ai-spawning §4 mob-entered-room
	// hook).
	Evaluator *Evaluator

	// Combat is the optional combat-state gate (M7.6 follow-up).
	// When non-nil, the dispatcher skips behavior dispatch for any
	// mob whose CombatantID is currently in combat — preventing the
	// wander-during-fight cascade where an AI tick moves the mob
	// between rounds and the auto-attack pre-flight then
	// disengages on different-room. nil disables gating (tests that
	// don't wire combat don't need to know about it).
	Combat CombatGate
}

// Behavior is one named AI handler. It runs once per tick for every
// mob whose Properties()[PropBehavior] equals the registry key under
// which the function was registered (spec §4.2 + §4.3).
//
// Returning an error does not abort the dispatch loop — the
// Dispatcher logs it and moves to the next mob. This matches the
// spec's "behavior failure is a warning, not a fatal" implicit
// contract: one buggy behavior must not freeze every other mob.
type Behavior func(ctx context.Context, mob *entities.MobInstance, deps Deps) error

// Errors callers may distinguish at the boundary.
var (
	ErrDuplicateBehavior = errors.New("ai: behavior name already registered")
	ErrUnknownBehavior   = errors.New("ai: behavior name not registered")
)

// Registry is the boot-time map of behavior name → handler. Mirrors
// item.Templates / mob.Templates in shape: writes happen at boot
// (RegisterEngineBaseline + pack registrations), reads happen on the
// tick goroutine. Safe for concurrent reads under sync.RWMutex.
type Registry struct {
	mu      sync.RWMutex
	byName  map[string]Behavior
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Behavior)}
}

// Register binds name to fn. Returns ErrDuplicateBehavior if name is
// already registered; later content can use Replace if intentional
// override is needed (no consumer yet, deferred).
func (r *Registry) Register(name string, fn Behavior) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byName[name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateBehavior, name)
	}
	r.byName[name] = fn
	return nil
}

// Get returns the behavior bound to name, or ErrUnknownBehavior.
func (r *Registry) Get(name string) (Behavior, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.byName[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownBehavior, name)
	}
	return fn, nil
}

// Has reports whether name is registered.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.byName[name]
	return ok
}

// Count returns the number of registered behaviors.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byName)
}
