package session

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// IdleConfig is the warn-then-disconnect policy applied to inactive
// sessions, per docs/specs/session-lifecycle.md §5. Zero WarnAfter
// AND zero TimeoutAfter disables idle handling entirely.
//
// Warn / timeout messages MAY contain ANSI markup; they pass through
// connActor.Write so each session's color preference applies.
type IdleConfig struct {
	WarnAfter      time.Duration
	TimeoutAfter   time.Duration
	WarnMessage    string
	TimeoutMessage string
}

// DefaultIdleConfig returns the production policy: warn at 5 minutes,
// disconnect at 10 minutes.
func DefaultIdleConfig() IdleConfig {
	return IdleConfig{
		WarnAfter:      5 * time.Minute,
		TimeoutAfter:   10 * time.Minute,
		WarnMessage:    "You have been idle for a while. You will be disconnected soon.",
		TimeoutMessage: "Disconnected: idle timeout.",
	}
}

// disabled reports whether idle handling is a no-op for this config.
func (c IdleConfig) disabled() bool {
	return c.WarnAfter <= 0 && c.TimeoutAfter <= 0
}

// IdleSweep walks every active session and applies the idle policy:
//   - If a session has been silent ≥ TimeoutAfter, send the timeout
//     message and Close() the connection. The session's read loop
//     unwinds naturally on the next Read error, taking the normal
//     teardown path (Remove from manager + Persist + departure
//     broadcast).
//   - Else if silent ≥ WarnAfter AND not previously warned, send the
//     warn message and set the per-session idle-warned flag.
//
// Recipients are snapshotted under the manager's read lock and acted
// on outside the lock. Each per-session decision is computed against
// the actor's mutex-protected lastInputAt so a concurrent input that
// just landed cannot be racily timed out.
//
// Admin-tag exemption (spec §5.2 step 2) is deferred — the engine
// has no role system yet.
func (m *Manager) IdleSweep(ctx context.Context, cfg IdleConfig, clk clock.Clock) {
	if cfg.disabled() {
		return
	}
	if clk == nil {
		clk = clock.RealClock{}
	}
	now := clk.Now()

	m.mu.RLock()
	snapshot := make([]*connActor, 0, len(m.byConn))
	for _, a := range m.byConn {
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	for _, a := range snapshot {
		switch a.idleDecision(now, cfg) {
		case idleQuiet:
			// do nothing
		case idleWarn:
			if cfg.WarnMessage != "" {
				_ = a.Write(ctx, cfg.WarnMessage)
			}
			logging.From(ctx).Debug("idle warn",
				slog.String("player", a.PlayerName()))
		case idleTimeout:
			if cfg.TimeoutMessage != "" {
				_ = a.Write(ctx, cfg.TimeoutMessage)
			}
			logging.From(ctx).Info("idle disconnect",
				slog.String("player", a.PlayerName()))
			// Close drives the read loop into ErrClosed, which triggers
			// the deferred Remove + Persist + departure-broadcast.
			_ = a.conn.Close()
		}
	}
}

type idleAction int

const (
	idleQuiet idleAction = iota
	idleWarn
	idleTimeout
)

// idleDecision evaluates one actor against the policy. Takes the actor
// lock to read lastInputAt and (on warn) flip idleWarned.
func (a *connActor) idleDecision(now time.Time, cfg IdleConfig) idleAction {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lastInputAt.IsZero() {
		// Session hasn't recorded its first tick yet. Treat as fresh
		// so the very first sweep can't disconnect a just-spawned
		// session that hasn't had a chance to type.
		return idleQuiet
	}
	idle := now.Sub(a.lastInputAt)
	if cfg.TimeoutAfter > 0 && idle >= cfg.TimeoutAfter {
		return idleTimeout
	}
	if cfg.WarnAfter > 0 && idle >= cfg.WarnAfter && !a.idleWarned {
		a.idleWarned = true
		return idleWarn
	}
	return idleQuiet
}

// noteInput records that the session received input now. Clears the
// warn latch so the next quiet window earns a fresh warning.
func (a *connActor) noteInput(now time.Time) {
	a.mu.Lock()
	a.lastInputAt = now
	a.idleWarned = false
	a.mu.Unlock()
}
