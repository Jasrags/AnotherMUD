package session

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// renderTakeoverNotice is the canonical line delivered to the existing
// session right before its connection is closed. Kept as a helper so
// tests (and a future ui-rendering-help system) can match it without
// hard-coding the string in two places.
func renderTakeoverNotice() string {
	return "Another connection has taken over this character."
}

// renderTakeoverPrompt is the yes/no question put to the new connection
// when an existing live session is found for the character.
func renderTakeoverPrompt() string {
	return "That character is already online. Take over the existing session? (yes/no): "
}

// renderTakeoverDeclined is the reply when the user answers "no" to the
// takeover prompt. The existing session is untouched; the new connection
// is closed by the caller via the normal handler return.
func renderTakeoverDeclined() string {
	return "Login cancelled; the existing session remains online."
}

// renderTakeoverRaced is shown when a concurrent login already won the
// takeover claim for this character; the new connection is closed.
func renderTakeoverRaced() string {
	return "The existing session was just replaced; please try again."
}

// markTakenOver flips the takenOver latch on the actor. Returns true on
// the transition; idempotent on a second call. Once set, dispatchTeardown
// short-circuits so the old conn's eventual EOF cannot tear down indices
// that the replacement session now owns (spec §6.1 stale-event guard).
//
// This is the claim point for the takeover race: two concurrent logins
// for the same Playing character both reach performTakeover, but only
// the goroutine that wins this CAS owns the side-effects.
func (a *connActor) markTakenOver() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.takenOver {
		return false
	}
	a.takenOver = true
	a.disconnecting = true // so dispatchTeardown's link-dead branch is never taken
	return true
}

// performTakeover executes the spec §8 sequence on the existing actor:
// claim it (CAS via markTakenOver), persist its in-flight state to disk,
// snapshot its save+room, remove it from every index, and close its
// connection with reason `session takeover`.
//
// Returns the (save, room) the caller MUST use when spawning the new
// session — these preserve any mutations the displaced session had not
// yet autosaved (location, dirty flags would otherwise be lost because
// login.Run already loaded a stale snapshot from disk).
//
// Returns ok=false if a concurrent takeover for the same character won
// the claim; the caller must reject the new connection.
func performTakeover(ctx context.Context, cfg Config, existing *connActor) (save *player.Save, room *world.Room, ok bool) {
	// Claim first. Lose → bail without touching any side effects.
	if !existing.markTakenOver() {
		return nil, nil, false
	}

	// Notify the displaced session. Best-effort: a failed write here just
	// means the user won't see the courtesy line; we still proceed with
	// the takeover.
	if err := existing.Write(ctx, renderTakeoverNotice()); err != nil {
		logging.From(ctx).Debug("takeover: notice write failed",
			slog.String("player", existing.PlayerName()),
			slog.Any("err", err))
	}

	// Persist the old actor's in-memory state before scrubbing it. This
	// is belt-and-braces: the new actor will inherit the same *save
	// pointer below, so even if Persist fails the new session still
	// holds the live record. The disk write matters only if the server
	// crashes between here and the next autosave.
	if err := existing.Persist(ctx); err != nil {
		logging.From(ctx).Warn("takeover: persist old actor failed",
			slog.String("player", existing.PlayerName()),
			slog.Any("err", err))
	}

	// Snapshot save + room + conn under the actor lock so we can scrub
	// indices and close the conn without racing a still-pumping old
	// goroutine. The save pointer is transferred to the new session —
	// the old actor's pump is about to unwind and will not mutate it
	// again once its conn is closed.
	existing.mu.Lock()
	save = existing.save
	room = existing.room
	oldConn := existing.conn
	existing.mu.Unlock()

	// Scrub indices. The old conn's read loop will unwind shortly
	// (because we close its socket next) and call dispatchTeardown,
	// which short-circuits on takenOver. Doing the Remove here means
	// the new session can re-occupy byPlayerID / byName / byRoom for
	// the same pid without colliding.
	cfg.Manager.Remove(existing)

	if oldConn != nil {
		if err := oldConn.Close(); err != nil && !errors.Is(err, conn.ErrClosed) {
			logging.From(ctx).Debug("takeover: old conn close failed",
				slog.String("player", existing.PlayerName()),
				slog.Any("err", err))
		}
	}

	logging.From(ctx).Info("session: taken over",
		slog.String("player", existing.PlayerName()),
		slog.String("player_id", existing.PlayerID()),
		slog.String("reason", "session takeover"))
	return save, room, true
}

// promptTakeoverConfirm reads a yes/no answer from the new connection.
// Returns true on yes, false on no or any read error (defaulting to a
// rejection avoids accidental hostile takeovers on flaky inputs).
//
// Accepts "y" / "yes" (case-insensitive) as affirmative; anything else
// is treated as a decline.
func promptTakeoverConfirm(ctx context.Context, c conn.Connection) bool {
	if _, err := c.Write(ctx, []byte(renderTakeoverPrompt())); err != nil {
		return false
	}
	line, err := c.Read(ctx)
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
