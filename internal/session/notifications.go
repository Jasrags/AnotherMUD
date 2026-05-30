package session

import (
	"context"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
)

// actorSink adapts a *connActor to notifications.Sink so the
// notification manager can deliver text through the session writer
// without importing session into notifications (which would create
// a cycle).
type actorSink struct{ a *connActor }

// Deliver writes the notification text to the connected client.
// Errors propagate so the notifications manager can re-enqueue on
// writer failure per spec §5.
func (s actorSink) Deliver(ctx context.Context, n notifications.Notification) error {
	return s.a.Write(ctx, n.Text)
}

// notifRegister binds the actor to the notifications manager and
// drains any persisted backlog through the session writer. Called
// after the actor is fully added to the session manager so
// in-room broadcasts and the welcome line have already settled.
// nil-safe: a Config without a Notifications manager is a no-op.
func notifRegister(ctx context.Context, cfg Config, a *connActor) {
	if cfg.Notifications == nil {
		return
	}
	pid := a.PlayerID()
	name := a.Name()
	sink := actorSink{a: a}
	if err := cfg.Notifications.Register(ctx, pid, name, sink); err != nil {
		logging.From(ctx).Warn("notify register failed",
			slog.String("event", "notify.register.err"),
			slog.String("player_id", pid),
			slog.Any("err", err))
		return
	}
	if err := cfg.Notifications.Drain(ctx, pid); err != nil {
		logging.From(ctx).Warn("notify drain failed",
			slog.String("event", "notify.drain.err"),
			slog.String("player_id", pid),
			slog.Any("err", err))
	}
}

// notifUnregister flushes any dirty queue to disk and releases the
// actor's in-memory notification state. Called from every "session
// is gone" path (fullTeardown, linkdead reap, takeover).
// nil-safe.
func notifUnregister(ctx context.Context, cfg Config, a *connActor) {
	if cfg.Notifications == nil {
		return
	}
	pid := a.PlayerID()
	if err := cfg.Notifications.Unregister(ctx, pid); err != nil {
		logging.From(ctx).Warn("notify unregister failed",
			slog.String("event", "notify.unregister.err"),
			slog.String("player_id", pid),
			slog.Any("err", err))
	}
}
