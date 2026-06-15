package command

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/visibility"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// This file bridges the engine's concrete types to the decoupled visibility
// filter (internal/visibility) and builds the per-room visibility predicate
// (visibility §2, §4, §5.4). The SAME predicate gates both command target
// resolution (ResolveContext.CanSee) and the room render occupant list, so
// "what you can target" and "what you can see" stay consistent — and the
// observer's sticky detection set converges across both.
//
// Sources wired so far: darkness (S2, via internal/light) and hide (S3).
// Sneak / invisibility (later slices) extend the layer assembly + observer
// capabilities below; the predicate seam does not move.

// hideable is the target-read capability: an occupant that may be hide-
// concealed (visibility §3.1). connActor implements it; non-implementers are
// treated as never hidden. Optional interface (the LightViewer pattern) so
// the broad Actor interface need not grow.
type hideable interface {
	IsHidden() bool
	ConcealmentScore() int
	HiddenInstance() uint64
}

// perceiver is the observer-side capability: perception + sticky detection
// memory for the §4.2 contest (visibility §4.1). connActor implements it; a
// nil perceiver cannot pierce roll-gated concealment (it never wins a
// contest), which is the correct degraded behavior for a viewer with no
// perception wired (tests).
type perceiver interface {
	PerceptionBonus() int
	HasPiercedConcealment(instance uint64) bool
	RecordConcealmentPierce(instance uint64)
}

// visObserver adapts a viewer to visibility.Observer. PiercesDarkness comes
// from the light system; the roll-gated path (hide/sneak) delegates to the
// optional perceiver + a d20 roller. SeesInvisible/AdminRank/DetectsHidden
// stay at "pierces nothing" until the invisibility/detect-trait slices.
type visObserver struct {
	id          string
	piercesDark bool
	per         perceiver          // nil ⇒ cannot pierce roll-gated concealment
	roller      progression.Roller // nil ⇒ ditto (no contest possible)
}

func (o visObserver) VisibilityID() string  { return o.id }
func (o visObserver) Bypass() bool          { return false }
func (o visObserver) PiercesDarkness() bool { return o.piercesDark }
func (o visObserver) SeesInvisible() bool   { return false }
func (o visObserver) AdminRank() int        { return 0 }
func (o visObserver) DetectsHidden() bool   { return false }

func (o visObserver) AlreadyPierced(instance uint64) bool {
	return o.per != nil && o.per.HasPiercedConcealment(instance)
}

// Contest runs the §4.2 perception contest (d20 + perception vs the layer's
// concealment score, reusing the skill-check primitive) and records a win in
// the observer's detection set so subsequent checks skip the roll (§4.1).
// Without a perceiver or roller the observer cannot pierce — returns false.
func (o visObserver) Contest(layer visibility.Layer) bool {
	if o.per == nil || o.roller == nil {
		return false
	}
	if progression.ResolveSkillCheck(o.roller, o.per.PerceptionBonus(), layer.Score).Success {
		o.per.RecordConcealmentPierce(layer.Instance)
		return true
	}
	return false
}

// visTarget adapts a room occupant to visibility.Target, carrying the
// concealment layers the caller assembled for it.
type visTarget struct {
	id     string
	layers []visibility.Layer
}

func (t visTarget) VisibilityID() string                  { return t.id }
func (t visTarget) ConcealmentLayers() []visibility.Layer { return t.layers }

// visibilityPredicate builds the per-room CanSee closure for the actor
// (visibility §4, §5.4), or nil when nothing in the room is concealed from
// this viewer — the legacy permissive path, so a plain lit room with no
// hidden occupants allocates no closure and consumers skip filtering.
//
// Two sources compose (AND, via the filter): darkness (the viewer's
// effective light is Black → non-luminous occupants concealed; §3.3) and
// hide (a room occupant carrying the `hidden` tag → concealed behind a
// perception contest; §3.1, §4.2). A luminous item is visible in the dark to
// anyone; the viewer's own perception + sticky memory pierce hides.
func (c *Context) visibilityPredicate() func(string) bool {
	if c.Actor == nil {
		return nil
	}
	room := c.Actor.Room()
	if room == nil {
		return nil
	}

	// Darkness term: the viewer's per-viewer effective light (darkvision +
	// carried light already folded in). Black ⇒ darkness conceals.
	lvl := light.Lit
	if c.Light != nil {
		lvl = EffectiveLight(c.Light, room, c.Actor, c.Items, c.Placement)
	}
	dark := lvl <= light.Black

	// Hide term: the hidden occupants of the room and their concealment
	// layers, keyed by entity id. Built once so the closure is a cheap map
	// lookup per candidate.
	hidden := hiddenOccupants(c, room.ID)

	if !dark && len(hidden) == 0 {
		return nil // nothing concealed from this viewer
	}

	obs := visObserver{
		id:          c.Actor.PlayerID(),
		piercesDark: !dark, // sees adequately when above Black (§3.3)
		roller:      c.SkillRoller,
	}
	if p, ok := c.Actor.(perceiver); ok {
		obs.per = p
	}
	items := c.Items
	return func(id string) bool {
		var layers []visibility.Layer
		if dark && !luminousItemID(items, id) {
			layers = append(layers, visibility.Layer{Source: visibility.SourceDarkness})
		}
		if hl, ok := hidden[id]; ok {
			layers = append(layers, hl)
		}
		return visibility.CanSee(obs, visTarget{id: id, layers: layers})
	}
}

// hiddenOccupants returns the hide concealment layer of every currently-
// hidden player in the room, keyed by player id (visibility §3.1). Empty
// when the locator is unwired or no one is hidden. Mobs are not hideable in
// v1, so only players contribute (§9).
func hiddenOccupants(c *Context, roomID world.RoomID) map[string]visibility.Layer {
	if c.Locator == nil {
		return nil
	}
	var out map[string]visibility.Layer
	for _, p := range c.Locator.PlayersInRoom(roomID) {
		h, ok := p.(hideable)
		if !ok || !h.IsHidden() {
			continue
		}
		if out == nil {
			out = make(map[string]visibility.Layer)
		}
		out[p.PlayerID()] = visibility.Layer{
			Source:   visibility.SourceHide,
			Score:    h.ConcealmentScore(),
			Instance: h.HiddenInstance(),
		}
	}
	return out
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
// for the current callers (the predicate, fed only room candidates), but
// revisit this if the helper gains other callers.
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
