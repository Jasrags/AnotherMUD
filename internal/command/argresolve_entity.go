package command

import (
	"errors"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/keyword"
)

// M17.2b — entity / inventory / room resolvers.
//
// These resolvers need to look into the actor's contents and the
// current room. The driver supplies them via ResolveContext on
// ResolverInput; each resolver narrows the candidate set by
// scope (inventory / room items / room entities) and applies the
// shared keyword.Resolve match chain.
//
// Result shapes mirror spec §5.6: item-flavored types return
// `ItemRef`, entity-flavored types return `EntityRef`, the
// `visible` type returns `VisibleRef` carrying a source
// discriminator. Pack handlers and Lua scripts (M17.2d) read
// these directly from the resolved-args map.

// ItemCandidate is the per-item shape resolvers need: keyword
// matching (via the keyword.Named methods) plus the runtime id +
// template id for the result struct. ItemInstance from
// internal/entities satisfies it; tests use a minimal fake.
type ItemCandidate interface {
	keyword.Named
	EntityID() string
	TemplateID() string
}

// EntityCandidate is the per-entity shape for player / mob room
// occupants. EntityType is "player" or "mob" — used by the player
// / npc resolvers to filter and by the entity / visible resolvers
// to populate the Type field on the result.
type EntityCandidate interface {
	keyword.Named
	EntityID() string
	EntityType() string
}

// ContainerCandidate identifies an ItemCandidate that can hold
// other items. Used by the container resolver to filter the
// inventory / room scopes before keyword matching.
type ContainerCandidate interface {
	ItemCandidate
	IsContainer() bool
}

// ResolveContext is the per-resolve scope the driver hands to
// every resolver. Each slice is the pre-filtered candidate set
// for one scope:
//
//   - Inventory: items the actor is carrying (NOT equipped).
//   - RoomItems: non-actor items in the current room.
//   - RoomEntities: players + mobs in the current room,
//     excluding the actor self.
//
// ActorName + ActorID let the visible resolver tag self-matches.
// All fields are zero-valued by default — a caller that passes
// the zero ResolveContext just gets the keyword/text/number
// resolvers (which ignore Context).
type ResolveContext struct {
	Inventory    []ItemCandidate
	RoomItems    []ItemCandidate
	RoomEntities []EntityCandidate

	// ActorName + ActorID feed the visible resolver's "self"
	// source tag. Empty disables self-matching at the visible
	// path (player/npc/entity/inventory/room_item paths don't
	// consult these).
	ActorName string
	ActorID   string
}

// ItemRef is the spec §5.6 resolved shape for the item-flavored
// types (inventory / room_item / container / findable). All
// fields are populated from the matched candidate; Keyword is
// the best-effort canonical keyword (the first entry of
// candidate.Keywords()).
type ItemRef struct {
	ID         string
	Name       string
	Keyword    string
	TemplateID string
}

// EntityRef is the spec §5.6 resolved shape for the entity-
// flavored types (entity / player / npc). EntityType is the
// candidate's reported type — typically "player" or "mob".
type EntityRef struct {
	ID   string
	Name string
	Type string
}

// VisibleRef extends EntityRef with the §5.6 `source`
// discriminator so commands can render differently based on
// where the match came from. Source is one of:
//   - "self"      — input matched the actor's own name/keyword
//   - "inventory" — an item the actor is carrying
//   - "room"      — an item or entity in the room
type VisibleRef struct {
	EntityRef
	Source string
}

// Standard not-found error sentinels for the M17.2b resolvers.
// These surface as the spec's per-type "default not-found error"
// when ResolveArgs wraps them in an ArgResolveError; tests match
// the sentinel directly to keep diagnostics readable.
var (
	ErrItemNotInInventory = errors.New("You aren't carrying that.")
	ErrItemNotInRoom      = errors.New("You don't see that here.")
	ErrEntityNotInRoom    = errors.New("You don't see that here.")
	ErrPlayerNotInRoom    = errors.New("No player by that name.")
	ErrNpcNotInRoom       = errors.New("No such mob here.")
	ErrContainerNotFound  = errors.New("You don't see a container by that name.")
	ErrNotVisible         = errors.New("You don't see that.")
	ErrNotFindable        = errors.New("You don't see that.")
)

// --- Resolvers ---

// resolveInventory matches input against the actor's carried
// items. Spec §5.2 inventory row.
func resolveInventory(in ResolverInput) (ResolverOutput, error) {
	cand := itemsAsNamed(in.Context.Inventory)
	match := keyword.Resolve(cand, in.Tokens[0])
	if match == nil {
		return ResolverOutput{}, ErrItemNotInInventory
	}
	item := match.(ItemCandidate)
	return ResolverOutput{Value: itemRefFrom(item), Consumed: 1}, nil
}

// resolveRoomItem matches input against the non-actor items in
// the current room.
func resolveRoomItem(in ResolverInput) (ResolverOutput, error) {
	cand := itemsAsNamed(in.Context.RoomItems)
	match := keyword.Resolve(cand, in.Tokens[0])
	if match == nil {
		return ResolverOutput{}, ErrItemNotInRoom
	}
	item := match.(ItemCandidate)
	return ResolverOutput{Value: itemRefFrom(item), Consumed: 1}, nil
}

// resolveEntity matches input against any player or mob in the
// current room. Self is intentionally excluded from candidates
// (spec §5.2 note) — `kill self` must be the handler's explicit
// concern.
func resolveEntity(in ResolverInput) (ResolverOutput, error) {
	cand := entitiesAsNamed(in.Context.RoomEntities)
	match := keyword.Resolve(cand, in.Tokens[0])
	if match == nil {
		return ResolverOutput{}, ErrEntityNotInRoom
	}
	ent := match.(EntityCandidate)
	return ResolverOutput{Value: entityRefFrom(ent), Consumed: 1}, nil
}

// resolvePlayer filters room entities to players only, then
// keyword-matches.
func resolvePlayer(in ResolverInput) (ResolverOutput, error) {
	filtered := filterEntityType(in.Context.RoomEntities, "player")
	match := keyword.Resolve(entitiesAsNamed(filtered), in.Tokens[0])
	if match == nil {
		return ResolverOutput{}, ErrPlayerNotInRoom
	}
	ent := match.(EntityCandidate)
	return ResolverOutput{Value: entityRefFrom(ent), Consumed: 1}, nil
}

// resolveNPC filters room entities to mobs only.
func resolveNPC(in ResolverInput) (ResolverOutput, error) {
	filtered := filterEntityType(in.Context.RoomEntities, "mob")
	match := keyword.Resolve(entitiesAsNamed(filtered), in.Tokens[0])
	if match == nil {
		return ResolverOutput{}, ErrNpcNotInRoom
	}
	ent := match.(EntityCandidate)
	return ResolverOutput{Value: entityRefFrom(ent), Consumed: 1}, nil
}

// resolveContainer tries inventory containers first, then room
// containers. Filters via the ContainerCandidate interface so
// non-container items aren't even considered.
func resolveContainer(in ResolverInput) (ResolverOutput, error) {
	// Inventory containers first (spec §5.2 row).
	if match := keyword.Resolve(containersAsNamed(in.Context.Inventory), in.Tokens[0]); match != nil {
		item := match.(ItemCandidate)
		return ResolverOutput{Value: itemRefFrom(item), Consumed: 1}, nil
	}
	if match := keyword.Resolve(containersAsNamed(in.Context.RoomItems), in.Tokens[0]); match != nil {
		item := match.(ItemCandidate)
		return ResolverOutput{Value: itemRefFrom(item), Consumed: 1}, nil
	}
	return ResolverOutput{}, ErrContainerNotFound
}

// resolveVisible scans self → inventory → room items → room
// entities, tagging the result's Source. The spec wording is
// "any visible entity ... with source tag"; we order
// self-first so commands like `look at <self-keyword>` resolve
// the player rather than a same-keyword room item.
func resolveVisible(in ResolverInput) (ResolverOutput, error) {
	token := in.Tokens[0]
	// Self check: literal name match (case-insensitive). The
	// keyword resolver doesn't run because Self isn't a Named
	// implementation — we keep the self surface narrow.
	if in.Context.ActorName != "" && strings.EqualFold(token, in.Context.ActorName) {
		return ResolverOutput{
			Value: VisibleRef{
				EntityRef: EntityRef{
					ID:   in.Context.ActorID,
					Name: in.Context.ActorName,
					Type: "player",
				},
				Source: "self",
			},
			Consumed: 1,
		}, nil
	}
	if match := keyword.Resolve(itemsAsNamed(in.Context.Inventory), token); match != nil {
		item := match.(ItemCandidate)
		return ResolverOutput{
			Value: VisibleRef{
				EntityRef: EntityRef{
					ID:   item.EntityID(),
					Name: item.Name(),
					Type: "item",
				},
				Source: "inventory",
			},
			Consumed: 1,
		}, nil
	}
	if match := keyword.Resolve(itemsAsNamed(in.Context.RoomItems), token); match != nil {
		item := match.(ItemCandidate)
		return ResolverOutput{
			Value: VisibleRef{
				EntityRef: EntityRef{
					ID:   item.EntityID(),
					Name: item.Name(),
					Type: "item",
				},
				Source: "room",
			},
			Consumed: 1,
		}, nil
	}
	if match := keyword.Resolve(entitiesAsNamed(in.Context.RoomEntities), token); match != nil {
		ent := match.(EntityCandidate)
		return ResolverOutput{
			Value: VisibleRef{
				EntityRef: entityRefFrom(ent),
				Source:    "room",
			},
			Consumed: 1,
		}, nil
	}
	return ResolverOutput{}, ErrNotVisible
}

// resolveFindable scans inventory → room items, returning an
// ItemRef. Like container but without the IsContainer filter.
func resolveFindable(in ResolverInput) (ResolverOutput, error) {
	if match := keyword.Resolve(itemsAsNamed(in.Context.Inventory), in.Tokens[0]); match != nil {
		item := match.(ItemCandidate)
		return ResolverOutput{Value: itemRefFrom(item), Consumed: 1}, nil
	}
	if match := keyword.Resolve(itemsAsNamed(in.Context.RoomItems), in.Tokens[0]); match != nil {
		item := match.(ItemCandidate)
		return ResolverOutput{Value: itemRefFrom(item), Consumed: 1}, nil
	}
	return ResolverOutput{}, ErrNotFindable
}

// --- helpers ---

func itemsAsNamed(items []ItemCandidate) []keyword.Named {
	out := make([]keyword.Named, len(items))
	for i, it := range items {
		out[i] = it
	}
	return out
}

func entitiesAsNamed(ents []EntityCandidate) []keyword.Named {
	out := make([]keyword.Named, len(ents))
	for i, e := range ents {
		out[i] = e
	}
	return out
}

// containersAsNamed filters a candidate list down to its
// containers (per ContainerCandidate.IsContainer) and wraps as
// the keyword.Named slice the resolver consumes. Items that
// don't satisfy ContainerCandidate are skipped — they're
// non-containers by definition.
func containersAsNamed(items []ItemCandidate) []keyword.Named {
	out := make([]keyword.Named, 0, len(items))
	for _, it := range items {
		c, ok := it.(ContainerCandidate)
		if !ok || !c.IsContainer() {
			continue
		}
		out = append(out, it)
	}
	return out
}

func filterEntityType(ents []EntityCandidate, kind string) []EntityCandidate {
	out := make([]EntityCandidate, 0, len(ents))
	for _, e := range ents {
		if e.EntityType() == kind {
			out = append(out, e)
		}
	}
	return out
}

func itemRefFrom(item ItemCandidate) ItemRef {
	var kw string
	if kws := item.Keywords(); len(kws) > 0 {
		kw = kws[0]
	}
	return ItemRef{
		ID:         item.EntityID(),
		Name:       item.Name(),
		Keyword:    kw,
		TemplateID: item.TemplateID(),
	}
}

func entityRefFrom(ent EntityCandidate) EntityRef {
	return EntityRef{
		ID:   ent.EntityID(),
		Name: ent.Name(),
		Type: ent.EntityType(),
	}
}
