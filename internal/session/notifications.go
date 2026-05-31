package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

// actorSink adapts a *connActor to notifications.Sink so the
// notification manager can deliver text through the session writer
// without importing session into notifications (which would create
// a cycle).
type actorSink struct{ a *connActor }

// Deliver writes the notification text to the connected client.
// Errors propagate so the notifications manager can re-enqueue on
// writer failure per spec §5.
//
// For tell-kind notifications, the recipient's session-local
// reply slot + recent-tells ring are also updated. This couples
// tells to the sink but keeps the substrate free of tell-specific
// state.
func (s actorSink) Deliver(ctx context.Context, n notifications.Notification) error {
	if err := s.a.Write(ctx, n.Text); err != nil {
		return err
	}
	switch n.Kind {
	case "tell":
		s.a.SetLastTellPartner(n.Sender)
		s.a.AppendRecentTell(n.Text)
	case "channel":
		// M16.4g: parallel-emit a Comm.Channel.Text GMCP frame so
		// clients with a chat panel can route per-channel. The
		// text field carries the FULL rendered line (same string
		// the main window got) for plugin compatibility — Mudlet
		// chat profiles typically strip the prefix client-side.
		// Empty Channel id (notification not produced by the chat
		// verb) silently skips the GMCP emit; the main-window
		// text still went out above.
		if n.Channel != "" {
			s.a.sendCommChannelText(ctx, n.Channel, n.Sender, n.Text)
		}
	}
	return nil
}

// sendCommChannelText emits one Comm.Channel.Text GMCP frame on
// the actor's conn. Silent no-op when the conn doesn't speak GMCP
// or GMCP hasn't been negotiated. JSON marshal / SendGmcp errors
// log at Debug — the main-window line already shipped, so a GMCP
// send failure must not bubble up and trigger notification re-
// enqueue (which would double-write the text line).
func (a *connActor) sendCommChannelText(ctx context.Context, channelID, talker, text string) {
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}
	payload := gmcp.CommChannelText{
		Channel: channelID,
		Talker:  talker,
		Text:    text,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := sender.SendGmcp(ctx, gmcp.PackageCommChannelText, data); err != nil {
		logging.From(ctx).Debug("gmcp comm.channel send failed",
			slog.String("player", a.PlayerName()),
			slog.String("channel", channelID),
			slog.Any("err", err))
	}
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

// TellResolver bridges the M13.5 tell verb's player-name lookup
// to the session manager (for online players) and the player
// store (for offline-known players). The composition root
// constructs one and threads it through session.Config.
type TellResolver struct {
	Manager *Manager
	Players *player.Store
}

// ResolveOnline returns the live actor with the given name (case-
// insensitive exact match) when a session is logged in. The typed-
// nil pitfall is avoided by returning the interface form directly.
func (r TellResolver) ResolveOnline(name string) (command.Actor, bool) {
	if r.Manager == nil {
		return nil, false
	}
	a, ok := r.Manager.GetByName(name)
	if !ok {
		return nil, false
	}
	return a, true
}

// ResolveOffline returns (entityID, canonicalName, true) when a
// player save exists with the given name but no live session is
// up. A single Load reads the save file to extract the entity id;
// for v1 this is acceptable (offline tells are rare) — a
// name→id cache can be added if a real workload demands it.
func (r TellResolver) ResolveOffline(ctx context.Context, name string) (string, string, bool) {
	if r.Players == nil {
		return "", "", false
	}
	if !r.Players.Exists(name) {
		return "", "", false
	}
	save, err := r.Players.Load(ctx, name)
	if err != nil || save == nil || save.ID == "" {
		return "", "", false
	}
	return save.ID, player.CanonicalName(name), true
}
