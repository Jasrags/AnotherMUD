package session

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/faction"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/reputation"
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
// known may be nil (tests / headless boots that don't wire crafting), in
// which case recipe rewards stay a no-op.
func NewQuestRewards(
	mgr *Manager,
	prog *progression.Manager,
	prof *progression.ProficiencyManager,
	tpls *item.Templates,
	store *entities.Store,
	currency *economy.CurrencyService,
	known *recipe.KnownManager,
	factionMgr *faction.Manager,
	reputationMgr *reputation.Manager,
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
	if known != nil {
		opts = append(opts, quest.WithRecipes(questRecipes{mgr: mgr, known: known}))
	}
	if factionMgr != nil {
		opts = append(opts, quest.WithFaction(questFaction{mgr: mgr, faction: factionMgr}))
	}
	if reputationMgr != nil {
		opts = append(opts, quest.WithRenown(questRenown{mgr: mgr, reputation: reputationMgr}))
	}
	return quest.NewDispatcher(opts...)
}

// NewQuestFactionGate builds the faction-standing prerequisite resolver
// (quest.FactionGate, faction.md §6) wired to the live faction manager. Returns
// a NopFactionGate when factionMgr is nil (tests / headless boots), so quests
// with a faction prereq stay acceptable rather than silently locked. The
// recipient is resolved through the session manager — acceptance always happens
// online, so an offline player is treated as not meeting the gate.
func NewQuestFactionGate(mgr *Manager, factionMgr *faction.Manager) quest.FactionGate {
	if factionMgr == nil {
		return quest.NopFactionGate{}
	}
	return questFactionGate{mgr: mgr, faction: factionMgr}
}

// questFaction bridges the quest FactionShifter (entityId-addressed) to the
// faction manager (entity + Definition addressed): it resolves the faction
// definition from the registry and the recipient actor through the session
// manager, then routes the standing change through Shift so the cancellable
// faction.shift.check pipeline and admin-immunity apply (faction.md §5.1). An
// unregistered faction or offline recipient is skipped silently, mirroring the
// other granters; the detached grant uses a background context like questGold.
type questFaction struct {
	mgr     *Manager
	faction *faction.Manager
}

func (q questFaction) ShiftStanding(entityID, factionID string, delta int, reason string) {
	def, ok := q.faction.Registry().Get(factionID)
	if !ok {
		return // faction not in content — skip silently
	}
	a, ok := q.mgr.GetByPlayerID(entityID)
	if !ok {
		return // recipient offline
	}
	q.faction.Shift(context.Background(), a, def, delta, reason)
}

// questRenown bridges the quest RenownShifter (entityId-addressed) to the
// reputation manager (entity-addressed): it resolves the recipient actor
// through the session manager, then routes the renown change through Shift so
// the cancellable reputation.shift.check pipeline and admin-immunity apply
// (reputation.md §5.3). An offline recipient is skipped silently, mirroring the
// other granters; the detached grant uses a background context like questGold.
type questRenown struct {
	mgr        *Manager
	reputation *reputation.Manager
}

func (q questRenown) ShiftRenown(entityID string, delta int, reason string) {
	a, ok := q.mgr.GetByPlayerID(entityID)
	if !ok {
		return // recipient offline
	}
	q.reputation.Shift(context.Background(), a, delta, reason)
}

// questFactionGate bridges the quest FactionGate to faction.Manager.MeetsStanding,
// resolving the faction definition from the registry and the player actor through
// the session manager. An unregistered faction admits (a content typo must not
// silently lock a quest, mirroring the shifter's fail-silent); an offline player
// fails the gate (acceptance happens online).
type questFactionGate struct {
	mgr     *Manager
	faction *faction.Manager
}

func (q questFactionGate) MeetsStanding(playerID, factionID string, min int) bool {
	def, ok := q.faction.Registry().Get(factionID)
	if !ok {
		return true // unknown faction → don't block
	}
	a, ok := q.mgr.GetByPlayerID(playerID)
	if !ok {
		return false // offline can't be evaluated
	}
	return q.faction.MeetsStanding(a, def, min)
}

// questRecipes bridges the quest RecipeTeacher to the per-character
// KnownManager. The recipient is resolved through the session manager so an
// offline grant is skipped silently (§5.2, mirroring questGold) — quest
// turn-in always happens while the player is online, and the learned recipe
// then persists automatically (Persist syncs known recipes unconditionally).
// An already-known recipe is a no-op inside KnownManager.Learn.
type questRecipes struct {
	mgr   *Manager
	known *recipe.KnownManager
}

func (q questRecipes) GrantRecipe(entityID, recipeID string) {
	if _, ok := q.mgr.GetByPlayerID(entityID); !ok {
		return
	}
	q.known.Learn(entityID, recipe.RecipeID(recipeID))
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
	// SR-M5: a karma-ledger character banks the quest's advancement reward as
	// spendable karma rather than track XP. The quest completion message is the
	// player's feedback (as with the XP path), so nothing is written here.
	if a.UsesKarmaLedger() {
		a.GrantKarma(context.Background(), source, amount)
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
