package command

import (
	"context"
	"log/slog"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// Announcer is the all-sessions broadcast seam the `announce` admin verb
// needs (admin-verbs §5): deliver one line to every connected session. The
// session Manager satisfies it via SendToAll. Kept separate from the
// room-scoped Broadcaster so room-only mocks aren't forced to grow an
// all-sessions method (the same many-small-interfaces pattern as Locator /
// RoleTargetResolver). Handlers MUST tolerate a nil Announcer.
type Announcer interface {
	SendToAll(ctx context.Context, text string, excludePlayerIDs ...string)
}

// announceLabel prefixes a server announcement so it reads as an
// administrative broadcast distinct from any chat channel (admin-verbs
// §5 / §8 attribution). A literal here, not yet externalized — the
// configuration surface (§8 "Announce attribution") is noted as an open
// knob for a later slice.
const announceLabel = "[Announcement]"

// auditAdmin records a successful admin action (admin-verbs §6): it
// publishes the non-cancellable admin.action fact on the bus and writes a
// structured audit line to the operator log. target is empty for verbs with
// no single target; args is the salient argument string. Every admin verb
// calls this on success — the single accountability choke point, so a new
// admin verb cannot ship without leaving an audit trail.
func auditAdmin(ctx context.Context, c *Context, verb, target, args string) {
	c.Publish(ctx, eventbus.AdminAction{
		Actor:  c.Actor.PlayerID(),
		Verb:   verb,
		Target: target,
		Args:   args,
	})
	logging.From(ctx).Info("admin.action",
		slog.String("actor", c.Actor.PlayerID()),
		slog.String("verb", verb),
		slog.String("target", target),
		slog.String("args", args))
}

// AnnounceHandler implements `announce <message>` (admin-verbs §5): an
// admin broadcasts a server-wide message to every connected session,
// attributed as an administrative announcement and distinct from any
// channel. Admin-gated at dispatch (M19.3); reaching the handler already
// means the actor holds the admin role, so no re-check here (§2 — the gate
// is the single source of truth).
func AnnounceHandler(ctx context.Context, c *Context) error {
	if c.Announcer == nil {
		return c.Actor.Write(ctx, "Announcements are not enabled in this build.")
	}

	msg := strings.TrimSpace(strings.Join(c.Args, " "))
	if msg == "" {
		return c.Actor.Write(ctx, "Usage: announce <message>")
	}

	// Broadcast to everyone, including the announcing admin — an
	// announcement is a fact the whole server (the admin included) sees,
	// not a directed tell, so no self-exclusion.
	line := announceLabel + " " + msg
	c.Announcer.SendToAll(ctx, line)

	auditAdmin(ctx, c, "announce", "", msg)
	return nil
}
