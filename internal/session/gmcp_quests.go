package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/quest"
)

// flushGmcpQuests snapshots the actor's active quests into the rich Char.Quests
// journal (web-client-plan P3 Slice C) and emits a frame when it differs from
// the last-sent shadow. Rides the same gmcp-items-flush tick pass as the shop
// form (objective progress changes as the player plays) with the same no-op
// guards: non-GMCP conn, GMCP inactive, or no quest service wired.
//
// Because it is rebuilt and byte-diffed every tick, accepting/abandoning a
// quest, advancing an objective, or completing a stage all re-emit. Marshaled-
// bytes shadow (like Char.Shop), guarded by gmcpItemsMu (shared with its
// siblings on the same flush pass).
func (a *connActor) flushGmcpQuests(ctx context.Context, svc *quest.Service) {
	if svc == nil {
		return
	}
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}

	data, err := json.Marshal(a.buildQuestForm(svc))
	if err != nil {
		return
	}

	a.gmcpItemsMu.Lock()
	unchanged := a.gmcpQuestsValid && string(a.gmcpQuestsLast) == string(data)
	if !unchanged {
		a.gmcpQuestsLast = data
		a.gmcpQuestsValid = true
	}
	a.gmcpItemsMu.Unlock()
	if unchanged {
		return
	}

	if err := sender.SendGmcp(ctx, gmcp.PackageCharQuests, data); err != nil {
		logging.From(ctx).Debug("gmcp quests send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// buildQuestForm projects the actor's active quests into the Char.Quests journal
// via the quest service's read-only Snapshot + Definition — the SAME projection
// the `quests` verb renders (command/quests.go), so the panel and the CLI never
// drift. A nil snapshot (a player with no quest state) yields an empty journal.
// The Quests slice is always non-nil (make) so the wire carries `[]`, not
// `null`.
func (a *connActor) buildQuestForm(svc *quest.Service) gmcp.CharQuests {
	snap := svc.Snapshot(a.PlayerID())
	entries := make([]gmcp.QuestEntry, 0)
	if snap == nil {
		return gmcp.CharQuests{Quests: entries}
	}
	for i := range snap.Active {
		active := &snap.Active[i]
		def, _ := svc.Definition(active.QuestID)
		entries = append(entries, questEntryRow(active, def))
	}
	return gmcp.CharQuests{Quests: entries}
}

// questEntryRow converts one active quest into the wire QuestEntry: the display
// name + classification, the current stage's description/hint, the per-objective
// progress rows, and — when the quest is abandonable — the full `abandon <id>`
// command. Mirrors command.questSection's field resolution (name/classification
// fallbacks, stage-index bound check, objective description precedence).
func questEntryRow(active *quest.ActiveQuest, def *quest.Definition) gmcp.QuestEntry {
	entry := gmcp.QuestEntry{
		ID:             active.QuestID,
		Name:           questEntryName(def, active.QuestID),
		Classification: questEntryClassification(def),
		Objectives:     make([]gmcp.QuestObjective, 0, len(active.Objectives)),
		AwaitingTurnIn: active.AwaitingTurnIn,
	}
	// A quest with no resolved definition (orphaned content) counts as
	// abandonable, matching quest.Service.countAbandonableLocked and the
	// Abandon verb's own gate. Only surface the command when it will work.
	if def == nil || def.Abandonable {
		entry.Abandonable = true
		entry.AbandonCmd = "abandon " + active.QuestID
	}

	var stage quest.Stage
	haveStage := def != nil && active.StageIndex < len(def.Stages)
	if haveStage {
		stage = def.Stages[active.StageIndex]
		entry.Stage = stage.Description
		entry.Hint = stage.Hint
	}
	for j := range active.Objectives {
		op := active.Objectives[j]
		entry.Objectives = append(entry.Objectives, gmcp.QuestObjective{
			Desc:     questObjectiveDesc(haveStage, stage, j, op),
			Current:  op.Current,
			Required: op.Required,
			Complete: op.Complete(),
		})
	}
	return entry
}

// questObjectiveDesc resolves the display string for one objective, mirroring
// command.objectiveDesc: the stage objective's description, else its type, else
// the raw objective id (when the stage/index is unavailable).
func questObjectiveDesc(haveStage bool, stage quest.Stage, idx int, op quest.ObjectiveProgress) string {
	if haveStage && idx < len(stage.Objectives) {
		if d := stage.Objectives[idx].Description; d != "" {
			return d
		}
		if t := stage.Objectives[idx].Type; t != "" {
			return t
		}
	}
	return op.ObjectiveID
}

// questEntryName resolves the display name with the same fallback as
// command.questName (definition name, else the quest id).
func questEntryName(def *quest.Definition, questID string) string {
	if def != nil && def.Name != "" {
		return def.Name
	}
	return questID
}

// questEntryClassification resolves the classification tag (empty when the
// definition is missing or unset), mirroring command.classification.
func questEntryClassification(def *quest.Definition) string {
	if def != nil {
		return def.Classification
	}
	return ""
}
