package session

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// errInvalidFollow is returned for a nil manager or an empty player id — a
// corrupt/degenerate call, distinct from a genuine self-follow. The verb maps it
// to the generic "you can't follow that" rather than the self-follow message.
var errInvalidFollow = errors.New("follow: invalid player id")

// maxFollowDepth bounds the recursive pull when a follow chain (A→B→C…) all
// relocates on one move: each follower's re-dispatched step re-publishes
// PlayerMoved, re-entering PullFollowers on the same goroutine. Cycle-prevention
// already keeps the graph acyclic; this is a backstop against a pathologically
// long chain blowing the stack. follow.md §6 (chain-depth guard).
const maxFollowDepth = 25

type followDepthKey struct{}

// Manager implements command.FollowService (follow.md). The graph is guarded by
// followMu, held only for the brief map mutations — never across a GetByPlayerID
// or a re-dispatch.

// Follow records followerID trailing leaderID, replacing any prior leader.
// Refuses self-follow and any follow that would close a cycle.
func (m *Manager) Follow(followerID, leaderID string) error {
	if m == nil || followerID == "" || leaderID == "" {
		return errInvalidFollow
	}
	if followerID == leaderID {
		return command.ErrFollowSelf
	}
	m.followMu.Lock()
	defer m.followMu.Unlock()
	// Cycle check: walk the prospective leader's own leader-chain; if it reaches
	// the follower, the new edge would close a loop.
	for cur := leaderID; cur != ""; cur = m.followLeader[cur] {
		if cur == followerID {
			return command.ErrFollowCycle
		}
	}
	if old, ok := m.followLeader[followerID]; ok {
		m.removeFollowerLocked(old, followerID)
	}
	m.followLeader[followerID] = leaderID
	if m.followers[leaderID] == nil {
		m.followers[leaderID] = make(map[string]bool)
	}
	m.followers[leaderID][followerID] = true
	return nil
}

// Unfollow ends followerID's relationship, returning the former leader.
func (m *Manager) Unfollow(followerID string) (string, bool) {
	if m == nil {
		return "", false
	}
	m.followMu.Lock()
	defer m.followMu.Unlock()
	leader, ok := m.followLeader[followerID]
	if !ok {
		return "", false
	}
	delete(m.followLeader, followerID)
	m.removeFollowerLocked(leader, followerID)
	return leader, true
}

// Lose drops every follower of leaderID, returning their ids.
func (m *Manager) Lose(leaderID string) []string {
	if m == nil {
		return nil
	}
	m.followMu.Lock()
	defer m.followMu.Unlock()
	set := m.followers[leaderID]
	if len(set) == 0 {
		return nil
	}
	ids := make([]string, 0, len(set))
	for f := range set {
		ids = append(ids, f)
		delete(m.followLeader, f)
	}
	delete(m.followers, leaderID)
	return ids
}

// Following reports followerID's current leader.
func (m *Manager) Following(followerID string) (string, bool) {
	if m == nil {
		return "", false
	}
	m.followMu.Lock()
	defer m.followMu.Unlock()
	l, ok := m.followLeader[followerID]
	return l, ok
}

// removeFollowerLocked drops followerID from leaderID's set (followMu held).
func (m *Manager) removeFollowerLocked(leaderID, followerID string) {
	if set := m.followers[leaderID]; set != nil {
		delete(set, followerID)
		if len(set) == 0 {
			delete(m.followers, leaderID)
		}
	}
}

// dropFollow fully removes id from the graph on logout/teardown (follow.md §4):
// it unfollows id's own leader (notifying that leader) and drops everyone
// following id (notifying each). No dangling half-edge survives either party
// leaving the world.
func (m *Manager) dropFollow(ctx context.Context, id, name string) {
	if leaderID, had := m.Unfollow(id); had {
		if la, ok := m.GetByPlayerID(leaderID); ok {
			_ = la.Write(ctx, name+" stops following you.")
		}
	}
	for _, fid := range m.Lose(id) {
		if fa, ok := m.GetByPlayerID(fid); ok {
			_ = fa.Write(ctx, name+" departs, and you lose the trail.")
		}
	}
}

// PullFollowers is the PlayerMoved reaction (follow.md §3): when leaderID arrives
// in `to` from `from`, each follower attempts the same step. A follower keeps up
// only when `to` is reachable by a traversable exit from `from` (the adjacency
// rule, §1) AND they are co-located in `from`; the move re-runs the normal path
// (all gates), and a follower who can't make it has the follow broken. Runs on
// the mover's goroutine; a followed follower's re-dispatched step re-enters here
// for chains (bounded by maxFollowDepth + cycle-prevention).
func (m *Manager) PullFollowers(ctx context.Context, leaderID string, from, to world.RoomID) {
	if m == nil || m.actionCommands == nil {
		return
	}
	depth, _ := ctx.Value(followDepthKey{}).(int)
	if depth >= maxFollowDepth {
		logging.From(ctx).Warn("follow pull depth cap reached",
			slog.String("event", "follow.depth_cap"), slog.String("leader", leaderID))
		return
	}

	m.followMu.Lock()
	set := m.followers[leaderID]
	if len(set) == 0 {
		m.followMu.Unlock()
		return
	}
	followers := make([]string, 0, len(set))
	for f := range set {
		followers = append(followers, f)
	}
	m.followMu.Unlock()

	leaderName := "someone"
	if la, ok := m.GetByPlayerID(leaderID); ok {
		leaderName = la.Name()
	}

	dir, adjacent := m.directionBetween(from, to)
	childCtx := context.WithValue(ctx, followDepthKey{}, depth+1)

	// The graph is a FOREST — Follow gives each follower exactly one leader (a
	// re-target removes the old edge), so no node is reachable by two leader
	// chains and no follower is pulled twice in one event. The `!= from` check
	// below therefore only fires for a follower genuinely left behind, never for
	// one already relocated by a sibling chain (that case can't arise).

	for _, fid := range followers {
		fa, ok := m.GetByPlayerID(fid)
		if !ok || fa == nil {
			continue // offline follower; teardown will reap the edge
		}
		// Non-adjacent leader move (recall/teleport) or a follower not in the
		// leader's source room → can't keep up.
		if !adjacent || fa.Room() == nil || fa.Room().ID != from {
			m.breakFollow(ctx, fid, fa, leaderName)
			continue
		}
		// Re-run the follower's step through the normal move path (all gates,
		// broadcasts, render, and the PlayerMoved that chains their own followers).
		_ = m.actionCommands.Dispatch(childCtx, m.actionEnv, fa, dir.Long())
		if fa.Room() == nil || fa.Room().ID != to {
			m.breakFollow(ctx, fid, fa, leaderName) // blocked / exhausted / in combat
		}
	}
}

// breakFollow ends fid's follow and tells them they lost the leader.
func (m *Manager) breakFollow(ctx context.Context, fid string, fa *connActor, leaderName string) {
	if _, had := m.Unfollow(fid); had && fa != nil {
		_ = fa.Write(ctx, "You lose sight of "+leaderName+".")
	}
}

// directionBetween finds a traversable exit direction from `from` whose target
// is `to` (the adjacency test). Returns false when none — a non-adjacent jump.
func (m *Manager) directionBetween(from, to world.RoomID) (world.Direction, bool) {
	w := m.actionEnv.World
	if w == nil {
		return 0, false
	}
	r, err := w.Room(from)
	if err != nil || r == nil {
		return 0, false
	}
	for d, ex := range r.Exits {
		if ex.Target == to {
			return d, true
		}
	}
	return 0, false
}
