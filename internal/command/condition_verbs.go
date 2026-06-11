package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/condition"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// EffectTemplateSource resolves an effect template by id (conditions §5).
// effect.Registry satisfies it; the afflict verb uses it to map a condition
// name to its effect template.
type EffectTemplateSource interface {
	Get(id string) (progression.EffectTemplate, bool)
}

// AfflictHandler implements `afflict <target> <condition> [duration]`
// (conditions §5) — the admin inflict verb. Applies a condition effect to a
// target by FORCE (no entry save; that is the ability path's job). Admin-
// marked (dispatcher gate) and audited. Duration (effect-ticks ≈ rounds) is
// optional; 0 / omitted uses the effect template's own duration.
func AfflictHandler(ctx context.Context, c *Context) error {
	if c.Effects == nil || c.EffectTemplates == nil {
		return c.Actor.Write(ctx, "Conditions are not enabled.")
	}
	args := c.Args
	if len(args) < 2 {
		return c.Actor.Write(ctx, "Afflict whom with what? (afflict <target> <condition> [duration])")
	}
	// Last token may be a numeric duration override; if so, peel it off. A
	// non-positive override is rejected rather than silently ignored — it
	// would otherwise leave the template's own duration in place, which for a
	// permanent template would lock the target in the condition unexpectedly.
	duration := 0
	if n, err := strconv.Atoi(args[len(args)-1]); err == nil && len(args) > 2 {
		if n <= 0 {
			return c.Actor.Write(ctx, "Duration must be a positive number of rounds.")
		}
		duration = n
		args = args[:len(args)-1]
	}
	condName := strings.ToLower(strings.TrimSpace(args[len(args)-1]))
	targetToken := strings.Join(args[:len(args)-1], " ")

	_, name, id, ok := resolveSetTarget(c, targetToken)
	if !ok {
		return c.Actor.Write(ctx, "You don't see them here.")
	}
	tpl, ok := c.EffectTemplates.Get(condName)
	if !ok || !condition.AnyCondition(tpl.Flags) {
		return c.Actor.Write(ctx, fmt.Sprintf("There is no such condition: %q.", condName))
	}
	// tpl is a value copy from the registry; mutating Duration here does not
	// affect the canonical template (the registry returns EffectTemplate by
	// value).
	if duration > 0 {
		tpl.Duration = duration
	}
	if !c.Effects.Apply(ctx, id, tpl, c.Actor.PlayerID(), "") {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is already %s.", name, condName))
	}
	auditAdmin(ctx, c, "afflict", id, condName)
	return c.Actor.Write(ctx, fmt.Sprintf("You afflict %s with %s.", name, condName))
}

// CureHandler implements `cure <target> [condition]` (conditions §5) — the
// admin counterpart to afflict. With a condition name it removes that one;
// without, it clears every active condition (leaving non-condition effects
// like bless untouched). Admin-marked and audited.
func CureHandler(ctx context.Context, c *Context) error {
	if c.Effects == nil {
		return c.Actor.Write(ctx, "Conditions are not enabled.")
	}
	if len(c.Args) < 1 {
		return c.Actor.Write(ctx, "Cure whom? (cure <target> [condition])")
	}
	// An optional trailing condition name; the rest is the target.
	var condName string
	args := c.Args
	if len(args) > 1 {
		condName = strings.ToLower(strings.TrimSpace(args[len(args)-1]))
		args = args[:len(args)-1]
	}
	_, name, id, ok := resolveSetTarget(c, strings.Join(args, " "))
	if !ok {
		return c.Actor.Write(ctx, "You don't see them here.")
	}

	removed := 0
	if condName != "" {
		if c.Effects.RemoveByID(ctx, id, condName) {
			removed = 1
		}
	} else {
		for _, flag := range condition.Flags() {
			removed += c.Effects.RemoveByFlag(ctx, id, flag)
		}
	}
	auditAdmin(ctx, c, "cure", id, condName)
	if removed == 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s has no such condition to cure.", name))
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You cure %s (%d condition(s) lifted).", name, removed))
}

// AffectsHandler implements `affects` (alias `effects`) — lists the actor's
// active effects, including conditions (conditions §6). Each line shows the
// effect name, its remaining duration (rounds, or "permanent"), and a
// `[condition]` tag for status conditions so the player can see what's
// gripping them. Self-only.
func AffectsHandler(ctx context.Context, c *Context) error {
	if c.Effects == nil {
		return c.Actor.Write(ctx, "You feel nothing unusual.")
	}
	effs := c.Effects.Effects(c.Actor.PlayerID())
	if len(effs) == 0 {
		return c.Actor.Write(ctx, "You are under no active effects.")
	}
	var b strings.Builder
	b.WriteString("<highlight>Active effects:</highlight>\n")
	for _, e := range effs {
		dur := fmt.Sprintf("%d round(s)", e.Remaining)
		if e.IsPermanent() {
			dur = "permanent"
		}
		tag := ""
		if condition.AnyCondition(e.Flags) {
			tag = " <warning>[condition]</warning>"
		}
		b.WriteString(fmt.Sprintf("  %s — %s%s\n", upperFirstASCII(e.ID), dur, tag))
	}
	return c.Actor.Write(ctx, strings.TrimRight(b.String(), "\n"))
}

// upperFirstASCII capitalizes the first byte of a single lowercase token
// (effect ids are lowercase ASCII like "stunned").
func upperFirstASCII(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// conditionProneEffectID is the effect id the `prone` condition ships under
// (content/core/effects/condition-prone.yaml). The `stand`/`wake` verb
// (WakeHandler) removes exactly this to get a prone combatant up (conditions
// §5).
const conditionProneEffectID = "prone"
