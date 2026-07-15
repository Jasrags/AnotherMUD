package command_test

import (
	"context"
	"strings"
	"testing"
)

// `firemode <mode>` sets a supported mode; an unsupported mode is refused and the
// selection is unchanged (ranged-combat §5.5).
func TestFireMode_SetSupportedAndRejectUnsupported(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Runner", "p-run", f.room)
	a.wieldedModes = []string{"single", "burst", "auto"}
	r := newRegistry(t)

	if err := r.Dispatch(context.Background(), f.env(), a, "firemode burst"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if a.FireMode() != "burst" {
		t.Fatalf("fire mode = %q, want burst", a.FireMode())
	}
	if got := a.lastLine(); !strings.Contains(strings.ToLower(got), "burst") {
		t.Errorf("set confirmation = %q, want it to mention burst", got)
	}
}

// A weapon that does not support a mode refuses it and keeps the current mode.
func TestFireMode_UnsupportedRejected(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Runner", "p-run", f.room)
	a.wieldedModes = nil // single-only (e.g. a pistol/bow)
	r := newRegistry(t)

	if err := r.Dispatch(context.Background(), f.env(), a, "firemode auto"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if a.FireMode() != "single" {
		t.Errorf("an unsupported mode must not be set; fire mode = %q, want single", a.FireMode())
	}
	if got := a.lastLine(); !strings.Contains(strings.ToLower(got), "can't fire") {
		t.Errorf("refusal = %q, want a can't-fire message", got)
	}
}

// Single is ALWAYS available, even on a weapon that declares no modes.
func TestFireMode_SingleAlwaysAvailable(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Runner", "p-run", f.room)
	a.wieldedModes = nil
	a.fireMode = "burst" // pretend a prior weapon set it
	r := newRegistry(t)

	if err := r.Dispatch(context.Background(), f.env(), a, "firemode single"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if a.FireMode() != "single" {
		t.Errorf("single must always be settable; got %q", a.FireMode())
	}
}
