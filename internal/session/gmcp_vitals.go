package session

import "context"

// FlushGmcpVitals walks every live session and emits a Char.Vitals
// GMCP frame to actors whose snapshot has changed since the last
// emission. Called once per simulation tick from the
// gmcp-vitals-flush handler the composition root registers.
//
// PD-3 (modern-client-plan): the "dirty bit" the pre-decision
// described is implemented as poll-and-diff against a per-actor
// last-sent shadow — flushGmcpVitals reads the current vitals via
// cheap thread-safe accessors and only writes the wire frame
// when the payload differs. The user-observable contract from
// PD-3 is preserved: at most one Char.Vitals frame per session
// per tick, and zero frames when nothing changed. The deviation
// avoids instrumenting every Vitals.ApplyDamage / Heal / SetMax
// call site with mark-dirty hooks.
//
// Link-dead sessions and non-GMCP transports are skipped silently
// via the per-actor flushGmcpVitals; this walker is the
// scatter-gather point.
func (m *Manager) FlushGmcpVitals(ctx context.Context) {
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
		a.flushGmcpVitals(ctx)
	}
}
