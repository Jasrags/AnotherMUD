package command

import (
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// propSkillTool is the item property naming the skill an item assists as a
// tool (the skills.md tool seam) — e.g. a lockpick declares
// `skill_tool: open-lock`. Mirrors crafting's craft_tool (the discipline an
// item serves), but for skill checks.
const propSkillTool = "skill_tool"

// propSkillToolBonus is a tool's BASE bonus to the skill it assists, before
// any quality-grade scaling. Default 0.
const propSkillToolBonus = "skill_tool_bonus"

// skillToolBonus returns the best bonus the actor's CARRIED tools grant to the
// named skill check (skills.md tool seam). For each held item that assists the
// skill (its skill_tool == skill), the contribution is the item's base
// skill_tool_bonus PLUS its quality grade's ToolSkill (masterwork §3). The
// result is the single HIGHEST contribution — multiple tools toward one check
// do NOT stack, only the best applies (masterwork §3). 0 when no carried tool
// assists the skill.
func (c *Context) skillToolBonus(skill string) int {
	if c.Items == nil {
		return 0
	}
	best := 0
	for _, id := range c.Actor.Inventory() {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		served, ok := it.Property(propSkillTool)
		if !ok {
			continue
		}
		s, ok := served.(string)
		if !ok || !strings.EqualFold(strings.TrimSpace(s), skill) {
			continue
		}
		bonus := intProp(it, propSkillToolBonus)
		if c.Grades != nil {
			if g, ok := c.Grades.Get(it.Grade()); ok {
				bonus += g.ToolSkill // masterwork §3: a graded tool aids the check more.
			}
		}
		if bonus > best {
			best = bonus
		}
	}
	return best
}
