package session

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/quest"
)

// NewQuestRewards builds the quest reward dispatcher wired to the live
// engine services (M10.10b, quests.md §5). XP grants through the
// progression manager onto the recipient's track; abilities through the
// proficiency manager (whose Learn already matches quest.AbilityTeacher);
// items spawn from a template into the recipient's inventory. Gold has no
// service yet (economy-survival, M11), so it stays a no-op. Each granter
// resolves the recipient via the session manager and is a silent no-op
// when the player is offline or the template is missing (§5.2).
func NewQuestRewards(
	mgr *Manager,
	prog *progression.Manager,
	prof *progression.ProficiencyManager,
	tpls *item.Templates,
	store *entities.Store,
) *quest.Dispatcher {
	return quest.NewDispatcher(
		quest.WithExperience(questXP{mgr: mgr, prog: prog}),
		quest.WithAbilities(prof),
		quest.WithItems(questItems{mgr: mgr, tpls: tpls, store: store}),
	)
}

// questMarkerFor builds a RenderRoom marker checker for a player from the
// quest service, or nil when quests aren't wired (M10.10b, §8).
func questMarkerFor(q *quest.Service, playerID string) func(templateID string) bool {
	if q == nil {
		return nil
	}
	return func(templateID string) bool {
		return q.HasMarker(playerID, templateID)
	}
}

type questXP struct {
	mgr  *Manager
	prog *progression.Manager
}

func (q questXP) GrantExperience(entityID string, amount int64, track, source string) {
	a, ok := q.mgr.GetByPlayerID(entityID)
	if !ok {
		return
	}
	// Reward grants are detached from the triggering request, so a
	// background context is appropriate for the XP grant's logging.
	a.GrantXP(context.Background(), q.prog, track, source, amount)
}

type questItems struct {
	mgr   *Manager
	tpls  *item.Templates
	store *entities.Store
}

func (q questItems) GrantItem(entityID, templateID string, _ bool) {
	a, ok := q.mgr.GetByPlayerID(entityID)
	if !ok {
		return
	}
	tpl, err := q.tpls.Get(item.TemplateID(templateID))
	if err != nil {
		return // missing template skipped silently (§5.2 step 6)
	}
	inst, err := q.store.Spawn(tpl)
	if err != nil {
		return
	}
	a.AddToInventory(inst.ID())
	a.MarkContentsDirty()
}
