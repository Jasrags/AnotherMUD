package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/economy"
)

// LicensesHandler implements `licenses` (pack alias `sin`) — list the identity
// credentials the character is carrying and the permits each clears
// (sin-and-legality.md §5). Read-only: it charges nothing and changes no state,
// and works anywhere (not just in a shop). A character holding no credential is
// told they're running without valid papers.
func LicensesHandler(ctx context.Context, c *Context) error {
	if c.Shop == nil {
		return c.Actor.Write(ctx, "You have no way to check your credentials right now.")
	}
	sh, ok := c.Actor.(economy.Shopper)
	if !ok {
		return c.Actor.Write(ctx, "You have no valid credentials — you're running SINless.")
	}
	creds := c.Shop.CarriedCredentials(sh)
	if len(creds) == 0 {
		return c.Actor.Write(ctx, "You have no valid credentials — you're running SINless.")
	}
	var b strings.Builder
	b.WriteString("You are carrying:")
	for _, cred := range creds {
		b.WriteString("\n  ")
		b.WriteString(cred.Name)
		switch {
		case cred.Burned:
			// A spent fake (sin-and-legality.md §7) — flagged so the player knows
			// to replace it rather than wondering why stores keep refusing them.
			b.WriteString(" — BURNED (flagged, useless)")
		case len(cred.Permits) > 0:
			b.WriteString(fmt.Sprintf(" — licensed for: %s", strings.Join(cred.Permits, ", ")))
		default:
			b.WriteString(" — identity only, no licenses")
		}
	}
	return c.Actor.Write(ctx, b.String())
}
