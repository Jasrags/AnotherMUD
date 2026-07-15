package command

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// FireModeHandler implements the `firemode` verb (ranged-combat §5.5): select the
// active firing mode for the wielded ranged weapon. `firemode` alone reports the
// current mode and what the weapon supports; `firemode <single|burst|auto>` sets
// it. Burst and full-auto trade ammunition and accuracy (recoil) for damage;
// single is always available. The selection is transient — a tactical choice, not
// a persisted preference — and is clamped to the wielded weapon at combat time.
func FireModeHandler(ctx context.Context, c *Context) error {
	pref, ok := c.Actor.(interface {
		FireMode() string
		SetFireMode(string)
		WieldedFireModes() []string
	})
	if !ok {
		return c.Actor.Write(ctx, "You can't do that right now.")
	}
	supported := pref.WieldedFireModes()

	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, fireModeStatus(pref.FireMode(), supported))
	}

	mode := strings.ToLower(strings.TrimSpace(c.Args[0]))
	if !item.ValidFireMode(mode) {
		return c.Actor.Write(ctx, fmt.Sprintf("%q is not a firing mode. Choose from: %s.",
			mode, strings.Join(item.FireModeNames(), ", ")))
	}
	// Single is always available; burst/auto require the wielded weapon to support
	// the mode (a semi-auto pistol can't chatter, a bow has no modes at all).
	if mode != item.FireModeSingle && !slices.Contains(supported, mode) {
		return c.Actor.Write(ctx, fmt.Sprintf("Your weapon can't fire in %s mode.", mode))
	}
	pref.SetFireMode(mode)
	return c.Actor.Write(ctx, fmt.Sprintf("Firing mode set to %s.", mode))
}

// fireModeStatus renders the current mode and the weapon's available modes
// (single first, then any declared extras), for the no-argument `firemode`.
func fireModeStatus(current string, supported []string) string {
	avail := []string{item.FireModeSingle}
	for _, m := range supported {
		if m != item.FireModeSingle {
			avail = append(avail, m)
		}
	}
	return fmt.Sprintf("Firing mode: %s. Available on this weapon: %s.",
		current, strings.Join(avail, ", "))
}
