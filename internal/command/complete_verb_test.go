package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
)

// §9: the `complete` debug verb runs the query for an admin and prints
// the candidate set.
func TestCompleteVerb_AdminListsCandidates(t *testing.T) {
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatal(err)
	}
	admin := newRoleActor("Maerys", "p-1", "admin")
	if err := r.Dispatch(context.Background(), command.Env{AdminRole: "admin"}, admin, "complete loo"); err != nil {
		t.Fatal(err)
	}
	out := admin.lastLine()
	if !strings.Contains(out, "look") {
		t.Errorf("complete loo should list 'look', got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "verb") {
		t.Errorf("complete loo should report the verb-slot target, got:\n%s", out)
	}
}

// A HandParsed command declares Args (for completion/help) but the
// dispatcher must NOT auto-resolve them — the handler runs and parses
// raw Args itself, even when a declared arg would fail to resolve.
func TestDispatch_HandParsedSkipsAutoResolve(t *testing.T) {
	r := command.New()
	ran := false
	sawResolved := false
	if err := r.RegisterCommand(command.Command{
		Keyword:    "probe",
		HandParsed: true,
		Args:       []command.ArgDefinition{{Name: "item", Type: command.ArgInventory}},
		Handler: func(ctx context.Context, c *command.Context) error {
			ran = true
			sawResolved = c.Resolved != nil
			return c.Actor.Write(ctx, "ok")
		},
	}); err != nil {
		t.Fatal(err)
	}
	a := newTestActor(nil) // empty inventory → the declared arg would fail
	if err := r.Dispatch(context.Background(), command.Env{}, a, "probe nonexistent"); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("HandParsed handler must run despite an unresolvable declared arg")
	}
	if sawResolved {
		t.Error("HandParsed must not populate c.Resolved")
	}
}

// §9: a non-admin's `complete` is refused IDENTICALLY to an unknown verb
// — the debug tool's existence is not disclosed.
func TestCompleteVerb_NonAdminRefusedLikeUnknown(t *testing.T) {
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatal(err)
	}
	pleb := newRoleActor("Bob", "p-2") // no admin role
	env := command.Env{AdminRole: "admin"}

	if err := r.Dispatch(context.Background(), env, pleb, "complete loo"); err != nil {
		t.Fatal(err)
	}
	refused := pleb.lastLine()

	if err := r.Dispatch(context.Background(), env, pleb, "xyzzy-not-a-verb"); err != nil {
		t.Fatal(err)
	}
	unknown := pleb.lastLine()

	if refused != unknown {
		t.Errorf("refusal %q must equal unknown-verb %q (no disclosure)", refused, unknown)
	}
	if refused != "Huh?" {
		t.Errorf("refusal = %q, want %q", refused, "Huh?")
	}
}
