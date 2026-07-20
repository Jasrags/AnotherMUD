package session

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/wizard"
)

// Creation benefit summaries — the "what does this choice grant" enrichment
// for the character-creation wizard (role×origin creation). Two consumers:
//
//   - The race/class/background option builders fold the per-choice benefit
//     into each option's Description (surfaced via the wizard's `? <n>` inspect
//     affordance), so a player can weigh what a metatype/role/origin gives
//     before picking it.
//   - summaryStep renders a review sheet of every choice — with its benefits —
//     immediately before the confirm prompt (§ review step).
//
// All logic reads only engine-generic progression fields (StatBonuses, save
// progressions, path grants, background skills/items/gold), so it is
// pack-agnostic: the WoT and shadowrun flows get it for free, and the wizard
// engine stays free of setting knowledge.

// withBenefit appends a benefit line to an inspect Description, separated by a
// blank line. Either half may be empty.
func withBenefit(desc, benefit string) string {
	switch {
	case benefit == "":
		return desc
	case desc == "":
		return benefit
	default:
		return desc + "\n\n" + benefit
	}
}

// raceBenefit summarizes a metatype's mechanical grants: its starting attribute
// skew, vision, and size. A metatype with no skew (the human baseline) reads
// "no attribute skew" rather than an empty line.
func raceBenefit(r *progression.Race) string {
	if r == nil {
		return ""
	}
	parts := []string{}
	if skew := statBonusList(r.StatBonuses); skew != "" {
		parts = append(parts, skew)
	} else {
		parts = append(parts, "no attribute skew")
	}
	parts = append(parts, "vision: "+visionLabel(r.RacialFlags))
	parts = append(parts, "size: "+sizeLabel(r.Size))
	return strings.Join(parts, " · ")
}

// classBenefit summarizes a role's mechanical grants: weapon + armour
// proficiency tiers and its strong saving throw. The granted starting skills
// are shown separately on the review sheet (they can be a long list), so this
// short form stays menu-friendly.
func classBenefit(c *progression.Class) string {
	if c == nil {
		return ""
	}
	parts := []string{}
	if len(c.ProficiencyTiers) > 0 {
		parts = append(parts, "weapons: "+strings.Join(c.ProficiencyTiers, "+"))
	}
	if len(c.ArmorProficiencyTiers) > 0 {
		parts = append(parts, "armor: "+strings.Join(c.ArmorProficiencyTiers, "+"))
	}
	if s := strongSave(c.SaveProgressions); s != "" {
		parts = append(parts, "strong "+s+" save")
	}
	return strings.Join(parts, " · ")
}

// backgroundBenefit summarizes an origin's grants: its life skills, a feat pick
// (when it offers a choice), starting funds, and gear. Gear ids are rendered
// namespace-stripped via packageLabel (no item-template registry needed here).
func backgroundBenefit(b *progression.Background) string {
	if b == nil {
		return ""
	}
	parts := []string{}
	if sk := bgSkillList(b.Skills); sk != "" {
		parts = append(parts, "skills: "+sk)
	}
	if len(b.FeatOptions) >= 2 {
		parts = append(parts, "talent: pick one of "+strings.Join(b.FeatOptions, " / "))
	} else if len(b.FeatOptions) == 1 {
		// Exactly one option is auto-granted by the granter (no pick step) — show
		// it so the guaranteed talent is still visible when weighing the origin.
		parts = append(parts, "talent: "+b.FeatOptions[0])
	}
	if b.Gold > 0 {
		parts = append(parts, fmt.Sprintf("funds: %d", b.Gold))
	}
	if len(b.Items) > 0 {
		parts = append(parts, "gear: "+packageLabel(b.Items))
	}
	return strings.Join(parts, "\n")
}

// summaryStep is the review sheet shown immediately before the confirm prompt:
// a recap of gender/metatype/role/origin/talent plus the merged skills, funds,
// and full starting gear, with each pick's mechanical benefit folded in. It is
// a non-interactive InfoStep whose TextFn reads the in-progress entity, so it
// always reflects the choices actually made (including skipped steps, whose
// fields stay empty and are omitted).
func summaryStep(races *progression.RaceRegistry, classes *progression.ClassRegistry, backgrounds *progression.BackgroundRegistry, feats *feat.Registry) *wizard.InfoStep {
	return &wizard.InfoStep{
		ID: "summary",
		TextFn: func(e wizard.Entity) string {
			// Safe: every flow built here runs against a *creationEntity (runCreation
			// constructs it before Start), the same assumption the ChoiceStep
			// OptionsFn/OnSelect closures make throughout creation_flow.go.
			return renderCreationSummary(e.(*creationEntity), races, classes, backgrounds, feats)
		},
	}
}

// renderCreationSummary builds the aligned review sheet from the entity's
// chosen ids, resolving each against its registry. Empty rows are omitted so a
// minimal flow (gender only) still reads cleanly.
func renderCreationSummary(ce *creationEntity, races *progression.RaceRegistry, classes *progression.ClassRegistry, backgrounds *progression.BackgroundRegistry, feats *feat.Registry) string {
	r := lookupRace(races, ce.raceID)
	c := lookupClass(classes, ce.classID)
	b := lookupBackground(backgrounds, ce.backgroundID)

	var sb strings.Builder
	sb.WriteString("\n─── Review your character ───\n")
	row := func(label, val string) {
		if val == "" {
			return
		}
		sb.WriteString(fmt.Sprintf("  %-9s %s\n", label, val))
	}

	row("Gender", titleWord(ce.gender))
	if r != nil {
		row("Metatype", displayOr(r.DisplayName, r.ID))
		if bn := raceBenefit(r); bn != "" {
			row("", bn)
		}
	}
	if c != nil {
		row("Role", displayOr(c.DisplayName, c.ID))
		if bn := classBenefit(c); bn != "" {
			row("", bn)
		}
	}
	if b != nil {
		row("Origin", displayOr(b.DisplayName, b.ID))
	}
	if talent := featDisplay(feats, resolvedFeatID(ce, b)); talent != "" {
		row("Talent", talent)
	}
	if skills := creationSkills(c, b); len(skills) > 0 {
		row("Skills", strings.Join(skills, ", "))
	}
	if b != nil && b.Gold > 0 {
		row("Funds", fmt.Sprintf("%d", b.Gold))
	}
	if gear := creationGear(ce, c, b); len(gear) > 0 {
		row("Gear", packageLabel(gear))
	}
	sb.WriteString("─────────────────────────────\n")
	return sb.String()
}

// creationSkills is the deduplicated union of the role's level-1 granted
// abilities and the origin's life skills — the character's actual starting
// skill sheet. Origin skills merge over the class floor rather than stacking
// (see the role×origin creation build log), so a name shared by both appears
// once. Order is class-first, then origin, preserving discovery order.
func creationSkills(c *progression.Class, b *progression.Background) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	if c != nil {
		for _, p := range c.Path {
			if p.Level <= 1 {
				add(p.AbilityID)
			}
		}
	}
	if b != nil {
		for _, s := range b.Skills {
			add(s.AbilityID)
		}
	}
	return out
}

// creationGear is the full starting kit the character actually receives: the
// role's floor item(s), the origin's always-granted items, and the chosen
// equipment package (when the origin offers packages). Returned as raw item
// ids; callers render via packageLabel.
func creationGear(ce *creationEntity, c *progression.Class, b *progression.Background) []string {
	var items []string
	if c != nil {
		items = append(items, c.StartingItems...)
	}
	if b != nil {
		items = append(items, b.Items...)
		if n := len(b.EquipmentPackages); n > 0 {
			idx := ce.backgroundEquipment
			if idx < 0 || idx >= n {
				idx = 0 // the granter's default when no choice was made
			}
			items = append(items, b.EquipmentPackages[idx]...)
		}
	}
	return items
}

// --- small resolvers + label helpers -----------------------------------

func lookupRace(reg *progression.RaceRegistry, id string) *progression.Race {
	if reg == nil || id == "" {
		return nil
	}
	r, _ := reg.Get(id)
	return r
}

func lookupClass(reg *progression.ClassRegistry, id string) *progression.Class {
	if reg == nil || id == "" {
		return nil
	}
	c, _ := reg.Get(id)
	return c
}

func lookupBackground(reg *progression.BackgroundRegistry, id string) *progression.Background {
	if reg == nil || id == "" {
		return nil
	}
	b, _ := reg.Get(id)
	return b
}

// resolvedFeatID is the talent the character actually receives: the explicit
// pick when the feat step ran (the origin offered ≥2 options), else the origin's
// single guaranteed feat (auto-granted by the granter when it offers exactly
// one, so no pick step runs and ce.backgroundFeat stays empty). Empty when the
// origin grants no feat. Mirrors creationGear's package-0 fallback.
func resolvedFeatID(ce *creationEntity, b *progression.Background) string {
	if ce.backgroundFeat != "" {
		return ce.backgroundFeat
	}
	if b != nil && len(b.FeatOptions) == 1 {
		return b.FeatOptions[0]
	}
	return ""
}

func featDisplay(reg *feat.Registry, id string) string {
	if id == "" {
		return ""
	}
	if reg != nil {
		if f, ok := reg.Get(id); ok {
			return displayOr(f.DisplayName, f.ID)
		}
	}
	return id
}

// statBonusList renders a stat-bonus map as "+N Label, +N Label" in a
// deterministic (stat-key-sorted) order. Empty map returns "".
func statBonusList(m map[progression.StatType]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]progression.StatType, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := m[k]
		sign := "+"
		if v < 0 {
			sign = ""
		}
		parts = append(parts, fmt.Sprintf("%s%d %s", sign, v, statBonusLabel(k)))
	}
	return strings.Join(parts, ", ")
}

// statBonusLabel renders a StatType for a benefit line. The SR/WoT attribute keys
// are clean single tokens, so a local title-case keeps this self-contained (no
// StatDisplayNames registry threaded through the flow); hp_max is special-cased
// to "HP" (the flat Physical-monitor bump metatypes grant).
func statBonusLabel(s progression.StatType) string {
	if s == progression.StatHPMax {
		return "HP"
	}
	return titleWord(string(s))
}

// visionLabel names a metatype's vision from its racial flags, defaulting to
// "normal" when it carries neither. Preference-ordered (thermographic is the
// stronger sense) rather than authoring-order-dependent, so a metatype carrying
// both flags reads consistently.
func visionLabel(flags []string) string {
	has := func(want string) bool {
		for _, f := range flags {
			if f == want {
				return true
			}
		}
		return false
	}
	switch {
	case has("thermographic"):
		return "thermographic"
	case has("low-light"):
		return "low-light"
	default:
		return "normal"
	}
}

func sizeLabel(size string) string {
	if strings.TrimSpace(size) == "" {
		return "medium"
	}
	return size
}

// strongSave returns the title-cased name of a class's strong saving throw
// (deterministic across ties by sorting the save-type keys), or "" if none.
func strongSave(m map[progression.SaveType]progression.SaveProgression) string {
	keys := make([]progression.SaveType, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		if m[k] == progression.SaveStrong {
			return titleWord(string(k))
		}
	}
	return ""
}

func bgSkillList(skills []progression.BackgroundSkill) string {
	if len(skills) == 0 {
		return ""
	}
	parts := make([]string, 0, len(skills))
	for _, s := range skills {
		parts = append(parts, s.AbilityID)
	}
	return strings.Join(parts, ", ")
}

// titleWord upper-cases the first rune of a single lowercase token
// ("female" → "Female", "will" → "Will"). Empty in, empty out.
func titleWord(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
