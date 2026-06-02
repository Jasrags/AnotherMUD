// Package corpse owns the death → drop path: turning a mob-killed
// signal into a lootable corpse (loot-and-corpses §2–§3). It subscribes
// to the combat `mob.killed` event, mints a corpse container in the
// victim's room, moves the mob's spawn-time loot into it, rolls the
// loot table's coin block, and records the metadata later stages
// (looting rights, autoloot, decay) read back.
//
// Loot *generation* is not owned here — the mob already carries its
// items from spawn (mobs-ai-spawning §6.3). This package only moves
// them and adds coins.
package corpse

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/loot"
	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// Reserved corpse property keys. They live on the corpse ItemInstance's
// property bag and are read by the looting (§4–§5), autoloot (§6), and
// decay (§7) stages.
const (
	// PropKiller is the killer's combatant/entity id (the owner-set
	// seed). Empty when combat reported no killer (§4 → open corpse).
	PropKiller = "corpse_killer"
	// PropCreatedTick is the tick the corpse was created on (uint64);
	// the ownership window and decay deadline are measured from it.
	PropCreatedTick = "corpse_created_tick"
	// PropCoins is the coin pile amount (int) credited to a looter's
	// currency balance, not their item inventory (§3).
	PropCoins = "corpse_coins"
	// PropOwners is the looting-rights owner set ([]string). Today it
	// is exactly the killer id; grouping fills it with party members
	// (§4 — the rights seam).
	PropOwners = "corpse_owners"
)

// Corpse-identifying tags. The corpse type is entities.ContainerType so
// the container-access machinery applies; these tags carry the
// corpse-specific rules.
const (
	// TagCorpse marks the entity as a corpse (decay sweep + display).
	TagCorpse = "corpse"
	// TagNoGet makes the corpse itself impossible to pick up — only its
	// contents leave it (§2.2). Honored by the existing get handler.
	TagNoGet = "no_get"
	// TagNoPut makes the corpse refuse `put` — it is a loot source, not
	// storage (§2.2). Honored by the put handler.
	TagNoPut = "no_put"
)

// DefaultNameTemplate formats a corpse's display name from the mob
// name (§2.1 / §9 "Corpse display-name template"). The mob name already
// carries its article ("a village guard" → "the corpse of a village
// guard").
const DefaultNameTemplate = "the corpse of %s"

// Service creates corpses on mob death. Wire one at the composition
// root and subscribe OnMobKilled to the mob.killed event.
type Service struct {
	store        *entities.Store
	contents     *entities.Contents
	placement    *entities.Placement
	bus          *eventbus.Bus
	mobs         *mob.Templates
	loot         *loot.Registry
	roller       loot.Roller
	now          func() uint64
	nameTemplate string
}

// Config bundles the Service dependencies. store/contents/placement are
// required; bus/mobs/loot/roller/now may be nil for narrow tests (a nil
// loot/mobs/roller simply yields no coins; a nil now stamps tick 0).
type Config struct {
	Store        *entities.Store
	Contents     *entities.Contents
	Placement    *entities.Placement
	Bus          *eventbus.Bus
	Mobs         *mob.Templates
	Loot         *loot.Registry
	Roller       loot.Roller
	Now          func() uint64
	NameTemplate string // defaults to DefaultNameTemplate when empty
}

// New constructs a Service from cfg.
func New(cfg Config) *Service {
	nt := strings.TrimSpace(cfg.NameTemplate)
	if nt == "" {
		nt = DefaultNameTemplate
	}
	now := cfg.Now
	if now == nil {
		now = func() uint64 { return 0 }
	}
	return &Service{
		store:        cfg.Store,
		contents:     cfg.Contents,
		placement:    cfg.Placement,
		bus:          cfg.Bus,
		mobs:         cfg.Mobs,
		loot:         cfg.Loot,
		roller:       cfg.Roller,
		now:          now,
		nameTemplate: nt,
	}
}

// OnMobKilled is the bus handler. It casts the event and delegates to
// CreateOnDeath; a non-MobKilled event is ignored.
func (s *Service) OnMobKilled(ctx context.Context, ev eventbus.Event) {
	e, ok := ev.(eventbus.MobKilled)
	if !ok {
		return
	}
	s.CreateOnDeath(ctx, e)
}

// CreateOnDeath runs the §2.1 corpse-creation sequence for a single
// mob-killed event. Ordering note (a documented refinement of §2.1's
// step numbering): coins are rolled *before* the cancellable veto so
// both the "no items + no coins → no corpse" short-circuit and the
// event payload have the coin amount. Nothing else changes — on cancel
// no corpse is created and the contents stay on the (about-to-be-
// removed) mob.
func (s *Service) CreateOnDeath(ctx context.Context, e eventbus.MobKilled) {
	if s.store == nil || s.contents == nil || s.placement == nil {
		return
	}

	itemIDs := s.contents.In(e.MobID)
	coins := s.rollCoins(e.TemplateID)

	// §2.1: a mob with neither items nor a coin drop produces no corpse.
	if len(itemIDs) == 0 && coins <= 0 {
		return
	}

	// §2.1 step 1: cancellable pre-event. A canceller suppresses the
	// corpse and owns the loot it suppressed; mob removal stays the
	// death-cleanup path's job.
	creating := eventbus.NewCorpseCreating(e.MobID, e.MobName, e.TemplateID, e.KillerID, e.RoomID, len(itemIDs), coins)
	if s.bus != nil && s.bus.PublishCancellable(ctx, creating) {
		logging.From(ctx).Debug("corpse.creating cancelled",
			slog.String("mob_id", string(e.MobID)),
			slog.String("template", e.TemplateID))
		return
	}

	// §2.1 steps 2 + 5: mint the corpse container, recording killer,
	// creation tick, coin amount, and the owner set (today just the
	// killer — the §4 grouping seam).
	owners := []string{}
	if e.KillerID != "" {
		owners = []string{e.KillerID}
	}
	props := map[string]any{
		PropKiller:      e.KillerID,
		PropCreatedTick: s.now(),
		PropCoins:       coins,
		PropOwners:      owners,
	}
	corpseInst := s.store.SpawnContainer(
		fmt.Sprintf(s.nameTemplate, e.MobName),
		[]string{TagCorpse, TagNoGet, TagNoPut},
		corpseKeywords(e.MobName),
		props,
	)

	// §2.1 step 3: move the mob's contents into the corpse — each item
	// leaves the mob and is re-filed in the corpse, identity preserved.
	moved := 0
	for _, id := range itemIDs {
		if s.contents.Take(id) {
			s.contents.Put(corpseInst.ID(), id)
			moved++
		}
	}

	// Place the corpse where the mob died so it shows in the room and
	// the looting verbs can resolve it.
	s.placement.Place(corpseInst.ID(), e.RoomID)

	// §2.1 step 6: announce.
	if s.bus != nil {
		s.bus.Publish(ctx, eventbus.CorpseCreated{
			CorpseID:   corpseInst.ID(),
			RoomID:     e.RoomID,
			MobID:      e.MobID,
			MobName:    e.MobName,
			TemplateID: e.TemplateID,
			KillerID:   e.KillerID,
			ItemCount:  moved,
			Coins:      coins,
		})
	}
}

// rollCoins resolves the dead mob's loot-table coin block and rolls it.
// Any missing link (no registries, no loot table, no coin block) yields
// zero coins without touching the roller.
func (s *Service) rollCoins(templateID string) int {
	if s.mobs == nil || s.loot == nil {
		return 0
	}
	tpl, err := s.mobs.Get(mob.TemplateID(templateID))
	if err != nil || tpl.LootTable == "" {
		return 0
	}
	tbl, ok := s.loot.Get(tpl.LootTable)
	if !ok {
		return 0
	}
	return loot.RollCoins(tbl.Coin, s.roller)
}

// corpseKeywords derives lookup keywords from the mob's display name:
// "corpse" plus each significant word (articles dropped, lowercased) so
// `loot corpse`, `loot guard`, and `loot village` all resolve in M22.3.
func corpseKeywords(mobName string) []string {
	kws := []string{TagCorpse}
	for _, w := range strings.Fields(strings.ToLower(mobName)) {
		switch w {
		case "a", "an", "the", "of":
			continue
		}
		kws = append(kws, w)
	}
	return kws
}
