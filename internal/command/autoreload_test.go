package command_test

import "testing"

// The autoreload verb sets the preference explicitly with on/off and FLIPS it on
// a no-argument invocation (the standard binary-toggle grammar; autoreload.md §2).
func TestAutoreload_Toggles(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)

	dispatchLoot(t, f, a, "autoreload on")
	if !a.Autoreload() {
		t.Error("autoreload on did not enable")
	}
	dispatchLoot(t, f, a, "autoreload off")
	if a.Autoreload() {
		t.Error("autoreload off did not disable")
	}

	// No argument flips: off → on → off.
	dispatchLoot(t, f, a, "autoreload")
	if !a.Autoreload() {
		t.Error("bare `autoreload` should flip off → on")
	}
	dispatchLoot(t, f, a, "autoreload")
	if a.Autoreload() {
		t.Error("bare `autoreload` should flip on → off")
	}
}

// A non-on/off argument is a usage error and leaves the preference unchanged.
func TestAutoreload_RejectsJunk(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)

	dispatchLoot(t, f, a, "autoreload banana")
	if got := a.lastLine(); !contains(got, "Usage") {
		t.Errorf("junk arg message = %q, want usage", got)
	}
	if a.Autoreload() {
		t.Error("junk arg should not have enabled autoreload")
	}
}
