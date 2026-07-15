package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Light-source verbs (light-and-darkness §3.1). `light <item>` ignites
// a source; `extinguish <item>` puts it out. Both resolve the item over
// the union of the actor's inventory and equipped slots — you can light
// a torch sitting in your pack or one already held in your light slot.
//
// Lit/unlit state lives on the item instance (PropItemLit), so it
// travels with the item across pickup/drop/give/store and is
// admin-settable. Lighting and extinguishing are ordinary command
// results, not bus events — only the fuel gutter-out case (§3.2)
// publishes an event, and that lives in the fuel loop.

// LightHandler implements `light <item>`.
func LightHandler(ctx context.Context, c *Context) error {
	return lightVerb(ctx, c, true)
}

// ExtinguishHandler implements `extinguish <item>`.
func ExtinguishHandler(ctx context.Context, c *Context) error {
	return lightVerb(ctx, c, false)
}

func lightVerb(ctx context.Context, c *Context, ignite bool) error {
	verb := "extinguish"
	if ignite {
		verb = "light"
	}
	if c.Items == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("You can't %s anything right now.", verb))
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s what?", capitalize(verb)))
	}

	item, ok := resolveCarriedOrEquipped(c, strings.Join(c.Args, " "))
	if !ok {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}

	if !light.IsSource(item) {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is not a light source.", capitalize(item.Name())))
	}

	already := light.IsLit(item)
	if ignite {
		if already {
			return c.Actor.Write(ctx, fmt.Sprintf("%s is already lit.", capitalize(item.Name())))
		}
		// A spent fuel-burning source (fuel present and zero) cannot be
		// relit. A permanent source (no fuel property) always lights.
		if fuel, ok := item.Property(light.PropItemFuel); ok {
			if n, _ := fuel.(int); n <= 0 {
				return c.Actor.Write(ctx, fmt.Sprintf("%s is spent; there is no fuel left.", capitalize(item.Name())))
			}
		}
		item.SetProperty(light.PropItemLit, true)
		_ = c.Actor.Write(ctx, fmt.Sprintf("You light %s.", item.Name()))
		broadcastLight(ctx, c, fmt.Sprintf("%s lights %s.", c.Actor.Name(), item.Name()))
		return nil
	}

	if !already {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is not lit.", capitalize(item.Name())))
	}
	item.SetProperty(light.PropItemLit, false)
	_ = c.Actor.Write(ctx, fmt.Sprintf("You extinguish %s.", item.Name()))
	broadcastLight(ctx, c, fmt.Sprintf("%s extinguishes %s.", c.Actor.Name(), item.Name()))
	return nil
}

// broadcastLight sends a lit/extinguish observation to the rest of the
// room, nil-guarding the broadcaster and a nameless actor.
func broadcastLight(ctx context.Context, c *Context, msg string) {
	room := c.Actor.Room()
	if c.Broadcaster == nil || room == nil || c.Actor.Name() == "" {
		return
	}
	c.Broadcaster.SendToRoom(ctx, room.ID, msg, c.Actor.PlayerID())
}

// lightSlotKey is the equipment-slot key holding a viewer's active
// light source (the cap-1 "light" slot, slot baseline).
const lightSlotKey = "light"

// LightViewer is the per-viewer surface EffectiveLight reads. Both
// capabilities are optional, read via assertion: Equipment() supplies
// the held light (a player has a light slot; a mob does not) and
// HasTag() supplies darkvision. A viewer lacking one simply contributes
// nothing from it, so the same gather serves players and mobs. Modelled
// as an empty interface because the two capabilities are independent.
type LightViewer any

// equipmentHolder / taggable are the optional capability views
// EffectiveLight asserts a viewer against.
type equipmentHolder interface {
	Equipment() map[string]entities.EntityID
}
type taggable interface {
	HasTag(string) bool
}

// EffectiveLight computes a viewer's effective light level for room
// (light-and-darkness §2/§5), gathering the lit-source contribution
// (the viewer's held light + luminous items lying in the room) and the
// viewer's darkvision floor, then resolving. Returns light.Lit when the
// resolver is nil (light gating unwired) so tests and pre-light paths
// render exactly as before. Shared by the command handlers, the session
// login/link-dead renderers, and the combat darkness-penalty hook.
func EffectiveLight(resolver *light.Resolver, room *world.Room, viewer LightViewer, items *entities.Store, placement *entities.Placement) light.Level {
	if resolver == nil || room == nil {
		return light.Lit
	}
	sources, floor, lowLight := viewerLightTerms(resolver, viewer, room, items, placement)
	lvl := resolver.Effective(room, sources, floor)
	if !lowLight {
		return lvl
	}
	// Gate low-light on the room's viewer-floor-independent light (the real
	// photons: held/room sources + sky), NOT the floored lvl — so low-light
	// amplifies actual light and never a darkvision/thermographic floor, which
	// synthesize sight from none.
	return liftLowLight(resolver, lvl, resolver.Effective(room, sources, light.Black))
}

// EffectiveLightForPeriod is EffectiveLight resolved against an explicit
// time-of-day period rather than the clock's current one — the §6
// transition driver computes a viewer's level under the previous and
// new periods with it. Returns light.Lit when the resolver is nil.
func EffectiveLightForPeriod(resolver *light.Resolver, room *world.Room, viewer LightViewer, items *entities.Store, placement *entities.Placement, period string) light.Level {
	if resolver == nil || room == nil {
		return light.Lit
	}
	sources, floor, lowLight := viewerLightTerms(resolver, viewer, room, items, placement)
	lvl := resolver.EffectiveForPeriod(room, sources, floor, period)
	if !lowLight {
		return lvl
	}
	return liftLowLight(resolver, lvl, resolver.EffectiveForPeriod(room, sources, light.Black, period))
}

// visionCapable is the optional capability view supplying gear-sourced
// vision modes: a viewer that can report whether any equipped item
// grants a named capability (a cybereye enhancement's `grants`, per
// item-modification §6). Structurally satisfied by the session actor;
// mobs and login-time viewers that lack it simply contribute no
// gear-sourced vision.
type visionCapable interface {
	HasEquippedCapability(key string) bool
}

// viewerLightTerms gathers the per-viewer Sources and ViewerFloor terms
// (held light + room luminous items; darkvision + thermographic floor)
// plus whether the viewer has low-light vision, shared by the clock and
// period-explicit resolutions. Vision modes are dual-sourced: a racial
// tag (like darkvision — an SR metatype declares thermographic/low-light
// in its flags) OR an equipped capability (a cybereye enhancement).
func viewerLightTerms(resolver *light.Resolver, viewer LightViewer, room *world.Room, items *entities.Store, placement *entities.Placement) (sources, floor light.Level, lowLight bool) {
	sources = gatherRoomSources(viewer, room, items, placement)
	hasDarkvision := false
	var effectFlags []string
	if t, ok := viewer.(taggable); ok {
		hasDarkvision = t.HasTag(light.DarkvisionFlag)
		if t.HasTag(light.ThermographicFlag) {
			effectFlags = append(effectFlags, light.ThermographicFlag)
		}
		if t.HasTag(light.LowLightFlag) {
			lowLight = true
		}
	}
	if v, ok := viewer.(visionCapable); ok {
		if v.HasEquippedCapability(light.ThermographicFlag) {
			effectFlags = append(effectFlags, light.ThermographicFlag)
		}
		if v.HasEquippedCapability(light.LowLightFlag) {
			lowLight = true
		}
	}
	floor = resolver.Config().ViewerFloor(hasDarkvision, effectFlags)
	return sources, floor, lowLight
}

// liftLowLight raises a low-light viewer to the configured LowLightFloor
// when the room affords real light — roomLight is the level resolved with
// NO viewer floor (the actual photons: held/room sources + sky), so
// low-light amplifies genuine faint light (≥ Gloom) and never a
// darkvision/thermographic floor, which synthesize sight from none. A
// room with no real light (roomLight Black) leaves a low-light viewer
// exactly where the floor left them — a pitch-black room stays black.
// Only called when the viewer has low-light; a no-op if lvl already meets
// the floor.
func liftLowLight(resolver *light.Resolver, lvl, roomLight light.Level) light.Level {
	if f := resolver.Config().LowLightFloor; roomLight >= light.Gloom && lvl < f {
		return f
	}
	return lvl
}

// gatherRoomSources returns the brightest lit-source contribution for a
// viewer in room: the source in their light slot (only the slotted
// source lights its bearer, §3.3) plus any luminous items lying in the
// room. A viewer with no equipment (a mob) contributes only via room
// items; mobs as luminous sources are a future addition.
func gatherRoomSources(viewer LightViewer, room *world.Room, items *entities.Store, placement *entities.Placement) light.Level {
	best := light.Black
	if eq, ok := viewer.(equipmentHolder); ok && items != nil {
		if id, ok := eq.Equipment()[lightSlotKey]; ok {
			if it, ok := itemInstanceByID(items, id); ok {
				if c := light.Contribution(it); c > best {
					best = c
				}
			}
		}
	}
	if placement != nil && items != nil && room != nil {
		for _, id := range placement.InRoom(room.ID) {
			it, ok := itemInstanceByID(items, id)
			if !ok {
				continue
			}
			if c := light.Contribution(it); c > best {
				best = c
			}
		}
	}
	return best
}

// itemInstanceByID resolves an id to a live *ItemInstance, or (nil,
// false) when absent / not an item.
func itemInstanceByID(items *entities.Store, id entities.EntityID) (*entities.ItemInstance, bool) {
	e, ok := items.GetByID(id)
	if !ok {
		return nil, false
	}
	it, ok := e.(*entities.ItemInstance)
	return it, ok
}

// effectiveLight is the Context-scoped convenience wrapper over
// EffectiveLight, reading the resolver + stores from c.
func (c *Context) effectiveLight(room *world.Room) light.Level {
	return EffectiveLight(c.Light, room, c.Actor, c.Items, c.Placement)
}

// resolveCarriedOrEquipped resolves a keyword phrase against the union
// of the actor's top-level inventory and currently-equipped items,
// inventory first (deterministic). Returns the live instance. Mirrors
// unequip's manual resolution but widened to include carried items so
// the light verbs work on a torch in the pack or in the light slot.
func resolveCarriedOrEquipped(c *Context, phrase string) (*entities.ItemInstance, bool) {
	seen := make(map[entities.EntityID]struct{})
	var cands []*entities.ItemInstance
	add := func(id entities.EntityID) {
		if _, dup := seen[id]; dup {
			return
		}
		if e, ok := c.Items.GetByID(id); ok {
			if it, ok := e.(*entities.ItemInstance); ok {
				seen[id] = struct{}{}
				cands = append(cands, it)
			}
		}
	}
	for _, id := range c.Actor.Inventory() {
		add(id)
	}
	for _, k := range sortedSlotKeys(c.Actor.Equipment()) {
		add(c.Actor.Equipment()[k])
	}
	if len(cands) == 0 {
		return nil, false
	}
	match := keyword.Resolve(asNamed(cands), phrase)
	if match == nil {
		return nil, false
	}
	it, ok := match.(*entities.ItemInstance)
	return it, ok
}
