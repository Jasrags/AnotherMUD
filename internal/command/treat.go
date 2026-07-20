package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// treat is the First Aid action (healing-detailed.md — First Aid + Logic):
// use a carried medkit to close a target's wounds. It is the skill consumer
// the First Aid ability had been missing — a First Aid skill check (skills
// §3) whose margin, capped by the actor's rating, decides how much HP is
// restored, and every attempt trains the skill (use-based gain). Unlike the
// stimpatch (a flat-heal `use` consumable that routes through the same
// EntityHealed primitive), treat can patch up an ally: no argument treats
// the actor, otherwise the target resolves in the room (player or mob).

const (
	// skillFirstAid is the ability id of the First Aid skill (SR active
	// skill; content/shadowrun/abilities/first-aid.yaml).
	skillFirstAid = "first-aid"
	// propFirstAidKit flags an item as a medkit — the required tool the
	// treat action consumes a charge from. Freeform item property.
	propFirstAidKit = "first_aid_kit"
	// propMedkitRating is the medkit's SR5 Rating (1-6): it aids the First
	// Aid check and lifts the heal cap. Absent/low ⇒ a minimal rating-1 kit.
	propMedkitRating = "rating"
	// treatDC is the base First Aid threshold the d20 check is rolled
	// against (skills §3). Modest — a wounded runner should usually stabilize.
	treatDC = 10
	// treatBaseHeal is the HP restored on a bare success, before the
	// margin bonus; the floor an untrained defaulter's heal cannot fall below.
	treatBaseHeal = 4
)

// TreatHandler implements `treat [<target>]`.
func TreatHandler(ctx context.Context, c *Context) error {
	if c.Proficiency == nil || c.SkillRoller == nil {
		return c.Actor.Write(ctx, "You can't do that right now.")
	}

	// A medkit is the required tool (healing-detailed.md: "must have a
	// medkit"). Prefer one with supplies; fall back to an empty kit so the
	// out-of-supplies message beats the no-kit one.
	kit, ok := findMedkit(c)
	if !ok {
		return c.Actor.Write(ctx, "You need a medkit to treat wounds.")
	}
	charges := intProp(kit, economy.PropCharges)
	if charges <= 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is out of supplies.", capitalize(kit.Name())))
	}

	// Resolve the patient: no argument treats the actor; otherwise the same
	// room scope set/restore use.
	var (
		target   any
		name, id string
	)
	token := strings.TrimSpace(strings.Join(c.Args, " "))
	if token == "" {
		target, name, id = c.Actor, "yourself", c.Actor.PlayerID()
	} else {
		var ok bool
		target, name, id, ok = resolveSetTarget(c, token)
		if !ok {
			return c.Actor.Write(ctx, "You don't see them here.")
		}
	}
	cb, isCombatant := target.(combat.Combatant)
	if !isCombatant {
		return c.Actor.Write(ctx, "That target has no wounds you can treat.")
	}
	cur, maxHP := cb.Vitals().Snapshot()
	if cur >= maxHP {
		if token == "" {
			return c.Actor.Write(ctx, "You're already in good shape.")
		}
		return c.Actor.Write(ctx, fmt.Sprintf("%s is already in good shape.", capitalize(name)))
	}

	// First Aid skill check (skills §3), mirroring the pick verb: proficiency
	// + governing-stat bonus vs the DC, with defaulting for the untrained.
	prof, trained := c.Proficiency.Proficiency(c.Actor.PlayerID(), skillFirstAid)
	var skillDef *progression.Ability
	if c.Abilities != nil {
		if ab, ok := c.Abilities.Get(skillFirstAid); ok {
			skillDef = ab
		}
	}
	allowed, defaultPenalty := progression.SkillDefaulting(skillDef, trained)
	if !allowed {
		return c.Actor.Write(ctx, "You don't know the first thing about first aid.")
	}
	gov := progression.StatType("logic") // First Aid keys off Logic (its gain_stat)
	if skillDef != nil && skillDef.GainStat != "" {
		gov = skillDef.GainStat
	}
	sv, _ := c.Actor.(statValuer)
	statScore := 10
	if sv != nil {
		statScore = sv.StatValue(gov)
	}
	// The medkit's rating (SR5 Rating 1-6) aids the check like a dice-pool
	// bonus — a better kit both raises the success odds and, via a bigger
	// margin + a lifted cap below, heals more. It also lets an untrained
	// runner get real value from good gear (SR: use the device rating).
	rating := medkitRating(kit)
	cfg := progression.DefaultSkillConfig()
	bonus := progression.SkillBonus(prof, statScore, cfg) - defaultPenalty + rating
	outcome := progression.ResolveSkillCheck(c.SkillRoller, bonus, treatDC)

	// The skill improves on every attempt (reduced on a miss), and a charge
	// of supplies is spent whether or not the treatment took — the field
	// dressing is used up either way, which also blocks free-retry farming.
	var stats progression.StatReader
	if sv != nil {
		stats = actorStatReader{sv}
	}
	c.Proficiency.RollUseGain(c.Actor.PlayerID(), skillFirstAid, outcome.Success, c.SkillRoller, stats)
	kit.SetProperty(economy.PropCharges, charges-1)

	if !outcome.Success {
		return c.Actor.Write(ctx, "Your hands fumble the field dressing; the wound doesn't take.")
	}

	// Heal magnitude: the base plus the check margin, capped by the actor's
	// First Aid rating (SR5 caps healable at skill rating) — an untrained
	// defaulter barely helps, a trained medic closes real damage. Vitals.Heal
	// clamps to the target's missing HP.
	margin := max(outcome.Total-treatDC, 0)
	healCap := treatBaseHeal + progression.ProficiencyBonus(prof, cfg) + rating
	amount := min(treatBaseHeal+margin, healCap)
	// HealAmount returns the HP ACTUALLY restored (not the new current) with
	// the post-heal snapshot, atomically — so the "+N HP" line and the
	// EntityHealed amount are the true delta.
	healed, newHP, mx := cb.Vitals().HealAmount(amount)

	// Emit the observable heal primitive (economy-survival heal / the
	// EntityHealed event). The composition-root subscriber notifies the
	// patient when it is a different player; self-heals it skips.
	if c.Bus != nil && healed > 0 {
		c.Bus.Publish(ctx, eventbus.EntityHealed{
			TargetID:   entities.EntityID(id),
			SourceID:   entities.EntityID(c.Actor.PlayerID()),
			TargetName: cb.Name(),
			SourceName: c.Actor.Name(),
			Amount:     healed,
			NewHP:      newHP,
			MaxHP:      mx,
			Source:     skillFirstAid,
		})
	}

	if token == "" {
		return c.Actor.Write(ctx, fmt.Sprintf("You patch yourself up. (+%d HP)", healed))
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You work the trauma sealant into %s's wounds. (+%d HP)", name, healed))
}

// findMedkit returns the actor's medkit, preferring one that still has
// supplies over an empty one (so the caller can distinguish "no kit" from
// "out of supplies"). Scans top-level inventory only.
func findMedkit(c *Context) (*entities.ItemInstance, bool) {
	var empty *entities.ItemInstance
	for _, it := range collectItems(c.Items, c.Actor.Inventory()) {
		if !isFirstAidKit(it) {
			continue
		}
		if intProp(it, economy.PropCharges) > 0 {
			return it, true
		}
		if empty == nil {
			empty = it
		}
	}
	if empty != nil {
		return empty, true
	}
	return nil, false
}

// isFirstAidKit reports whether the item is flagged as a medkit.
func isFirstAidKit(it *entities.ItemInstance) bool {
	v, ok := it.Property(propFirstAidKit)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// medkitRating reads the kit's SR5 Rating, flooring at 1 so any medkit — even
// one authored without a rating — is a usable rating-1 kit rather than a
// bonus-less tool.
func medkitRating(it *entities.ItemInstance) int {
	if r := intProp(it, propMedkitRating); r > 1 {
		return r
	}
	return 1
}
