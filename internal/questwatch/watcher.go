// Package questwatch translates world events into quest objective
// progress (quests.md §7). It subscribes to a fixed set of bus events
// and routes each to the quest service's AdvanceMatching against the
// source player, so most content needs no explicit quest calls. The
// watcher never mutates quest state directly — it always goes through the
// service so the service's own events fire (§7).
package questwatch

import (
	"context"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Watcher subscribes quest tracking to the world event bus.
//
// §7.2 side channels: the `quest_advance` pickup-payload channel is wired
// (maybeAdvance) — the pickup command carries the item's quest_advance
// property onto the event, and a script publishing the event MAY set it
// directly. The item-template `quest_grant` channel is wired when an Accept
// resolver is provided via SetItemGrant; the room-side variant (M14.6) is
// wired through SetRoomGrant.
type Watcher struct {
	svc   *quest.Service
	store *entities.Store // resolves entity ids → template ids for collect/deliver
	// grant resolves a player id to a quest.Player for both the §7.2
	// quest_grant item side channel and the M14.6 room-side variant.
	// nil disables both.
	grant func(playerID string) (quest.Player, bool)
	// roomGrant resolves a room id to its quest_grant property value
	// (bare or namespaced quest id), or "" when the room has no
	// grant set. nil disables the room-side channel even when grant
	// is wired.
	roomGrant func(world.RoomID) string
}

// New builds a watcher over the quest service and entity store.
func New(svc *quest.Service, store *entities.Store) *Watcher {
	return &Watcher{svc: svc, store: store}
}

// SetItemGrant enables the §7.2 quest_grant item side channel: picking up
// an item whose template carries a `quest_grant` property auto-accepts
// that quest for the picker (resolver maps the picker's id to a
// quest.Player). Silent — acceptance failures are ignored per spec.
//
// Also enables the M14.6 room-side variant when SetRoomGrant is also
// wired; the same player resolver serves both channels.
func (w *Watcher) SetItemGrant(resolver func(playerID string) (quest.Player, bool)) {
	w.grant = resolver
}

// SetRoomGrant enables the M14.6 quest_grant room side channel: moving
// into a room whose Properties bag carries a `quest_grant` string
// auto-accepts that quest for the mover. resolver returns the room's
// quest_grant property value (or "" when absent / not a string).
//
// SetItemGrant MUST also be wired — the room-side channel reuses the
// player resolver. A room grant set without an item grant is logged
// as a partial wiring and is otherwise a no-op.
func (w *Watcher) SetRoomGrant(resolver func(world.RoomID) string) {
	w.roomGrant = resolver
}

// Subscribe registers the watcher's handlers on the bus (§7.1).
func (w *Watcher) Subscribe(bus *eventbus.Bus) {
	bus.Subscribe(eventbus.EventMobKilled, w.onMobKilled)
	bus.Subscribe(eventbus.EventItemPickedUp, w.onItemPickedUp)
	bus.Subscribe(eventbus.EventItemGiven, w.onItemGiven)
	bus.Subscribe(eventbus.EventItemConsumed, w.onItemConsumed)
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
	// quest_advance is conceptually item-independent, but it sits after the
	// inst!=nil gate above: safe today because Publish dispatches synchronously
	// from the pickup site while the instance is still in the store. If
	// ItemPickedUp is ever published async, or from a site where the item may
	// already be gone, hoist this above the gate so the payload isn't lost.
	w.maybeAdvance(ev.QuestAdvance, holder)
}

// maybeAdvance honors the §7.2 quest_advance side channel on the pickup event
// payload: a "<packId>:<questId>:<objectiveId>" string advances the named
// objective by 1 for the holder. The objective id is the segment after the
// last colon; everything before it is the quest term, resolved (bare or
// namespaced) through the registry. Malformed strings and unknown quests are
// silently ignored (§7.2); AdvanceObjective itself no-ops when the holder is
// not on that quest or the objective is absent/complete.
func (w *Watcher) maybeAdvance(payload, holder string) {
	payload = strings.TrimSpace(payload)
	i := strings.LastIndex(payload, ":")
	if i <= 0 || i == len(payload)-1 {
		return // no separator, or an empty quest term / objective id
	}
	questID, ok := w.svc.ResolveID(payload[:i])
	if !ok {
		return
	}
	w.svc.AdvanceObjective(holder, questID, payload[i+1:], 1)
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

// onItemConsumed advances `use` objectives whose target is the consumed
// item's template id, for the actor (§7.1). The event carries only the
// instance id and fires BEFORE the instance is destroyed (economy §6.2), so
// the template is resolved through the store; a missing instance is tolerated
// (§7.4).
func (w *Watcher) onItemConsumed(_ context.Context, e eventbus.Event) {
	ev, ok := e.(eventbus.ItemConsumed)
	if !ok || ev.ActorID == "" {
		return
	}
	inst := w.itemInstance(ev.ItemID)
	if inst == nil {
		return
	}
	if target := string(inst.TemplateID()); target != "" {
		w.svc.AdvanceMatching(string(ev.ActorID), "use", func(o quest.Objective) bool {
			return o.Target == target
		})
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
// destination room id, for the mover (§7.1). Also honors the M14.6
// room-side `quest_grant` channel: if the destination room declares
// a quest_grant property and both grant resolvers are wired, the
// quest is auto-accepted for the mover.
func (w *Watcher) onPlayerMoved(_ context.Context, e eventbus.Event) {
	ev, ok := e.(eventbus.PlayerMoved)
	if !ok || ev.PlayerID == "" || ev.To == "" {
		return
	}
	to := string(ev.To)
	w.svc.AdvanceMatching(ev.PlayerID, "visit", func(o quest.Objective) bool {
		return o.Target == to
	})
	w.maybeRoomGrant(ev.PlayerID, ev.To)
}

// maybeRoomGrant honors the M14.6 quest_grant property on the
// destination room: it accepts the named quest for the mover
// (silent; failures ignored, matching the item-side path).
func (w *Watcher) maybeRoomGrant(playerID string, roomID world.RoomID) {
	if w.roomGrant == nil || w.grant == nil {
		return
	}
	raw := w.roomGrant(roomID)
	if raw == "" {
		return
	}
	questID, ok := w.svc.ResolveID(raw)
	if !ok {
		return
	}
	if player, ok := w.grant(playerID); ok {
		w.svc.Accept(player, questID, true)
	}
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
