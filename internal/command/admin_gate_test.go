package command_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/help"
)

func adminCmdRegistry(t *testing.T) *command.Registry {
	t.Helper()
	r := command.New()
	run := func(ctx context.Context, c *command.Context) error { return c.Actor.Write(ctx, "ran") }
	if err := r.RegisterCommand(command.Command{Keyword: "secret", Admin: true, Brief: "x", Handler: run}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterCommand(command.Command{Keyword: "open", Brief: "y", Handler: run}); err != nil {
		t.Fatal(err)
	}
	return r
}

// An admin-marked command runs for an actor holding the admin role (§2).
func TestAdminGate_AllowsAdmin(t *testing.T) {
	r := adminCmdRegistry(t)
	admin := newRoleActor("Maerys", "p-1", "admin")
	if err := r.Dispatch(context.Background(), command.Env{AdminRole: "admin"}, admin, "secret"); err != nil {
		t.Fatal(err)
	}
	if admin.lastLine() != "ran" {
		t.Errorf("admin should run the command, got %q", admin.lastLine())
	}
}

// The admin role defaults to `admin` when Env.AdminRole is unset.
func TestAdminGate_DefaultsToAdmin(t *testing.T) {
	r := adminCmdRegistry(t)
	admin := newRoleActor("Maerys", "p-1", "admin")
	if err := r.Dispatch(context.Background(), command.Env{}, admin, "secret"); err != nil {
		t.Fatal(err)
	}
	if admin.lastLine() != "ran" {
		t.Errorf("admin should run with default admin role, got %q", admin.lastLine())
	}
}

// A non-admin's refusal is IDENTICAL to the unknown-verb response — the
// verb's existence is not disclosed (§2).
func TestAdminGate_RefusesNonAdminIndistinguishablyFromUnknown(t *testing.T) {
	r := adminCmdRegistry(t)
	plebe := newRoleActor("Bob", "p-2") // no admin role
	env := command.Env{AdminRole: "admin"}

	if err := r.Dispatch(context.Background(), env, plebe, "secret"); err != nil {
		t.Fatal(err)
	}
	refused := plebe.lastLine()

	if err := r.Dispatch(context.Background(), env, plebe, "xyzzy-not-a-verb"); err != nil {
		t.Fatal(err)
	}
	unknown := plebe.lastLine()

	if refused != unknown {
		t.Errorf("admin refusal %q must equal unknown-verb %q (no disclosure)", refused, unknown)
	}
	if refused != "Huh?" {
		t.Errorf("refusal = %q, want %q", refused, "Huh?")
	}
}

// An actor that doesn't expose HasRole is treated as holding no roles.
func TestAdminGate_NonRoleHolderRefused(t *testing.T) {
	r := adminCmdRegistry(t)
	plain := newTestActor(nil) // plain testActor: no RoleHolder
	if err := r.Dispatch(context.Background(), command.Env{AdminRole: "admin"}, plain, "secret"); err != nil {
		t.Fatal(err)
	}
	if plain.lastLine() != "Huh?" {
		t.Errorf("non-role-holder = %q, want Huh?", plain.lastLine())
	}
}

// A non-admin command is unaffected by the gate.
func TestAdminGate_NonAdminCommandRunsForAnyone(t *testing.T) {
	r := adminCmdRegistry(t)
	plebe := newRoleActor("Bob", "p-2")
	if err := r.Dispatch(context.Background(), command.Env{AdminRole: "admin"}, plebe, "open"); err != nil {
		t.Fatal(err)
	}
	if plebe.lastLine() != "ran" {
		t.Errorf("non-admin command should run for anyone, got %q", plebe.lastLine())
	}
}

// Admin commands are hidden from non-admins in help (§2): their generated
// topic takes the admin tier, which the help service filters out for a
// non-admin (player-tier) viewer — closing the help enumeration vector.
func TestAdminGate_HidesAdminCommandFromNonAdminHelp(t *testing.T) {
	r := adminCmdRegistry(t)
	svc := help.NewService()
	command.GenerateHelpTopics(r, svc)

	if res := svc.Query("", "secret"); res.Topic != nil {
		t.Error("admin command `secret` should be hidden from a non-admin in help")
	}
	if res := svc.Query("", "open"); res.Topic == nil {
		t.Error("normal command `open` should be visible in help")
	}
}

// The standing ungated verbs reload and xp are now admin-gated (§2).
func TestAdminGate_GatesReloadAndXP(t *testing.T) {
	r := newRegistry(t) // builtins, incl. admin-marked reload + xp
	plebe := newRoleActor("Bob", "p-2")
	env := command.Env{AdminRole: "admin"}
	for _, verb := range []string{"reload", "xp", "grant", "revoke"} {
		if err := r.Dispatch(context.Background(), env, plebe, verb+" x"); err != nil {
			t.Fatal(err)
		}
		if plebe.lastLine() != "Huh?" {
			t.Errorf("%s for non-admin = %q, want Huh? (admin-gated)", verb, plebe.lastLine())
		}
	}
}
