package eventbus

import (
	"context"
	"testing"
)

// TestAlignmentShiftCheck_CancelFlagPropagates exercises the
// cancellable + rewritable surface that the production
// alignmentSink relies on. Two subscribers run in order: the
// first rewrites the delta; the second observes the rewrite and
// cancels. The publisher should see cancelled=true and the
// final-rewritten delta.
func TestAlignmentShiftCheck_CancelFlagPropagates(t *testing.T) {
	bus := New()

	var observedByLast int
	bus.Subscribe(EventAlignmentShiftCheck, func(_ context.Context, ev Event) {
		e := ev.(*AlignmentShiftCheck)
		// Halve negative shifts (an item-style modifier).
		if e.SuggestedDelta() < 0 {
			e.RewriteDelta(e.SuggestedDelta() / 2)
		}
	})
	bus.Subscribe(EventAlignmentShiftCheck, func(_ context.Context, ev Event) {
		e := ev.(*AlignmentShiftCheck)
		observedByLast = e.SuggestedDelta()
		// Second listener vetos: a quest reward locking alignment.
		if e.SuggestedDelta() < -3 {
			e.Cancel()
		}
	})

	ev := NewAlignmentShiftCheck("p:alice", "evil-deed", -10)
	cancelled := bus.PublishCancellable(context.Background(), ev)
	if !cancelled {
		t.Error("PublishCancellable returned false; expected true (cancelled)")
	}
	// First listener halved -10 → -5; second listener observed -5
	// and cancelled (because -5 < -3).
	if observedByLast != -5 {
		t.Errorf("second listener observed %d, want -5 (rewrite must propagate)", observedByLast)
	}
	if ev.SuggestedDelta() != -5 {
		t.Errorf("final SuggestedDelta = %d, want -5", ev.SuggestedDelta())
	}
}

func TestAlignmentShiftCheck_NoCancelKeepsRewrite(t *testing.T) {
	bus := New()
	bus.Subscribe(EventAlignmentShiftCheck, func(_ context.Context, ev Event) {
		ev.(*AlignmentShiftCheck).RewriteDelta(42)
	})
	ev := NewAlignmentShiftCheck("e", "r", 1)
	cancelled := bus.PublishCancellable(context.Background(), ev)
	if cancelled {
		t.Error("PublishCancellable returned true without cancel")
	}
	if ev.SuggestedDelta() != 42 {
		t.Errorf("SuggestedDelta = %d, want 42", ev.SuggestedDelta())
	}
}
