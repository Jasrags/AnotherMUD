package session

import (
	"context"
	"strings"

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

// BackgroundChoices carries the pick-one selections a character made at the
// background chooser (backgrounds §2): the chosen feat (from FeatOptions) and
// the index of the chosen equipment package (into EquipmentPackages). Zero
// values mean "no choice" — a background with no FeatOptions/EquipmentPackages
// ignores them, and a single-option list auto-resolves.
type BackgroundChoices struct {
	Feat           string
	EquipmentIndex int
}

// Grant applies bg's package to the online character playerID (backgrounds §4):
// each skill is learned at its starting proficiency (a non-positive value
// defaults to the baseline 1), the always-granted items + the chosen equipment
// package are spawned into inventory, the always-granted feats + the chosen
// feat are granted, and the gold is credited. An offline recipient or a missing
// skill/item is skipped silently (mirrors quest-reward grants). Idempotency is
// the caller's job — this is fired only on character.created, which happens
// once per character.
func (g *BackgroundGranter) Grant(ctx context.Context, playerID string, bg *progression.Background, choices BackgroundChoices) {
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
	// Always-granted items, then the chosen equipment package (backgrounds §2 —
	// the pick-one chooser). A background with no packages grants only Items.
	g.grantItems(a, bg.Items)
	if len(bg.EquipmentPackages) > 0 {
		idx := choices.EquipmentIndex
		if idx < 0 || idx >= len(bg.EquipmentPackages) {
			idx = 0 // out-of-range (stale choice / content change) → first package
		}
		g.grantItems(a, bg.EquipmentPackages[idx])
	}
	// EPIC S4 Phase 5: authored background feats — granted free (no slot, no
	// prereq check), recorded + applied via the actor's GrantFeat. Unknown ids
	// skip fail-soft. The always-granted Feats, then the chosen FeatOption (only
	// if it is actually one of the options — guards a stale/forged choice).
	for _, featID := range bg.Feats {
		a.GrantFeat(featID, "")
	}
	// The chosen FeatOption: the player's pick when valid, else the first option
	// (a single-option background auto-grants without a choice step; a missing
	// or forged choice safely falls back to the first).
	if len(bg.FeatOptions) > 0 {
		chosen := bg.FeatOptions[0]
		if choices.Feat != "" && containsFold(bg.FeatOptions, choices.Feat) {
			chosen = strings.ToLower(strings.TrimSpace(choices.Feat))
		}
		a.GrantFeat(chosen, "")
	}
	// The home language (languages.md §3): added to the character's known set,
	// idempotent + fail-soft. An empty home_language grants nothing; an
	// unregistered id is still recorded (it renders by id) — the granter does
	// not gate on the registry, matching the fail-soft item/feat grants.
	if bg.HomeLanguage != "" {
		a.LearnLanguage(bg.HomeLanguage)
	}
	if bg.Gold > 0 && g.currency != nil {
		g.currency.AddGold(ctx, a, bg.Gold, "background:"+bg.ID)
	}
}

// grantItems spawns each item template into the actor's inventory, skipping a
// missing template fail-soft (shared by the always-grant and the chosen
// equipment package).
func (g *BackgroundGranter) grantItems(a *connActor, ids []string) {
	if g.tpls == nil || g.store == nil {
		return
	}
	for _, itemID := range ids {
		tpl, err := g.tpls.Get(item.TemplateID(itemID))
		if err != nil {
			continue // missing template skipped (fail-soft)
		}
		inst, err := g.store.Spawn(tpl)
		if err != nil {
			continue
		}
		// AddToInventory syncs the save tree + marks the actor dirty per item,
		// so no trailing MarkContentsDirty is needed here.
		a.AddToInventory(inst.ID())
	}
}

// containsFold reports whether list contains target (case-insensitive). The
// FeatOptions are Register-lowercased; the chosen feat comes from the wizard.
func containsFold(list []string, target string) bool {
	for _, v := range list {
		if strings.EqualFold(v, target) {
			return true
		}
	}
	return false
}
