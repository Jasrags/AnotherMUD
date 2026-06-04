package command_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/command"
)

// TestDispatch_BadInputTracking proves §6 routing: an unknown verb is
// recorded; a known verb is not; and an admin verb refused to a non-admin
// (also "Huh?") is NOT recorded — it's a known verb, not bad input.
func TestDispatch_BadInputTracking(t *testing.T) {
	r := command.New()
	noop := func(ctx context.Context, c *command.Context) error { return nil }
	if err := r.RegisterCommand(command.Command{Keyword: "n", Brief: "n", Handler: noop}); err != nil {
		t.Fatalf("register n: %v", err)
	}
	if err := r.RegisterCommand(command.Command{Keyword: "secret", Admin: true, Brief: "s", Handler: noop}); err != nil {
		t.Fatalf("register secret: %v", err)
	}

	tr := command.NewBadInputTracker(clock.NewManual(time.Unix(0, 0)))
	a := newNamedTestActor("Alice", "p1", nil) // not an admin role-holder
	env := command.Env{BadInput: tr}
	ctx := context.Background()

	_ = r.Dispatch(ctx, env, a, "n")         // known → no record
	_ = r.Dispatch(ctx, env, a, "xyzzy")     // unknown → record
	_ = r.Dispatch(ctx, env, a, "Xyzzy arg") // unknown again (case-folded) → count 2
	_ = r.Dispatch(ctx, env, a, "secret")    // admin verb, non-admin → Huh? but NOT bad input

	snap := tr.Snapshot()
	if len(snap) != 1 || snap[0].Verb != "xyzzy" || snap[0].Count != 2 {
		t.Errorf("snapshot = %+v, want only xyzzy×2", snap)
	}
}

func TestBadInputHandler_RendersAndClears(t *testing.T) {
	tr := command.NewBadInputTracker(clock.NewManual(time.Unix(0, 0)))
	tr.Record("xyzzy")
	tr.Record("xyzzy")
	tr.Record("frobnicate")

	a := newNamedTestActor("Admin", "p1", nil)
	c := &command.Context{Actor: a, BadInput: tr}
	if err := command.BadInputHandler(context.Background(), c); err != nil {
		t.Fatalf("BadInputHandler: %v", err)
	}
	out := a.lastLine()
	if !strings.Contains(out, "xyzzy") || !strings.Contains(out, "frobnicate") {
		t.Errorf("report missing verbs: %q", out)
	}

	c.Args = []string{"clear"}
	if err := command.BadInputHandler(context.Background(), c); err != nil {
		t.Fatalf("BadInputHandler clear: %v", err)
	}
	if !strings.Contains(a.lastLine(), "cleared") {
		t.Errorf("clear reply = %q", a.lastLine())
	}
	if len(tr.Snapshot()) != 0 {
		t.Error("tracker not cleared")
	}
}

func TestBadInputHandler_NilTracker(t *testing.T) {
	a := newNamedTestActor("Admin", "p1", nil)
	c := &command.Context{Actor: a} // no BadInput
	if err := command.BadInputHandler(context.Background(), c); err != nil {
		t.Fatalf("BadInputHandler: %v", err)
	}
	if !strings.Contains(a.lastLine(), "not enabled") {
		t.Errorf("nil-tracker reply = %q", a.lastLine())
	}
}
