package entities

// This file holds the hireling-facing surface of MobInstance (hireable-mobs.md
// §2): is this owned mob a hireling (a companion that fights for its owner) as
// opposed to a mount? Both reuse the generic ownerID + SetOwner/OwnerID/IsOwnedBy
// in mob_mount.go; this only distinguishes the two roles. A hireling IS a mob with
// an owner — it reuses Vitals (it can be killed, §6.2), Stats, and Contents
// unchanged.

// IsHireling reports whether this mob is a hireling (materialized under an owner
// via the hireling service). Guarded by ownerMu — the same runtime-state lock
// the owner field uses, set at materialization.
func (m *MobInstance) IsHireling() bool {
	m.ownerMu.RLock()
	defer m.ownerMu.RUnlock()
	return m.hireling
}

// SetHireling marks (or clears) this mob as a hireling (hireable-mobs.md §3.1).
// Called when a hireling is materialized into the world under its owner, alongside
// SetOwner.
func (m *MobInstance) SetHireling(v bool) {
	m.ownerMu.Lock()
	defer m.ownerMu.Unlock()
	m.hireling = v
}
