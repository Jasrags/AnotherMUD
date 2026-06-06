package session

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// sessionPhase is the lifecycle state of a connActor with respect to its
// underlying transport. Per docs/specs/session-lifecycle.md §7, a session
// can survive a connection drop in LinkDead phase, waiting for either a
// reconnect (which returns it to Playing) or the cleanup sweep (which
// runs full teardown).
type sessionPhase int

const (
	phasePlaying sessionPhase = iota
	phaseLinkDead
	// phaseTearing is set briefly when the link-dead cleanup sweep has
	// claimed an actor for teardown. Reconnect aborts if it sees this
	// phase, so the same actor cannot be both reconnected and reaped.
	phaseTearing
	// phaseCreating is the character-creation window (character-creation
	// §2): a new character's actor exists locally but is NOT yet
	// persisted, in the world, or in the session Manager. The completion
	// pipeline (§6.4) flips it to phasePlaying at commit. With the M12.2
	// no-content flow the window is synchronous (immediate commit); the
	// M12.3 interactive flow holds the actor here while it reads wizard
	// input, at which point a mid-creation disconnect (§8) leaves nothing
	// on disk because commit hasn't run.
	phaseCreating
)

// LinkDeadConfig is the policy for surviving connection drops, per
// docs/specs/session-lifecycle.md §7.
//
// When Enabled is false every disconnect goes straight to full
// teardown (the M3 / pre-M4.4 behavior). When Enabled is true a
// connection drop that did not originate from the server (i.e. the
// actor's disconnecting latch is not set) parks the session in
// LinkDead phase. A returning login for the same player id reattaches;
// otherwise the linkdead-cleanup tick handler reaps the session after
// TimeoutSeconds.
type LinkDeadConfig struct {
	Enabled        bool
	TimeoutSeconds int
}

// DefaultLinkDeadConfig returns the spec default: enabled, 120s.
func DefaultLinkDeadConfig() LinkDeadConfig {
	return LinkDeadConfig{Enabled: true, TimeoutSeconds: 120}
}

func (c LinkDeadConfig) timeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// enterLinkDead transitions the actor into LinkDead phase. Returns true
// if the transition happened. Idempotent: a second call on a session
// that is already LinkDead returns false. Must be called WITHOUT the
// actor lock held (it takes the lock internally).
func (a *connActor) enterLinkDead(now time.Time) bool {
	a.mu.Lock()
	if a.phase != phasePlaying {
		a.mu.Unlock()
		return false
	}
	a.phase = phaseLinkDead
	a.linkDeadAt = now
	a.mu.Unlock()
	return true
}

// reattach swaps the actor's connection for a freshly-authenticated one
// and returns it to Playing phase. Returns false if the actor is no
// longer link-dead (e.g. the cleanup sweep beat us to it).
//
// Does NOT mutate a.id — that write is owned by
// Manager.ReRegisterConnectionForSession, which performs it under the
// manager lock alongside the byConn index update so no observer can
// see a half-swapped (id, byConn) pair.
func (a *connActor) reattach(newConn conn.Connection, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.phase != phaseLinkDead {
		return false
	}
	a.conn = newConn
	a.phase = phasePlaying
	a.linkDeadAt = time.Time{}
	a.lastInputAt = now
	a.idleWarned = false
	a.disconnecting = false
	// Fresh flood gate. Spec §7.4 step 7 clears the input queue; we
	// don't queue commands at this layer, but a stale token-bucket
	// balance from before the drop should not punish the returning
	// player either.
	if a.floodCfg != nil {
		a.flood = newFloodGate(*a.floodCfg, a.clk)
		a.gmcpFlood = newFloodGate(gmcpFloodConfig(*a.floodCfg), a.clk)
	}
	// M16.4a follow-up: clear the GMCP Char.Vitals shadow so the
	// next gmcp-vitals-flush tick emits a baseline frame to the
	// new peer. The previous peer's panel state is gone with its
	// conn; the new client expects a fresh baseline even when the
	// engine state hasn't changed across the drop.
	a.resetGmcpVitalsShadow()
	// M16.4c: same reset for the Char.Items shadows so the new
	// peer's inventory panel gets a baseline for both inv and
	// wear locations.
	a.resetGmcpItemsShadow()
	// M16.4d: same reset for the Char.Combat shadow so the new
	// peer's combat HUD gets a baseline frame even when the
	// engagement state didn't change across the drop.
	a.resetGmcpCombatShadow()
	// M16.4e: same reset for the Char.Effects shadow so the new
	// peer's effects panel gets a baseline frame on reattach.
	a.resetGmcpEffectsShadow()
	// M16.4f: same reset for the Char.Experience shadow so the
	// new peer's XP-bar gets a baseline frame on reattach.
	a.resetGmcpExperienceShadow()
	// M16.4h: clear the boot-identity sent flags so the new peer
	// gets fresh Char.Login + Char.StatusVars + Char.Status
	// baseline frames on reattach.
	a.resetGmcpCharStatusShadow()
	return true
}

// isLinkDead reports whether the actor is currently parked in LinkDead
// phase. Takes the actor lock; safe from any goroutine.
func (a *connActor) isLinkDead() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.phase == phaseLinkDead
}

// claimForTeardown atomically moves the actor from LinkDead to Tearing
// if the elapsed link-dead duration has met the timeout. Returns true
// on a successful claim; the caller then owns the teardown.
func (a *connActor) claimForTeardown(now time.Time, timeout time.Duration) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.phase != phaseLinkDead {
		return false
	}
	// Zero / negative timeout = never auto-reap. A misconfigured
	// LinkDeadConfig with TimeoutSeconds <= 0 must NOT result in
	// every parked session being destroyed on the next sweep.
	if timeout <= 0 {
		return false
	}
	if now.Sub(a.linkDeadAt) < timeout {
		return false
	}
	a.phase = phaseTearing
	a.disconnecting = true
	return true
}

// LinkDeadCleanup reaps link-dead sessions whose grace window has
// expired. Persists the player, broadcasts departure to the room they
// were in at drop time, and removes every index entry.
//
// Snapshots the link-dead set under the read lock and processes each
// candidate outside the lock so per-actor work doesn't serialize the
// manager. claimForTeardown guarantees only one path wins the race.
func (m *Manager) LinkDeadCleanup(ctx context.Context, cfg LinkDeadConfig, clk clock.Clock) {
	if !cfg.Enabled {
		return
	}
	timeout := cfg.timeout()
	if clk == nil {
		clk = clock.RealClock{}
	}
	now := clk.Now()

	candidates := m.AllLinkDeadSessions()
	for _, a := range candidates {
		if !a.claimForTeardown(now, timeout) {
			continue
		}

		// Canonical teardown order — broadcast → remove → persist —
		// matches fullTeardown in session.go so a future consolidation
		// cannot accidentally transpose the steps.
		room := a.Room()
		if room != nil {
			m.SendToRoom(ctx, room.ID,
				fmt.Sprintf("%s has left.", a.Name()), a.PlayerID())
		}
		m.Remove(a)
		if err := a.Persist(ctx); err != nil {
			logging.From(ctx).Warn("linkdead cleanup: persist failed",
				slog.String("player", a.PlayerName()),
				slog.Any("err", err))
		}

		logging.From(ctx).Info("linkdead cleanup: session reaped",
			slog.String("player", a.PlayerName()),
			slog.String("player_id", a.PlayerID()),
			slog.Int("timeout_seconds", cfg.TimeoutSeconds))
	}
}

// IsPlayerLinkDead reports whether the named player has an active
// session parked in LinkDead phase. Exported for end-to-end tests in
// the session_test package; production callers should use the
// connActor directly.
func (m *Manager) IsPlayerLinkDead(name string) bool {
	a, ok := m.GetByName(name)
	if !ok {
		return false
	}
	return a.isLinkDead()
}

// renderReconnect is the post-reattach text payload. Exists so tests
// (and a future ui-rendering-help system) can match the canonical
// reconnect banner without hard-coding it everywhere.
func renderReconnect() string {
	return "Reconnected."
}

// renderRoomForReconnect re-renders the actor's current room so the
// returning player sees where they are. Kept as a helper to mirror the
// "enqueue a look" step in spec §7.4 without dragging the dispatcher
// into the reconnect path.
func renderRoomForReconnect(a *connActor, cfg Config) string {
	r := a.Room()
	if r == nil {
		return ""
	}
	lvl := command.EffectiveLight(cfg.Light, r, a, cfg.Items, cfg.Placement)
	return command.RenderRoom(r, cfg.Placement, cfg.Items, questMarkerFor(cfg.Quests, a.PlayerID()), cfg.Ambience, nil, lvl, otherPlayerNames(cfg.Manager, r.ID, a.PlayerID())...)
}
