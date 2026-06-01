package session

import (
	"sort"
	"strings"
)

// Roles & permissions — the per-character role set, the read-only HasRole
// check, and the construction-time seed/restore (roles-and-permissions.md
// §2/§3/§5/§6). Grant/revoke verbs + their events land in M19.2; the
// grant/revoke mutators here are the substrate those verbs will drive.

// normalizeRole lowercases + trims a role name so "Admin", "ADMIN", and
// " admin " denote the same role (roles-and-permissions §2). The empty
// string is not a role.
func normalizeRole(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// HasRole reports whether the actor holds role, compared case-insensitively
// (roles-and-permissions §3 — the one authorization question). Read-only:
// it never mutates the role set.
func (a *connActor) HasRole(role string) bool {
	r := normalizeRole(role)
	if r == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.roles[r]
	return ok
}

// Roles returns a sorted snapshot of the actor's held roles. Fresh slice —
// safe to mutate. Empty set returns nil.
func (a *connActor) Roles() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return rolesSnapshotLocked(a.roles)
}

// rolesSnapshotLocked returns the set's keys sorted. Caller holds a.mu.
func rolesSnapshotLocked(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// grantRoleLocked adds a normalized role to the live set and the save,
// marking the save dirty when the set actually changes. Idempotent: a
// no-op (returns false) when the role is already held or empty. Caller
// holds a.mu. (Seeds the bootstrap admin today; the grant verb drives it
// in M19.2.)
func (a *connActor) grantRoleLocked(role string) bool {
	r := normalizeRole(role)
	if r == "" {
		return false
	}
	if a.roles == nil {
		a.roles = make(map[string]struct{}, 1)
	}
	if _, ok := a.roles[r]; ok {
		return false
	}
	a.roles[r] = struct{}{}
	a.syncRolesToSaveLocked()
	a.markDirtyLocked()
	return true
}

// revokeRoleLocked removes a normalized role from the live set and the
// save, marking dirty when it changes. Idempotent: a no-op (returns false)
// when the role is not held. Caller holds a.mu.
func (a *connActor) revokeRoleLocked(role string) bool {
	r := normalizeRole(role)
	if _, ok := a.roles[r]; !ok {
		return false
	}
	delete(a.roles, r)
	a.syncRolesToSaveLocked()
	a.markDirtyLocked()
	return true
}

// syncRolesToSaveLocked mirrors the live set into save.Roles (sorted) so it
// persists. Caller holds a.mu. An empty set clears the field so a roleless
// save writes no `roles:` key.
func (a *connActor) syncRolesToSaveLocked() {
	if a.save == nil {
		return
	}
	a.save.Roles = rolesSnapshotLocked(a.roles)
}

// applyRoles builds the actor's live role set at construction: the saved
// roles first (restored, normalized), then the config seed for this
// character ensured present (roles-and-permissions §5/§6 — additive,
// idempotent, never revokes). A seed that adds a new role marks the save
// dirty so the bootstrap admin persists on first login. Called once during
// construction, beside applyRace/applyClass; the actor is not yet serving,
// so the lock is uncontended.
func applyRoles(a *connActor, cfg *Config, saved []string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.roles = make(map[string]struct{}, len(saved))
	for _, r := range saved {
		if n := normalizeRole(r); n != "" {
			a.roles[n] = struct{}{}
		}
	}

	// Seed is keyed by lowercased character name (what an operator
	// configures). Additive over the restored set; a seeded role survives
	// even on a save that predates roles.
	if cfg == nil || len(cfg.RoleSeed) == 0 || a.save == nil {
		return
	}
	changed := false
	for _, r := range cfg.RoleSeed[normalizeRole(a.save.Name)] {
		n := normalizeRole(r)
		if n == "" {
			continue
		}
		if _, ok := a.roles[n]; !ok {
			a.roles[n] = struct{}{}
			changed = true
		}
	}
	if changed {
		a.syncRolesToSaveLocked()
		a.markDirtyLocked()
	}
}
