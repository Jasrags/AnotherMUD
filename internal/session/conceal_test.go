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

// The sneak (moving) concealment state machine on connActor (visibility §3.2).
// Mirrors the hide machine but the state SURVIVES a room change — that
// survive-the-move behavior is enforced at the move-drops-hide call site, not
// here, so this just exercises the toggle.
func TestConnActor_SneakStateMachine(t *testing.T) {
	a := &connActor{}

	if a.IsSneaking() || a.SneakConcealmentScore() != 0 {
		t.Fatalf("zero-value actor should not be sneaking: sneaking=%v score=%d",
			a.IsSneaking(), a.SneakConcealmentScore())
	}

	inst1 := a.Sneak(12)
	if !a.IsSneaking() || a.SneakConcealmentScore() != 12 {
		t.Errorf("after Sneak(12): sneaking=%v score=%d, want true/12", a.IsSneaking(), a.SneakConcealmentScore())
	}
	if inst1 == 0 {
		t.Error("Sneak should return a non-zero instance")
	}

	// Re-sneak bumps the instance and updates the score.
	inst2 := a.Sneak(18)
	if inst2 == inst1 {
		t.Errorf("re-sneak must allocate a new instance (%d == %d)", inst2, inst1)
	}
	if a.SneakConcealmentScore() != 18 {
		t.Errorf("after re-sneak: score=%d, want 18", a.SneakConcealmentScore())
	}

	if !a.Unsneak() {
		t.Error("Unsneak should report the actor was sneaking")
	}
	if a.IsSneaking() || a.SneakConcealmentScore() != 0 {
		t.Errorf("after Unsneak: sneaking=%v score=%d, want cleared", a.IsSneaking(), a.SneakConcealmentScore())
	}
	if a.Unsneak() {
		t.Error("Unsneak on a non-sneaking actor should report false")
	}
}

// Hide and sneak are independent layers (visibility §3.2: "Sneak and hide may
// both be active"). Toggling one must not disturb the other, and their
// instance ids never collide (shared monotonic counter).
func TestConnActor_HideAndSneakIndependent(t *testing.T) {
	a := &connActor{}

	hi := a.Hide(15)
	si := a.Sneak(12)
	if hi == si {
		t.Errorf("hide and sneak must not share an instance id (%d == %d)", hi, si)
	}
	if !a.IsHidden() || !a.IsSneaking() {
		t.Fatalf("both layers should be active: hidden=%v sneaking=%v", a.IsHidden(), a.IsSneaking())
	}

	// Revealing hide leaves sneak intact (and vice versa).
	a.Reveal()
	if a.IsHidden() || !a.IsSneaking() {
		t.Errorf("after Reveal: hidden=%v sneaking=%v, want false/true", a.IsHidden(), a.IsSneaking())
	}
	if a.SneakConcealmentScore() != 12 {
		t.Errorf("sneak score should be untouched by Reveal: got %d", a.SneakConcealmentScore())
	}
	a.Unsneak()
	if a.IsSneaking() {
		t.Error("Unsneak should clear sneak")
	}
}
