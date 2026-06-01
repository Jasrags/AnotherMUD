package command

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/chat"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
)

// ChatSubscribers maps a channel id to the entityID→canonicalName set
// of online subscribers. The composition root implements it against
// session.Manager. v1 returns "every online player" for every channel
// (everyone auto-subscribed); M13.6b adds explicit subscription state.
//
// The returned map is the caller's to mutate. Implementations MUST
// return a fresh map per call — the chat publish path filters the
// publisher out by deleting their entry before fan-out, and concurrent
// publishes on the same channel would race if the map were shared.
type ChatSubscribers interface {
	Subscribers(channelID string) map[string]string
}

// ChatScrollbacks maps a channel id to its per-channel ring buffer.
// The composition root owns the scrollback instances and provides
// this lookup so the publish path can append after fan-out.
type ChatScrollbacks interface {
	Scrollback(channelID string) *chat.Scrollback
}

// MakeChannelHandler returns a verb handler that publishes whatever
// the player typed onto `ch`. Registered dynamically at composition
// time from chat.Registry.All() so packs can ship channels without
// touching the static builtins list.
//
// Spec: docs/specs/chat-channels-and-tells.md §6.1.
func MakeChannelHandler(ch *chat.Channel) func(context.Context, *Context) error {
	return func(ctx context.Context, c *Context) error {
		if len(c.Args) == 0 {
			return c.Actor.Write(ctx, fmt.Sprintf("%s what?", ch.DisplayName))
		}
		msg := strings.TrimSpace(strings.Join(c.Args, " "))
		if msg == "" {
			return c.Actor.Write(ctx, fmt.Sprintf("%s what?", ch.DisplayName))
		}
		return doChannelPublish(ctx, c, ch, msg)
	}
}

// ChatListHandler implements `chat list` (§8.3). Shows every channel
// the actor can currently see. v1 doesn't enforce listen gates, so
// every channel appears; M13.6b will filter by gate.
func ChatListHandler(ctx context.Context, c *Context) error {
	if c.ChatRegistry == nil {
		return c.Actor.Write(ctx, "Channels are not enabled.")
	}
	channels := c.ChatRegistry.All()
	if len(channels) == 0 {
		return c.Actor.Write(ctx, "No channels are configured.")
	}
	lines := []string{"Available channels:"}
	for _, ch := range channels {
		lines = append(lines, fmt.Sprintf("  %s", ch.DisplayName))
	}
	return c.Actor.Write(ctx, strings.Join(lines, "\n"))
}

// ChatHistoryHandler implements `chat history <channel> [n]` (§8.4).
// Renders the most recent N messages from the channel's ring buffer.
func ChatHistoryHandler(ctx context.Context, c *Context) error {
	if c.ChatRegistry == nil || c.ChatScrollbacks == nil {
		return c.Actor.Write(ctx, "Channels are not enabled.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "History of which channel?")
	}
	name := c.Args[0]
	ch, ok := c.ChatRegistry.ByDisplayName(name)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("No channel named %q.", name))
	}
	n := 20 // §10 default
	if len(c.Args) > 1 {
		if v, err := parsePositiveInt(c.Args[1]); err == nil {
			n = v
		}
	}
	scrollback := c.ChatScrollbacks.Scrollback(ch.ID)
	if scrollback == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("No history for %s.", ch.DisplayName))
	}
	tail := scrollback.Tail(n)
	if len(tail) == 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("No history for %s.", ch.DisplayName))
	}
	lines := make([]string, 0, len(tail)+1)
	lines = append(lines, fmt.Sprintf("Recent on %s:", ch.DisplayName))
	for _, m := range tail {
		lines = append(lines, m.Text)
	}
	return c.Actor.Write(ctx, strings.Join(lines, "\n"))
}

// doChannelPublish builds the rendered message, fans out via the
// notifications substrate, appends to the scrollback, and writes a
// confirmation line back to the publisher.
func doChannelPublish(ctx context.Context, c *Context, ch *chat.Channel, msg string) error {
	if c.Notifications == nil || c.ChatSubscribers == nil {
		return c.Actor.Write(ctx, "Channels are not enabled.")
	}

	senderName := c.Actor.Name()
	senderID := c.Actor.PlayerID()
	rendered := fmt.Sprintf("[%s] %s: %s", ch.DisplayName, senderName, msg)

	// Build recipient set: every online subscriber except the
	// publisher (self-echo is confirmation-only per the locked
	// decision in docs/archive/themes/social-mud-plan.md).
	//
	// Defensive clone: the ChatSubscribers contract requires
	// implementations to return a fresh map per call, but the
	// publish path is hot enough — and the failure mode (silent
	// cross-publish data races) bad enough — that we copy here
	// too. The wasted allocation is bounded by the active
	// online set.
	src := c.ChatSubscribers.Subscribers(ch.ID)
	subs := make(map[string]string, len(src))
	for id, name := range src {
		if id == senderID {
			continue
		}
		subs[id] = name
	}

	if len(subs) > 0 {
		recipients := make([]string, 0, len(subs))
		for id := range subs {
			recipients = append(recipients, id)
		}
		// Deterministic order for tests and reproducible logs.
		sort.Strings(recipients)
		n := notifications.Notification{
			Recipients: recipients,
			Priority:   notifications.PriorityChannel,
			Kind:       "channel",
			Text:       rendered,
			Sender:     senderName,
			Channel:    ch.ID,
		}
		if err := c.Notifications.Publish(ctx, n, subs); err != nil {
			// Substrate logs detail; surface a single generic line.
			return c.Actor.Write(ctx, "Your message could not be sent.")
		}
	}

	// Append to scrollback regardless of recipient count — the
	// transcript is the canonical record.
	if c.ChatScrollbacks != nil {
		if sb := c.ChatScrollbacks.Scrollback(ch.ID); sb != nil {
			sb.Append(chat.Message{
				PublishedAt: nowFromCtx(c),
				SenderID:    senderID,
				SenderName:  senderName,
				Text:        rendered,
			})
		}
	}

	return c.Actor.Write(ctx, fmt.Sprintf("You %s: %s", ch.DisplayName, msg))
}

// nowFromCtx returns the engine-clock current time. Falls back to
// the stdlib wall clock when the Context has no Clock wired (test
// fixtures); production paths always pass the real clock through
// session.Config so the F3 foundation rule (no direct time.Now in
// engine packages) holds for live traffic.
func nowFromCtx(c *Context) time.Time {
	if c.Clock != nil {
		return c.Clock.Now()
	}
	return clock.RealClock{}.Now()
}

// parsePositiveInt returns a strictly positive integer parsed from
// s, or an error. "0" is rejected — the chat history verb's count
// argument must be >= 1 so the no-arg-default fall-through stays
// distinguishable from "user asked for nothing". Empty input,
// non-digit characters, and overflow are all errors.
func parsePositiveInt(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int(r-'0')
		if n > 1_000_000 {
			return 0, fmt.Errorf("too big: %q", s)
		}
	}
	if n == 0 {
		return 0, fmt.Errorf("must be positive: %q", s)
	}
	return n, nil
}
