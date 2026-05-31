package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// FlushGmcpExperience walks every live session and emits
// Char.Experience GMCP frames for actors whose per-track snapshot
// has changed since the last emission. Called once per simulation
// tick from the gmcp-experience-flush handler.
//
// Mirrors FlushGmcpVitals: cadence-1 poll-and-diff, per-actor
// snapshots compared against a last-sent shadow. One frame per
// session per tick max; the frame carries every registered track,
// so a single XP grant on any track triggers a single re-emission
// of the whole list (cheap relative to a typical Tapestry-class
// track count).
func (m *Manager) FlushGmcpExperience(ctx context.Context) {
	m.mu.RLock()
	snapshot := make([]*connActor, 0, len(m.byConn))
	for _, a := range m.byConn {
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	for _, a := range snapshot {
		if a.isLinkDead() {
			continue
		}
		a.flushGmcpExperience(ctx)
	}
}

// flushGmcpExperience snapshots the actor's per-track progression,
// builds a CharExperience payload, and emits a Char.Experience
// frame when the snapshot differs from the last-sent shadow.
//
// Silent no-op when:
//
//   - the underlying conn doesn't speak GMCP;
//   - GMCP hasn't been negotiated;
//   - the actor has no progression.Manager wired (test fakes).
//
// The valid flag (gmcpLastExperienceValid) distinguishes "never
// sent" from "sent and matches empty" — without it, a fresh actor
// in a content pack that registered zero tracks would never get
// the baseline `[]` frame the panel needs.
func (a *connActor) flushGmcpExperience(ctx context.Context) {
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}
	if a.progression == nil {
		return
	}

	snap := a.snapshotExperience()

	a.gmcpExperienceMu.Lock()
	if a.gmcpLastExperienceValid && charExperienceEqual(a.gmcpLastExperience, snap) {
		a.gmcpExperienceMu.Unlock()
		return
	}
	a.gmcpLastExperience = snap
	a.gmcpLastExperienceValid = true
	a.gmcpExperienceMu.Unlock()

	payload := gmcp.CharExperience{Tracks: snap}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := sender.SendGmcp(ctx, gmcp.PackageCharExperience, data); err != nil {
		logging.From(ctx).Debug("gmcp experience send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// snapshotExperience builds the CharExperienceTrack slice for the
// actor's progression. Enumerates every registered track via
// TrackRegistry.All (already sorted by name), calls GetTrackInfo
// per track, and translates the result to the wire shape. The
// returned slice is always non-nil (possibly length zero) so the
// JSON encoder emits `[]` rather than `null` per CharExperience's
// contract.
//
// Note: GetTrackInfo lazy-inits the state to (level=1, xp=0) for
// any track the actor has never touched — same behavior as the
// `xp` verb. That keeps the frame stable: a fresh actor sees one
// row per registered track at level 1 rather than an empty list.
func (a *connActor) snapshotExperience() []gmcp.CharExperienceTrack {
	tracks := a.progression.Tracks().All()
	out := make([]gmcp.CharExperienceTrack, 0, len(tracks))
	for _, td := range tracks {
		info, ok := a.progression.GetTrackInfo(a.progress, td.Name)
		if !ok {
			continue
		}
		entry := gmcp.CharExperienceTrack{
			Track:    info.Track,
			Level:    info.Level,
			XP:       info.XP,
			MaxLevel: info.MaxLevel,
		}
		// Omit Name when equal to Track — saves wire bytes on the
		// common case where content doesn't configure a separate
		// display label.
		if td.DisplayName != "" && td.DisplayName != info.Track {
			entry.Name = td.DisplayName
		}
		if info.Level >= info.MaxLevel {
			entry.AtMax = true
			entry.Overflow = info.Overflow
		} else {
			entry.XPNext = info.XpToNext
		}
		out = append(out, entry)
	}
	return out
}

// charExperienceEqual reports whether two CharExperienceTrack
// slices are element-wise equal. Both inputs come from
// snapshotExperience which iterates TrackRegistry.All — a
// deterministic alphabetical order — so the positional compare
// is meaningful across snapshots.
func charExperienceEqual(a, b []gmcp.CharExperienceTrack) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// resetGmcpExperienceShadow marks the last-sent experience shadow
// invalid so the next flushGmcpExperience call emits
// unconditionally. Called on link-dead reattach: the new peer's
// Mudlet XP-bar module needs a baseline frame even when the
// engine-side state hasn't changed across the drop.
func (a *connActor) resetGmcpExperienceShadow() {
	a.gmcpExperienceMu.Lock()
	a.gmcpLastExperienceValid = false
	a.gmcpLastExperience = nil
	a.gmcpExperienceMu.Unlock()
}
