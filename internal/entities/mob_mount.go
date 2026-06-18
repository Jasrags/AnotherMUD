package entities

import "github.com/Jasrags/AnotherMUD/internal/mount"

// This file holds the mount-facing surface of MobInstance (mounts.md §2): is
// this mob a rideable mount, who owns it, its temperament, and its travel
// resource. The fields and build logic live in mob.go; these methods read
// them. A mount IS a mob with an owner and a travel pool — it reuses Vitals
// (it can be killed, §7.3), Stats (it can be barded, §8), and Contents (it can
// carry saddlebags, §8.3) unchanged.

// IsMount reports whether this mob is a rideable mount (its template carried a
// `mount:` block). Non-mounts answer false to every other method here.
func (m *MobInstance) IsMount() bool { return m.mountSpec != nil }

// Temperament returns the mount's danger-entry temperament (mounts.md §7.2),
// resolved at spawn. A non-mount returns the empty Temperament (which resolves
// to the cautious default if ever read) — callers gate on IsMount first.
func (m *MobInstance) Temperament() mount.Temperament { return m.temperament }

// OwnerID returns the id of the character who owns this mount (mounts.md §2.2),
// or "" when unowned. Read under RLock because ownership is runtime state
// (assigned at materialization, potentially reassigned by a future transfer).
func (m *MobInstance) OwnerID() string {
	m.ownerMu.RLock()
	defer m.ownerMu.RUnlock()
	return m.ownerID
}

// SetOwner records the owning character's id (mounts.md §2.2). Called when a
// mount is materialized into the world under its owner (purchase / un-stable).
// Ownership is exclusive — setting a new owner replaces any prior one.
func (m *MobInstance) SetOwner(id string) {
	m.ownerMu.Lock()
	defer m.ownerMu.Unlock()
	m.ownerID = id
}

// IsOwnedBy reports whether characterID owns this mount. The room-shared
// `mount`/`lead` verbs gate on this (§2.2: only the owner may ride or command
// it). An empty characterID never matches an unowned mount.
func (m *MobInstance) IsOwnedBy(characterID string) bool {
	if characterID == "" {
		return false
	}
	return m.OwnerID() == characterID
}

// Travel returns the mount's current travel-resource value (mounts.md §5.1),
// or 0 on a non-mount. This is the budget a mounted step spends (§5).
func (m *MobInstance) Travel() int {
	if m.travel == nil {
		return 0
	}
	return m.travel.Current()
}

// TravelMax returns the mount's travel-resource ceiling, or 0 on a non-mount.
func (m *MobInstance) TravelMax() int {
	if m.travel == nil {
		return 0
	}
	return m.travel.Max()
}

// TrySpendTravel deducts amount from the travel pool only if the mount can
// afford it, reporting success (mounts.md §5.4). A non-mount, a non-positive
// amount, or an insufficient pool returns false without mutating — the mounted
// step is then refused and the rider may dismount and walk (the never-strand
// rule, §6). The mounted-travel gate (§5) calls this with the destination's
// step cost as borne by the mount.
func (m *MobInstance) TrySpendTravel(amount int) bool {
	if m.travel == nil || amount <= 0 {
		return false
	}
	return m.travel.TrySpend(amount)
}

// RestoreTravel adds amount to the travel pool, capped at max (mounts.md §5.4).
// Called by the regen tick out of combat. No-op on a non-mount or a
// non-positive amount.
func (m *MobInstance) RestoreTravel(amount int) {
	if m.travel == nil || amount <= 0 {
		return
	}
	m.travel.Restore(amount)
}

// TravelRegenAmount returns the mount's per-regen-tick travel restore from
// content (mounts.md §5.4), or 0 when the template left it unset (the regen
// tick then applies the engine default) or the mob is not a mount.
func (m *MobInstance) TravelRegenAmount() int {
	if m.mountSpec == nil {
		return 0
	}
	return m.mountSpec.TravelRegen
}

// CannotEnterTerrain reports whether this mount is barred from a destination of
// the given terrain id (mounts.md §5.3) by its per-type impassable list. The
// room-level mount-impassable flag is a separate, broader gate checked by the
// travel handler. A non-mount or an empty terrain answers false.
func (m *MobInstance) CannotEnterTerrain(terrain string) bool {
	if m.mountSpec == nil || terrain == "" {
		return false
	}
	for _, t := range m.mountSpec.Impassable {
		if t == terrain {
			return true
		}
	}
	return false
}
