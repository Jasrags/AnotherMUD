package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// FlushGmcpEffects walks every live session and emits Char.Effects
// GMCP frames for actors whose active-effect snapshot has changed
// since the last emission. Called once per simulation tick from
// the gmcp-effects-flush handler.
//
// Mirrors FlushGmcpVitals: cadence-1 poll-and-diff, per-actor
// snapshots compared against a last-sent shadow. Single frame
// per session per tick max — one full Char.Effects list goes out
// only when the snapshot differs.
func (m *Manager) FlushGmcpEffects(ctx context.Context) {
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
		a.flushGmcpEffects(ctx)
	}
}

// flushGmcpEffects snapshots the actor's active effects, builds a
// CharEffectsList payload, and emits a Char.Effects frame when the
// snapshot differs from the last-sent shadow.
//
// Silent no-op when:
//
//   - the underlying conn doesn't speak GMCP;
//   - GMCP hasn't been negotiated;
//   - the actor has no EffectManager wired (test fakes).
//
// The valid flag (gmcpLastEffectsValid) distinguishes "never sent"
// from "sent and matches empty" — without it, an actor with no
// active effects at login would never get the baseline `[]` frame
// the panel needs to clear stale rows from a previous character.
func (a *connActor) flushGmcpEffects(ctx context.Context) {
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}
	if a.effects == nil {
		return
	}

	snap := a.snapshotEffects()

	a.gmcpEffectsMu.Lock()
	if a.gmcpLastEffectsValid && charEffectsEqual(a.gmcpLastEffects, snap) {
		a.gmcpEffectsMu.Unlock()
		return
	}
	a.gmcpLastEffects = snap
	a.gmcpLastEffectsValid = true
	a.gmcpEffectsMu.Unlock()

	payload := gmcp.CharEffectsList{Effects: snap}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := sender.SendGmcp(ctx, gmcp.PackageCharEffects, data); err != nil {
		logging.From(ctx).Debug("gmcp effects send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// snapshotEffects builds the sorted CharEffect slice for the
// actor's active effects. EffectManager.Effects already returns a
// deep copy sorted by id, so no further sort is needed here. The
// returned slice is always non-nil (possibly length zero) so the
// JSON encoder emits `[]` rather than `null` per CharEffectsList's
// contract.
func (a *connActor) snapshotEffects() []gmcp.CharEffect {
	src := a.effects.Effects(a.PlayerID())
	out := make([]gmcp.CharEffect, 0, len(src))
	for _, e := range src {
		out = append(out, effectToCharEffect(e))
	}
	return out
}

// effectToCharEffect converts a progression.Effect snapshot to the
// GMCP wire shape. Permanent effects clear remaining and set the
// permanent flag (the JSON omit-empty rule then strips both
// zeroed-out fields). Flag/source slices are copied through as-is
// — EffectManager.Effects already returned a deep copy, so the
// wire payload owns its own backing memory.
func effectToCharEffect(e progression.Effect) gmcp.CharEffect {
	out := gmcp.CharEffect{
		ID:     e.ID,
		Source: e.SourceAbilityID,
	}
	if e.IsPermanent() {
		out.Permanent = true
	} else {
		out.Remaining = e.Remaining
	}
	if len(e.Flags) > 0 {
		out.Flags = append([]string(nil), e.Flags...)
	}
	return out
}

// charEffectsEqual reports whether two CharEffect slices are
// element-wise equal. Both inputs are expected sorted by id
// (EffectManager.Effects guarantees this) so a positional compare
// is meaningful. Flags compare ordinally too — flag order is
// stable across snapshots because the manager copies the
// template's flag slice once at apply time and never reorders it.
func charEffectsEqual(a, b []gmcp.CharEffect) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID ||
			a[i].Remaining != b[i].Remaining ||
			a[i].Permanent != b[i].Permanent ||
			a[i].Source != b[i].Source ||
			!stringSlicesEqual(a[i].Flags, b[i].Flags) {
			return false
		}
	}
	return true
}

// stringSlicesEqual reports whether two string slices are
// element-wise equal. Local helper so gmcp_effects stays
// self-contained without pulling slices.Equal into a hot path
// shared with the flusher's per-tick allocation budget.
func stringSlicesEqual(a, b []string) bool {
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

// resetGmcpEffectsShadow marks the last-sent effects shadow
// invalid so the next flushGmcpEffects call emits unconditionally.
// Called on link-dead reattach: the new peer's Mudlet effects
// module needs a baseline frame even when the engine-side state
// hasn't changed across the drop.
func (a *connActor) resetGmcpEffectsShadow() {
	a.gmcpEffectsMu.Lock()
	a.gmcpLastEffectsValid = false
	a.gmcpLastEffects = nil
	a.gmcpEffectsMu.Unlock()
}
