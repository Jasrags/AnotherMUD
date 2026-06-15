package command

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/visibility"
)

// This file bridges the engine's concrete types to the decoupled
// visibility filter (internal/visibility) and builds the per-room
// ResolveContext.CanSee predicate (visibility §2, §5.4).
//
// Slice 2 wires the ONE concealment source that already has a shipped
// substrate — darkness, via internal/light. Hide / sneak / invisibility
// (later slices) extend visObserver's capabilities and the layer
// assembly below; the seam (the predicate + the resolver filtering) does
// not move.

// visObserver adapts a viewer to visibility.Observer. Slice 2 only needs
// PiercesDarkness (from the light system); the other capabilities are
// stubbed at their "pierces nothing" defaults and grow as hide/sneak/
// invis land. AlreadyPierced/Contest are never invoked while no roll-gated
// layer exists, so they return false (no detection set yet).
type visObserver struct {
	id          string
	piercesDark bool
}

func (o visObserver) VisibilityID() string          { return o.id }
func (o visObserver) Bypass() bool                  { return false }
func (o visObserver) PiercesDarkness() bool         { return o.piercesDark }
func (o visObserver) SeesInvisible() bool           { return false }
func (o visObserver) AdminRank() int                { return 0 }
func (o visObserver) DetectsHidden() bool           { return false }
func (o visObserver) AlreadyPierced(uint64) bool    { return false }
func (o visObserver) Contest(visibility.Layer) bool { return false }

// visTarget adapts a room occupant to visibility.Target, carrying the
// concealment layers the caller assembled for it.
type visTarget struct {
	id     string
	layers []visibility.Layer
}

func (t visTarget) VisibilityID() string                  { return t.id }
func (t visTarget) ConcealmentLayers() []visibility.Layer { return t.layers }

// visibilityPredicate builds the ResolveContext.CanSee closure for the
// actor's current room (visibility §5.4), or nil when nothing is concealed
// — the legacy permissive path, so the common case (a lit room) allocates
// no closure and the resolvers skip filtering entirely.
//
// Slice 2 scope — darkness only: if the actor's effective light is Black
// (pitch dark, the light system already withholds the room render), every
// non-luminous occupant is concealed; a luminous item (a dropped lit
// torch) stays visible — you see the glow. At any tier above Black the
// actor sees adequately (light/darkvision already folded into
// EffectiveLight), so there is no concealment and this returns nil.
func (c *Context) visibilityPredicate() func(string) bool {
	if c.Light == nil || c.Actor == nil {
		return nil
	}
	room := c.Actor.Room()
	if room == nil {
		return nil
	}
	if EffectiveLight(c.Light, room, c.Actor, c.Items, c.Placement) > light.Black {
		return nil // the actor sees adequately — no concealment (legacy path)
	}
	// piercesDark is false here by construction, NOT a stub: EffectiveLight is
	// PER-VIEWER and already max-combines this actor's darkvision floor
	// (light.Resolve: eff = max(ambient, sources, viewerFloor); darkvision's
	// floor is Gloom > Black). So a darkvision (or lit) actor never reaches
	// this branch — their effective light is ≥ Gloom and the predicate is nil
	// above. Reaching here means *this* actor's own effective light is Black,
	// i.e. they genuinely cannot pierce the dark, so false is correct.
	obs := visObserver{id: c.Actor.PlayerID(), piercesDark: false}
	items := c.Items
	return func(id string) bool {
		var layers []visibility.Layer
		if !luminousItemID(items, id) {
			layers = []visibility.Layer{{Source: visibility.SourceDarkness}}
		}
		return visibility.CanSee(obs, visTarget{id: id, layers: layers})
	}
}

// luminousItemID reports whether the id names a room item that emits light
// (a lit torch on the ground) — such a target is visible in the dark to
// anyone (visibility §3.3). Mobs and players are not luminous in v1, so a
// non-item id is never luminous.
//
// Precondition: callers pass only IDs that came from the room's candidate
// lists (RoomItems/RoomEntities), which BuildResolveContext already filters
// to room-placed entities. The luminosity check itself reads c.Items by id
// and does NOT re-validate room placement, so passing an arbitrary in-world
// item id (e.g. one in an inventory) would report its raw lit state — fine
// for the current single caller (the predicate, fed only room candidates),
// but revisit this if the helper gains other callers.
func luminousItemID(items *entities.Store, id string) bool {
	if items == nil {
		return false
	}
	it, ok := itemInstanceByID(items, entities.EntityID(id))
	if !ok {
		return false
	}
	return light.Contribution(it) > light.Black
}
