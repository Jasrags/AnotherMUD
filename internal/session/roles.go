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

	// Restore the saved set directly — a load, not a mutation, so it must
	// NOT mark the save dirty.
	a.roles = make(map[string]struct{}, len(saved))
	for _, r := range saved {
		if n := normalizeRole(r); n != "" {
			a.roles[n] = struct{}{}
		}
	}

	// Seed (keyed by lowercased character name — what an operator
	// configures) is applied through grantRoleLocked: the same idempotent
	// add → sync → dirty path the grant verb (M19.2) will use. A newly
	// seeded role persists + dirties; an already-held one is a silent
	// no-op. The seed re-ensures on EVERY login (spec §5 "ensured
	// present"), so a seeded role cannot be revoked in-game while the name
	// stays in the seed — the bootstrap admin can't lock themselves out.
	if cfg == nil || a.save == nil {
		return
	}
	for _, r := range cfg.RoleSeed[normalizeRole(a.save.Name)] {
		a.grantRoleLocked(r)
	}
}
