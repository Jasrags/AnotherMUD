package session

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/quest"
)

// NewQuestRewards builds the quest reward dispatcher wired to the live
// engine services (M10.10b, quests.md §5). XP grants through the
// progression manager onto the recipient's track; abilities through the
// proficiency manager (whose Learn already matches quest.AbilityTeacher);
// items spawn from a template into the recipient's inventory; gold
// credits through the currency service (M11.1 — closes the M10.10b
// "gold stays nop" note). Each granter resolves the recipient via the
// session manager and is a silent no-op when the player is offline or
// the template is missing (§5.2). currency may be nil (tests / headless
// boots that don't wire economy), in which case gold stays a no-op.
//
// defaultTrack binds the engine's primary XP track at the composition
// root (mirroring StartRoom / DefaultRace). The spec's quest.DefaultTrack
// is "main" (setting-agnostic), but content registers its own track name
// (e.g. "adventurer") — and progression.GrantExperience silently drops a
// grant on an unregistered track, so a mismatch means quest XP is lost
// with no error. Passing the real track here keeps the spec default
// untouched while making quest XP actually land. Empty string keeps the
// dispatcher's spec default (used by tests that don't grant XP).
func NewQuestRewards(
	mgr *Manager,
	prog *progression.Manager,
	prof *progression.ProficiencyManager,
	tpls *item.Templates,
	store *entities.Store,
	currency *economy.CurrencyService,
	defaultTrack string,
) *quest.Dispatcher {
	opts := []quest.DispatcherOption{
		quest.WithExperience(questXP{mgr: mgr, prog: prog}),
		quest.WithAbilities(prof),
		quest.WithItems(questItems{mgr: mgr, tpls: tpls, store: store}),
	}
	if defaultTrack != "" {
		opts = append(opts, quest.WithTrack(defaultTrack))
	}
	if currency != nil {
		opts = append(opts, quest.WithGold(questGold{mgr: mgr, currency: currency}))
	}
	return quest.NewDispatcher(opts...)
}

// questGold bridges the quest GoldGranter (entityId-addressed) to the
// economy CurrencyService (entity-addressed) by resolving the recipient
// actor through the session manager. Offline recipients are skipped
// silently (§5.2). The quest interface carries no ctx — mirroring
// questXP, the grant uses a background context, which is acceptable for
// a detached reward credit (the currency.credited event still fires on
// the bus).
type questGold struct {
	mgr      *Manager
	currency *economy.CurrencyService
}

func (q questGold) AddGold(entityID string, delta int, reason string) {
	a, ok := q.mgr.GetByPlayerID(entityID)
	if !ok {
		return
	}
	q.currency.AddGold(context.Background(), a, delta, reason)
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
	// quest.ExperienceGranter has no ctx, so XP-grant logging loses the
	// session-scoped fields and the GrantResult (level-up message) is
	// discarded — the level-up still fires on the bus, and the quest
	// completion itself is the player's feedback. Acceptable for a
	// detached reward grant; threading ctx would mean widening the
	// reward interface all the way from Accept.
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
