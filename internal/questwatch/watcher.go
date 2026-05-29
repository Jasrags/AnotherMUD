// Package questwatch translates world events into quest objective
// progress (quests.md §7). It subscribes to a fixed set of bus events
// and routes each to the quest service's AdvanceMatching against the
// source player, so most content needs no explicit quest calls. The
// watcher never mutates quest state directly — it always goes through the
// service so the service's own events fire (§7).
package questwatch

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/quest"
)

// Watcher subscribes quest tracking to the world event bus.
//
// Still deferred: `quest_grant` on a destination ROOM (world.Room has no
// property bag yet) and `quest_advance` on the pickup payload (the event
// carries no such field — a scripting-era channel). The item-template
// `quest_grant` side channel (§7.2) is wired when an Accept resolver is
// provided via SetItemGrant.
type Watcher struct {
	svc   *quest.Service
	store *entities.Store // resolves entity ids → template ids for collect/deliver
	// grant resolves a player id to a quest.Player for the §7.2
	// quest_grant item side channel. nil disables it.
	grant func(playerID string) (quest.Player, bool)
}

// New builds a watcher over the quest service and entity store.
func New(svc *quest.Service, store *entities.Store) *Watcher {
	return &Watcher{svc: svc, store: store}
}

// SetItemGrant enables the §7.2 quest_grant item side channel: picking up
// an item whose template carries a `quest_grant` property auto-accepts
// that quest for the picker (resolver maps the picker's id to a
// quest.Player). Silent — acceptance failures are ignored per spec.
func (w *Watcher) SetItemGrant(resolver func(playerID string) (quest.Player, bool)) {
	w.grant = resolver
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
	// KillerID is a combat-prefixed id ("player:<id>"); quest state is
	// keyed by the bare player entity id. EntityIDOf strips the prefix
	// (idempotent on an already-bare id).
	killer := combat.EntityIDOf(combat.CombatantID(ev.KillerID))
	target := ev.TemplateID
	w.svc.AdvanceMatching(killer, "kill", func(o quest.Objective) bool {
		return o.Target == target
	})
}

// onItemPickedUp advances `collect` objectives whose target is the
// picked-up item's template id, for the picker (§7.1), and honors the
// `quest_grant` item side channel (§7.2). The pickup event carries only
// the instance id, so the instance is resolved through the entity store;
// a missing instance is tolerated (§7.4).
func (w *Watcher) onItemPickedUp(_ context.Context, e eventbus.Event) {
	ev, ok := e.(eventbus.ItemPickedUp)
	if !ok || ev.HolderID == "" {
		return
	}
	inst := w.itemInstance(ev.ItemID)
	if inst == nil {
		return
	}
	holder := string(ev.HolderID)
	if target := string(inst.TemplateID()); target != "" {
		w.svc.AdvanceMatching(holder, "collect", func(o quest.Objective) bool {
			return o.Target == target
		})
	}
	w.maybeGrant(inst, holder)
}

// maybeGrant honors the §7.2 quest_grant property on the picked-up item:
// it accepts the named quest for the picker (silent; failures ignored).
// The property value may be a bare or namespaced quest id — ResolveID
// maps it to the registered id.
func (w *Watcher) maybeGrant(inst *entities.ItemInstance, holder string) {
	if w.grant == nil {
		return
	}
	pv, _ := inst.Property("quest_grant")
	raw, ok := pv.(string)
	if !ok || raw == "" {
		return
	}
	questID, ok := w.svc.ResolveID(raw)
	if !ok {
		return
	}
	if player, ok := w.grant(holder); ok {
		w.svc.Accept(player, questID, true)
	}
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

// itemInstance resolves an item instance id to its *ItemInstance, or nil
// when the instance is gone or is not an item.
func (w *Watcher) itemInstance(id entities.EntityID) *entities.ItemInstance {
	if w.store == nil {
		return nil
	}
	ent, ok := w.store.GetByID(id)
	if !ok {
		return nil
	}
	inst, _ := ent.(*entities.ItemInstance)
	return inst
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
