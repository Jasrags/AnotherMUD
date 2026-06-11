package command

import "context"

// FeatActor is the capability a connActor exposes for the feat verbs (EPIC S4
// Phase 4 — docs/proposals/wot-feats.md §2.3). The session layer owns the
// take/list logic (it holds the credits, known_feats, and the registry); these
// handlers are thin, mirroring train/practice.
type FeatActor interface {
	// FeatCredits is the count of banked-but-unspent feat slots.
	FeatCredits() int
	// TakeFeat spends one slot to take featID (param binds a per-parameter
	// feat). Returns (true, confirmation) or (false, reason).
	TakeFeat(featID, param string) (bool, string)
	// FeatListing renders the held feats + banked slots + eligible-to-take.
	FeatListing() string
}

// FeatsHandler implements `feats` — list held feats, banked slots, and the
// feats currently eligible to take.
func FeatsHandler(ctx context.Context, c *Context) error {
	holder, ok := c.Actor.(FeatActor)
	if !ok {
		return c.Actor.Write(ctx, "You have no feats.")
	}
	return c.Actor.Write(ctx, holder.FeatListing())
}

// FeatHandler implements `feat <id> [target]` — spend a banked slot to take a
// feat. The optional target binds a per-parameter feat (e.g. a weapon for
// Weapon Focus). With no argument it falls back to the listing, so a bare
// `feat` is a friendly alias for `feats`.
func FeatHandler(ctx context.Context, c *Context) error {
	holder, ok := c.Actor.(FeatActor)
	if !ok {
		return c.Actor.Write(ctx, "You cannot take feats.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, holder.FeatListing())
	}
	featID := c.Args[0]
	param := ""
	if len(c.Args) > 1 {
		param = c.Args[1]
	}
	_, msg := holder.TakeFeat(featID, param)
	return c.Actor.Write(ctx, msg)
}
