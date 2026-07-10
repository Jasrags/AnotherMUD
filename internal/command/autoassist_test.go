package command_test

import (
	"strings"
	"testing"
)

// TestAutoAssist_Toggles exercises the `autoassist [on|off]` verb (grouping.md §9):
// default off, explicit on/off set it, and a NO-argument invocation flips the
// current state (the standard binary-toggle grammar).
func TestAutoAssist_Toggles(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newNamedTestActor("Alice", "p-1", f.room)

	dispatchActor(t, r, f.env(), a, "autoassist on")
	if !a.AutoAssistEnabled() {
		t.Error("autoassist on did not enable")
	}
	dispatchActor(t, r, f.env(), a, "autoassist off")
	if a.AutoAssistEnabled() {
		t.Error("autoassist off did not disable")
	}

	// No argument flips: off → on → off.
	dispatchActor(t, r, f.env(), a, "autoassist")
	if !a.AutoAssistEnabled() {
		t.Error("bare `autoassist` should flip off → on")
	}
	dispatchActor(t, r, f.env(), a, "autoassist")
	if a.AutoAssistEnabled() {
		t.Error("bare `autoassist` should flip on → off")
	}
}

// TestAutoAssist_RejectsJunk: a non on/off argument gets a usage line, state
// unchanged.
func TestAutoAssist_RejectsJunk(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newNamedTestActor("Alice", "p-1", f.room)

	dispatchActor(t, r, f.env(), a, "autoassist banana")
	if got := a.lastLine(); !strings.Contains(got, "Usage") {
		t.Errorf("junk arg message = %q, want usage", got)
	}
	if a.AutoAssistEnabled() {
		t.Error("junk arg should not have enabled auto-assist")
	}
}
