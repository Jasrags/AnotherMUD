package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/scrap"
)

// magazineReloader is the session surface for topping up an INTERNALLY-FED
// magazine weapon (SR-M3e — a revolver/cylinder). before/after are the loaded-
// round counts around the reload, capacity is the magazine size, isMagazine is
// false when the wielded weapon isn't internally-fed.
type magazineReloader interface {
	ReloadWieldedMagazine() (before, after, capacity int, isMagazine bool)
}

// holderReloader is the session surface for the holder-fed model (ammo-and-
// reloading §5): insert a loaded holder into the wielded weapon, or fill a
// carried holder from loose rounds.
type holderReloader interface {
	InsertHolder() (outcome, weapon string, loaded, capacity int, ejectedTpl string, ejectedLoaded int)
	FillHolder(holderID entities.EntityID) (before, after, capacity int, ok bool)
}

// ReloadHandler implements the unified `reload` verb (ammo-and-reloading §3):
// "top up the target from the tier below."
//   - `reload <holder>` fills a carried ammunition holder from loose rounds.
//   - `reload` reloads the wielded weapon: a holder-fed weapon takes a loaded
//     holder (ejecting the spent one); an internally-fed magazine weapon takes
//     loose rounds (SR-M3e); a reload-gated crossbow chambers a bolt (`load`).
func ReloadHandler(ctx context.Context, c *Context) error {
	if token := strings.TrimSpace(strings.Join(c.Args, " ")); token != "" {
		return reloadNamedHolder(ctx, c, token)
	}

	cb, ok := c.Actor.(combat.Combatant)
	if !ok {
		return c.Actor.Write(ctx, "You can't reload anything right now.")
	}
	st := cb.Stats()
	switch {
	case st.AcceptsHolder != "":
		return reloadHolderFedWeapon(ctx, c)
	case st.Magazine > 0:
		return reloadMagazineWeapon(ctx, c)
	case st.RangedClass == combat.RangedProjectile && st.ReloadTicks > 0:
		return LoadHandler(ctx, c) // a crossbow chambers a bolt via the load path
	default:
		return c.Actor.Write(ctx, "You aren't wielding anything that needs reloading.")
	}
}

// reloadHolderFedWeapon inserts a compatible loaded holder into the wielded
// weapon and ejects the displaced holder into the room (ammo-and-reloading §5/§7).
func reloadHolderFedWeapon(ctx context.Context, c *Context) error {
	reloader, ok := c.Actor.(holderReloader)
	if !ok {
		return c.Actor.Write(ctx, "You can't reload anything right now.")
	}
	outcome, weapon, loaded, capacity, ejectedTpl, ejectedLoaded := reloader.InsertHolder()
	switch outcome {
	case "not-holder-fed":
		return c.Actor.Write(ctx, "You aren't wielding anything that needs reloading.")
	case "no-holder":
		return c.Actor.Write(ctx, fmt.Sprintf("You have no loaded clip to load into %s.", weapon))
	}
	if ejectedTpl != "" {
		ejectHolderToRoom(ctx, c, ejectedTpl, ejectedLoaded)
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You slap a fresh clip into %s. (%d/%d)", weapon, loaded, capacity))
}

// ejectHolderToRoom mints the displaced holder (from its template + remaining
// rounds) and drops it on the floor (ammo-and-reloading §7). Best-effort: with
// no spawn service / placement / room it simply doesn't eject.
func ejectHolderToRoom(ctx context.Context, c *Context, tpl string, loaded int) {
	if c.Spawn == nil || c.Placement == nil || c.Items == nil {
		return
	}
	room := c.Actor.Room()
	if room == nil {
		return
	}
	id, _, err := c.Spawn.SpawnItem(ctx, tpl)
	if err != nil {
		return
	}
	if e, ok := c.Items.GetByID(id); ok {
		if h, ok := e.(*entities.ItemInstance); ok {
			h.SetMagazineLoaded(loaded)
			// Mark the spent clip as ephemeral scrap so it decays off the ground
			// after its lifetime (ammo-and-reloading §7). Recoverable until then.
			if c.NowTick != nil {
				scrap.Mark(c.Items, h, c.NowTick())
			}
		}
	}
	c.Placement.Place(id, room.ID)
	_ = c.Actor.Write(ctx, "The spent clip ejects and clatters to the ground.")
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("A spent clip ejects from %s's weapon and clatters to the ground.", c.Actor.Name()),
			c.Actor.PlayerID())
	}
}

// reloadMagazineWeapon is the SR-M3e internally-fed path: fill the wielded
// weapon's own magazine from carried loose rounds (a revolver / cylinder).
func reloadMagazineWeapon(ctx context.Context, c *Context) error {
	reloader, ok := c.Actor.(magazineReloader)
	if !ok {
		return c.Actor.Write(ctx, "You can't reload anything right now.")
	}
	name := wieldedWeaponName(c)
	before, after, capacity, isMag := reloader.ReloadWieldedMagazine()
	switch {
	case !isMag:
		return c.Actor.Write(ctx, "You aren't wielding anything that needs reloading.")
	case before >= capacity:
		return c.Actor.Write(ctx, fmt.Sprintf("It's already fully loaded. (%d/%d)", capacity, capacity))
	case after == before:
		return c.Actor.Write(ctx, "You have no rounds left to reload with.")
	default:
		return c.Actor.Write(ctx, fmt.Sprintf("You reload %s. (%d/%d)", name, after, capacity))
	}
}

// reloadNamedHolder fills a carried ammunition holder from loose rounds
// (ammo-and-reloading §4). The token resolves against the actor's inventory.
func reloadNamedHolder(ctx context.Context, c *Context, token string) error {
	if c.Items == nil {
		return c.Actor.Write(ctx, "You can't reload anything right now.")
	}
	target := c.resolveHeldItem(token)
	if target == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("You aren't carrying %q.", token))
	}
	reloader, ok := c.Actor.(holderReloader)
	if !ok {
		return c.Actor.Write(ctx, "You can't reload anything right now.")
	}
	before, after, capacity, isHolder := reloader.FillHolder(target.ID())
	name := target.Name()
	switch {
	case !isHolder:
		return c.Actor.Write(ctx, fmt.Sprintf("You can't load loose rounds into %s.", name))
	case before >= capacity:
		return c.Actor.Write(ctx, fmt.Sprintf("It's already full. (%d/%d)", capacity, capacity))
	case after == before:
		return c.Actor.Write(ctx, fmt.Sprintf("You have no rounds to load into %s.", name))
	default:
		return c.Actor.Write(ctx, fmt.Sprintf("You load rounds into %s. (%d/%d)", name, after, capacity))
	}
}
