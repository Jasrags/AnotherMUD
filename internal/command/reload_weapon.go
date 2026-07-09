package command

import (
	"context"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// magazineReloader is the session surface for topping up a wielded magazine
// weapon (the live *connActor satisfies it). before/after are the loaded-round
// counts around the reload, capacity is the magazine size, and isMagazine is
// false when the wielded weapon isn't magazine-fed (the caller then falls back
// to the crossbow load path or a generic message).
type magazineReloader interface {
	ReloadWieldedMagazine() (before, after, capacity int, isMagazine bool)
}

// ReloadHandler implements the player-facing `reload` verb. A MAGAZINE weapon (a
// firearm) refills its loaded rounds from carried ammo of the weapon's kind; a
// reload-gated crossbow (ReloadTicks > 0) chambers a bolt through the existing
// `load` path. Anything else has nothing to reload.
func ReloadHandler(ctx context.Context, c *Context) error {
	cb, ok := c.Actor.(combat.Combatant)
	if !ok {
		return c.Actor.Write(ctx, "You can't reload anything right now.")
	}
	st := cb.Stats()
	if st.Magazine <= 0 {
		// Not magazine-fed: a crossbow chambers via `load`; anything else has
		// nothing to reload.
		if st.RangedClass == combat.RangedProjectile && st.ReloadTicks > 0 {
			return LoadHandler(ctx, c)
		}
		return c.Actor.Write(ctx, "You aren't wielding anything that needs reloading.")
	}
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
		return c.Actor.Write(ctx, fmt.Sprintf("You slap a fresh magazine into %s. (%d/%d)", name, after, capacity))
	}
}
