package session

import (
	"context"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// AutoReloadOnDry is the autoreload.md §3 dry-fire trigger. The combat sink calls
// it when a wielded firearm produces a dry attempt (RangedDry with Unloaded set);
// it runs on the tick goroutine, exactly like CompleteReadyActions, and mutates the
// actor only through the same mutex-guarded connActor methods a live command uses.
//
// Behavior, keyed on the wielder's autoreload preference:
//   - pref off, or the wielded weapon isn't an autoreload firearm (a bow/crossbow) →
//     returns false so the caller runs its default dry-fire narration + manual reload.
//   - a reload is already in flight → returns true (owns the moment; no re-arm, no spam).
//   - ammo available → delegates to the `reload` verb through the same env the action
//     sweep replays with, so the timed busy action, clip ejection, and narration are
//     byte-identical to the wielder typing `reload` (§3). Returns true.
//   - out of ammo → a rate-limited "nothing to reload with" notice (§5). Returns true.
//
// noAmmoWindow is the minimum ticks between repeated no-ammo notices (0 = every
// dry attempt). Returning true tells the caller to suppress the default narration —
// autoreload owned the dry moment.
func (m *Manager) AutoReloadOnDry(ctx context.Context, bareID string, noAmmoWindow uint64) bool {
	if m == nil {
		return false
	}
	a, ok := m.GetByPlayerID(bareID)
	if !ok || !a.Autoreload() {
		return false
	}
	isFirearm, canReload := a.WieldedFirearmReload()
	if !isFirearm {
		// A bow/crossbow keeps its own dry narration and manual reload/load path.
		return false
	}
	// A RELOAD already armed for this actor: don't re-arm or narrate — the timed
	// action is in flight and will complete on its own. This MUST gate on the action
	// KIND, not merely "busy": action.Tracker has one busy slot shared across all
	// kinds (don/doff, …), and a ranged swing can still fire mid-don, so a non-reload
	// busy action must not be mistaken for an in-flight reload (that would swallow the
	// dry moment with no reload armed and no message). Busy with something else falls
	// through to the default "reload first" narration — the reload can't arm now anyway
	// (the one slot is taken), so telling the player to reload is the honest outcome.
	if m.actionTracker != nil {
		if act, busy := m.actionTracker.Active(a.PlayerID()); busy {
			if act.Kind == command.KindReload {
				return true
			}
			return false
		}
	}
	if canReload {
		// Delegate to the `reload` verb through the cached action env (the same env
		// the completion sweep replays with). Phase 1 arms the KindReload busy action
		// and narrates "You begin reloading."; the sweep replays phase 2 to insert the
		// clip + eject the spent one. Identical to a player-typed `reload` (§3). With no
		// dispatch path (headless/tests) or on a dispatch error, fall through to the
		// default dry narration rather than leaving the player silently uninformed.
		if m.actionCommands == nil {
			return false
		}
		env := m.actionEnv
		if err := m.actionCommands.Dispatch(ctx, env, a, "reload"); err != nil {
			logging.From(ctx).Warn("autoreload dispatch failed",
				slog.String("event", "autoreload.dispatch_failed"),
				slog.String("player", a.Name()),
				slog.Any("err", err))
			return false
		}
		// A successful reload clears the no-ammo suppression window so the next genuine
		// dry-out reports at once rather than staying muted (§5).
		a.resetAutoReloadNotice()
		return true
	}
	// Out of ammo entirely — report only, rate-limited (§5). No fall-through to a
	// weapon switch (explicitly out of scope, §5).
	var now uint64
	if m.actionEnv.NowTick != nil {
		now = m.actionEnv.NowTick()
	}
	if a.autoReloadNoticeDue(now, noAmmoWindow) {
		_ = a.Write(ctx, "*click* — you're out of ammo, with nothing to reload.")
	}
	return true
}
