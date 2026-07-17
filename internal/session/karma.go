package session

import (
	"context"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// karma.go — the connActor surface over the karma-ledger advancement strategy
// (shadowrun-mvp.md SR-M5). A connActor holds a non-nil *karma.Ledger only when
// its world selected `advancement: karma-ledger`; every method here is nil-safe
// so a level-track character (the default, nil ledger) reads as "no karma" and
// the reward paths fall through to the XP/level engine.

// UsesKarmaLedger reports whether this character advances by spending karma
// (karma-ledger world) rather than by earning track XP and leveling. It is the
// single branch the reward paths (kill, quest) and the score sheet key off.
func (a *connActor) UsesKarmaLedger() bool {
	return a != nil && a.karma != nil
}

// KarmaBalance returns the character's spendable (current) and lifetime-earned
// (total) karma. Both are 0 for a level-track character with no ledger, so a
// caller can render unconditionally.
func (a *connActor) KarmaBalance() (current, total int64) {
	if a == nil || a.karma == nil {
		return 0, 0
	}
	return a.karma.Current(), a.karma.Total()
}

// GrantKarma banks an earned advancement reward into the ledger, raising both
// the spendable balance and the lifetime total. No-op (returns 0) for a
// level-track character or a non-positive amount. Returns the post-grant
// spendable balance. Logged so a content author tuning reward rates can see
// where karma is flowing, mirroring the kill-XP debug lines.
func (a *connActor) GrantKarma(ctx context.Context, source string, amount int64) int64 {
	if a == nil || a.karma == nil || amount <= 0 {
		return 0
	}
	current := a.karma.Grant(amount)
	logging.From(ctx).Debug("karma granted",
		slog.String("event", "karma.grant"),
		slog.String("source", source),
		slog.Int64("amount", amount),
		slog.Int64("current", current))
	return current
}

// SpendKarma deducts amount from the spendable balance when it covers the cost,
// leaving the lifetime total untouched. Returns false (no mutation) for a
// level-track character, a non-positive amount, or an insufficient balance —
// the `improve` verb (SR-M5b) reports the shortfall to the player.
func (a *connActor) SpendKarma(amount int64) bool {
	if a == nil || a.karma == nil {
		return false
	}
	return a.karma.Spend(amount)
}
