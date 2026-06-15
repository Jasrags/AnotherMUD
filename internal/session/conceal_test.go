package session

import "testing"

// The hide concealment state machine on connActor (visibility §3.1, §4.1).
// A zero-value connActor suffices — these methods touch only the mutex and
// the concealment fields, not the stat block or save.
func TestConnActor_HideRevealStateMachine(t *testing.T) {
	a := &connActor{}

	// Fresh actor is unhidden.
	if a.IsHidden() || a.ConcealmentScore() != 0 || a.HiddenInstance() != 0 {
		t.Fatalf("zero-value actor should be unhidden: hidden=%v score=%d inst=%d",
			a.IsHidden(), a.ConcealmentScore(), a.HiddenInstance())
	}

	// Hide sets the state and returns a non-zero instance.
	inst1 := a.Hide(15)
	if !a.IsHidden() || a.ConcealmentScore() != 15 {
		t.Errorf("after Hide(15): hidden=%v score=%d, want true/15", a.IsHidden(), a.ConcealmentScore())
	}
	if inst1 == 0 || a.HiddenInstance() != inst1 {
		t.Errorf("HiddenInstance = %d, want returned %d (non-zero)", a.HiddenInstance(), inst1)
	}

	// Re-hiding bumps the instance (invalidates observers' prior pierces) and
	// updates the score.
	inst2 := a.Hide(20)
	if inst2 == inst1 {
		t.Errorf("re-hide must allocate a new instance (%d == %d)", inst2, inst1)
	}
	if a.ConcealmentScore() != 20 || a.HiddenInstance() != inst2 {
		t.Errorf("after re-hide: score=%d inst=%d, want 20/%d", a.ConcealmentScore(), a.HiddenInstance(), inst2)
	}

	// Reveal reports it was hidden and clears the state.
	if !a.Reveal() {
		t.Error("Reveal should report the actor was hidden")
	}
	if a.IsHidden() || a.ConcealmentScore() != 0 || a.HiddenInstance() != 0 {
		t.Errorf("after Reveal: hidden=%v score=%d inst=%d, want all cleared",
			a.IsHidden(), a.ConcealmentScore(), a.HiddenInstance())
	}

	// Reveal again is a no-op reporting false.
	if a.Reveal() {
		t.Error("Reveal on an unhidden actor should report false")
	}
}
