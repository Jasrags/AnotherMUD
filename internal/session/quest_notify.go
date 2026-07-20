package session

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/economy"
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
	mgr         *Manager
	registry    *quest.Registry
	giverName   func(templateID string) string // mob template id → display name
	itemName    func(templateID string) string // item template id → display name
	factionName func(id string) string         // faction id → display name
	money       economy.CurrencyLabel          // currency-label seam: reward "gold"/"¥"
	logger      *slog.Logger
}

// NewQuestNotifier builds the runtime quest sink. giverName / itemName /
// factionName resolve ids to display names for turn-in prompts and reward
// lines; any may be nil (falls back to the raw / prettified id). money is the
// pack's currency label so the reward banner reads "25¥" / "25 gold".
func NewQuestNotifier(
	mgr *Manager,
	registry *quest.Registry,
	giverName func(string) string,
	itemName func(string) string,
	factionName func(string) string,
	money economy.CurrencyLabel,
	logger *slog.Logger,
) quest.EventSink {
	if logger == nil {
		logger = slog.Default()
	}
	return &questNotifier{mgr: mgr, registry: registry, giverName: giverName, itemName: itemName, factionName: factionName, money: money, logger: logger}
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

// completionBannerWidth is the fixed rule width for the completion block. The
// rules are plain `=` runs (no vertical frame), so a multibyte reward value like
// "500¥" sits in an indented content line and never drifts a border
// (render.Panel's width count is byte-based — see render-panel-width-multibyte).
const completionBannerWidth = 60

// completionBanner renders the player-visible "quest complete" block: a titled
// rule, the quest name + classification, and the itemized rewards granted (from
// the event payload). Framed with fixed-width rules + a leading blank line so a
// visit-completed quest's banner separates from the room/movement text that
// would otherwise bury it (the "my quest vanished" trap).
func (n *questNotifier) completionBanner(e quest.CompletedEvent) string {
	var b strings.Builder
	b.WriteString("\r\n<good>" + titledRule(completionBannerWidth, "QUEST COMPLETE") + "</good>\r\n")
	if cls := n.classification(e.QuestID); cls != "" {
		fmt.Fprintf(&b, "  <title>%s</title> <subtle>(%s)</subtle>\r\n", n.questName(e.QuestID), cls)
	} else {
		fmt.Fprintf(&b, "  <title>%s</title>\r\n", n.questName(e.QuestID))
	}
	if lines := n.rewardLines(e); len(lines) > 0 {
		b.WriteString("  <good>Rewards</good>\r\n")
		for _, ln := range lines {
			b.WriteString("    " + ln + "\r\n")
		}
	}
	b.WriteString("<good>" + strings.Repeat("=", completionBannerWidth) + "</good>\r\n")
	return b.String()
}

// rewardLines itemizes a completion's rewards, one display line each. XP/gold are
// always positive grants; faction standing + renown carry their sign (a quest
// may cost standing with a rival). Faction ids resolve to display names.
func (n *questNotifier) rewardLines(e quest.CompletedEvent) []string {
	var lines []string
	if e.XP > 0 {
		lines = append(lines, fmt.Sprintf("+ %d experience", e.XP))
	}
	if e.Gold > 0 {
		// Currency-label seam: "25¥" in Shadowrun, "25 gold" in the fantasy default.
		lines = append(lines, "+ "+n.money.Format(e.Gold))
	}
	for _, fr := range e.Faction {
		if fr.Delta == 0 {
			continue // a 0-delta reward (author slip) shows no line — mirror the renown guard
		}
		lines = append(lines, fmt.Sprintf("%s standing with %s", signedAmount(fr.Delta), n.factionLabel(fr.Faction)))
	}
	if e.Reputation != 0 {
		lines = append(lines, signedAmount(e.Reputation)+" renown")
	}
	for _, it := range e.Items {
		lines = append(lines, "+ "+n.itemLabel(it))
	}
	for _, ab := range e.Abilities {
		lines = append(lines, "+ "+ab)
	}
	if e.ClassUnlock != "" {
		lines = append(lines, "+ class: "+e.ClassUnlock)
	}
	if e.RaceUnlock != "" {
		lines = append(lines, "+ race: "+e.RaceUnlock)
	}
	return lines
}

// classification returns the quest's classification (main/side/daily), or "".
func (n *questNotifier) classification(questID string) string {
	if def, ok := n.registry.Lookup(questID); ok {
		return def.Classification
	}
	return ""
}

// factionLabel resolves a faction id to its display name, falling back to the
// prettified id ("the-streets" → "The Streets") when no resolver is wired.
func (n *questNotifier) factionLabel(id string) string {
	if n.factionName != nil {
		if nm := n.factionName(id); nm != "" {
			return nm
		}
	}
	return prettifyID(id)
}

// signedAmount renders a signed reward magnitude with a spaced sign
// ("+ 50" / "- 50"), matching the "+ " prefix the always-positive rewards use.
func signedAmount(n int) string {
	if n < 0 {
		return fmt.Sprintf("- %d", -n)
	}
	return fmt.Sprintf("+ %d", n)
}

// titledRule builds a fixed-width `=` rule with a centered label, e.g.
// "=========== QUEST COMPLETE ===========". ASCII only, so len == visible width.
func titledRule(width int, label string) string {
	label = " " + label + " "
	if len(label) >= width {
		return strings.Repeat("=", width)
	}
	left := (width - len(label)) / 2
	return strings.Repeat("=", left) + label + strings.Repeat("=", width-left-len(label))
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
