package command_test

import "testing"

// The autoreload toggle verb reports its state and flips the preference,
// mirroring the autoloot toggle (autoreload.md §2).
func TestAutoreload_ReportsAndToggles(t *testing.T) {
	f := newLootFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)

	// Default: off, and a bare `autoreload` reports the state without changing it.
	dispatchLoot(t, f, a, "autoreload")
	if got := a.lastLine(); got == "" || !contains(got, "off") {
		t.Errorf("default report = %q, want it to say off", got)
	}
	if a.Autoreload() {
		t.Error("bare `autoreload` should not enable the preference")
	}

	dispatchLoot(t, f, a, "autoreload on")
	if !a.Autoreload() {
		t.Error("autoreload on did not enable")
	}
	dispatchLoot(t, f, a, "autoreload")
	if got := a.lastLine(); !contains(got, "on") {
		t.Errorf("report after enable = %q, want on", got)
	}

	dispatchLoot(t, f, a, "autoreload off")
	if a.Autoreload() {
		t.Error("autoreload off did not disable")
	}
}
