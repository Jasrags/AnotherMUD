package command

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Reserved property/tag keys consulted by FillHandler (spec
// inventory-equipment-items §4.6).
const (
	propMaxCharges = "max_charges"
	propCharges    = "charges"
	propFillSource = "fill_source"
	propFillSupply = "fill_supply"
	propFillType   = "fill_type"
	tagFillSource  = "fill_source"
	// defaultFillType is the spec fallback when a source carries only
	// the `fill_source` *tag* with no `fill_source` *property* naming a
	// liquid (§4.6 step 2 — "fall back to water").
	defaultFillType = "water"
)

// FillHandler implements `fill <target> [from] <source>` (spec
// inventory-equipment-items §4.6).
//
// Validation order matches §4.6:
//  1. Resolve target against the actor's inventory.
//  2. Resolve source against items placed in the actor's current room.
//     §4.6 explicitly scopes sources to room-static entities; the
//     fill-from-container case is an open question and not in scope.
//  3. `not_fillable` — target MUST declare `max_charges`.
//  4. `no_fill_source` — source MUST declare a `fill_source` property
//     OR carry the `fill_source` tag (in which case the fill type
//     defaults to "water").
//  5. `source_empty` — if source has `fill_supply` and it is <= 0.
//  6. `mixed_liquids` — target's `fill_type` differs from source's
//     AND target's current `charges` > 0. Empty targets accept any
//     liquid; matching liquids top up freely.
//  7. Mutate: target.charges = target.max_charges; target.fill_type =
//     source liquid; if source declared fill_supply, decrement it.
//  8. Publish `item.filled`.
//
// Persistence caveat: per-instance properties on items do not survive
// logout today (player save tree records template ids only). A filled
// waterskin is observable within a session but resets on next login;
// noted as deferred until instance-state persistence lands.
func FillHandler(ctx context.Context, c *Context) error {
	if c.Items == nil {
		return c.Actor.Write(ctx, "You can't fill anything right now.")
	}
	targetArg, sourceArg, ok := parseFillArgs(c.Args)
	if !ok {
		return c.Actor.Write(ctx, "Fill what from what?")
	}

	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You float in formless void.")
	}

	carried := collectItems(c.Items, c.Actor.Inventory())
	if len(carried) == 0 {
		return c.Actor.Write(ctx, "You aren't carrying anything.")
	}
	targetMatch := keyword.Resolve(asNamed(carried), targetArg)
	if targetMatch == nil {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
	target := targetMatch.(*entities.ItemInstance)

	// §4.6 step 1 — not_fillable. Resolved before source lookup so a
	// player who types `fill rock fountain` gets the target-side error
	// regardless of whether the named source exists in the room. The
	// spec lists target validation first; checking it before any room
	// scan also avoids surfacing a misleading "nothing here" message
	// when the real problem is the target.
	maxCharges := intProp(target, propMaxCharges)
	if maxCharges <= 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s isn't something you can fill.", target.Name()))
	}

	sources := roomItems(c, room.ID)
	if len(sources) == 0 {
		return c.Actor.Write(ctx, "There is nothing here you could fill from.")
	}
	sourceMatch := keyword.Resolve(asNamed(sources), sourceArg)
	if sourceMatch == nil {
		return c.Actor.Write(ctx, "You don't see that here.")
	}
	source := sourceMatch.(*entities.ItemInstance)

	// §4.6 step 2 — no_fill_source. Resolve the liquid label: a
	// `fill_source` property names it directly; falling back to the
	// `fill_source` tag yields the default ("water"). A source with
	// neither is not a fill source at all.
	fillType, hasFillSrc := sourceFillType(source)
	if !hasFillSrc {
		return c.Actor.Write(ctx, fmt.Sprintf("%s isn't a source of anything.", source.Name()))
	}

	// §4.6 step 3 — source_empty. A missing `fill_supply` means
	// infinite (the typical fountain case); a present value <= 0 means
	// the source is tapped out.
	if hasFillSupply(source) && intProp(source, propFillSupply) <= 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is empty.", source.Name()))
	}

	// §4.6 step 4 — mixed_liquids. The guard only fires when the
	// target *currently* holds a different liquid (charges > 0). An
	// empty target re-typing happens for free.
	if existing, ok := stringProp(target, propFillType); ok && existing != fillType {
		if intProp(target, propCharges) > 0 {
			return c.Actor.Write(ctx,
				fmt.Sprintf("%s already holds %s; you can't mix it with %s.",
					target.Name(), existing, fillType))
		}
	}

	// §4.6 steps 5-6 — mutate target, then decrement source supply if
	// it was a finite source. Order is target-first so a future
	// supply-write failure (currently impossible — these are in-memory
	// maps) doesn't leave the target unset.
	target.SetProperty(propCharges, maxCharges)
	target.SetProperty(propFillType, fillType)
	// Per-instance property mutation only marks dirty in the save tree
	// if MarkContentsDirty is wired to walk properties. Today the save
	// tree carries template ids only, so this Mutation is intentionally
	// not persisted; see persistence caveat in the package comment.

	if hasFillSupply(source) {
		source.SetProperty(propFillSupply, intProp(source, propFillSupply)-1)
	}

	_ = c.Actor.Write(ctx, fmt.Sprintf("You fill %s with %s from %s.",
		target.Name(), fillType, source.Name()))
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s fills %s from %s.", c.Actor.Name(), target.Name(), source.Name()),
			c.Actor.PlayerID())
	}
	c.Publish(ctx, eventbus.ItemFilled{
		ActorID:  holderEntityIDForPlayer(c.Actor.PlayerID()),
		SourceID: source.ID(),
		TargetID: target.ID(),
		RoomID:   room.ID,
		FillType: fillType,
	})
	return nil
}

// roomItems returns the item instances placed top-level in roomID.
// "Top-level" excludes anything nested inside a container — fill
// sources are static room entities (§4.6), so an item that's been put
// inside a sack is not a candidate.
func roomItems(c *Context, roomID world.RoomID) []*entities.ItemInstance {
	if c.Placement == nil {
		return nil
	}
	out := make([]*entities.ItemInstance, 0)
	for _, id := range c.Placement.InRoom(roomID) {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		if c.Contents != nil {
			if _, nested := c.Contents.ContainerOf(it.ID()); nested {
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

// sourceFillType reports the liquid label this source produces, or
// false if the source is not a fill source at all. The property wins
// over the tag — a `fill_source: wine` property on a source also
// tagged `fill_source` produces "wine," not "water."
func sourceFillType(it *entities.ItemInstance) (string, bool) {
	if v, ok := stringProp(it, propFillSource); ok && v != "" {
		return v, true
	}
	if slices.Contains(it.Tags(), tagFillSource) {
		return defaultFillType, true
	}
	return "", false
}

// hasFillSupply reports whether the source declares a `fill_supply`
// property at all. A source without the property is treated as
// infinite; a source with the property and value <= 0 is empty.
func hasFillSupply(it *entities.ItemInstance) bool {
	_, ok := it.Property(propFillSupply)
	return ok
}

// stringProp returns the string-typed value at key, or ("", false) if
// the property is missing or non-string.
func stringProp(it *entities.ItemInstance, key string) (string, bool) {
	v, ok := it.Property(key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
}

// parseFillArgs splits the verb's arguments into (targetArg,
// sourceArg). Returns ok=false when fewer than two meaningful tokens
// are present. Mirrors parsePutArgs' shape with "from" instead of
// "in"/"into".
//
// Forms accepted:
//
//	fill skin fountain        → target="skin",       source="fountain"
//	fill skin from fountain   → target="skin",       source="fountain"
//	fill water skin from well → target="water skin", source="well"
//
// The explicit "from" wins when present; otherwise the last token is
// the source. Standalone "from" as the literal source (`fill x from`)
// is treated as missing.
func parseFillArgs(args []string) (targetArg, sourceArg string, ok bool) {
	if len(args) < 2 {
		return "", "", false
	}
	for i := len(args) - 1; i >= 1; i-- {
		if !strings.EqualFold(args[i], "from") {
			continue
		}
		if i == len(args)-1 {
			return "", "", false
		}
		targetArg = strings.Join(args[:i], " ")
		sourceArg = strings.Join(args[i+1:], " ")
		if targetArg == "" || sourceArg == "" {
			return "", "", false
		}
		return targetArg, sourceArg, true
	}
	targetArg = strings.Join(args[:len(args)-1], " ")
	sourceArg = args[len(args)-1]
	return targetArg, sourceArg, true
}
