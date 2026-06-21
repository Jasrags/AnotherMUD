package command_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/action"
	"github.com/Jasrags/AnotherMUD/internal/command"
)

// An IsAction command runs while idle, is refused while the actor has a timed
// action in flight (with the "You are busy <label>." message), and is allowed
// through when this dispatch is the deferred completion replaying it
// (Env.ReplayAction) — action-economy.md §4.
func TestBusyGate(t *testing.T) {
	r := command.New()
	var ran bool
	run := func(ctx context.Context, c *command.Context) error {
		ran = true
		return c.Actor.Write(ctx, "ran")
	}
	if err := r.RegisterCommand(command.Command{Keyword: "don", IsAction: true, Brief: "x", Handler: run}); err != nil {
		t.Fatal(err)
	}

	tr := action.NewTracker()
	actor := newRoleActor("Bob", "p-2")
	env := command.Env{Actions: tr}
	ctx := context.Background()

	// Idle: the action command runs.
	if err := r.Dispatch(ctx, env, actor, "don"); err != nil {
		t.Fatal(err)
	}
	if !ran || actor.lastLine() != "ran" {
		t.Fatalf("idle action should run, ran=%v last=%q", ran, actor.lastLine())
	}

	// Busy: the same command is refused, naming the occupation.
	ran = false
	tr.Begin("p-2", action.Action{Label: "buckling on a breastplate"})
	if err := r.Dispatch(ctx, env, actor, "don"); err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Error("action command ran while actor was busy")
	}
	if want := "You are busy buckling on a breastplate."; actor.lastLine() != want {
		t.Errorf("busy refusal = %q, want %q", actor.lastLine(), want)
	}

	// Replay: the completion sweep bypasses the gate to perform the deferred act.
	ran = false
	env.ReplayAction = true
	if err := r.Dispatch(ctx, env, actor, "don"); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Error("ReplayAction dispatch should bypass the busy gate")
	}
}

// A non-IsAction command is never gated, even while the actor is busy — a busy
// player can still look, talk, and stop (action-economy.md §4).
func TestBusyGate_PassiveCommandUnaffected(t *testing.T) {
	r := command.New()
	run := func(ctx context.Context, c *command.Context) error { return c.Actor.Write(ctx, "ran") }
	if err := r.RegisterCommand(command.Command{Keyword: "look", Brief: "x", Handler: run}); err != nil {
		t.Fatal(err)
	}
	tr := action.NewTracker()
	tr.Begin("p-2", action.Action{Label: "busy"})
	actor := newRoleActor("Bob", "p-2")
	if err := r.Dispatch(context.Background(), command.Env{Actions: tr}, actor, "look"); err != nil {
		t.Fatal(err)
	}
	if actor.lastLine() != "ran" {
		t.Errorf("passive command should run while busy, got %q", actor.lastLine())
	}
}

// StopHandler reports idle, interrupts an interruptible action, and refuses to
// stop a non-interruptible one (action-economy.md §5).
func TestStopHandler(t *testing.T) {
	tr := action.NewTracker()
	actor := newRoleActor("Bob", "p-2")
	c := &command.Context{Actor: actor, Actions: tr}
	ctx := context.Background()

	// Idle.
	if err := command.StopHandler(ctx, c); err != nil {
		t.Fatal(err)
	}
	if actor.lastLine() != "You aren't doing anything." {
		t.Errorf("idle stop = %q", actor.lastLine())
	}

	// Interruptible: stopped and cleared.
	tr.Begin("p-2", action.Action{Interruptible: true, Label: "buckling on a breastplate"})
	if err := command.StopHandler(ctx, c); err != nil {
		t.Fatal(err)
	}
	if want := "You stop buckling on a breastplate."; actor.lastLine() != want {
		t.Errorf("stop = %q, want %q", actor.lastLine(), want)
	}
	if tr.IsBusy("p-2") {
		t.Error("interruptible action should be cleared by stop")
	}

	// Non-interruptible: refused and left in place.
	tr.Begin("p-2", action.Action{Interruptible: false, Label: "channeling"})
	if err := command.StopHandler(ctx, c); err != nil {
		t.Fatal(err)
	}
	if actor.lastLine() != "You can't stop what you're doing now." {
		t.Errorf("non-interruptible stop = %q", actor.lastLine())
	}
	if !tr.IsBusy("p-2") {
		t.Error("non-interruptible action should remain after a refused stop")
	}
}
