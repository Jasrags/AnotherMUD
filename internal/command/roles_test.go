package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// roleActor is a namedActor plus the command.RoleController surface, so it
// can act as the granter (c.Actor) and as a grant target.
type roleActor struct {
	*namedActor
	roles map[string]bool
	tags  []string // admin-applied gameplay tags, for `set tag` on a player target
}

// AddTag / RemoveTag give the test player the connActor tagger surface
// (admin-verbs §4 `set tag`) so applyTag can route a player target in
// command-layer tests. Idempotent, mirroring the real connActor.
func (r *roleActor) AddTag(tag string) bool {
	for _, t := range r.tags {
		if t == tag {
			return false
		}
	}
	r.tags = append(r.tags, tag)
	return true
}

func (r *roleActor) RemoveTag(tag string) bool {
	out := r.tags[:0]
	removed := false
	for _, t := range r.tags {
		if t == tag {
			removed = true
			continue
		}
		out = append(out, t)
	}
	r.tags = out
	return removed
}

func (r *roleActor) hasTag(tag string) bool {
	for _, t := range r.tags {
		if t == tag {
			return true
		}
	}
	return false
}

func newRoleActor(name, playerID string, roles ...string) *roleActor {
	ra := &roleActor{
		namedActor: &namedActor{testActor: newTestActor(nil), name: name, playerID: playerID},
		roles:      map[string]bool{},
	}
	for _, r := range roles {
		ra.roles[strings.ToLower(r)] = true
	}
	return ra
}

func (r *roleActor) HasRole(role string) bool {
	return r.roles[strings.ToLower(strings.TrimSpace(role))]
}

func (r *roleActor) Grant(role string) bool {
	k := strings.ToLower(strings.TrimSpace(role))
	if r.roles[k] {
		return false
	}
	r.roles[k] = true
	return true
}

func (r *roleActor) Revoke(role string) bool {
	k := strings.ToLower(strings.TrimSpace(role))
	if !r.roles[k] {
		return false
	}
	delete(r.roles, k)
	return true
}

// fakeRoleResolver maps lowercased names to online role targets.
type fakeRoleResolver map[string]command.RoleController

func (f fakeRoleResolver) ResolveRoleTarget(name string) (command.RoleController, bool) {
	c, ok := f[strings.ToLower(strings.TrimSpace(name))]
	return c, ok
}

func dispatchRole(t *testing.T, env command.Env, actor command.Actor, input string) {
	t.Helper()
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, actor, input); err != nil {
		t.Fatalf("dispatch %q: %v", input, err)
	}
}

// A non-granter is refused generically — and the refusal must not disclose
// the gating role's name (§3). Nothing changes; no event.
func TestGrant_RefusesNonGranterWithoutDisclosure(t *testing.T) {
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventRoleGranted)
	bob := newRoleActor("Bob", "p-bob")
	alice := newRoleActor("Alice", "p-alice") // no admin role
	env := command.Env{Bus: bus, GrantingRole: "admin", RoleTargetResolver: fakeRoleResolver{"bob": bob}}

	dispatchRole(t, env, alice, "grant builder to bob")

	if bob.HasRole("builder") {
		t.Error("a non-granter must not be able to grant")
	}
	if len(*got) != 0 {
		t.Errorf("no event should fire on refusal, got %d", len(*got))
	}
	line := alice.lastLine()
	if strings.Contains(strings.ToLower(line), "admin") || strings.Contains(strings.ToLower(line), "role") {
		t.Errorf("refusal %q discloses the gating role", line)
	}
}

// An admin grants a role to another online player; the target gains it and
// one role.granted fires carrying actor/target/role.
func TestGrant_GrantsAndPublishes(t *testing.T) {
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventRoleGranted)
	bob := newRoleActor("Bob", "p-bob")
	admin := newRoleActor("Maerys", "p-admin", "admin")
	env := command.Env{Bus: bus, GrantingRole: "admin", RoleTargetResolver: fakeRoleResolver{"bob": bob}}

	dispatchRole(t, env, admin, "grant Builder to bob")

	if !bob.HasRole("builder") {
		t.Error("target should hold the granted role (normalized)")
	}
	if len(*got) != 1 {
		t.Fatalf("role.granted count = %d, want 1", len(*got))
	}
	ev := (*got)[0].(eventbus.RoleGranted)
	if ev.Actor != "p-admin" || ev.Target != "p-bob" || ev.Role != "builder" {
		t.Errorf("event = %+v, want actor=p-admin target=p-bob role=builder", ev)
	}
}

// Granting a held role is an idempotent no-op: no event, friendly message.
func TestGrant_IdempotentNoEvent(t *testing.T) {
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventRoleGranted)
	bob := newRoleActor("Bob", "p-bob", "builder")
	admin := newRoleActor("Maerys", "p-admin", "admin")
	env := command.Env{Bus: bus, GrantingRole: "admin", RoleTargetResolver: fakeRoleResolver{"bob": bob}}

	dispatchRole(t, env, admin, "grant builder to bob")

	if len(*got) != 0 {
		t.Errorf("idempotent grant should not publish, got %d", len(*got))
	}
	if !strings.Contains(admin.lastLine(), "already has") {
		t.Errorf("message = %q, want 'already has'", admin.lastLine())
	}
}

// Revoke removes a held role and publishes role.revoked.
func TestRevoke_RevokesAndPublishes(t *testing.T) {
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventRoleRevoked)
	bob := newRoleActor("Bob", "p-bob", "builder")
	admin := newRoleActor("Maerys", "p-admin", "admin")
	env := command.Env{Bus: bus, GrantingRole: "admin", RoleTargetResolver: fakeRoleResolver{"bob": bob}}

	dispatchRole(t, env, admin, "revoke builder from bob")

	if bob.HasRole("builder") {
		t.Error("role should be revoked")
	}
	if len(*got) != 1 {
		t.Fatalf("role.revoked count = %d, want 1", len(*got))
	}
	if ev := (*got)[0].(eventbus.RoleRevoked); ev.Target != "p-bob" || ev.Role != "builder" {
		t.Errorf("event = %+v", ev)
	}
}

// Revoking an unheld role is a no-op: no event.
func TestRevoke_IdempotentNoEvent(t *testing.T) {
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventRoleRevoked)
	bob := newRoleActor("Bob", "p-bob")
	admin := newRoleActor("Maerys", "p-admin", "admin")
	env := command.Env{Bus: bus, GrantingRole: "admin", RoleTargetResolver: fakeRoleResolver{"bob": bob}}

	dispatchRole(t, env, admin, "revoke builder from bob")
	if len(*got) != 0 {
		t.Errorf("idempotent revoke should not publish, got %d", len(*got))
	}
}

// An admin cannot grant or revoke their own roles (§1.1 — not self-service).
func TestGrant_SelfBlocked(t *testing.T) {
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventRoleGranted)
	admin := newRoleActor("Maerys", "p-admin", "admin")
	env := command.Env{Bus: bus, GrantingRole: "admin", RoleTargetResolver: fakeRoleResolver{"maerys": admin}}

	dispatchRole(t, env, admin, "grant builder to maerys")

	if admin.HasRole("builder") {
		t.Error("self-grant must be blocked even for an admin")
	}
	if len(*got) != 0 {
		t.Errorf("self-grant should publish nothing, got %d", len(*got))
	}
	if !strings.Contains(admin.lastLine(), "yourself") {
		t.Errorf("message = %q, want a self-grant refusal", admin.lastLine())
	}
}

// Targeting an offline / unknown player is refused (v1 online-only).
func TestGrant_TargetNotOnline(t *testing.T) {
	admin := newRoleActor("Maerys", "p-admin", "admin")
	env := command.Env{GrantingRole: "admin", RoleTargetResolver: fakeRoleResolver{}}
	dispatchRole(t, env, admin, "grant builder to ghost")
	if !strings.Contains(strings.ToLower(admin.lastLine()), "online") {
		t.Errorf("message = %q, want a not-online refusal", admin.lastLine())
	}
}

// With no resolver wired, role administration reports disabled.
func TestGrant_ResolverNilDisabled(t *testing.T) {
	admin := newRoleActor("Maerys", "p-admin", "admin")
	env := command.Env{GrantingRole: "admin"} // no RoleTargetResolver
	dispatchRole(t, env, admin, "grant builder to bob")
	if !strings.Contains(admin.lastLine(), "not enabled") {
		t.Errorf("message = %q, want 'not enabled'", admin.lastLine())
	}
}

// Bad argument forms render usage; the lenient parser accepts both
// `grant <role> to <player>` and `grant <role> <player>`.
func TestGrant_UsageAndLenientParse(t *testing.T) {
	admin := newRoleActor("Maerys", "p-admin", "admin")
	bob := newRoleActor("Bob", "p-bob")
	env := command.Env{GrantingRole: "admin", RoleTargetResolver: fakeRoleResolver{"bob": bob}}

	dispatchRole(t, env, admin, "grant builder") // too few args
	if !strings.Contains(admin.lastLine(), "Usage") {
		t.Errorf("message = %q, want usage", admin.lastLine())
	}

	// No preposition — lenient parse should still work.
	dispatchRole(t, env, admin, "grant builder bob")
	if !bob.HasRole("builder") {
		t.Error("`grant <role> <player>` (no preposition) should work")
	}
}
