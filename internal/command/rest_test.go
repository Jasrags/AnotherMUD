package command_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

func restCtx(a *testActor, svc *economy.RestService) *command.Context {
	return &command.Context{Actor: a, Rest: svc}
}

func TestRestVerb_EntersResting(t *testing.T) {
	a := newTestActor(&world.Room{ID: "x:1"})
	svc := economy.NewRestService(economy.DefaultRestConfig(), nil, nil)

	if err := command.RestHandler(context.Background(), restCtx(a, svc)); err != nil {
		t.Fatalf("RestHandler: %v", err)
	}
	if a.restState != string(economy.StateResting) {
		t.Errorf("state = %q, want resting", a.restState)
	}
	if len(a.lines) != 1 || a.lines[0] != "You sit down and rest." {
		t.Errorf("output = %v, want sit-down message", a.lines)
	}
}

func TestSleepVerb_RecordsSleepAndStateOrder(t *testing.T) {
	a := newTestActor(&world.Room{ID: "x:1"})
	svc := economy.NewRestService(economy.DefaultRestConfig(), nil, func() uint64 { return 99 })

	if err := command.SleepHandler(context.Background(), restCtx(a, svc)); err != nil {
		t.Fatalf("SleepHandler: %v", err)
	}
	if a.restState != string(economy.StateSleeping) {
		t.Errorf("state = %q, want sleeping", a.restState)
	}
	if a.sleepStartTick != 99 {
		t.Errorf("sleepStart = %d, want 99", a.sleepStartTick)
	}
}

func TestWakeVerb_AlreadyAwake(t *testing.T) {
	a := newTestActor(&world.Room{ID: "x:1"}) // unset → awake
	svc := economy.NewRestService(economy.DefaultRestConfig(), nil, nil)

	if err := command.WakeHandler(context.Background(), restCtx(a, svc)); err != nil {
		t.Fatalf("WakeHandler: %v", err)
	}
	if len(a.lines) != 1 || a.lines[0] != "You are already awake." {
		t.Errorf("output = %v, want already-awake message", a.lines)
	}
}

func TestRestVerb_NilServiceGuards(t *testing.T) {
	a := newTestActor(&world.Room{ID: "x:1"})
	if err := command.RestHandler(context.Background(), restCtx(a, nil)); err != nil {
		t.Fatalf("RestHandler: %v", err)
	}
	if len(a.lines) != 1 || a.lines[0] != "You can't do that right now." {
		t.Errorf("output = %v, want nil-service guard message", a.lines)
	}
	if a.restState != "" {
		t.Errorf("state should be untouched, got %q", a.restState)
	}
}

// rest → wake round-trip clears state back to awake.
func TestRestThenWake(t *testing.T) {
	a := newTestActor(&world.Room{ID: "x:1"})
	svc := economy.NewRestService(economy.DefaultRestConfig(), nil, nil)

	_ = command.RestHandler(context.Background(), restCtx(a, svc))
	if a.restState != string(economy.StateResting) {
		t.Fatalf("not resting after rest verb: %q", a.restState)
	}
	_ = command.WakeHandler(context.Background(), restCtx(a, svc))
	if a.restState != string(economy.StateAwake) {
		t.Errorf("state after wake = %q, want awake", a.restState)
	}
}
