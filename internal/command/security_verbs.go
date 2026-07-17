package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Bribe pricing (security-response.md §7 v2). A named base plus a per-heat and
// per-wanted-level surcharge — burying a bigger record costs more nuyen.
const (
	bribeBaseCost      = 100
	bribeCostPerHeat   = 15
	bribeCostPerWanted = 500
	tagFixer           = "fixer"
)

// reportBurnCrime feeds a caught-fake-SIN burn to the heat engine as a crime
// (security-response.md §7 v2). No-op when security is unwired, the actor has no
// player id, or it isn't in a room.
func reportBurnCrime(ctx context.Context, c *Context) {
	if c.Security == nil {
		return
	}
	room := c.Actor.Room()
	pid := c.Actor.PlayerID()
	if room == nil || pid == "" {
		return
	}
	c.Security.ReportBurn(ctx, pid, room.ID)
}

// WantedHandler implements `wanted` (alias `heat`) — the offender's visibility
// into their own heat + wanted level (security-response.md §7 v2). Read-only.
func WantedHandler(ctx context.Context, c *Context) error {
	if c.Security == nil {
		return c.Actor.Write(ctx, "No one's keeping score on you here.")
	}
	heat, wanted := c.Security.Status(c.Actor.PlayerID())
	if heat == 0 && wanted == 0 {
		return c.Actor.Write(ctx, "You're clean — no heat, no record. The law isn't looking for you.")
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Heat: %s.", heatBand(heat)))
	if wanted > 0 {
		b.WriteString(fmt.Sprintf(" Wanted level %d — each brush with the law brings a harder response.", wanted))
	}
	return c.Actor.Write(ctx, b.String())
}

// heatBand renders a raw heat value as a flavored band (the player never sees the
// number — enforcement is about how hunted you feel).
func heatBand(heat int) string {
	switch {
	case heat <= 0:
		return "cold"
	case heat < 30:
		return "a faint trace on you"
	case heat < 60:
		return "warm — someone's taken notice"
	case heat < 100:
		return "hot — a patrol is coming"
	default:
		return "blazing — they want you badly"
	}
}

// BribeHandler implements `bribe <fixer>` — pay a fixer nuyen to bury your record,
// clearing heat and easing your wanted level (security-response.md §7 v2, the
// de-escalation valve). The cost scales with the record being buried.
func BribeHandler(ctx context.Context, c *Context) error {
	if c.Security == nil {
		return c.Actor.Write(ctx, "There's no one here who can make your problems disappear.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "There's no fixer here to grease.")
	}
	fixer := findFixerInRoom(c, room.ID)
	if fixer == nil {
		return c.Actor.Write(ctx, "There's no fixer here to grease.")
	}
	pid := c.Actor.PlayerID()
	heat, wanted := c.Security.Status(pid)
	if heat == 0 && wanted == 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s waves you off. \"You're clean, chummer. Nothing to bury.\"", capitalize(fixer.Name())))
	}
	holder, ok := c.Actor.(economy.Entity)
	if !ok || c.Currency == nil {
		return c.Actor.Write(ctx, "You've no way to pay right now.")
	}
	cost := bribeBaseCost + heat*bribeCostPerHeat + wanted*bribeCostPerWanted
	balance := c.Currency.Read(holder)
	if balance < cost {
		return c.Actor.Write(ctx, fmt.Sprintf("%s wants %s to bury your record; you only have %s.", capitalize(fixer.Name()), c.Money.Format(cost), c.Money.Format(balance)))
	}
	if _, okDebit := c.Currency.Debit(ctx, holder, cost, "bribe:"+pid); !okDebit {
		return c.Actor.Write(ctx, fmt.Sprintf("%s wants %s; you can't cover it.", capitalize(fixer.Name()), c.Money.Format(cost)))
	}
	c.Security.ClearHeat(pid)
	return c.Actor.Write(ctx, fmt.Sprintf("%s makes a few quiet calls and slots your record into a memory hole. The heat's off you — for now. (-%s)", capitalize(fixer.Name()), c.Money.Format(cost)))
}

// findFixerInRoom returns the first fixer-tagged mob in the room (the bribe
// target), or nil. Mirrors findShopInRoom's "first tagged NPC" resolution.
func findFixerInRoom(c *Context, roomID world.RoomID) *entities.MobInstance {
	if c.Items == nil || c.Placement == nil {
		return nil
	}
	for _, id := range c.Placement.InRoom(roomID) {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		if c.questSpawnBlockedFrom(e) {
			continue
		}
		mob, ok := e.(*entities.MobInstance)
		if !ok {
			continue
		}
		if mobHasTag(mob, tagFixer) {
			return mob
		}
	}
	return nil
}
