package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// grantRole/revokeRole wrap the locked mutators for tests (they assert the
// a.mu contract the verb path will satisfy).
func grantRole(a *connActor, role string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.grantRoleLocked(role)
}

func revokeRole(a *connActor, role string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.revokeRoleLocked(role)
}

func newRoleActor(name string, saved ...string) *connActor {
	a := &connActor{save: &player.Save{Version: player.CurrentVersion, Name: name, Roles: saved}}
	applyRoles(a, &Config{}, saved)
	return a
}

// HasRole is case-insensitive and an empty/new actor holds no roles (§2/§3).
func TestHasRole_CaseInsensitiveAndEmptyDefault(t *testing.T) {
	a := newRoleActor("Alice")
	if a.HasRole("admin") {
		t.Error("new actor should hold no roles")
	}
	if !grantRole(a, "Admin") {
		t.Fatal("grant should report a change")
	}
	for _, q := range []string{"admin", "ADMIN", " Admin "} {
		if !a.HasRole(q) {
			t.Errorf("HasRole(%q) = false, want true (case-insensitive)", q)
		}
	}
	if a.HasRole("builder") {
		t.Error("HasRole(builder) should be false")
	}
}

// HasRole never mutates the set (§3).
func TestHasRole_ReadOnly(t *testing.T) {
	a := newRoleActor("Alice", "admin")
	_ = a.HasRole("admin")
	_ = a.HasRole("nope")
	if got := a.Roles(); len(got) != 1 || got[0] != "admin" {
		t.Errorf("Roles after HasRole calls = %v, want [admin]", got)
	}
}

// Grant and revoke are idempotent (§2).
func TestGrantRevoke_Idempotent(t *testing.T) {
	a := newRoleActor("Alice")
	if !grantRole(a, "admin") {
		t.Fatal("first grant should change")
	}
	if grantRole(a, "ADMIN") {
		t.Error("granting an already-held role (normalized) should be a no-op")
	}
	if !revokeRole(a, "admin") {
		t.Fatal("revoke of held role should change")
	}
	if revokeRole(a, "admin") {
		t.Error("revoking an unheld role should be a no-op")
	}
	if a.HasRole("admin") {
		t.Error("admin should be revoked")
	}
}

// applyRoles restores saved roles (normalized) into the live set (§6).
func TestApplyRoles_RestoresSaved(t *testing.T) {
	a := newRoleActor("Alice", "Admin", " builder ", "")
	got := a.Roles()
	if len(got) != 2 || got[0] != "admin" || got[1] != "builder" {
		t.Errorf("restored roles = %v, want [admin builder] (normalized, blank dropped)", got)
	}
}

// The config seed is additive over the restored set, persists, and marks
// the save dirty (§5/§6).
func TestApplyRoles_SeedAdditiveAndPersists(t *testing.T) {
	a := &connActor{save: &player.Save{Version: player.CurrentVersion, Name: "Maerys", Roles: []string{"builder"}}}
	cfg := &Config{RoleSeed: map[string][]string{"maerys": {"admin"}}}
	applyRoles(a, cfg, a.save.Roles)

	if !a.HasRole("admin") || !a.HasRole("builder") {
		t.Errorf("after seed, roles = %v, want both admin (seeded) + builder (saved)", a.Roles())
	}
	if got := a.save.Roles; len(got) != 2 || got[0] != "admin" || got[1] != "builder" {
		t.Errorf("save.Roles = %v, want persisted [admin builder]", got)
	}
	if !a.dirty {
		t.Error("a seed that added a role should mark the save dirty")
	}
}

// Re-applying the seed is idempotent — no duplication, not dirty when the
// role is already present (§5).
func TestApplyRoles_SeedIdempotent(t *testing.T) {
	a := &connActor{save: &player.Save{Version: player.CurrentVersion, Name: "Maerys", Roles: []string{"admin"}}}
	cfg := &Config{RoleSeed: map[string][]string{"maerys": {"Admin"}}}
	applyRoles(a, cfg, a.save.Roles)

	if got := a.Roles(); len(got) != 1 || got[0] != "admin" {
		t.Errorf("roles = %v, want [admin] (no duplication)", got)
	}
	if a.dirty {
		t.Error("a seed that adds nothing new should not mark dirty")
	}
}

// A character not named in the seed gains nothing from it (§5).
func TestApplyRoles_SeedDoesNotTouchOthers(t *testing.T) {
	a := &connActor{save: &player.Save{Version: player.CurrentVersion, Name: "Alice"}}
	cfg := &Config{RoleSeed: map[string][]string{"maerys": {"admin"}}}
	applyRoles(a, cfg, nil)
	if len(a.Roles()) != 0 {
		t.Errorf("unseeded character gained roles: %v", a.Roles())
	}
}

// Roles are a separate namespace from gameplay tags: granting a role does
// not appear in Tags(), and a racial/alignment tag is not a role (§2).
func TestRoles_SeparateFromGameplayTags(t *testing.T) {
	a := newRoleActor("Alice")
	a.racialTags = []string{"humanoid"}
	a.alignmentTag = "good"
	grantRole(a, "admin")

	for _, tg := range a.Tags() {
		if tg == "admin" {
			t.Error("a role leaked into Tags()")
		}
	}
	if a.HasRole("humanoid") || a.HasRole("good") {
		t.Error("a gameplay tag must not be a role")
	}
}

// Grant/revoke mirror into save.Roles so the change persists (§6).
func TestGrantRevoke_SyncsToSave(t *testing.T) {
	a := newRoleActor("Alice")
	grantRole(a, "admin")
	if got := a.save.Roles; len(got) != 1 || got[0] != "admin" {
		t.Errorf("save.Roles after grant = %v, want [admin]", got)
	}
	revokeRole(a, "admin")
	if len(a.save.Roles) != 0 {
		t.Errorf("save.Roles after revoke = %v, want empty", a.save.Roles)
	}
}
