package session

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/quest"
)

// questNotifier is the runtime quest.EventSink: it bridges quest
// lifecycle events to the acting player's screen — the M10.10b sink only
// logged, so completions/progress were invisible in-game — while keeping
// the structured server log.
//
// Lock note (load-bearing): the quest service emits these events while
// holding its own lock; resolving the player and writing then takes the
// Manager lock and the connActor write path. That is the SAME
// s.mu → Manager → connActor order the reward dispatcher already
// establishes (quest/service.go completeLocked), so it introduces no new
// lock-order regime. The sink never calls a Service method that takes the
// service lock (registry.Lookup uses the registry's own lock), so there
// is no re-entrancy.
type questNotifier struct {
	mgr       *Manager
	registry  *quest.Registry
	giverName func(templateID string) string // mob template id → display name
	itemName  func(templateID string) string // item template id → display name
	logger    *slog.Logger
}

// NewQuestNotifier builds the runtime quest sink. giverName / itemName
// resolve template ids to display names for turn-in prompts and reward
// lines; either may be nil (falls back to the raw / short id).
func NewQuestNotifier(
	mgr *Manager,
	registry *quest.Registry,
	giverName func(string) string,
	itemName func(string) string,
	logger *slog.Logger,
) quest.EventSink {
	if logger == nil {
		logger = slog.Default()
	}
	return &questNotifier{mgr: mgr, registry: registry, giverName: giverName, itemName: itemName, logger: logger}
}

// write delivers a line to an online player; an offline recipient is a
// silent no-op (their /quests journal reflects the state on return).
func (n *questNotifier) write(playerID, msg string) {
	a, ok := n.mgr.GetByPlayerID(playerID)
	if !ok {
		return
	}
	_ = a.Write(context.Background(), msg)
}

// questName resolves a quest's display name, falling back to its id.
func (n *questNotifier) questName(questID string) string {
	if def, ok := n.registry.Lookup(questID); ok && def.Name != "" {
		return def.Name
	}
	return questID
}

// objectiveDesc finds an objective's description across the quest's
// stages (the event carries only its id), falling back to type or id.
func (n *questNotifier) objectiveDesc(questID, objectiveID string) string {
	def, ok := n.registry.Lookup(questID)
	if !ok {
		return objectiveID
	}
	for _, st := range def.Stages {
		for _, o := range st.Objectives {
			if o.ID == objectiveID {
				switch {
				case o.Description != "":
					return o.Description
				case o.Type != "":
					return o.Type
				}
				return objectiveID
			}
		}
	}
	return objectiveID
}

func (n *questNotifier) Started(e quest.StartedEvent) {
	// No player write here: the accept command renders the acceptance
	// banner synchronously, and auto-grants get their notice from the
	// watcher grant callbacks. Writing again would double-message.
	n.logger.Info("quest started", slog.String("event", "quest.started"),
		slog.String("entity_id", e.PlayerID), slog.String("quest", e.QuestID))
}

func (n *questNotifier) ObjectiveAdvanced(e quest.ObjectiveAdvancedEvent) {
	n.logger.Debug("quest objective advanced", slog.String("event", "quest.objective_advanced"),
		slog.String("entity_id", e.PlayerID), slog.String("quest", e.QuestID),
		slog.String("objective", e.ObjectiveID), slog.Int("current", e.Current), slog.Int("required", e.Required))
	n.write(e.PlayerID, fmt.Sprintf("<subtle>%s:</subtle> %s (%d/%d)",
		n.questName(e.QuestID), n.objectiveDesc(e.QuestID, e.ObjectiveID), e.Current, e.Required))
}

func (n *questNotifier) StageAdvanced(e quest.StageAdvancedEvent) {
	n.logger.Debug("quest stage advanced", slog.String("event", "quest.stage_advanced"),
		slog.String("entity_id", e.PlayerID), slog.String("quest", e.QuestID), slog.Int("stage", e.StageIndex))
	line := n.questName(e.QuestID) + ": new objectives."
	if def, ok := n.registry.Lookup(e.QuestID); ok && e.StageIndex < len(def.Stages) {
		st := def.Stages[e.StageIndex]
		switch {
		case st.Description != "":
			line = n.questName(e.QuestID) + ": " + st.Description
		case st.Hint != "":
			line = n.questName(e.QuestID) + ": " + st.Hint
		}
	}
	n.write(e.PlayerID, "<subtle>"+line+"</subtle>")
}

func (n *questNotifier) ReadyToTurnIn(e quest.ReadyToTurnInEvent) {
	n.logger.Info("quest ready to turn in", slog.String("event", "quest.ready_to_turn_in"),
		slog.String("entity_id", e.PlayerID), slog.String("quest", e.QuestID), slog.String("giver", e.Giver))
	giver := e.Giver
	if n.giverName != nil {
		if nm := n.giverName(e.Giver); nm != "" {
			giver = nm
		}
	}
	n.write(e.PlayerID, fmt.Sprintf("<good>%s complete!</good> Return to %s to claim your reward.",
		n.questName(e.QuestID), giver))
}

func (n *questNotifier) Completed(e quest.CompletedEvent) {
	n.logger.Info("quest completed", slog.String("event", "quest.completed"),
		slog.String("entity_id", e.PlayerID), slog.String("quest", e.QuestID),
		slog.Int64("xp", e.XP), slog.Int("gold", e.Gold))
	n.write(e.PlayerID, n.completionBanner(e))
}

func (n *questNotifier) Abandoned(e quest.AbandonedEvent) {
	// The abandon command writes its own confirmation; just log here.
	n.logger.Info("quest abandoned", slog.String("event", "quest.abandoned"),
		slog.String("entity_id", e.PlayerID), slog.String("quest", e.QuestID))
}

// completionBanner renders the player-visible "quest complete" block with
// the rewards granted (from the event payload).
func (n *questNotifier) completionBanner(e quest.CompletedEvent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<title>Quest complete: %s</title>", n.questName(e.QuestID))

	var rewards []string
	if e.XP > 0 {
		rewards = append(rewards, fmt.Sprintf("%d experience", e.XP))
	}
	if e.Gold > 0 {
		rewards = append(rewards, fmt.Sprintf("%d gold", e.Gold))
	}
	for _, it := range e.Items {
		rewards = append(rewards, n.itemLabel(it))
	}
	for _, ab := range e.Abilities {
		rewards = append(rewards, ab)
	}
	if e.ClassUnlock != "" {
		rewards = append(rewards, "class: "+e.ClassUnlock)
	}
	if e.RaceUnlock != "" {
		rewards = append(rewards, "race: "+e.RaceUnlock)
	}
	if len(rewards) > 0 {
		fmt.Fprintf(&b, "\r\n<good>Rewards:</good> %s", strings.Join(rewards, ", "))
	}
	return b.String()
}

// itemLabel resolves an item template id to a display name, falling back
// to the short id (segment after the last ':') when no resolver is wired.
func (n *questNotifier) itemLabel(templateID string) string {
	if n.itemName != nil {
		if nm := n.itemName(templateID); nm != "" {
			return nm
		}
	}
	if i := strings.LastIndex(templateID, ":"); i >= 0 {
		return templateID[i+1:]
	}
	return templateID
}
