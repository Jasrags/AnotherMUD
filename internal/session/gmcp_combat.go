package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// FlushGmcpCombat walks every live session and emits Char.Combat
// GMCP frames for actors whose combat snapshot has changed since
// the last emission. Called once per simulation tick from the
// gmcp-combat-flush handler.
//
// Mirrors FlushGmcpVitals: cadence-1 poll-and-diff, per-actor
// snapshots compared against a last-sent shadow. Snapshot
// captures the in-combat flag plus the primary target's name +
// id + HP — the rest of the actor's combat list is intentionally
// dropped (M16.4d ships a single-target HUD; multi-opponent
// panels can extend the payload later).
func (m *Manager) FlushGmcpCombat(ctx context.Context) {
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
		a.flushGmcpCombat(ctx)
	}
}

// flushGmcpCombat snapshots the actor's combat state, builds a
// CharCombat payload, and emits a Char.Combat frame when the
// snapshot differs from the last-sent shadow.
//
// Silent no-op when:
//
//   - the underlying conn doesn't speak GMCP;
//   - GMCP hasn't been negotiated;
//   - the actor has no combat.Manager wired (test fakes);
//   - the payload matches the last-sent shadow exactly.
//
// The combat locator is optional: when nil, the payload carries
// only the in_combat flag (no target name / HP). Avoids forcing
// every wiring path to set up a locator just to opt into GMCP.
func (a *connActor) flushGmcpCombat(ctx context.Context) {
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}
	if a.combat == nil {
		return
	}

	payload := a.snapshotCombat()

	a.gmcpCombatMu.Lock()
	if a.gmcpLastCombatValid && a.gmcpLastCombat == payload {
		a.gmcpCombatMu.Unlock()
		return
	}
	a.gmcpLastCombat = payload
	a.gmcpLastCombatValid = true
	a.gmcpCombatMu.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := sender.SendGmcp(ctx, gmcp.PackageCharCombat, data); err != nil {
		logging.From(ctx).Debug("gmcp combat send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// snapshotCombat builds the CharCombat payload from the actor's
// current combat state. Resolves the primary target via the
// combat locator to fill name + HP. Returns the zero (in_combat
// false, all target fields empty) when the actor is not engaged
// — the JSON omit-empty rules then strip the target half from
// the wire.
func (a *connActor) snapshotCombat() gmcp.CharCombat {
	cid := a.CombatantID()
	if !a.combat.InCombat(cid) {
		return gmcp.CharCombat{InCombat: false}
	}
	targetID, ok := a.combat.PrimaryTargetOf(cid)
	if !ok {
		// InCombat said yes but the list is now empty — race
		// between the two reads. Treat as not-in-combat for this
		// snapshot; the next tick will see the consistent state.
		return gmcp.CharCombat{InCombat: false}
	}
	out := gmcp.CharCombat{
		InCombat: true,
		TargetID: string(targetID),
	}
	if a.combatLocator != nil {
		if target, ok := a.combatLocator.LookupCombatant(targetID); ok {
			out.Target = target.Name()
			if vit := target.Vitals(); vit != nil {
				hp, max := vit.Snapshot()
				out.TargetHP = hp
				out.TargetMaxHP = max
				if max > 0 {
					out.TargetHPPercent = (hp * 100) / max
				}
			}
		}
	}
	return out
}

// resetGmcpCombatShadow marks the last-sent combat shadow invalid
// so the next flush emits unconditionally. Called on link-dead
// reattach alongside the vitals + items shadows.
func (a *connActor) resetGmcpCombatShadow() {
	a.gmcpCombatMu.Lock()
	a.gmcpLastCombatValid = false
	a.gmcpCombatMu.Unlock()
}
