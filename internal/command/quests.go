package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/render"
)

// AcceptHandler implements `accept <quest>` (quests.md §3). It resolves
// the term to a quest id, accepts it for the actor, and surfaces the
// outcome — the acceptance banner on success, or a reason otherwise.
func AcceptHandler(ctx context.Context, c *Context) error {
	if c.Quests == nil {
		return c.Actor.Write(ctx, "Quests are not available right now.")
	}
	term := strings.TrimSpace(strings.Join(c.Args, " "))
	if term == "" {
		return c.Actor.Write(ctx, "Accept which quest?")
	}
	questID, ok := c.Quests.ResolveID(term)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("No quest matches %q.", term))
	}
	player, ok := c.Actor.(quest.Player)
	if !ok {
		return c.Actor.Write(ctx, "You can't accept quests.")
	}
	res := c.Quests.Accept(player, questID, false)
	switch res.Status {
	case quest.Accepted:
		if res.Banner != "" {
			return c.Actor.Write(ctx, res.Banner)
		}
		return c.Actor.Write(ctx, "<good>Quest accepted.</good>")
	case quest.AlreadyActive:
		return c.Actor.Write(ctx, "You're already on that quest.")
	case quest.AlreadyCompleted:
		return c.Actor.Write(ctx, "You've already completed that quest.")
	case quest.PrereqNotMet:
		return c.Actor.Write(ctx, "You don't meet the requirements for that quest.")
	case quest.CapReached:
		return c.Actor.Write(ctx, "You have too many active quests. Abandon one first.")
	default:
		return c.Actor.Write(ctx, "You can't accept that quest.")
	}
}

// AbandonHandler implements `abandon <quest>` (quests.md §4.5). It checks
// the actor is actually on an abandonable quest before dropping it so the
// player gets precise feedback (the service abandon is silent).
func AbandonHandler(ctx context.Context, c *Context) error {
	if c.Quests == nil {
		return c.Actor.Write(ctx, "Quests are not available right now.")
	}
	term := strings.TrimSpace(strings.Join(c.Args, " "))
	if term == "" {
		return c.Actor.Write(ctx, "Abandon which quest?")
	}
	questID, ok := c.Quests.ResolveID(term)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("No quest matches %q.", term))
	}
	pid := c.Actor.PlayerID()
	snap := c.Quests.Snapshot(pid)
	if snap == nil || !activeHas(snap, questID) {
		return c.Actor.Write(ctx, "You're not on that quest.")
	}
	def, _ := c.Quests.Definition(questID)
	if def != nil && !def.Abandonable {
		return c.Actor.Write(ctx, "That quest can't be abandoned.")
	}
	c.Quests.Abandon(pid, questID)
	return c.Actor.Write(ctx, fmt.Sprintf("<warning>You abandon %s.</warning>", questName(def, questID)))
}

// QuestsHandler implements `quests` — the journal (quests.md §1; the
// command surface, not the service). It renders the actor's active quests
// with current-stage objective progress through the panel + color
// primitives.
func QuestsHandler(ctx context.Context, c *Context) error {
	if c.Quests == nil {
		return c.Actor.Write(ctx, "Quests are not available right now.")
	}
	snap := c.Quests.Snapshot(c.Actor.PlayerID())
	if snap == nil || len(snap.Active) == 0 {
		return c.Actor.Write(ctx, "<subtle>You have no active quests.</subtle>")
	}

	panel := render.Panel{Width: 64, Sections: []render.Section{{
		Rows: []render.Row{render.TitleRow("Quest Journal", fmt.Sprintf("%d active", len(snap.Active)))},
	}}}
	for i := range snap.Active {
		active := &snap.Active[i]
		def, _ := c.Quests.Definition(active.QuestID)
		panel.Sections = append(panel.Sections, questSection(active, def))
	}
	out, err := panel.Render()
	if err != nil {
		return c.Actor.Write(ctx, "Your quest journal could not be rendered.")
	}
	return c.Actor.Write(ctx, out)
}

// questSection builds the journal section for one active quest.
func questSection(active *quest.ActiveQuest, def *quest.Definition) render.Section {
	rows := []render.Row{render.TitleRow(questName(def, active.QuestID), classification(def))}
	if def != nil && active.StageIndex < len(def.Stages) {
		stage := def.Stages[active.StageIndex]
		if stage.Description != "" {
			rows = append(rows, render.TextRow("<subtle>"+stage.Description+"</subtle>", render.AlignLeft, true))
		}
		for j := range active.Objectives {
			op := active.Objectives[j]
			desc := objectiveDesc(stage, j, op)
			mark := " "
			if op.Complete() {
				mark = "x"
			}
			rows = append(rows, render.TextRow(
				fmt.Sprintf("  [%s] %s (%d/%d)", mark, desc, op.Current, op.Required),
				render.AlignLeft, true))
		}
	}
	return render.Section{SeparatorAbove: render.RuleMinor, Rows: rows}
}

func objectiveDesc(stage quest.Stage, idx int, op quest.ObjectiveProgress) string {
	if idx < len(stage.Objectives) {
		if d := stage.Objectives[idx].Description; d != "" {
			return d
		}
		if t := stage.Objectives[idx].Type; t != "" {
			return t
		}
	}
	return op.ObjectiveID
}

func questName(def *quest.Definition, questID string) string {
	if def != nil && def.Name != "" {
		return def.Name
	}
	return questID
}

func classification(def *quest.Definition) string {
	if def != nil {
		return def.Classification
	}
	return ""
}

func activeHas(s *quest.State, questID string) bool {
	for i := range s.Active {
		if s.Active[i].QuestID == questID {
			return true
		}
	}
	return false
}
