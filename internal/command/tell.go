package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/notifications"
)

// TellsBuffer is the session-local state used by tell / reply /
// tells. The connActor implements it; verbs type-assert. A test or
// admin actor without this state falls through to the
// "feature not enabled" branch — handlers nil-tolerate the same
// way they do for Env services.
//
// Spec: docs/specs/chat-channels-and-tells.md §7 (v1: in-memory).
type TellsBuffer interface {
	// LastTellPartner returns the display name of the most recent
	// counterparty in a tell exchange, or "" if none.
	LastTellPartner() string
	// SetLastTellPartner records a new counterparty. Implementations
	// MUST silently ignore the empty string so a delivery with no
	// sender cannot blank an existing slot.
	SetLastTellPartner(string)
	// RecentTells returns a copy of the session's recent received-
	// tell lines (oldest first).
	RecentTells() []string
	// AppendRecentTell appends a freshly-delivered tell line to the
	// session ring (capped per spec §10).
	AppendRecentTell(string)
}

// TellHandler implements `tell <name> <message>` (spec §6.2).
// Publishes a tell-priority notification to the recipient, prints a
// confirmation line to the sender, and updates the sender's reply
// slot. Recipient resolution uses Env.TellResolver: online first
// (live session), then offline-known (save file exists). Case-
// insensitive exact match per the locked decision in
// `docs/themes/social-mud-plan.md`.
func TellHandler(ctx context.Context, c *Context) error {
	if len(c.Args) < 2 {
		return c.Actor.Write(ctx, "Tell whom what?")
	}
	name := c.Args[0]
	msg := strings.Join(c.Args[1:], " ")
	return doTell(ctx, c, name, msg)
}

// ReplyHandler implements `reply <message>` (spec §6.2). Reuses
// doTell with the actor's stored last-tell-partner. NoReplyTarget
// when the slot is empty (no prior tell exchange this session).
func ReplyHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Reply with what?")
	}
	buf, ok := c.Actor.(TellsBuffer)
	if !ok {
		return c.Actor.Write(ctx, "You have no one to reply to.")
	}
	target := buf.LastTellPartner()
	if target == "" {
		return c.Actor.Write(ctx, "You have no one to reply to.")
	}
	return doTell(ctx, c, target, strings.Join(c.Args, " "))
}

// TellsHandler implements `tells` (spec §8.5) — print the session's
// recent received-tell history. In-memory only; not the persisted
// substrate queue (that's drained on login and gone from the queue).
func TellsHandler(ctx context.Context, c *Context) error {
	buf, ok := c.Actor.(TellsBuffer)
	if !ok {
		return c.Actor.Write(ctx, "No recent tells.")
	}
	recent := buf.RecentTells()
	if len(recent) == 0 {
		return c.Actor.Write(ctx, "No recent tells.")
	}
	lines := make([]string, 0, len(recent)+1)
	lines = append(lines, "Recent tells:")
	lines = append(lines, recent...)
	return c.Actor.Write(ctx, strings.Join(lines, "\n"))
}

// doTell is the shared publish path for `tell` and `reply`. It
// resolves the recipient, publishes through the notification
// manager, writes the sender confirmation, and updates the
// sender's reply slot. On any unrecoverable failure it surfaces
// a single user-facing failure line; the substrate logs detail.
func doTell(ctx context.Context, c *Context, recipientName, msg string) error {
	if c.Notifications == nil || c.TellResolver == nil {
		return c.Actor.Write(ctx, "Tells are not enabled.")
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return c.Actor.Write(ctx, "Tell them what?")
	}

	var (
		recipientID        string
		recipientCanonical string
		recipientDisplay   string
	)
	if a, ok := c.TellResolver.ResolveOnline(recipientName); ok {
		recipientID = a.PlayerID()
		recipientCanonical = strings.ToLower(a.Name())
		recipientDisplay = a.Name()
	} else if id, canon, ok := c.TellResolver.ResolveOffline(ctx, recipientName); ok {
		recipientID = id
		recipientCanonical = canon
		// We don't load the save just to read the display-cased
		// name — the user typed something close enough; echo it
		// back as they typed it for the confirmation line.
		recipientDisplay = recipientName
	} else {
		return c.Actor.Write(ctx, fmt.Sprintf("No player named %q is here or known.", recipientName))
	}

	senderName := c.Actor.Name()
	n := notifications.Notification{
		Recipients: []string{recipientID},
		Priority:   notifications.PriorityTell,
		Kind:       "tell",
		Text:       fmt.Sprintf("%s tells you: %s", senderName, msg),
		Sender:     senderName,
	}
	if err := c.Notifications.Publish(ctx, n, map[string]string{recipientID: recipientCanonical}); err != nil {
		return c.Actor.Write(ctx, "Your tell could not be sent.")
	}

	if buf, ok := c.Actor.(TellsBuffer); ok {
		buf.SetLastTellPartner(recipientDisplay)
	}

	return c.Actor.Write(ctx, fmt.Sprintf("You tell %s: %s", recipientDisplay, msg))
}
