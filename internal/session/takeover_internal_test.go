package session

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TestMarkTakenOver_IsOneShotLatch is the H2 invariant: only the first
// caller wins the claim. Two concurrent goroutines must observe exactly
// one true and one false, so performTakeover's side-effects (Notify,
// Persist, Manager.Remove, Close) run at most once per displaced actor.
func TestMarkTakenOver_IsOneShotLatch(t *testing.T) {
	r := &world.Room{ID: "x:1"}
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", r)

	const N = 32
	var wins int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range N {
		wg.Go(func() {
			<-start
			if a.markTakenOver() {
				atomic.AddInt64(&wins, 1)
			}
		})
	}
	close(start)
	wg.Wait()

	if wins != 1 {
		t.Errorf("markTakenOver wins = %d, want 1 (exactly one claimant)", wins)
	}
	a.mu.Lock()
	taken := a.takenOver
	disc := a.disconnecting
	a.mu.Unlock()
	if !taken {
		t.Error("takenOver latch never set")
	}
	if !disc {
		t.Error("disconnecting latch not set alongside takenOver")
	}
}

// TestPerformTakeover_ReturnsLiveSaveAndRoom is the H1 invariant: the
// new session must receive the existing actor's in-memory save + room,
// not a fresh disk load. Verifies the (save, room, ok=true) contract.
func TestPerformTakeover_ReturnsLiveSaveAndRoom(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:beta"}
	a, fc := newFakeActor("c1", "p1", "acc1", "Alice", r)
	mgr.Add(a)
	// Simulate state that hasn't been autosaved: Location updated to a
	// post-walk room.
	a.save.Location = "x:beta"

	cfg := Config{Manager: mgr}
	save, room, ok := performTakeover(context.Background(), cfg, a)

	if !ok {
		t.Fatal("performTakeover returned ok=false on fresh actor")
	}
	if save != a.save {
		t.Error("returned save pointer is not the live actor save")
	}
	if save.Location != "x:beta" {
		t.Errorf("save.Location = %q, want x:beta", save.Location)
	}
	if room != r {
		t.Error("returned room is not the live actor room")
	}
	if !fc.closed() {
		t.Error("old conn was not closed by takeover")
	}
	// Manager indices scrubbed.
	if _, present := mgr.GetByPlayerID("p1"); present {
		t.Error("byPlayerID still has the displaced actor")
	}
}

// TestPerformTakeover_LosesRaceReturnsNotOk: a second performTakeover on
// an already-claimed actor must return ok=false and touch nothing.
func TestPerformTakeover_LosesRaceReturnsNotOk(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:1"}
	a, fc := newFakeActor("c1", "p1", "acc1", "Alice", r)
	mgr.Add(a)

	// Pre-claim.
	if !a.markTakenOver() {
		t.Fatal("setup: first claim should have succeeded")
	}

	_, _, ok := performTakeover(context.Background(), Config{Manager: mgr}, a)
	if ok {
		t.Fatal("loser returned ok=true; expected false")
	}
	if fc.closed() {
		t.Error("loser closed the conn; should be a no-op")
	}
	// Manager indices untouched by the loser.
	if _, present := mgr.GetByPlayerID("p1"); !present {
		t.Error("loser removed byPlayerID; should be a no-op")
	}
}
