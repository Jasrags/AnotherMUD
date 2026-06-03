package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
)

// charModeActor is a testActor that implements command.CharModeController.
type charModeActor struct {
	*testActor
	on        bool
	supported bool
}

func (a *charModeActor) SetCharMode(_ context.Context, on bool) bool {
	if !a.supported {
		return false
	}
	a.on = on
	return true
}
func (a *charModeActor) CharModeActive() bool { return a.on }

func TestTabComplete_Toggle(t *testing.T) {
	r := newRegistry(t)
	a := &charModeActor{testActor: newTestActor(nil), supported: true}

	dispatchActor(t, r, command.Env{}, a, "tabcomplete on")
	if !a.on || !strings.Contains(a.lastLine(), "ON") {
		t.Errorf("on: active=%v line=%q", a.on, a.lastLine())
	}
	dispatchActor(t, r, command.Env{}, a, "tabcomplete off")
	if a.on || !strings.Contains(a.lastLine(), "OFF") {
		t.Errorf("off: active=%v line=%q", a.on, a.lastLine())
	}
	dispatchActor(t, r, command.Env{}, a, "tabcomplete") // status
	if !strings.Contains(a.lastLine(), "OFF") {
		t.Errorf("status: %q", a.lastLine())
	}
}

func TestTabComplete_UnsupportedTransport(t *testing.T) {
	r := newRegistry(t)
	a := &charModeActor{testActor: newTestActor(nil), supported: false} // SetCharMode returns false
	dispatchActor(t, r, command.Env{}, a, "tabcomplete on")
	if !strings.Contains(a.lastLine(), "isn't available") {
		t.Errorf("unsupported on: %q", a.lastLine())
	}
}

func TestTabComplete_NotAController(t *testing.T) {
	r := newRegistry(t)
	a := newTestActor(nil) // plain testActor: not a CharModeController
	dispatchActor(t, r, command.Env{}, a, "tabcomplete on")
	if !strings.Contains(a.lastLine(), "isn't available") {
		t.Errorf("non-controller: %q", a.lastLine())
	}
}
