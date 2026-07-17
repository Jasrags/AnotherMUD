package command

import "context"

// ImproveActor is the capability a connActor exposes for the `improve` verb
// (karma-ledger advancement — shadowrun-mvp.md SR-M5b). The session layer owns
// the resolve/spend logic (it holds the ledger, proficiency, stat block, and
// feat registry); this handler is thin, mirroring feat/practice.
type ImproveActor interface {
	// UsesKarmaLedger reports whether this character advances by spending karma.
	UsesKarmaLedger() bool
	// Improve spends karma to raise target (an attribute, skill, or quality);
	// param binds a per-parameter quality. Returns the player-facing result line.
	Improve(ctx context.Context, target, param string) string
	// ImproveListing renders what can be raised, each cost, and the balance.
	ImproveListing() string
}

// ImproveHandler implements `improve <target> [param]` — spend karma to raise an
// attribute, a skill's ceiling, or buy a quality. With no argument it lists what
// is improvable and the current balance. Only meaningful for a karma-ledger
// character; a level-track character is told they advance by leveling.
func ImproveHandler(ctx context.Context, c *Context) error {
	actor, ok := c.Actor.(ImproveActor)
	if !ok || !actor.UsesKarmaLedger() {
		return c.Actor.Write(ctx, "You advance by leveling up, not by spending karma.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, actor.ImproveListing())
	}
	param := ""
	if len(c.Args) > 1 {
		param = c.Args[1]
	}
	return c.Actor.Write(ctx, actor.Improve(ctx, c.Args[0], param))
}
