package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// PurgeHandler implements `purge <target>` (admin-verbs §5): remove a
// non-player entity — a mob or a room item — from the world, untracking it.
// It NEVER targets a player (§5 / §9): a player match is refused. Admin-
// marked (M19.3 gate); audited via the M19.4a auditAdmin choke point.
//
// Removal mirrors the canonical death-cleanup path (Placement.Remove +
// Store.Untrack): an untracked mob fails the spawn tracker's alive check on
// the next area sweep, so a purged mob's slot can respawn. Container /
// mob-carried contents are not recursively cleaned (the death path doesn't
// either); orphaned contents are an accepted v1 limitation — see the
// deferred note.
func PurgeHandler(ctx context.Context, c *Context) error {
	if c.Placement == nil || c.Items == nil {
		return c.Actor.Write(ctx, "You can't purge anything right now.")
	}
	token := strings.TrimSpace(strings.Join(c.Args, " "))
	if token == "" {
		return c.Actor.Write(ctx, "Purge what?")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You see nothing here.")
	}

	// Players + mobs first (mobs win ties, as everywhere). A player match
	// is refused — purge never removes a player.
	if cb, name, ok := findCombatantInRoom(c, room.ID, token); ok {
		mob, isMob := cb.(*entities.MobInstance)
		if !isMob {
			return c.Actor.Write(ctx, "You can't purge a player.")
		}
		return c.purgeEntity(ctx, room.ID, mob.ID(), name, "npc")
	}

	// Otherwise a room item.
	if it, ok := findRoomItemByKeyword(c, token); ok {
		return c.purgeEntity(ctx, room.ID, it.ID(), it.Name(), "item")
	}

	return c.Actor.Write(ctx, "You don't see that here.")
}

// purgeEntity removes a resolved non-player entity from the room and the
// entity store, broadcasts its disappearance, audits, and confirms.
func (c *Context) purgeEntity(ctx context.Context, roomID world.RoomID, id entities.EntityID, name, kind string) error {
	c.Placement.Remove(id)
	_ = c.Items.Untrack(id) // benign if already untracked

	// A purged mob emits no MobKilled, so release any players trailing it
	// (follow.md §3 mob-leader) here — otherwise they keep a dead follow edge
	// to a leader that can never move again. Only mobs are followable.
	if kind == "npc" && c.Follow != nil {
		for _, fid := range c.Follow.Lose(string(id)) {
			if fa, ok := c.actorByID(fid); ok {
				_ = fa.Write(ctx, fmt.Sprintf("You lose the trail of %s.", name))
			}
		}
	}

	if c.Broadcaster != nil && name != "" {
		c.Broadcaster.SendToRoom(ctx, roomID,
			fmt.Sprintf("%s is purged from existence.", name), c.Actor.PlayerID())
	}

	auditAdmin(ctx, c, "purge", string(id), kind+":"+name)
	return c.Actor.Write(ctx, fmt.Sprintf("You purge %s.", name))
}

// findRoomItemByKeyword resolves a room item by keyword via the shared §5
// room_item arg path, then re-fetches the live instance by id. Mirrors
// findCombatantInRoom's resolve→re-fetch shape. Returns (nil, false) on a
// miss or when the store is unwired.
func findRoomItemByKeyword(c *Context, token string) (*entities.ItemInstance, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}
	reg := c.ArgResolver
	if reg == nil {
		reg = NewArgResolverRegistry()
	}
	out, _, _, err := reg.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "item", Type: ArgRoomItem}},
		strings.Fields(token),
		c.BuildResolveContext(),
	)
	if err != nil {
		return nil, false
	}
	ref, ok := out["item"].(ItemRef)
	if !ok || c.Items == nil {
		return nil, false
	}
	e, ok := c.Items.GetByID(entities.EntityID(ref.ID))
	if !ok {
		return nil, false
	}
	it, ok := e.(*entities.ItemInstance)
	return it, ok
}
