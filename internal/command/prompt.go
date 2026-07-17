package command

import (
	"context"
	"fmt"
	"strings"
)

// MaxPromptTemplateLen bounds a player-set prompt template
// (ui-rendering-help §7.4 — *Max prompt template length*). Generous
// enough for an elaborate multi-token template, small enough that one
// setting can't bloat a save or a per-tick prompt render.
const MaxPromptTemplateLen = 240

// promptController is the mutation surface a connActor exposes for the
// `prompt` verb (ui-rendering-help §7.4). Defined here so the command
// package doesn't import session just for two methods; the production
// actor satisfies it, test fakes that don't care about prompts don't.
type promptController interface {
	// PromptTemplate returns the player's stored template, or "" when
	// none is set (the renderer then uses the default, §7.1).
	PromptTemplate() string
	// SetPromptTemplate stores template ("" clears it → default), marks
	// the save dirty, and flags a prompt refresh so the change shows on
	// the next flush (§7.3).
	SetPromptTemplate(template string)
	// DefaultPromptTemplate returns the player's *effective* default
	// template (unrendered, {token} placeholders intact) — the one they
	// get when no custom template is set. It is pool-adaptive, so a
	// mana-less archetype's default omits the mana segment; bare `prompt`
	// shows this so what the player is told matches what they see (§7.1).
	DefaultPromptTemplate() string
}

// PromptHandler implements the `prompt` verb (ui-rendering-help §7.4):
// inspect, set, or reset the player's prompt template. It is the only
// writer of the prompt_template property.
func PromptHandler(ctx context.Context, c *Context) error {
	ctrl, ok := c.Actor.(promptController)
	if !ok {
		return c.Actor.Write(ctx, "You can't change your prompt.")
	}

	arg := promptArg(c.Raw)
	switch {
	case arg == "":
		return showPrompt(ctx, c, ctrl)
	case strings.EqualFold(arg, "default"), strings.EqualFold(arg, "reset"):
		if ctrl.PromptTemplate() == "" {
			return c.Actor.Write(ctx, "Your prompt is already the default.")
		}
		ctrl.SetPromptTemplate("")
		return c.Actor.Write(ctx, "Prompt reset to the default.")
	default:
		if len(arg) > MaxPromptTemplateLen {
			return c.Actor.Write(ctx, fmt.Sprintf(
				"That prompt is too long (max %d characters).", MaxPromptTemplateLen))
		}
		ctrl.SetPromptTemplate(arg)
		return c.Actor.Write(ctx, "Prompt updated.")
	}
}

// showPrompt renders the actor's current template (or the default) back
// to them. Written through the normal output path, so color tags render
// while `{token}` placeholders stay literal — the player sees the tokens
// they can edit, with the styling applied.
func showPrompt(ctx context.Context, c *Context, ctrl promptController) error {
	if tpl := ctrl.PromptTemplate(); tpl != "" {
		return c.Actor.Write(ctx,
			"Your prompt:\r\n"+tpl+"\r\nUse `prompt default` to reset it.")
	}
	return c.Actor.Write(ctx,
		"Your prompt is the default:\r\n"+ctrl.DefaultPromptTemplate()+
			"\r\nUse `prompt <template>` to change it.")
}

// promptArg returns the rest of the input line after the verb token,
// with internal spacing and color tags preserved (the dispatcher has
// already trimmed the outer whitespace from c.Raw). Rejoining c.Args
// would collapse runs of spaces inside a template.
func promptArg(raw string) string {
	i := strings.IndexAny(raw, " \t")
	if i < 0 {
		return ""
	}
	return strings.TrimLeft(raw[i+1:], " \t")
}
