// Package questwatch translates world events into quest objective
// progress (quests.md §7). It subscribes to a fixed set of bus events
// and routes each to the quest service's AdvanceMatching against the
// source player, so most content needs no explicit quest calls. The
// watcher never mutates quest state directly — it always goes through the
// service so the service's own events fire (§7).
package questwatch

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/quest"
)

// Watcher subscribes quest tracking to the world event bus.
//
// Deferred (not wired this slice): the §7.2/§7.3 side channels —
// `quest_grant` on an item template or destination room, and
// `quest_advance` on the pickup payload. Room has no property bag yet,
// the pickup event carries no quest_advance field, and the grant path
// needs the Accept Player resolver that lands with the M10.10 verbs.
type Watcher struct {
	svc   *quest.Service
	store *entities.Store // resolves entity ids → template ids for collect/deliver
}

// New builds a watcher over the quest service and entity store.
func New(svc *quest.Service, store *entities.Store) *Watcher {
	return &Watcher{svc: svc, store: store}
}

// Subscribe registers the watcher's handlers on the bus (§7.1).
func (w *Watcher) Subscribe(bus *eventbus.Bus) {
	bus.Subscribe(eventbus.EventMobKilled, w.onMobKilled)
	bus.Subscribe(eventbus.EventItemPickedUp, w.onItemPickedUp)
	bus.Subscribe(eventbus.EventItemGiven, w.onItemGiven)
	bus.Subscribe(eventbus.EventPlayerMoved, w.onPlayerMoved)
}

// onMobKilled advances `kill` objectives whose target is the killed
// mob's template id, for the killer (§7.1).
func (w *Watcher) onMobKilled(_ context.Context, e eventbus.Event) {
	ev, ok := e.(eventbus.MobKilled)
	if !ok || ev.KillerID == "" || ev.TemplateID == "" {
		return
	}
	target := ev.TemplateID
	w.svc.AdvanceMatching(ev.KillerID, "kill", func(o quest.Objective) bool {
		return o.Target == target
	})
}

// onItemPickedUp advances `collect` objectives whose target is the
// picked-up item's template id, for the picker (§7.1). The pickup event
// carries only the instance id, so the template is resolved through the
// entity store; a missing instance is tolerated (§7.4).
func (w *Watcher) onItemPickedUp(_ context.Context, e eventbus.Event) {
	ev, ok := e.(eventbus.ItemPickedUp)
	if !ok || ev.HolderID == "" {
		return
	}
	target := w.itemTemplate(ev.ItemID)
	if target == "" {
		return
	}
	w.svc.AdvanceMatching(string(ev.HolderID), "collect", func(o quest.Objective) bool {
		return o.Target == target
	})
}

// onItemGiven advances `deliver` objectives whose item target matches the
// given item AND whose npc target matches the recipient's template id,
// for the giver (§7.1). The recipient template is resolved from the
// store; a non-mob or missing recipient yields "" and simply won't match
// a deliver objective's npc (§7.4).
func (w *Watcher) onItemGiven(_ context.Context, e eventbus.Event) {
	ev, ok := e.(eventbus.ItemGiven)
	if !ok || ev.GiverID == "" || ev.TemplateID == "" {
		return
	}
	itemTarget := ev.TemplateID
	npcTarget := w.mobTemplate(ev.RecipientID)
	w.svc.AdvanceMatching(string(ev.GiverID), "deliver", func(o quest.Objective) bool {
		return o.Target == itemTarget && o.NPC == npcTarget
	})
}

// onPlayerMoved advances `visit` objectives whose target is the
// destination room id, for the mover (§7.1).
func (w *Watcher) onPlayerMoved(_ context.Context, e eventbus.Event) {
	ev, ok := e.(eventbus.PlayerMoved)
	if !ok || ev.PlayerID == "" || ev.To == "" {
		return
	}
	to := string(ev.To)
	w.svc.AdvanceMatching(ev.PlayerID, "visit", func(o quest.Objective) bool {
		return o.Target == to
	})
}

// itemTemplate resolves an item instance id to its template id, or ""
// when the instance is gone or is not an item.
func (w *Watcher) itemTemplate(id entities.EntityID) string {
	if w.store == nil {
		return ""
	}
	ent, ok := w.store.GetByID(id)
	if !ok {
		return ""
	}
	if inst, ok := ent.(*entities.ItemInstance); ok {
		return string(inst.TemplateID())
	}
	return ""
}

// mobTemplate resolves a mob instance id to its template id, or "" when
// the instance is gone or is not a mob.
func (w *Watcher) mobTemplate(id entities.EntityID) string {
	if w.store == nil {
		return ""
	}
	ent, ok := w.store.GetByID(id)
	if !ok {
		return ""
	}
	if inst, ok := ent.(*entities.MobInstance); ok {
		return string(inst.TemplateID())
	}
	return ""
}
