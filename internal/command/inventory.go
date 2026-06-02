package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/corpse"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// coinKeywords are the reserved tokens that, as the item argument of
// `get … from <corpse>`, take the corpse's coin pile rather than an
// item (loot-and-corpses §5.2).
var coinKeywords = map[string]bool{"coins": true, "coin": true, "gold": true, "money": true}

// Reserved tags that gate `get` (spec inventory-equipment-items §4.2
// step 1). Items carrying either tag never leave a room via pick-up;
// fixture is for in-world dressing (signs, statues), no_get is for
// quest-bound objects that should appear in inventory listings only
// when granted by another mechanism.
const (
	tagFixture = "fixture"
	tagNoGet   = "no_get"
)

// GetHandler implements `get <item>` (room pickup, spec §4.2) and
// `get <item|coins> from <container>` (container extraction, §5.2).
//
// Hand-parsed rather than declarative: the item's scope is conditional
// on the `from` preposition — room items for the bare form, the named
// container's contents for the `from` form — which the single-scope arg
// pipeline can't express (the M17.2d "non-fit" precedent). The room form
// still resolves through resolveRoomItem so its messages and ordinal
// handling (`get 2.sword`) are unchanged.
func GetHandler(ctx context.Context, c *Context) error {
	if c.Items == nil || c.Placement == nil {
		// Sub-system not wired; fail closed rather than panic.
		return c.Actor.Write(ctx, "You can't pick anything up right now.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "There is nothing here.")
	}

	if itemToks, contToks, fromContainer := splitOnPreposition(c.Args, "from"); fromContainer {
		return c.getFromContainer(ctx, room, itemToks, contToks)
	}
	return c.getFromRoom(ctx, room, c.Args)
}

// getFromRoom is the §4.2 room pickup. Resolves the item via the shared
// resolveRoomItem (so ordinals + messages match the declarative path),
// validates the tag gate, and moves it room → inventory.
func (c *Context) getFromRoom(ctx context.Context, room *world.Room, toks []string) error {
	if len(toks) == 0 {
		return c.Actor.Write(ctx, "What item?")
	}
	out, err := resolveRoomItem(ResolverInput{Tokens: toks, Context: c.BuildResolveContext()})
	if err != nil {
		return c.Actor.Write(ctx, "You don't see that here.")
	}
	ref := out.Value.(ItemRef)
	item, ok := c.liveItem(ref.ID)
	if !ok {
		return c.Actor.Write(ctx, "You don't see that here.")
	}

	if hasAnyTag(item, tagFixture, tagNoGet) {
		return c.Actor.Write(ctx, fmt.Sprintf("You can't take %s.", item.Name()))
	}

	// Placement.Remove is the atomic ownership claim. Two concurrent
	// gets against the same placement entry both pass keyword.Resolve
	// and hasAnyTag (which run outside any lock) — only the goroutine
	// whose Remove call returns true actually got the item. The loser
	// reports the same message it would for any other resolver miss,
	// without leaking that a sibling session just snatched it.
	//
	// AddToInventory must happen AFTER the successful Remove so a
	// failed claim can't leave the item present in both inventory and
	// Placement. Broadcast follows so the actor can immediately
	// reference the item by keyword if they pipeline commands.
	//
	// The gap between Remove and AddToInventory is safe because each
	// concurrent caller owns its own Actor (one connection ↔ one
	// connActor). The only shared mutable state is Placement, which
	// the Remove already serialized. If a future path ever routes
	// two sessions through a single shared Actor (group-loot,
	// auto-split), revisit this section.
	if !c.Placement.Remove(item.ID()) {
		return c.Actor.Write(ctx, "You don't see that here.")
	}

	// Currency auto-convert (spec §2.3): a currency-tagged item with a
	// positive value is credited as gold instead of entering inventory.
	// The item is now off the floor; the hook untracks it and credits
	// the holder. We still broadcast the visible pickup (others see the
	// coins grabbed) but suppress the ItemPickedUp bus event per §2.3
	// step 7 — the currency feature emits its own currency.credited.
	if value, converted := tryAutoConvert(ctx, c, c.Actor, item); converted {
		_ = c.Actor.Write(ctx, fmt.Sprintf("You pick up %s (%d gold).", item.Name(), value))
		if c.Broadcaster != nil && c.Actor.Name() != "" {
			c.Broadcaster.SendToRoom(ctx, room.ID,
				fmt.Sprintf("%s picks up %s.", c.Actor.Name(), item.Name()),
				c.Actor.PlayerID())
		}
		return nil
	}

	c.Actor.AddToInventory(item.ID())

	_ = c.Actor.Write(ctx, fmt.Sprintf("You pick up %s.", item.Name()))
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s picks up %s.", c.Actor.Name(), item.Name()),
			c.Actor.PlayerID())
	}
	c.Publish(ctx, eventbus.ItemPickedUp{
		HolderID: holderEntityIDForPlayer(c.Actor.PlayerID()),
		RoomID:   room.ID,
		ItemID:   item.ID(),
	})
	return nil
}

// getFromContainer implements `get <item|coins> from <container>`
// (spec §5.2). The container resolves inventory-first then room (shared
// resolveContainer); a corpse additionally enforces the §4 ownership
// window. The item is keyword-matched within the container's contents.
func (c *Context) getFromContainer(ctx context.Context, room *world.Room, itemToks, contToks []string) error {
	if len(itemToks) == 0 {
		return c.Actor.Write(ctx, "What item?")
	}
	if len(contToks) == 0 {
		return c.Actor.Write(ctx, "Get it from what?")
	}
	if c.Contents == nil {
		return c.Actor.Write(ctx, "You can't take anything from there right now.")
	}

	cout, err := resolveContainer(ResolverInput{Tokens: contToks, Context: c.BuildResolveContext()})
	if err != nil {
		return c.Actor.Write(ctx, "You don't see that container here.")
	}
	container, ok := c.liveItem(cout.Value.(ItemRef).ID)
	if !ok {
		return c.Actor.Write(ctx, "You don't see that container here.")
	}

	isCorpse := corpse.IsCorpse(container)
	if isCorpse && !c.mayLootCorpse(container) {
		// §4 — refuse without naming the owner.
		return c.Actor.Write(ctx, fmt.Sprintf("You don't have the right to loot %s yet.", container.Name()))
	}

	if coinKeywords[strings.ToLower(itemToks[0])] {
		return c.getCoinsFrom(ctx, room, container, isCorpse)
	}

	contents := collectItems(c.Items, c.Contents.In(container.ID()))
	match := keyword.Resolve(asNamed(contents), itemToks[0])
	if match == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't see that in %s.", container.Name()))
	}
	item := match.(*entities.ItemInstance)

	actorEID := holderEntityIDForPlayer(c.Actor.PlayerID())
	if c.Bus != nil {
		pre := eventbus.NewContainerItemRemoving(actorEID, container.ID(), item.ID(), room.ID)
		if c.Bus.PublishCancellable(ctx, pre) {
			return c.Actor.Write(ctx, fmt.Sprintf("You can't take %s from %s right now.", item.Name(), container.Name()))
		}
	}

	// Contents.Take is the atomic single-winner claim (mirrors the loot
	// verb / GetHandler room pickup).
	if !c.Contents.Take(item.ID()) {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't see that in %s.", container.Name()))
	}
	c.Actor.AddToInventory(item.ID())

	_ = c.Actor.Write(ctx, fmt.Sprintf("You take %s from %s.", decoratedName(c, item), container.Name()))
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s takes %s from %s.", c.Actor.Name(), item.Name(), container.Name()),
			c.Actor.PlayerID())
	}
	if c.Bus != nil {
		c.Publish(ctx, eventbus.ContainerItemRemoved{
			ActorID: actorEID, ContainerID: container.ID(), ItemID: item.ID(), RoomID: room.ID,
		})
	}

	if isCorpse {
		c.removeCorpseIfEmpty(ctx, container, room.ID, 1, 0)
	}
	return nil
}

// getCoinsFrom handles the reserved coin keyword of `get coins from
// <corpse>` (spec §5.2): claims the corpse's coin pile and credits the
// actor's currency balance. Only corpses carry coins.
func (c *Context) getCoinsFrom(ctx context.Context, room *world.Room, container *entities.ItemInstance, isCorpse bool) error {
	if !isCorpse {
		return c.Actor.Write(ctx, fmt.Sprintf("There are no coins in %s.", container.Name()))
	}
	holder, ok := c.Actor.(economy.Entity)
	if c.Currency == nil || !ok {
		return c.Actor.Write(ctx, "You can't carry coins right now.")
	}
	coins := corpse.ClaimCoins(container)
	if coins <= 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("There are no coins in %s.", container.Name()))
	}
	c.Currency.AddGold(ctx, holder, coins, "loot:"+string(container.ID()))
	_ = c.Actor.Write(ctx, fmt.Sprintf("You take %d gold from %s.", coins, container.Name()))
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s takes some coins from %s.", c.Actor.Name(), container.Name()),
			c.Actor.PlayerID())
	}
	c.removeCorpseIfEmpty(ctx, container, room.ID, 0, coins)
	return nil
}

// mayLootCorpse evaluates the §4 ownership window for the acting player
// against a corpse, using the live tick.
func (c *Context) mayLootCorpse(target *entities.ItemInstance) bool {
	actorID := string(combat.NewPlayerCombatantID(c.Actor.PlayerID()))
	now := uint64(0)
	if c.NowTick != nil {
		now = c.NowTick()
	}
	return corpse.MayLoot(target, actorID, now, c.CorpseOwnershipWindow)
}

// removeCorpseIfEmpty removes a corpse drained to nothing (items + coins)
// and emits corpse.looted, single-winner via Placement.Remove so a
// piecemeal get-from, a `loot`, and the decay sweep can't double-fire.
// itemCount/coins describe the action that emptied it.
func (c *Context) removeCorpseIfEmpty(ctx context.Context, target *entities.ItemInstance, roomID world.RoomID, itemCount, coins int) {
	if len(c.Contents.In(target.ID())) != 0 || corpse.Coins(target) != 0 {
		return
	}
	if !c.Placement.Remove(target.ID()) {
		return
	}
	_ = c.Items.Untrack(target.ID())
	c.Publish(ctx, eventbus.CorpseLooted{
		CorpseID:  target.ID(),
		RoomID:    roomID,
		LooterID:  string(combat.NewPlayerCombatantID(c.Actor.PlayerID())),
		ItemCount: itemCount,
		Coins:     coins,
	})
}

// liveItem re-fetches a live *ItemInstance by id (TOCTOU guard between
// resolution and mutation).
func (c *Context) liveItem(id string) (*entities.ItemInstance, bool) {
	e, ok := c.Items.GetByID(entities.EntityID(id))
	if !ok {
		return nil, false
	}
	it, ok := e.(*entities.ItemInstance)
	return it, ok
}

// splitOnPreposition splits tokens around the first case-insensitive
// occurrence of prep: (before, after, found). When prep is absent it
// returns (toks, nil, false).
func splitOnPreposition(toks []string, prep string) (before, after []string, found bool) {
	for i, t := range toks {
		if strings.EqualFold(t, prep) {
			return toks[:i], toks[i+1:], true
		}
	}
	return toks, nil, false
}

// DropHandler implements the `drop <item>` verb (spec §4.3).
//
// Resolves against the actor's inventory, removes from contents, places
// in the current room, and broadcasts. Drop is unconditional in this
// spec (no weight gate, no no_drop tag) — gates would be policy layered
// on top.
// DropHandler is the first verb migrated onto the §5 arg-typing
// pipeline (M17.2d₂). Its single `item` argument is declared as
// ArgInventory in RegisterBuiltins, so the dispatcher resolves it
// before this runs: a missing arg ("drop") or an unmatched keyword is
// reported by the dispatcher with the resolver's standardized message
// ("What item?" / "You aren't carrying that.") and this handler is
// never reached. The handler therefore starts from a resolved ItemRef
// and re-fetches the live instance by id — the store lookup is also
// the TOCTOU guard the hand-parsed version had (the item may have left
// the inventory between resolution and now).
func DropHandler(ctx context.Context, c *Context) error {
	if c.Items == nil || c.Placement == nil {
		return c.Actor.Write(ctx, "You can't drop anything right now.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You float in formless void; there is nowhere to drop anything.")
	}

	item, ok := resolvedItemInstance(c, "item")
	if !ok {
		// Defensive: only reached if the item left the store between
		// resolution and now, or the registration's Args drifted.
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}

	if !c.Actor.RemoveFromInventory(item.ID()) {
		// Vanishingly rare: resolution found it but the inventory
		// changed between resolve and remove. Treat as failure.
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
	c.Placement.Place(item.ID(), room.ID)

	_ = c.Actor.Write(ctx, fmt.Sprintf("You drop %s.", item.Name()))
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s drops %s.", c.Actor.Name(), item.Name()),
			c.Actor.PlayerID())
	}
	c.Publish(ctx, eventbus.ItemDropped{
		HolderID: holderEntityIDForPlayer(c.Actor.PlayerID()),
		RoomID:   room.ID,
		ItemID:   item.ID(),
	})
	return nil
}

// resolvedItemInstance re-fetches the live *ItemInstance for a §5
// arg keyed by name in c.Resolved. The arg resolvers return an ItemRef
// (id + display fields); handlers that mutate the entity re-fetch the
// instance by id here. The lookup doubles as the TOCTOU guard — the
// item may have left the store between resolution and now. Returns
// false when the arg is absent / not an ItemRef, or its id no longer
// resolves to a live item instance; callers map false to the
// appropriate not-found message. Requires c.Items non-nil (every
// caller guards it before reaching here).
func resolvedItemInstance(c *Context, name string) (*entities.ItemInstance, bool) {
	ref, ok := c.Resolved[name].(ItemRef)
	if !ok {
		return nil, false
	}
	e, ok := c.Items.GetByID(entities.EntityID(ref.ID))
	if !ok {
		return nil, false
	}
	it, ok := e.(*entities.ItemInstance)
	return it, ok
}

// collectItems resolves ids through store and filters to ItemInstances,
// preserving order. Unknown ids and non-item entities are skipped
// silently — they represent index corruption, not user-visible errors.
func collectItems(store *entities.Store, ids []entities.EntityID) []*entities.ItemInstance {
	out := make([]*entities.ItemInstance, 0, len(ids))
	for _, id := range ids {
		e, ok := store.GetByID(id)
		if !ok {
			continue
		}
		item, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func asNamed(items []*entities.ItemInstance) []keyword.Named {
	out := make([]keyword.Named, len(items))
	for i, it := range items {
		out[i] = it
	}
	return out
}

func hasAnyTag(item *entities.ItemInstance, tags ...string) bool {
	owned := item.Tags()
	for _, want := range tags {
		for _, have := range owned {
			if strings.EqualFold(have, want) {
				return true
			}
		}
	}
	return false
}
