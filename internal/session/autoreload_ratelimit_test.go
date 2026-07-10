package session

import "testing"

// autoReloadNoticeDue gates the "nothing to reload with" notice by a tick window
// and stamps the last-notice time (autoreload.md §5/§6). It operates only on the
// atomic, so a zero-value connActor exercises it directly.
func TestAutoReloadNoticeDue_RateLimits(t *testing.T) {
	var a connActor

	// First call (never shown) is always due, whatever the window.
	if !a.autoReloadNoticeDue(100, 50) {
		t.Fatal("first notice should be due")
	}
	// Within the window after the stamp: suppressed.
	if a.autoReloadNoticeDue(120, 50) {
		t.Error("notice at now=120 (last=100, window=50) should be suppressed")
	}
	// At the window boundary: due again, and re-stamps.
	if !a.autoReloadNoticeDue(150, 50) {
		t.Error("notice at now=150 (last=100, window=50) should be due")
	}
	if a.autoReloadNoticeDue(199, 50) {
		t.Error("notice at now=199 (last=150, window=50) should be suppressed")
	}
}

// A zero window disables suppression — every dry attempt notifies.
func TestAutoReloadNoticeDue_ZeroWindowAlwaysDue(t *testing.T) {
	var a connActor
	for _, now := range []uint64{10, 11, 12} {
		if !a.autoReloadNoticeDue(now, 0) {
			t.Errorf("window 0 should always be due (now=%d)", now)
		}
	}
}

// resetAutoReloadNotice clears the window so the next dry-out reports at once
// even inside what would have been the suppression interval.
func TestResetAutoReloadNotice_ClearsWindow(t *testing.T) {
	var a connActor
	a.autoReloadNoticeDue(100, 50) // stamp last=100
	a.resetAutoReloadNotice()
	if !a.autoReloadNoticeDue(110, 50) {
		t.Error("after reset, a notice at now=110 should be due despite the window")
	}
}
