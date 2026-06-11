package session

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// BackgroundGranter applies a background's starting package — skills, items,
// gold — to a newly-created character (backgrounds §4). It reuses the same
// grant mechanics quest rewards do (proficiency Learn, item spawn into
// inventory, currency credit), resolving the recipient through the session
// manager. The grant is fired once, from the character.created subscriber, so
// it never re-applies on login.
type BackgroundGranter struct {
	mgr      *Manager
	prof     *progression.ProficiencyManager
	tpls     *item.Templates
	store    *entities.Store
	currency *economy.CurrencyService
}

// NewBackgroundGranter wires the granter. currency may be nil (gold grants are
// then skipped); prof/tpls/store are required for skill + item grants.
func NewBackgroundGranter(mgr *Manager, prof *progression.ProficiencyManager, tpls *item.Templates, store *entities.Store, currency *economy.CurrencyService) *BackgroundGranter {
	return &BackgroundGranter{mgr: mgr, prof: prof, tpls: tpls, store: store, currency: currency}
}

// Grant applies bg's package to the online character playerID (backgrounds §4):
// each skill is learned at its starting proficiency (a non-positive value
// defaults to the baseline 1), each item is spawned into inventory, and the
// gold is credited. An offline recipient or a missing skill/item is skipped
// silently (mirrors quest-reward grants). Idempotency is the caller's job —
// this is fired only on character.created, which happens once per character.
func (g *BackgroundGranter) Grant(ctx context.Context, playerID string, bg *progression.Background) {
	if g == nil || bg == nil {
		return
	}
	a, ok := g.mgr.GetByPlayerID(playerID)
	if !ok {
		return
	}
	if g.prof != nil {
		for _, s := range bg.Skills {
			prof := s.Proficiency
			if prof < 1 {
				prof = 1 // baseline trained value (backgrounds §2)
			}
			g.prof.Learn(playerID, s.AbilityID, prof)
		}
	}
	if g.tpls != nil && g.store != nil {
		for _, itemID := range bg.Items {
			tpl, err := g.tpls.Get(item.TemplateID(itemID))
			if err != nil {
				continue // missing template skipped (fail-soft)
			}
			inst, err := g.store.Spawn(tpl)
			if err != nil {
				continue
			}
			// AddToInventory syncs the save tree + marks the actor dirty per
			// item, so no trailing MarkContentsDirty is needed here.
			a.AddToInventory(inst.ID())
		}
	}
	// EPIC S4 Phase 5: authored background feats — granted free (no slot, no
	// prereq check), recorded + applied via the actor's GrantFeat. Unknown ids
	// skip fail-soft. Closes the S9 deferred background-feat item.
	for _, featID := range bg.Feats {
		a.GrantFeat(featID, "")
	}
	if bg.Gold > 0 && g.currency != nil {
		g.currency.AddGold(ctx, a, bg.Gold, "background:"+bg.ID)
	}
}
