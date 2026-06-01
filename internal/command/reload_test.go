package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

func dispatchReload(t *testing.T, env command.Env, a *testActor, line string) {
	t.Helper()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), env, a, line); err != nil {
		t.Fatalf("dispatch %q: %v", line, err)
	}
}

func TestReload_DisabledWhenUnwired(t *testing.T) {
	a := newTestActor(&world.Room{ID: "x:1"})
	dispatchReload(t, command.Env{}, a, "reload") // no ReloadScripts
	if got := a.lastLine(); got != "Reloading is not enabled." {
		t.Errorf("got %q, want disabled message", got)
	}
}

func TestReload_ReportsCount(t *testing.T) {
	a := newTestActor(&world.Room{ID: "x:1"})
	called := false
	env := command.Env{ReloadScripts: func(context.Context) (int, error) {
		called = true
		return 3, nil
	}}
	dispatchReload(t, env, a, "reload")
	if !called {
		t.Error("ReloadScripts closure not invoked")
	}
	if got := a.lastLine(); got != "Reloaded 3 script(s)." {
		t.Errorf("got %q, want count message", got)
	}
}

func TestReload_SurfacesError(t *testing.T) {
	a := newTestActor(&world.Room{ID: "x:1"})
	env := command.Env{ReloadScripts: func(context.Context) (int, error) {
		return 0, errors.New("compile scripts/bad.lua: syntax error near 'end'")
	}}
	dispatchReload(t, env, a, "reload")
	if got := a.lastLine(); got != "Reload failed: compile scripts/bad.lua: syntax error near 'end'" {
		t.Errorf("got %q, want surfaced error", got)
	}
}
