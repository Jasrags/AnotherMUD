package command_test

import (
	"strings"
	"testing"
)

// TestAutoAssist_ReportsAndToggles exercises the `autoassist [on|off]` verb's
// read + write paths (grouping.md §9): default off, on enables, off disables,
// and the no-arg report mirrors the current state.
func TestAutoAssist_ReportsAndToggles(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newNamedTestActor("Alice", "p-1", f.room)

	dispatchActor(t, r, f.env(), a, "autoassist")
	if got := a.lastLine(); !strings.Contains(got, "off") {
		t.Errorf("default report = %q, want 'off'", got)
	}

	dispatchActor(t, r, f.env(), a, "autoassist on")
	if !a.AutoAssistEnabled() {
		t.Error("autoassist on did not enable")
	}
	dispatchActor(t, r, f.env(), a, "autoassist")
	if got := a.lastLine(); !strings.Contains(got, "on") {
		t.Errorf("report after enable = %q, want 'on'", got)
	}

	dispatchActor(t, r, f.env(), a, "autoassist off")
	if a.AutoAssistEnabled() {
		t.Error("autoassist off did not disable")
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
