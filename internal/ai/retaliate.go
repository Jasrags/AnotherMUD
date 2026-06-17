package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// RetaliateTimeout bounds how long a shot mob keeps chasing a shooter it can't
// reach (a closed door slammed in its face, the shooter fled out of the
// adjacent room). Past it, the grudge is dropped and the mob resumes its normal
// behavior. Hardcoded today; promote to a per-template knob if a content author
// wants a more or less dogged pursuer.
const RetaliateTimeout = 30 * time.Second

// propRetaliateExpireAt is the AI-managed grudge expiry (Unix nanos). The
// `shoot` verb sets only the target + room (it has no clock); the retaliation
// step arms this on first handling so the command layer stays clock-free.
const propRetaliateExpireAt = "retaliate_expire_at"

// tryRetaliate runs a shot mob's pursue-and-engage for ranged-combat Model C
// (§10) slice 2. It returns true when the mob held a live retaliation grudge —
// the dispatcher then skips the mob's normal behavior this tick (it is busy
// hunting). A mob with no grudge returns false immediately and falls through to
// wander/stationary/etc.
//
// The grudge is set by the `shoot` verb (entities.PropRetaliateTarget +
// PropRetaliateRoom) when a cross-room shot lands on a LIVING mob. Pursuit is
// adjacent-only, matching C1: the shooter is one room away, so the mob steps
// through the exit whose far side is the shooter's room and then engages. The
// engage is forced via MobAggro regardless of the mob's base disposition —
// being shot makes the fight personal even for an otherwise-neutral mob.
//
// Resolution per tick:
//   - grudge expired → drop it, resume normal behavior.
//   - already in the shooter's room → engage, drop the grudge.
//   - shooter's room is directly adjacent → step into it and engage (same tick).
//   - exit toward the shooter's room is blocked (closed door) → keep the grudge
//     and retry next tick until the timeout.
//   - shooter's room is not adjacent (mob moved, or multi-room) → drop the
//     grudge. Multi-room pursuit is a deferred refinement (§10).
func tryRetaliate(ctx context.Context, m *entities.MobInstance, deps Deps) bool {
	targetID, destRoomStr, ok := m.Retaliation()
	if !ok {
		return false
	}
	// Without world/placement/bus the step can't act; leave the grudge for a
	// properly-wired tick rather than silently dropping it.
	if deps.World == nil || deps.Placement == nil || deps.Bus == nil {
		return false
	}
	destRoom := world.RoomID(destRoomStr)
	if destRoom == "" {
		clearRetaliation(m)
		return true
	}
	if retaliationExpired(m, deps) {
		clearRetaliation(m)
		return true
	}

	srcID, ok := deps.Placement.RoomOf(m.ID())
	if !ok {
		clearRetaliation(m)
		return true
	}

	// Already with the shooter — settle it.
	if srcID == destRoom {
		engageRetaliation(ctx, m, targetID, srcID, deps)
		clearRetaliation(m)
		return true
	}

	// Step toward the shooter's room (adjacent in C1).
	dir, ok := exitToward(deps.World, srcID, destRoom)
	if !ok {
		// Not directly adjacent — the shooter moved, or it's multiple rooms
		// off (multi-room pursuit is deferred, §10). Drop the grudge.
		clearRetaliation(m)
		return true
	}
	dst, err := deps.World.Move(srcID, dir)
	if err != nil {
		if errors.Is(err, world.ErrDoorClosed) {
			// A closed door stands between them: keep the grudge and retry next
			// tick until the timeout — mobs don't open doors.
			return true
		}
		// Any other Move error means the exit is broken (the target room went
		// missing) — a content/wiring fault, not a door the mob can wait out.
		// Drop the grudge so it doesn't chase a phantom room until the timeout.
		logging.From(ctx).Warn("retaliate: broken exit during pursuit",
			slog.String("mob_id", string(m.ID())),
			slog.String("dir", dir.Long()),
			slog.Any("err", err))
		clearRetaliation(m)
		return true
	}

	deps.Placement.Remove(m.ID())
	deps.Placement.Place(m.ID(), dst.ID)
	announceCharge(ctx, deps.Broadcaster, m.Name(), srcID, dst.ID, dir)

	// Standard mob-entered-room hook so other players in the destination get
	// the ambient reaction (mirrors wander). Harmless against the shooter: the
	// explicit engage below is idempotent with any disposition-fired aggro.
	if deps.Evaluator != nil {
		deps.Evaluator.OnMobEntered(ctx, m, dst.ID)
	}

	// Arrived in the shooter's room — engage this same tick so the charge-in
	// flows straight into the fight.
	if dst.ID == destRoom {
		engageRetaliation(ctx, m, targetID, dst.ID, deps)
		clearRetaliation(m)
	}
	return true
}

// engageRetaliation publishes MobAggro so the existing aggro→Engage wiring
// (cmd/anothermud) starts the round loop. Reused rather than reaching for a new
// engage dependency — the same path a disposition-hostile reaction takes.
func engageRetaliation(ctx context.Context, m *entities.MobInstance, targetPlayerID string, room world.RoomID, deps Deps) {
	deps.Bus.Publish(ctx, eventbus.MobAggro{
		MobID:    m.ID(),
		MobName:  m.Name(),
		PlayerID: targetPlayerID,
		RoomID:   room,
	})
}

// exitToward returns the direction from srcID whose far side is destRoom, or
// false when no exit leads there directly. Sorted iteration so a room with two
// exits to the same room resolves deterministically. Hidden exits are not
// gated here — that discovery rule is player-only (movement command); a mob may
// pursue through any exit the graph holds.
func exitToward(w *world.World, srcID, destRoom world.RoomID) (world.Direction, bool) {
	src, err := w.Room(srcID)
	if err != nil {
		return 0, false
	}
	dirs := make([]world.Direction, 0, len(src.Exits))
	for d := range src.Exits {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i] < dirs[j] })
	for _, d := range dirs {
		if src.Exits[d].Target == destRoom {
			return d, true
		}
	}
	return 0, false
}

// announceCharge broadcasts a mob's pursuit step — angrier phrasing than the
// neutral wander arrival, so onlookers read it as a charge, not a stroll.
func announceCharge(ctx context.Context, b Broadcaster, name string, src, dst world.RoomID, dir world.Direction) {
	if b == nil || name == "" {
		return
	}
	to := dir.Long()
	if to == "" {
		to = "away"
	}
	b.SendToRoom(ctx, src, fmt.Sprintf("%s charges off to the %s.", name, to))
	from := dir.Opposite().Long()
	if from == "" {
		from = "elsewhere"
	}
	b.SendToRoom(ctx, dst, fmt.Sprintf("%s charges in from the %s!", name, from))
}

// retaliationExpired reports whether the grudge has timed out, arming the
// expiry on first handling. A nil clock (tests that don't wire one) disables
// expiry — the grudge then resolves on reach/blocked outcomes alone.
func retaliationExpired(m *entities.MobInstance, deps Deps) bool {
	if deps.Clock == nil {
		return false
	}
	now := deps.Clock.Now().UnixNano()
	raw, ok := m.Property(propRetaliateExpireAt)
	if !ok {
		m.SetProperty(propRetaliateExpireAt, now+int64(RetaliateTimeout))
		return false
	}
	var deadline int64
	switch v := raw.(type) {
	case int64:
		deadline = v
	case int:
		deadline = int64(v)
	default:
		// Corrupt value — re-arm rather than expire instantly.
		m.SetProperty(propRetaliateExpireAt, now+int64(RetaliateTimeout))
		return false
	}
	return now >= deadline
}

// clearRetaliation drops the grudge (target+room, atomically) and resets its
// AI-managed expiry. The expiry is a single-goroutine (tick) property, so it
// needs no pairing with the grudge — but it is reset here so a re-grudged mob
// re-arms a fresh deadline on its next handling.
func clearRetaliation(m *entities.MobInstance) {
	m.ClearRetaliation()
	m.SetProperty(propRetaliateExpireAt, int64(0))
}

// hasRetaliation reports whether a mob is carrying a live grudge — consulted by
// the dispatcher's combat-gate to drop a satisfied grudge when the mob is
// already fighting.
func hasRetaliation(m *entities.MobInstance) bool {
	_, _, ok := m.Retaliation()
	return ok
}
