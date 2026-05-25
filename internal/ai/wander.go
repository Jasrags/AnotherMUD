package ai

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Reserved property keys consulted by BehaviorWander.
//
// PropWanderNextAt records the next absolute time (Unix nanoseconds)
// at which the mob is permitted to wander. Initialized to 0 on the
// instance; the first dispatch reads 0, observes "time elapsed", and
// then sets the next gate. This means a freshly-spawned mob can move
// on its very first AI tick — acceptable for now; later content
// authoring may want a per-template warm-up gate.
const (
	PropWanderNextAt = "wander_next_at"

	// DefaultWanderInterval is the floor between two consecutive
	// moves for a wandering mob. Hardcoded today; promote to a
	// per-template property when a content author asks. 5 seconds
	// is slow enough that a single mob in a small map doesn't feel
	// frantic and fast enough that the player will see motion
	// within one in-game minute of arriving.
	DefaultWanderInterval = 5 * time.Second
)

// BehaviorWander moves the mob through a randomly-chosen exit no
// more often than DefaultWanderInterval. The pacing gate lives on
// the mob's per-instance Properties map, so two wandering mobs
// don't share a heartbeat — each one is on its own clock.
//
// Failure modes (silently no-op, log-level at most):
//   - World is nil → can't validate exits.
//   - Mob has no room (not tracked in Placement) → nothing to do.
//   - Mob's room has no exits → mob is stuck this tick.
//   - Picked exit fails World.Move validation → try nothing else
//     this tick; we'll roll the dice again next tick.
//
// All failures return nil because they are routine ("it's just not
// time / there's nowhere to go"), not exceptional. Reserve error
// returns for actual programmer mistakes (missing Deps fields).
func BehaviorWander(ctx context.Context, m *entities.MobInstance, deps Deps) error {
	if deps.World == nil || deps.Placement == nil || deps.Clock == nil {
		return errors.New("wander: incomplete deps (World/Placement/Clock required)")
	}

	now := deps.Clock.Now()
	if !readyToWander(m, now) {
		return nil
	}

	srcID, ok := deps.Placement.RoomOf(m.ID())
	if !ok {
		return nil
	}
	src, err := deps.World.Room(srcID)
	if err != nil {
		return nil
	}
	dir, ok := pickExit(src, deps.Rand)
	if !ok {
		// Re-arm the gate even when stuck so we don't burn one
		// dice-roll per tick on a dead-end mob.
		setNextWander(m, now)
		return nil
	}
	dst, err := deps.World.Move(srcID, dir)
	if err != nil {
		setNextWander(m, now)
		return nil
	}

	deps.Placement.Remove(m.ID())
	deps.Placement.Place(m.ID(), dst.ID)
	setNextWander(m, now)

	announce(ctx, deps.Broadcaster, m.Name(), srcID, dst.ID, dir)

	// Mob-entered-room hook (spec §4): evaluate the arriving mob
	// against every player already in dst. Evaluator handles nil
	// players / placement / store internally.
	if deps.Evaluator != nil {
		deps.Evaluator.OnMobEntered(ctx, m, dst.ID)
	}
	return nil
}

// readyToWander reports whether the gate has elapsed. A missing /
// non-int64 gate value is treated as 0, which means "ready" — the
// first tick after spawn is always eligible.
func readyToWander(m *entities.MobInstance, now time.Time) bool {
	raw, ok := m.Properties()[PropWanderNextAt]
	if !ok {
		return true
	}
	var next int64
	switch v := raw.(type) {
	case int64:
		next = v
	case int:
		next = int64(v)
	default:
		return true
	}
	return now.UnixNano() >= next
}

func setNextWander(m *entities.MobInstance, now time.Time) {
	m.Properties()[PropWanderNextAt] = now.Add(DefaultWanderInterval).UnixNano()
}

// pickExit returns one direction the room has an exit in, chosen
// uniformly at random. Returns ok=false when the room has no exits.
// Sorted iteration order means tests with a seeded Rand are
// deterministic across runs (Go map iteration would otherwise leak
// in).
func pickExit(r *world.Room, rng interface {
	IntN(int) int
}) (world.Direction, bool) {
	if len(r.Exits) == 0 {
		return 0, false
	}
	dirs := make([]world.Direction, 0, len(r.Exits))
	for d := range r.Exits {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i] < dirs[j] })
	if rng == nil {
		return dirs[0], true
	}
	return dirs[rng.IntN(len(dirs))], true
}

// announce broadcasts the departure and arrival lines. Mirrors the
// player-movement broadcasts in command.movementHandler — same
// phrasing so players experience mob and player movement
// identically.
func announce(ctx context.Context, b Broadcaster, name string, src, dst world.RoomID, dir world.Direction) {
	if b == nil || name == "" {
		return
	}
	b.SendToRoom(ctx, src, fmt.Sprintf("%s heads %s.", name, dir.Long()))
	from := dir.Opposite().Long()
	if from == "" {
		from = "elsewhere"
	}
	b.SendToRoom(ctx, dst, fmt.Sprintf("%s arrives from the %s.", name, from))
}
