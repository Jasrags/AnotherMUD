package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// AskHandler implements `ask <npc> about <topic>` — free-form NPC dialogue,
// distinct from the quest-giver `talk` verb. Spec: docs/specs/npc-dialogue.md. It reads the target mob's
// content-authored `dialogue` property (a topic -> line map) and speaks the
// matching line back. This is how an NPC delivers lore, rumours, hints, and
// flavour (e.g. Doug Coughlin's Laws) without any code per character — a
// builder adds a `properties.dialogue` block and the topics go live.
//
// Backward-compatible with the old `ask` alias of `talk`: when the player
// types `ask <npc>` with no "about <topic>", the call falls through to
// TalkHandler so quest discovery/turn-in still works. Only the presence of
// the "about" separator routes into dialogue.
//
// Kept a hand-resolved verb (HandParsed, no typed arg) for the same reason as
// talk — it resolves a room MOB by keyword via findMobByKeyword, and the tail
// after "about" is a free-text topic, not an enumerable arg.
func AskHandler(ctx context.Context, c *Context) error {
	// Locate the "about" separator; everything before is the NPC term,
	// everything after is the topic. EqualFold so "About"/"ABOUT" work.
	aboutIdx := -1
	for i, a := range c.Args {
		if strings.EqualFold(a, "about") {
			aboutIdx = i
			break
		}
	}
	// No topic clause → this is the quest-giver interaction (`ask` == `talk`).
	if aboutIdx == -1 {
		return TalkHandler(ctx, c)
	}

	npcTerm := strings.TrimSpace(strings.Join(c.Args[:aboutIdx], " "))
	topic := strings.TrimSpace(strings.Join(c.Args[aboutIdx+1:], " "))
	if npcTerm == "" {
		return c.Actor.Write(ctx, "Ask whom?")
	}
	if topic == "" {
		return c.Actor.Write(ctx, "Ask them about what?")
	}

	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You aren't anywhere; there is no one to ask.")
	}
	npc := findMobByKeyword(c, room.ID, npcTerm)
	if npc == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("There is no %q here to ask.", npcTerm))
	}

	line, ok := dialogueLine(c, npc, topic)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("%s has nothing to say about that.", capitalize(npc.Name())))
	}
	return c.Actor.Write(ctx, fmt.Sprintf("<highlight>%s</highlight> says, \"%s\"", capitalize(npc.Name()), line))
}

// dialogueLine resolves an NPC's spoken line for a topic from its
// content-authored `dialogue` property. The property is a map of
// topic -> line, where a line is either a single string or a list of
// strings (one is chosen per call — see pickLine). Topic lookup is
// case-insensitive; an unmatched topic falls back to the optional
// "default" entry. Returns ("", false) when the mob has no dialogue,
// the topic is unknown, and there is no default.
func dialogueLine(c *Context, npc *entities.MobInstance, topic string) (string, bool) {
	m, ok := dialogueMap(npc)
	if !ok {
		return "", false
	}
	if val, ok := lookupTopic(m, topic); ok {
		return pickLine(c, val), true
	}
	if val, ok := lookupTopic(m, "default"); ok {
		return pickLine(c, val), true
	}
	return "", false
}

// dialogueMap returns the mob's content-authored `dialogue` property as a
// topic->line map, or (nil, false) when the mob has no dialogue or it is the
// wrong shape. Shared by dialogueLine (topic lookup) and npcDialogueIntro
// (intro lookup) so the property fetch + type assertion lives in one place.
func dialogueMap(npc *entities.MobInstance) (map[string]any, bool) {
	raw, ok := npc.Property("dialogue")
	if !ok {
		return nil, false
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	return m, true
}

// npcDialogueIntro resolves an NPC's opening line for a bare `ask`/`talk`
// with no "about <topic>" and no quest to offer — so a dialogue-carrying NPC
// nudges the player toward its topics instead of dead-ending on "nothing for
// you right now" (the TalkHandler fallthrough). Tries a small priority list of
// intro keys, then the shared "default". Returns ("", false) when the mob has
// no usable dialogue, in which case the caller keeps the old message.
func npcDialogueIntro(c *Context, npc *entities.MobInstance) (string, bool) {
	m, ok := dialogueMap(npc)
	if !ok {
		return "", false
	}
	for _, key := range []string{"greeting", "started", "getting", "hello"} {
		if val, ok := lookupTopic(m, key); ok {
			return pickLine(c, val), true
		}
	}
	if val, ok := lookupTopic(m, "default"); ok {
		return pickLine(c, val), true
	}
	return "", false
}

// lookupTopic finds a topic in the dialogue map case-insensitively. Keys are
// authored as-is in YAML; matching normalizes both sides so `ask doug about
// LAWS` hits a `laws:` entry.
func lookupTopic(m map[string]any, topic string) (any, bool) {
	topic = strings.ToLower(strings.TrimSpace(topic))
	for k, v := range m {
		if strings.ToLower(strings.TrimSpace(k)) == topic {
			return v, true
		}
	}
	return nil, false
}

// pickLine reduces a dialogue value to a single spoken line. A plain string
// is returned verbatim; a list rotates by the engine clock so repeated asks
// cycle through the variants (e.g. Coughlin's Laws). Rotation via the clock
// (not math/rand) keeps the choice deterministic under a fixed test clock
// while still varying in production, where Now() advances every call.
func pickLine(c *Context, val any) string {
	switch v := val.(type) {
	case string:
		return v
	case []any:
		return pickFrom(c, v)
	default:
		return fmt.Sprint(val)
	}
}

func pickFrom(c *Context, list []any) string {
	if len(list) == 0 {
		return ""
	}
	idx := 0
	if c != nil && c.Clock != nil {
		l := int64(len(list))
		// Safe non-negative modulo: ((n % l) + l) % l. A plain -n on a
		// negative nano count would overflow at math.MinInt64 (−MinInt64 ==
		// MinInt64, still negative) and index out of range; this form can't.
		idx = int(((c.Clock.Now().UnixNano() % l) + l) % l)
	}
	if s, ok := list[idx].(string); ok {
		return s
	}
	return fmt.Sprint(list[idx])
}
