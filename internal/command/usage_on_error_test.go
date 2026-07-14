package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
)

// Usage-on-error (ui-rendering-help §10.4): when an auto-resolved typed command
// fails on a MISSING required argument, the dispatcher appends the synthesized
// usage line. Other resolution failures (a value that's present but invalid, or
// a named target that doesn't resolve) already carry a specific message, so the
// usage echo is suppressed.
func TestUsageOnError(t *testing.T) {
	r := command.New()
	run := func(ctx context.Context, c *command.Context) error { return c.Actor.Write(ctx, "ran") }
	// A `number`-typed required arg: dispatch auto-resolves it (not HandParsed),
	// so a bare call short-circuits on ErrMissingRequired and a non-numeric
	// token fails with ErrNotANumber — two distinct failure modes.
	if err := r.RegisterCommand(command.Command{
		Keyword: "wager", Brief: "Bet an amount.", Handler: run,
		Args: []command.ArgDefinition{{Name: "amount", Type: command.ArgNumber}},
	}); err != nil {
		t.Fatal(err)
	}
	actor := newRoleActor("Bob", "p-1")
	ctx := context.Background()

	t.Run("missing arg appends usage", func(t *testing.T) {
		if err := r.Dispatch(ctx, command.Env{}, actor, "wager"); err != nil {
			t.Fatal(err)
		}
		out := actor.lastLine()
		if !strings.Contains(out, "What amount?") || !strings.Contains(out, "Usage: wager [amount]") {
			t.Errorf("missing-arg output = %q, want prompt + usage line", out)
		}
	})

	t.Run("invalid value omits usage", func(t *testing.T) {
		if err := r.Dispatch(ctx, command.Env{}, actor, "wager lots"); err != nil {
			t.Fatal(err)
		}
		out := actor.lastLine()
		if strings.Contains(out, "Usage:") {
			t.Errorf("invalid-value output = %q, should NOT carry a usage line", out)
		}
		if !strings.Contains(out, "not a number") {
			t.Errorf("invalid-value output = %q, want the resolver's message", out)
		}
	})

	t.Run("usage shows the typed alias form", func(t *testing.T) {
		// An alias routes to the same registration; the usage line names the
		// verb the player's input resolved to.
		if err := r.RegisterCommand(command.Command{
			Keyword: "bet", Aliases: []string{"stake"}, Brief: "Bet.", Handler: run,
			Args: []command.ArgDefinition{{Name: "amount", Type: command.ArgNumber}},
		}); err != nil {
			t.Fatal(err)
		}
		if err := r.Dispatch(ctx, command.Env{}, actor, "stake"); err != nil {
			t.Fatal(err)
		}
		if out := actor.lastLine(); !strings.Contains(out, "Usage: stake [amount]") {
			t.Errorf("alias usage = %q, want 'Usage: stake [amount]'", out)
		}
	})
}
