package session

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// fakeCommlink is a test double for CommlinkOnboarding: a fixed message and a
// toggleable "carries a commlink" answer.
type fakeCommlink struct {
	msg     string
	ok      bool
	carries bool
}

func (f *fakeCommlink) Welcome() (string, bool)                  { return f.msg, f.ok }
func (f *fakeCommlink) CarriesCommlink([]entities.EntityID) bool { return f.carries }

// The first-entry commlink call is device-gated (no commlink → no call, and the
// shown-once flag is not burned), fires exactly once, and — being a story beat,
// not a tip — fires even with contextual tips disabled.
func TestDeliverCommlinkCall_DeviceGatedOnceAndFiresWithTipsOff(t *testing.T) {
	mgr := NewManager()
	a, fc := newFakeActor("c1", "p1", "acc1", "Runner", &world.Room{ID: "r"})
	mgr.Add(a)

	svc := &fakeCommlink{msg: "TEST-WELCOME-MSG", ok: true, carries: false}
	mgr.SetCommlink(svc)
	a.SetTipsEnabled(false) // the call must fire regardless of the tips opt-out
	ctx := context.Background()

	// No commlink → no call, and the shown-once flag stays unset (so acquiring one
	// later still triggers it).
	mgr.DeliverCommlinkCallFor(ctx, "p1")
	if got := fc.writes(); len(got) != 0 {
		t.Fatalf("delivered with no commlink carried: %v", got)
	}

	// Acquire a commlink → the call fires, framed, even with tips off.
	svc.carries = true
	mgr.DeliverCommlinkCallFor(ctx, "p1")
	got := strings.Join(fc.writes(), "")
	if !strings.Contains(got, "TEST-WELCOME-MSG") || !strings.Contains(got, "commlink chimes") {
		t.Fatalf("first call did not deliver the framed message: %q", got)
	}

	// Second call: shown-once → no further delivery.
	before := len(fc.writes())
	mgr.DeliverCommlinkCallFor(ctx, "p1")
	if now := len(fc.writes()); now != before {
		t.Errorf("second call re-delivered (not shown-once): %d writes, want %d", now, before)
	}
}

// A nil / unconfigured service, an empty message, and an offline recipient are
// all silent no-ops.
func TestDeliverCommlinkCall_UnconfiguredAndOfflineNoops(t *testing.T) {
	mgr := NewManager()
	a, fc := newFakeActor("c1", "p1", "acc1", "Runner", &world.Room{ID: "r"})
	mgr.Add(a)
	ctx := context.Background()

	// No service set → no-op.
	mgr.DeliverCommlinkCallFor(ctx, "p1")
	// Service set but message empty (Welcome reports not-configured) → no-op.
	mgr.SetCommlink(&fakeCommlink{msg: "", ok: false, carries: true})
	mgr.DeliverCommlinkCallFor(ctx, "p1")
	// Offline recipient → no-op, no panic.
	mgr.SetCommlink(&fakeCommlink{msg: "x", ok: true, carries: true})
	mgr.DeliverCommlinkCallFor(ctx, "ghost")

	if got := fc.writes(); len(got) != 0 {
		t.Errorf("unconfigured/offline paths wrote output: %v", got)
	}
}
