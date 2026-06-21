package command

import (
	"context"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/action"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/slot"
)

// KindReload is the action.Kind for a timed crossbow reload (action-economy.md
// §7.1). Like the don/doff timer, completion replays the `load` command with
// Env.ReplayAction set; the replay consumes a bolt and chambers the weapon.
const KindReload action.Kind = "reload"

// weaponLoader is the per-actor reload-state surface (the live *connActor
// satisfies it). A reload-gated projectile holds one chambered shot keyed to a
// specific weapon id; firing clears it, a load sets it. A test/headless actor
// omits these and a reload-gated weapon then can't be loaded — which is the
// safe degenerate (it simply won't fire).
type weaponLoader interface {
	IsWeaponLoaded() bool
	SetWeaponLoaded() bool
	ClearWeaponLoaded()
}

// LoadHandler implements the crossbow `load` verb (action-economy.md §7.1): a
// reload-gated projectile (weapon ReloadTicks > 0) must be loaded before it can
// fire, and loading is a timed busy action. Two-phase, exactly like the don/doff
// timer — phase 1 (a player-typed `load`) arms the action and returns; the
// action-complete sweep replays `load` with ReplayAction set, and phase 2
// consumes one bolt and chambers the weapon.
func LoadHandler(ctx context.Context, c *Context) error {
	cb, ok := c.Actor.(combat.Combatant)
	if !ok {
		return c.Actor.Write(ctx, "You can't load anything right now.")
	}
	st := cb.Stats()
	if st.RangedClass != combat.RangedProjectile || st.ReloadTicks <= 0 {
		return c.Actor.Write(ctx, "You aren't wielding anything that needs loading.")
	}
	loader, hasLoader := c.Actor.(weaponLoader)
	if !hasLoader {
		return c.Actor.Write(ctx, "You can't load anything right now.")
	}

	if c.ReplayAction {
		return c.completeLoad(ctx, st, loader)
	}

	// Phase 1: gate + arm the timer. (The busy gate already refused a `load`
	// issued while another action is in flight; Begin's own guard is the net.)
	if loader.IsWeaponLoaded() {
		return c.Actor.Write(ctx, "It's already loaded.")
	}
	if c.Actions == nil || c.NowTick == nil {
		// No timed-action substrate (headless/test) — load instantly.
		return c.completeLoad(ctx, st, loader)
	}
	ider, ok := c.Actor.(interface{ PlayerID() string })
	if !ok || ider.PlayerID() == "" {
		return c.completeLoad(ctx, st, loader)
	}
	name := wieldedWeaponName(c)
	if !c.Actions.Begin(ider.PlayerID(), action.Action{
		Kind:          KindReload,
		ReadyAt:       c.NowTick() + uint64(st.ReloadTicks),
		Interruptible: true,
		Label:         "loading " + name,
		Payload:       c.Raw,
	}) {
		return c.Actor.Write(ctx, "You're already busy with something.")
	}
	if room := c.Actor.Room(); room != nil && c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s begins loading %s.", c.Actor.Name(), name), ider.PlayerID())
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You begin loading %s.", name))
}

// completeLoad performs the actual reload: consume one matching bolt, then
// chamber the weapon. Lazy-completion — nothing was reserved at begin, so if the
// player dropped/spent the ammo or unwielded the weapon meanwhile, this refuses
// cleanly and chambers nothing.
func (c *Context) completeLoad(ctx context.Context, st combat.Stats, loader weaponLoader) error {
	name := wieldedWeaponName(c)
	// Chamber FIRST, spend the bolt second, so a weapon unwielded between begin
	// and completion costs no ammunition (the bolt is never lost). If ammo ran
	// out meanwhile, undo the chamber — lazy-completion: nothing was reserved at
	// begin, so a failed completion leaves the world untouched.
	if !loader.SetWeaponLoaded() {
		return c.Actor.Write(ctx, "You have nothing to load it into.")
	}
	if consumer, ok := c.Actor.(ammoConsumer); ok && st.AmmoKind != "" {
		if _, consumed := consumer.ConsumeAmmo(st.AmmoKind); !consumed {
			loader.ClearWeaponLoaded() // back out — nothing was chambered after all
			return c.Actor.Write(ctx, fmt.Sprintf("*click* — you have no %s to load.", st.AmmoKind))
		}
	}
	if room := c.Actor.Room(); room != nil && c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s loads %s.", c.Actor.Name(), name), c.Actor.PlayerID())
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You load %s. It's ready to fire.", name))
}

// wieldedWeaponName resolves the display name of the actor's wielded weapon for
// reload messaging, falling back to a generic phrase when it can't be resolved.
func wieldedWeaponName(c *Context) string {
	if c.Items == nil {
		return "your weapon"
	}
	id, ok := c.Actor.Equipment()[slot.WieldSlot]
	if !ok {
		return "your weapon"
	}
	if e, ok := c.Items.GetByID(entities.EntityID(id)); ok {
		if it, ok := e.(*entities.ItemInstance); ok {
			return it.Name()
		}
	}
	return "your weapon"
}
