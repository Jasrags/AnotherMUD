package command

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/visibility"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// skillPerception is the observer-side skill (collapsing Spot/Listen/Search)
// the `search` verb trains (skills §2 — EPIC S3). The hider-side ids (hide /
// move-silently, or a world's merged stealth skill) are resolved per-character
// and reached through concealer.HideSkill / sneaker.SneakSkill rather than a
// command-package constant, so a world that merges them (SR: `sneaking`) trains
// the right skill.
const skillPerception = "perception"

// rollSkillGain rolls one use-based proficiency gain for a skill the actor just
// exercised — the same loop the `pick` verb runs for Open Lock. success scales
// the gain (a miss gains at the ability's reduced rate). A no-op when the
// proficiency manager or skill roller isn't wired (test fakes), so it is safe
// to call unconditionally from a verb.
func rollSkillGain(c *Context, abilityID string, success bool) {
	if c.Proficiency == nil || c.SkillRoller == nil {
		return
	}
	var stats progression.StatReader
	if sv, ok := c.Actor.(statValuer); ok {
		stats = actorStatReader{sv}
	}
	c.Proficiency.RollUseGain(c.Actor.PlayerID(), abilityID, success, c.SkillRoller, stats)
}

// concealer is the optional actor capability the hide/reveal verbs need
// (visibility.md §3.1). connActor implements it; test actors that don't
// simply cannot hide (the verbs report a graceful refusal). Kept an
// optional interface — like LightViewer's capability assertions — so the
// broad Actor interface (and its many test fakes) need not grow.
type concealer interface {
	// IsHidden reports current hide concealment.
	IsHidden() bool
	// HideScore computes the would-be concealment difficulty (§4.2).
	HideScore() int
	// HideSkill is the skill ability id the hide check reads and the verb trains
	// (skills §2) — the world's stealth skill (SR: `sneaking`), or `hide` by
	// default. Kept on the capability so the verb trains the SAME skill the
	// concealment difficulty consumed, not a hardcoded axis id.
	HideSkill() string
	// Hide commits concealment at score and returns the new instance id (§4.1).
	Hide(score int) uint64
	// Reveal clears hide concealment, returning whether it was hidden.
	Reveal() bool
}

// sneaker is the optional actor capability the `sneak` verb and the
// reveal-on-action hook need (visibility.md §3.2). connActor implements it;
// test actors that don't simply cannot sneak. Like concealer, kept optional
// so the broad Actor interface (and its fakes) need not grow.
type sneaker interface {
	// IsSneaking reports current sneak (moving) concealment.
	IsSneaking() bool
	// SneakDifficulty computes the would-be per-observer contest score (§3.2).
	SneakDifficulty() int
	// SneakSkill is the skill ability id the sneak check reads and the verb
	// trains (skills §2) — the world's stealth skill (SR: `sneaking`), or
	// `move-silently` by default. Mirrors HideSkill on the concealer.
	SneakSkill() string
	// Sneak commits sneaking at score and returns the new instance id.
	Sneak(score int) uint64
	// Unsneak clears sneaking, returning whether it was sneaking.
	Unsneak() bool
}

// HideHandler implements `hide` (visibility.md §3.1): a stationary attempt
// to conceal the actor in its current room. Publishes the cancellable
// concealment.before so packs may forbid hiding (no cover, full light,
// sanctuary); on success sets the hide concealment and emits
// entity.concealed. Actor-only messaging — discovery is per-observer.
func HideHandler(ctx context.Context, c *Context) error {
	h, ok := c.Actor.(concealer)
	if !ok {
		return c.Actor.Write(ctx, "You can't hide.")
	}
	if h.IsHidden() {
		return c.Actor.Write(ctx, "You are already hidden.")
	}

	roomID := roomIDOf(c)

	// Cancellable pre-event (§3.1 step 1 / §6): a veto aborts with a generic
	// refusal so a subscribing pack owns the specific reason.
	pre := eventbus.NewConcealmentBefore(c.Actor.PlayerID(), string(visibility.SourceHide), roomID)
	if c.Bus != nil && c.Bus.PublishCancellable(ctx, pre) {
		return c.Actor.Write(ctx, "You can't hide here.")
	}

	score := h.HideScore()
	h.Hide(score)
	// The act of hiding trains the actor's stealth skill (use-gain; skills §2) —
	// the same id the HideScore check read, so SR's `sneaking` grows from hiding
	// (not the inert core `hide`). Concealment always establishes — the contest
	// comes later — so this is a successful use.
	rollSkillGain(c, h.HideSkill(), true)
	if c.Bus != nil {
		c.Bus.Publish(ctx, eventbus.EntityConcealed{
			EntityID:   c.Actor.PlayerID(),
			SourceType: string(visibility.SourceHide),
			Room:       roomID,
		})
	}
	return c.Actor.Write(ctx, "You slip into the shadows and go still.")
}

// RevealHandler implements `unhide` / `reveal`: the actor voluntarily
// steps out of hiding (visibility.md §3.1). Emits entity.revealed
// (reason = emerged) when it actually dropped a concealment.
func RevealHandler(ctx context.Context, c *Context) error {
	h, ok := c.Actor.(concealer)
	if !ok || !h.IsHidden() {
		return c.Actor.Write(ctx, "You aren't hidden.")
	}
	h.Reveal()
	if c.Bus != nil {
		c.Bus.Publish(ctx, eventbus.EntityRevealed{
			EntityID:   c.Actor.PlayerID(),
			SourceType: string(visibility.SourceHide),
			Reason:     "emerged",
			Room:       roomIDOf(c),
		})
	}
	return c.Actor.Write(ctx, "You step out of hiding.")
}

// SneakHandler implements `sneak` (visibility.md §3.2): toggles a MOVING
// concealment. Unlike hide, sneak survives room changes; it instead filters
// the per-observer enter/leave movement lines (§3.2, the movementHandler
// filter). Toggling it on publishes the cancellable concealment.before (so
// packs may forbid sneaking) and, when uncancelled, sets the sneak score +
// emits entity.concealed (type = sneak). Toggling it off is a plain
// actor-only action that emits entity.revealed (reason = emerged).
func SneakHandler(ctx context.Context, c *Context) error {
	s, ok := c.Actor.(sneaker)
	if !ok {
		return c.Actor.Write(ctx, "You can't sneak.")
	}
	roomID := roomIDOf(c)

	// Toggle off: a plain, uncontested action.
	if s.IsSneaking() {
		s.Unsneak()
		if c.Bus != nil {
			c.Bus.Publish(ctx, eventbus.EntityRevealed{
				EntityID:   c.Actor.PlayerID(),
				SourceType: string(visibility.SourceSneak),
				Reason:     "emerged",
				Room:       roomID,
			})
		}
		return c.Actor.Write(ctx, "You stop moving so carefully.")
	}

	// Toggle on: cancellable pre-event (§3.2 / §6) lets a pack veto with a
	// generic refusal so the pack owns the specific reason.
	pre := eventbus.NewConcealmentBefore(c.Actor.PlayerID(), string(visibility.SourceSneak), roomID)
	if c.Bus != nil && c.Bus.PublishCancellable(ctx, pre) {
		return c.Actor.Write(ctx, "You can't sneak here.")
	}
	s.Sneak(s.SneakDifficulty())
	// Beginning to sneak trains the actor's stealth skill (use-gain; skills §2) —
	// the same id SneakDifficulty read, so SR's `sneaking` grows from sneaking.
	rollSkillGain(c, s.SneakSkill(), true)
	if c.Bus != nil {
		c.Bus.Publish(ctx, eventbus.EntityConcealed{
			EntityID:   c.Actor.PlayerID(),
			SourceType: string(visibility.SourceSneak),
			Room:       roomID,
		})
	}
	return c.Actor.Write(ctx, "You begin moving quietly, keeping to the shadows.")
}

// breakConcealmentOnAction reveals a hidden or sneaking actor when a
// breaks_concealment command runs (visibility §4.5: attacking/casting/
// speaking/loud manipulation drops roll-based concealment so the action is
// observed). The dispatcher calls this BEFORE the handler, after any
// typed-arg resolution succeeded, so the action is seen the instant it
// resolves. A no-op unless the actor carries hide or sneak; flag-gated
// invisibility (S5) is exempt and not handled here.
func breakConcealmentOnAction(ctx context.Context, c *Context) {
	var broke bool
	if h, ok := c.Actor.(concealer); ok && h.IsHidden() {
		h.Reveal()
		broke = true
		if c.Bus != nil {
			c.Bus.Publish(ctx, eventbus.EntityRevealed{
				EntityID:   c.Actor.PlayerID(),
				SourceType: string(visibility.SourceHide),
				Reason:     "acted",
				Room:       roomIDOf(c),
			})
		}
	}
	if s, ok := c.Actor.(sneaker); ok && s.IsSneaking() {
		s.Unsneak()
		broke = true
		if c.Bus != nil {
			c.Bus.Publish(ctx, eventbus.EntityRevealed{
				EntityID:   c.Actor.PlayerID(),
				SourceType: string(visibility.SourceSneak),
				Reason:     "acted",
				Room:       roomIDOf(c),
			})
		}
	}
	if broke {
		_ = c.Actor.Write(ctx, "Your sudden action gives you away; you are no longer concealed.")
	}
}

// roomIDOf returns the actor's current room id, or empty when roomless
// (tests / pre-spawn).
func roomIDOf(c *Context) world.RoomID {
	if r := c.Actor.Room(); r != nil {
		return r.ID
	}
	return ""
}
