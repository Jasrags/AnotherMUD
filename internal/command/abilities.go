package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// abilityEntityID returns the id under which the ability managers key
// this actor's proficiency / action queue. Mirrors the practice verb:
// the persistent player id when present (production connActor), falling
// back to the per-session id for test actors without a player id.
func abilityEntityID(a Actor) string {
	if pid := a.PlayerID(); pid != "" {
		return pid
	}
	return a.ID()
}

// AbilitiesHandler implements the `abilities` verb (spec
// abilities-and-effects §3.3 "all learned abilities of entity X").
// Lists the actor's learned abilities with current proficiency and
// cap, annotated with the registered classification (skill/spell).
// Abilities that were granted but have no registry entry (declarative
// class-path grants without content) are shown with a "(unlearnable)"
// note so the gap is visible rather than silently dropped.
func AbilitiesHandler(ctx context.Context, c *Context) error {
	if c.Proficiency == nil {
		return c.Actor.Write(ctx, "Abilities are not enabled in this build.")
	}
	entityID := abilityEntityID(c.Actor)
	learned := c.Proficiency.LearnedAbilities(entityID)
	if len(learned) == 0 {
		return c.Actor.Write(ctx, "You haven't learned any abilities yet.")
	}

	// Pre-format each row so the name column can be padded to the
	// widest display name for stable alignment.
	type row struct {
		name string
		kind string
		prof int
		cap  int
	}
	rows := make([]row, 0, len(learned))
	widest := 0
	for _, e := range learned {
		name := e.ID
		kind := "unlearnable"
		if c.Abilities != nil {
			if ab, ok := c.Abilities.Get(e.ID); ok {
				if ab.DisplayName != "" {
					name = ab.DisplayName
				}
				kind = string(ab.Category)
			}
		}
		capValue, _, _ := c.Proficiency.GetCap(entityID, e.ID)
		rows = append(rows, row{name: name, kind: kind, prof: e.Value, cap: capValue})
		if len(name) > widest {
			widest = len(name)
		}
	}
	// Stable display order: by name, case-insensitive.
	sort.SliceStable(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name)
	})

	var b strings.Builder
	b.WriteString("You know the following abilities:\n")
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("  %-*s  (%-11s)  %d/%d\n",
			widest, r.name, r.kind, r.prof, r.cap))
	}
	return c.Actor.Write(ctx, strings.TrimRight(b.String(), "\n"))
}

// SkillsHandler implements the `skills` verb (skills §5) — the `abilities`
// view filtered to the `skill` category, so a player can see their skills
// (lockpicking, crafting disciplines, …) with proficiency / cap without the
// spells and combat abilities. Self-only.
func SkillsHandler(ctx context.Context, c *Context) error {
	if c.Proficiency == nil || c.Abilities == nil {
		return c.Actor.Write(ctx, "Skills are not enabled in this build.")
	}
	entityID := abilityEntityID(c.Actor)
	type row struct {
		name, group, category, attr string
		prof, capV                  int
	}
	var rows []row
	grouped := false // any skill declaring category/group flips this (skills §5)
	widest := 0
	for _, e := range c.Proficiency.LearnedAbilities(entityID) {
		ab, ok := c.Abilities.Get(e.ID)
		if !ok || ab.Category != progression.AbilitySkill {
			continue // skills only
		}
		name := ab.DisplayName
		if name == "" {
			name = e.ID
		}
		capValue, _, _ := c.Proficiency.GetCap(entityID, e.ID)
		// The linked attribute serves as the display tag; falls back to the gain
		// stat (the baseline where check-stat == gain-stat).
		attr := string(ab.LinkedAttribute)
		if attr == "" {
			attr = string(ab.GainStat)
		}
		rows = append(rows, row{
			name: name, group: ab.SkillGroup, category: ab.SkillCategory,
			attr: attr, prof: e.Value, capV: capValue,
		})
		if ab.SkillCategory != "" || ab.SkillGroup != "" {
			grouped = true
		}
		if len(name) > widest {
			widest = len(name)
		}
	}
	if len(rows) == 0 {
		return c.Actor.Write(ctx, "You haven't learned any skills yet.")
	}

	// Flat list — byte-identical to the pre-metadata baseline for any world that
	// declares no skill category/group (skills §5).
	if !grouped {
		sort.SliceStable(rows, func(i, j int) bool {
			return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name)
		})
		var b strings.Builder
		b.WriteString("Your skills:\n")
		for _, r := range rows {
			b.WriteString(fmt.Sprintf("  %-*s  %d/%d\n", widest, r.name, r.prof, r.capV))
		}
		return c.Actor.Write(ctx, strings.TrimRight(b.String(), "\n"))
	}

	// Grouped by category then group then name (skills §2.1/§5); an uncategorized
	// or ungrouped skill sorts last within its tier.
	sort.SliceStable(rows, func(i, j int) bool {
		if ci, cj := skillSortKey(rows[i].category), skillSortKey(rows[j].category); ci != cj {
			return ci < cj
		}
		if gi, gj := skillSortKey(rows[i].group), skillSortKey(rows[j].group); gi != gj {
			return gi < gj
		}
		return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name)
	})
	var b strings.Builder
	b.WriteString("Your skills:\n")
	// Track the header by its NORMALIZED key (the same key used to sort), not the
	// raw field, so two files authoring the same category with different casing
	// ("combat" vs "Combat") don't print a duplicate header at their boundary.
	curCat, curGroup, first := "", "", true
	for _, r := range rows {
		catKey, groupKey := skillSortKey(r.category), skillSortKey(r.group)
		if first || catKey != curCat || groupKey != curGroup {
			if !first {
				b.WriteString("\n")
			}
			b.WriteString(skillHeader(r.category, r.group) + "\n")
			curCat, curGroup, first = catKey, groupKey, false
		}
		tag := ""
		if r.attr != "" {
			tag = "  (" + attrAbbrev(r.attr) + ")"
		}
		b.WriteString(fmt.Sprintf("  %-*s  %d/%d%s\n", widest, r.name, r.prof, r.capV, tag))
	}
	return c.Actor.Write(ctx, strings.TrimRight(b.String(), "\n"))
}

// skillSortKey lowercases a category/group name for stable ordering and sorts
// empty (uncategorized/ungrouped) names last.
func skillSortKey(s string) string {
	if s == "" {
		return "\xff"
	}
	return strings.ToLower(s)
}

// skillHeader renders a "Category — Group" listing header (title-cased);
// an empty category reads "Other", an empty group drops the " — Group" tail.
func skillHeader(category, group string) string {
	cat := titleCaseWords(category)
	if cat == "" {
		cat = "Other"
	}
	if group == "" {
		return cat
	}
	return cat + " — " + titleCaseWords(group)
}

// attrAbbrev renders a linked-attribute tag as its first three letters uppercased
// (the SR 3-letter convention — agility→AGI, logic→LOG; dex→DEX unchanged).
func attrAbbrev(attr string) string {
	a := strings.ToUpper(strings.TrimSpace(attr))
	if len(a) > 3 {
		return a[:3]
	}
	return a
}

// titleCaseWords capitalizes each space-separated word ("close combat" →
// "Close Combat"), so a lowercase-authored category/group still reads well.
func titleCaseWords(s string) string {
	fields := strings.Fields(s)
	for i, w := range fields {
		fields[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(fields, " ")
}

// CastHandler implements the generic `cast <ability> [target]` verb.
// It resolves the named ability against the registry, optionally
// resolves an explicit target in the actor's room, and pushes a
// QueuedAction onto the action queue. The combat ability phase drains
// the queue on the next pulse (spec §4.2); abilities therefore resolve
// during combat only — a queued action sits until the entity is in a
// combat round. This is the M9.6 "combat-only resolution" model.
//
// `cast` is the discoverable spell verb; skill-named verbs (kick,
// bless, …) route to the same enqueue path via AbilityVerb.
// steddingTag marks a room as lying within a stedding's bound, where the One
// Power is still and a channeler cannot weave (other-worlds.md §Stedding).
// Authored as a room `tags:` entry; the cast gate reads it. WoT-only content.
const steddingTag = "stedding"

func CastHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Cast what?")
	}
	abilityArg := c.Args[0]
	target := strings.Join(c.Args[1:], " ")
	return enqueueAbility(ctx, c, abilityArg, target, false)
}

// OverchannelHandler implements `overchannel <weave> [target]` (WoT S2) —
// the deliberate draw past a channeler's safe reserve. It enqueues exactly
// like `channel`/`cast` but sets the Overchannel flag, so a weave the caster
// cannot safely afford resolves anyway (instead of fizzling) at the cost of
// a Fortitude save + condition cascade exacted by the host after it lands.
func OverchannelHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Overchannel what?")
	}
	abilityArg := c.Args[0]
	target := strings.Join(c.Args[1:], " ")
	return enqueueAbility(ctx, c, abilityArg, target, true)
}

// AbilityVerb returns a Handler that enqueues a fixed ability id,
// treating all args as the (optional) target. Registered once per
// active ability at boot so a player can type the ability's own id as
// a verb ("kick goblin", "bless"). The closure captures abilityID;
// the registry lookup still happens at enqueue time so a hot-reloaded
// registry (future) is honored.
func AbilityVerb(abilityID string) Handler {
	id := strings.ToLower(strings.TrimSpace(abilityID))
	return func(ctx context.Context, c *Context) error {
		return enqueueAbility(ctx, c, id, strings.Join(c.Args, " "), false)
	}
}

// enqueueAbility is the shared body for cast + skill-named verbs.
// abilityArg is the ability id (or, for cast, the player-typed name);
// targetArg is the optional target name (may have a leading "on"/"at").
//
// Refusal paths and their messages:
//   - ability system unwired      → "You can't use abilities right now."
//   - unknown ability             → "You don't know how to do that."
//   - target named but not found  → "You don't see them here."
//   - queue at capacity           → "You can't prepare any more actions right now."
//
// On success the actor sees a "you prepare …" confirmation. Hit/miss/
// fizzle feedback arrives later from the ability.* event renderer when
// the combat phase resolves the entry.
func enqueueAbility(ctx context.Context, c *Context, abilityArg, targetArg string, overchannel bool) error {
	if c.ActionQueue == nil || c.Abilities == nil {
		return c.Actor.Write(ctx, "You can't use abilities right now.")
	}
	ability, ok := c.Abilities.Get(abilityArg)
	if !ok {
		return c.Actor.Write(ctx, "You don't know how to do that.")
	}

	// Stedding still the One Power (other-worlds.md §Stedding): a channeler
	// cannot weave within a stedding's bound. Gate any One-Power weave
	// (AbilitySpell — the channeling category) cast from a stedding-tagged room;
	// mundane abilities (skills) are unaffected. Covers cast/channel,
	// overchannel, and ability-named verbs (all route through here). Stedding are
	// WoT-only content, so a "spell" there is always a weave.
	if ability.Category == progression.AbilitySpell {
		if room := c.Actor.Room(); room != nil && hasTag(room.Tags, steddingTag) {
			return c.Actor.Write(ctx, "Within the stedding the True Source lies beyond your reach. You cannot channel here.")
		}
	}

	targetArg = stripTargetPreposition(targetArg)
	var targetID string
	var targetName string
	if targetArg != "" {
		room := c.Actor.Room()
		if room == nil {
			return c.Actor.Write(ctx, "You don't see them here.")
		}
		cb, name, found := findCombatantInRoom(c, room.ID, targetArg)
		if !found {
			return c.Actor.Write(ctx, "You don't see them here.")
		}
		targetID = combat.EntityIDOf(cb.CombatantID())
		targetName = name
	}

	if !c.ActionQueue.Push(abilityEntityID(c.Actor), progression.QueuedAction{
		AbilityID:      ability.ID,
		TargetEntityID: targetID,
		Overchannel:    overchannel,
	}) {
		return c.Actor.Write(ctx, "You can't prepare any more actions right now.")
	}

	verb := "use"
	if overchannel {
		verb = "overchannel" // "You reach for more of the Power than you should…"
	}
	if targetName != "" {
		return c.Actor.Write(ctx,
			fmt.Sprintf("You prepare to %s %s on %s.", verb, ability.DisplayName, targetName))
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You prepare to %s %s.", verb, ability.DisplayName))
}

// stripTargetPreposition drops a single leading "on"/"at" token so
// "cast bless on alice" and "kick at goblin" resolve the trailing name.
// A bare "on"/"at" with nothing after collapses to empty (self / no
// explicit target).
func stripTargetPreposition(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	fields := strings.Fields(s)
	switch strings.ToLower(fields[0]) {
	case "on", "at":
		return strings.TrimSpace(strings.Join(fields[1:], " "))
	}
	return s
}
