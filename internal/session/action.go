package session

import (
	"context"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// enableActionSweep wires the Manager's timed-action sweep from the Config
// (action-economy.md §3). Called once at startup from Handler, where the
// in-package commandEnv is reachable. A nil Config.Actions leaves the sweep
// disabled (CompleteReadyActions then no-ops). The env is cached once: every
// field it carries is a stable boot singleton, so a per-tick rebuild would buy
// nothing.
func (m *Manager) enableActionSweep(cfg Config) {
	if m == nil {
		return
	}
	m.actionTracker = cfg.Actions
	m.actionCommands = cfg.Commands
	m.actionEnv = commandEnv(cfg)
}

// CompleteReadyActions is the action-complete tick handler's body
// (action-economy.md §3): it sweeps every logged-in actor and finishes any
// timed action whose occupation timer has reached now by REPLAYING the action's
// original command with Env.ReplayAction set — so the consumer (the don/doff
// equip, a future reload) performs the deferred mutation now instead of
// re-arming the timer. Runs on the tick goroutine, exactly like
// CompleteReadyCrafts; the replayed handler mutates the actor through the same
// mutex-guarded connActor methods a live command would, so there is no data
// race with a session-side mutation.
//
// A no-op until enableActionSweep has wired the tracker + registry (a build
// without timed actions). A claimed action with no replayable payload is
// dropped silently (defensive — every shipped consumer stores its command).
func (m *Manager) CompleteReadyActions(ctx context.Context, now uint64) {
	if m == nil || m.actionTracker == nil || m.actionCommands == nil {
		return
	}
	// Iterate only the occupied actors (typically zero or a handful), not every
	// logged-in player every tick. CompleteReady claims a due action atomically;
	// an action interrupted (moved / stopped) between BusyEntities and the claim
	// just reads not-due here.
	for _, id := range m.actionTracker.BusyEntities() {
		act, due := m.actionTracker.CompleteReady(id, now)
		if !due {
			continue
		}
		a, ok := m.GetByPlayerID(id)
		if !ok {
			// Actor vanished after BusyEntities (logout/teardown also Drops, so
			// this is the rare interleave) — the claimed action is simply lost.
			continue
		}
		raw, payloadOK := act.Payload.(string)
		if !payloadOK || raw == "" {
			// Every shipped consumer stores its replay command; a missing/non-
			// string payload is a consumer bug, so surface it rather than drop
			// silently (action-economy.md §3).
			logging.From(ctx).Warn("action completion has no replayable payload",
				slog.String("event", "action.bad_payload"),
				slog.String("kind", string(act.Kind)),
				slog.String("player", a.Name()))
			continue
		}
		env := m.actionEnv
		env.ReplayAction = true
		if err := m.actionCommands.Dispatch(ctx, env, a, raw); err != nil {
			logging.From(ctx).Warn("action replay failed",
				slog.String("event", "action.replay_failed"),
				slog.String("raw", logging.Sanitize(raw)),
				slog.String("player", a.Name()),
				slog.Any("err", err))
		}
	}
}
