package session

import (
	"context"
	"testing"
)

// TestConnActorTips exercises the session-side one-time-tip state machine:
// show-once, persistence into the save, opt-out, and reset (ui-rendering-help
// §12).
func TestConnActorTips(t *testing.T) {
	a, _ := newGmcpActor("p-1", 50, 100)
	ctx := context.Background()

	if !a.ShowTipOnce(ctx, "help", "hi") {
		t.Fatal("first ShowTipOnce should show")
	}
	if a.ShowTipOnce(ctx, "help", "hi") {
		t.Error("second ShowTipOnce of the same id should not re-show")
	}
	if len(a.save.TipsSeen) != 1 || a.save.TipsSeen[0] != "help" {
		t.Errorf("save.TipsSeen = %v, want [help]", a.save.TipsSeen)
	}

	a.SetTipsEnabled(false)
	if a.TipsEnabled() {
		t.Error("SetTipsEnabled(false) did not disable")
	}
	if !a.save.TipsDisabled {
		t.Error("save.TipsDisabled not set")
	}
	if a.ShowTipOnce(ctx, "other", "x") {
		t.Error("a disabled actor should not show a new tip")
	}

	a.ResetTips()
	if !a.TipsEnabled() {
		t.Error("ResetTips did not re-enable")
	}
	if len(a.save.TipsSeen) != 0 {
		t.Errorf("ResetTips did not clear save.TipsSeen: %v", a.save.TipsSeen)
	}
	// After reset, a previously-seen tip shows again.
	if !a.ShowTipOnce(ctx, "help", "hi") {
		t.Error("after reset, a re-armed tip should show again")
	}
}
