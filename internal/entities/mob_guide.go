package entities

// This file holds the guide-facing surface of MobInstance (onboarding-guide.md):
// is this owned mob an onboarding guide — a friendly, non-combat NPC that trails a
// new character and departs at a level threshold? Like the hireling marker it
// reuses the generic ownerID + SetOwner/OwnerID in mob_mount.go; this only
// distinguishes the guide role, so a guide is deliberately NOT a hireling
// (IsHireling stays false): it never fights, incurs no upkeep, counts against no
// cap, and credits no loot or XP.

// IsGuide reports whether this mob is an onboarding guide (materialized under an
// owner via the guide service). Guarded by ownerMu — the same runtime-state lock
// the owner + hireling fields use, set at materialization.
func (m *MobInstance) IsGuide() bool {
	m.ownerMu.RLock()
	defer m.ownerMu.RUnlock()
	return m.guide
}

// SetGuide marks (or clears) this mob as an onboarding guide (onboarding-guide.md).
// Called when a guide is materialized into the world under its owner, alongside
// SetOwner.
func (m *MobInstance) SetGuide(v bool) {
	m.ownerMu.Lock()
	defer m.ownerMu.Unlock()
	m.guide = v
}
