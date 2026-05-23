package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

func newLinkDeadActor(t *testing.T, mgr *Manager, room *world.Room, name, pid string) (*connActor, *fakeConn) {
	t.Helper()
	a, fc := newFakeActor("c1", pid, "acc1", name, room)
	mgr.Add(a)
	return a, fc
}

// TestManager_RemoveConnectionOnlyKeepsOtherIndices verifies that the
// link-dead transition drops the conn-id index but leaves byPlayerID /
// byName / byAccount / byRoom intact so a returning login can find the
// session.
func TestManager_RemoveConnectionOnlyKeepsOtherIndices(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:1", Name: "X"}
	a, _ := newLinkDeadActor(t, mgr, r, "Alice", "p1")

	mgr.RemoveConnectionOnly(a)

	// byConn must be empty for that conn id.
	mgr.mu.RLock()
	_, hasConn := mgr.byConn[a.id]
	mgr.mu.RUnlock()
	if hasConn {
		t.Fatalf("RemoveConnectionOnly left a byConn entry")
	}

	// Other indices intact.
	if got, ok := mgr.GetByPlayerID("p1"); !ok || got != a {
		t.Errorf("byPlayerID lost; want intact")
	}
	if got, ok := mgr.GetByName("Alice"); !ok || got != a {
		t.Errorf("byName lost; want intact")
	}
	if list := mgr.GetByAccountID("acc1"); len(list) != 1 {
		t.Errorf("byAccount = %d, want 1", len(list))
	}
	mgr.mu.RLock()
	roomOK := len(mgr.byRoom[r.ID]) == 1
	mgr.mu.RUnlock()
	if !roomOK {
		t.Errorf("byRoom lost the actor")
	}
}

// TestManager_ReRegisterConnectionForSessionSwapsConn verifies that
// reconnect's conn-swap path re-installs the byConn entry under the
// new id and updates the actor's id field atomically.
func TestManager_ReRegisterConnectionForSessionSwapsConn(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:1"}
	a, _ := newLinkDeadActor(t, mgr, r, "Alice", "p1")
	mgr.RemoveConnectionOnly(a)

	if err := mgr.ReRegisterConnectionForSession(a, "c-new"); err != nil {
		t.Fatalf("ReRegister: %v", err)
	}
	if a.id != "c-new" {
		t.Errorf("actor id = %q, want c-new", a.id)
	}
	mgr.mu.RLock()
	got, ok := mgr.byConn["c-new"]
	mgr.mu.RUnlock()
	if !ok || got != a {
		t.Errorf("byConn missing new id")
	}
}

// TestManager_ReRegisterReturnsErrSessionGoneWhenReaped covers the
// race where the cleanup sweep removed the actor before reconnect
// could re-register.
func TestManager_ReRegisterReturnsErrSessionGoneWhenReaped(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:1"}
	a, _ := newLinkDeadActor(t, mgr, r, "Alice", "p1")
	mgr.Remove(a) // simulate cleanup

	err := mgr.ReRegisterConnectionForSession(a, "c-new")
	if !errors.Is(err, ErrSessionGone) {
		t.Errorf("err = %v, want ErrSessionGone", err)
	}
}

// TestEnterLinkDeadIsIdempotent: a second enterLinkDead returns false
// instead of resetting linkDeadAt (which would push the cleanup window
// out indefinitely).
func TestEnterLinkDeadIsIdempotent(t *testing.T) {
	r := &world.Room{ID: "x:1"}
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", r)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if !a.enterLinkDead(t0) {
		t.Fatal("first enter returned false")
	}
	t1 := t0.Add(30 * time.Second)
	if a.enterLinkDead(t1) {
		t.Error("second enter returned true; should be no-op")
	}
	a.mu.Lock()
	got := a.linkDeadAt
	a.mu.Unlock()
	if !got.Equal(t0) {
		t.Errorf("linkDeadAt = %v, want %v", got, t0)
	}
}

// TestLinkDeadCleanup_ReapsExpired covers the sweep happy path: an
// actor whose link-dead window has elapsed gets persisted, departure-
// broadcast, and fully removed.
func TestLinkDeadCleanup_ReapsExpired(t *testing.T) {
	mc := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	st, err := player.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	mgr := NewManager()
	r := &world.Room{ID: "x:1"}
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", r)
	a.players = st
	a.dirty = true
	mgr.Add(a)

	// Peer in the same room to verify the departure broadcast.
	peer, peerConn := newFakeActor("c2", "p2", "acc1", "Bob", r)
	mgr.Add(peer)

	// Park Alice link-dead, then jump past the timeout.
	if !a.enterLinkDead(mc.Now()) {
		t.Fatal("enterLinkDead returned false")
	}
	mgr.RemoveConnectionOnly(a)

	cfg := LinkDeadConfig{Enabled: true, TimeoutSeconds: 30}
	mc.Advance(31 * time.Second)
	mgr.LinkDeadCleanup(context.Background(), cfg, mc)

	if _, ok := mgr.GetByPlayerID("p1"); ok {
		t.Error("Alice still indexed after cleanup")
	}
	saw := false
	for _, l := range peerConn.writes() {
		if l == "Alice has left.\r\n" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("peer did not see Alice depart; got %v", peerConn.writes())
	}
}

// TestLinkDeadCleanup_DoesNotReapBeforeTimeout — an actor within the
// grace window stays parked.
func TestLinkDeadCleanup_DoesNotReapBeforeTimeout(t *testing.T) {
	mc := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	mgr := NewManager()
	r := &world.Room{ID: "x:1"}
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", r)
	mgr.Add(a)
	a.enterLinkDead(mc.Now())
	mgr.RemoveConnectionOnly(a)

	cfg := LinkDeadConfig{Enabled: true, TimeoutSeconds: 120}
	mc.Advance(60 * time.Second)
	mgr.LinkDeadCleanup(context.Background(), cfg, mc)

	if _, ok := mgr.GetByPlayerID("p1"); !ok {
		t.Error("Alice prematurely reaped")
	}
	if !a.isLinkDead() {
		t.Error("phase changed before timeout")
	}
}

// TestLinkDeadCleanup_ZeroTimeoutDoesNotReap is the regression test
// for a footgun: TimeoutSeconds <= 0 must NOT mean "reap immediately".
// A misconfigured zero is treated as "never auto-reap" so an operator
// typo cannot destroy every parked session on the next sweep.
func TestLinkDeadCleanup_ZeroTimeoutDoesNotReap(t *testing.T) {
	mc := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	mgr := NewManager()
	r := &world.Room{ID: "x:1"}
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", r)
	mgr.Add(a)
	a.enterLinkDead(mc.Now())
	mgr.RemoveConnectionOnly(a)

	// One hour later, with a 0 timeout, nothing should be reaped.
	mc.Advance(time.Hour)
	mgr.LinkDeadCleanup(context.Background(), LinkDeadConfig{Enabled: true, TimeoutSeconds: 0}, mc)

	if _, ok := mgr.GetByPlayerID("p1"); !ok {
		t.Error("zero-timeout sweep reaped the actor; should be a no-op")
	}
}

// TestReconnect_IDOwnershipBelongsToManager guards HIGH-1: the actor's
// id must only be mutated by Manager.ReRegisterConnectionForSession,
// never by reattach. If reattach pre-emptively changed a.id, the
// defensive `delete(m.byConn, a.id)` in ReRegister would target the
// new key (a no-op against a phantom entry) and could leak the old
// byConn mapping under future refactors.
func TestReconnect_IDOwnershipBelongsToManager(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:1"}
	a, _ := newFakeActor("c-old", "p1", "acc1", "Alice", r)
	mgr.Add(a)
	a.enterLinkDead(time.Now())
	mgr.RemoveConnectionOnly(a)

	newConn := &fakeConn{id: "c-new"}
	if !a.reattach(newConn, time.Now()) {
		t.Fatal("reattach returned false")
	}
	// Crucial invariant: reattach must NOT have changed a.id.
	if a.id != "c-old" {
		t.Errorf("reattach mutated a.id to %q; ID ownership belongs to ReRegisterConnectionForSession", a.id)
	}

	if err := mgr.ReRegisterConnectionForSession(a, newConn.ID()); err != nil {
		t.Fatalf("ReRegister: %v", err)
	}
	if a.id != "c-new" {
		t.Errorf("after ReRegister, a.id = %q, want c-new", a.id)
	}
	mgr.mu.RLock()
	_, oldStuck := mgr.byConn["c-old"]
	_, newPresent := mgr.byConn["c-new"]
	mgr.mu.RUnlock()
	if oldStuck {
		t.Error("byConn still holds old id entry")
	}
	if !newPresent {
		t.Error("byConn missing new id entry")
	}
}

// TestLinkDeadCleanup_DisabledIsNoOp — when cfg.Enabled is false the
// sweep does nothing even if an actor is somehow link-dead.
func TestLinkDeadCleanup_DisabledIsNoOp(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:1"}
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", r)
	mgr.Add(a)
	a.enterLinkDead(time.Now().Add(-time.Hour))
	mgr.RemoveConnectionOnly(a)

	mgr.LinkDeadCleanup(context.Background(), LinkDeadConfig{Enabled: false, TimeoutSeconds: 1}, clock.RealClock{})
	if _, ok := mgr.GetByPlayerID("p1"); !ok {
		t.Error("disabled sweep reaped the actor anyway")
	}
}

// TestLinkDeadCleanup_RaceWithReattach: cleanup and reattach race for
// the same actor; only one wins. Repeats many trials under -race.
func TestLinkDeadCleanup_RaceWithReattach(t *testing.T) {
	const trials = 100
	for i := 0; i < trials; i++ {
		mc := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		mgr := NewManager()
		r := &world.Room{ID: "x:1"}
		a, _ := newFakeActor("c1", "p1", "acc1", "Alice", r)
		mgr.Add(a)
		a.enterLinkDead(mc.Now())
		mgr.RemoveConnectionOnly(a)
		mc.Advance(60 * time.Second)
		cfg := LinkDeadConfig{Enabled: true, TimeoutSeconds: 30}

		newConn := &fakeConn{id: "c-new"}
		var wg sync.WaitGroup
		wg.Add(2)
		var reattached bool
		go func() {
			defer wg.Done()
			mgr.LinkDeadCleanup(context.Background(), cfg, mc)
		}()
		go func() {
			defer wg.Done()
			reattached = a.reattach(newConn, mc.Now())
		}()
		wg.Wait()

		// One of two consistent end states:
		//   - cleanup won: actor is gone from indices, reattached=false.
		//   - reattach won: actor is back in Playing; cleanup left it.
		_, indexed := mgr.GetByPlayerID("p1")
		switch {
		case reattached && !indexed:
			t.Fatalf("trial %d: reattach won but actor was reaped from indices", i)
		case !reattached && indexed:
			t.Fatalf("trial %d: reattach lost but actor still indexed", i)
		}
	}
}
