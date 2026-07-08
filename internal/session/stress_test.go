package session

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TestManager_StressMixedWorkloadIsRaceClean is the M4 exit-criterion
// stress test: hammer every Manager surface in parallel (Add, Remove,
// SetRoom, SendToRoom, SendToAll, GetByName, GetByPlayerID, Count) for
// a bounded duration. The point is to give -race enough surface area
// to catch any silent data race introduced by the M4.x concurrency
// work. After all goroutines exit, indices must be consistent with the
// set of actors still present.
//
// Skipped under -short so the default `go test` stays fast; `make test`
// (no -short) exercises it. Budget ~250ms wall time.
func TestManager_StressMixedWorkloadIsRaceClean(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in -short")
	}

	const (
		nRooms          = 8
		nResidentActors = 32 // long-lived, never removed mid-run
		nChurnSlots     = 8  // slots that repeatedly Add/Remove the same actor
		duration        = 250 * time.Millisecond
	)

	mgr := NewManager()
	rooms := make([]*world.Room, nRooms)
	for i := range rooms {
		rooms[i] = &world.Room{ID: world.RoomID(fmt.Sprintf("x:%d", i)), Name: fmt.Sprintf("Room %d", i)}
	}

	// Long-lived residents — placed once, then moved around for the run.
	residents := make([]*connActor, nResidentActors)
	for i := range residents {
		a, _ := newFakeActor(
			fmt.Sprintf("res-c-%d", i),
			fmt.Sprintf("res-p-%d", i),
			"acc-res",
			fmt.Sprintf("Resident%d", i),
			rooms[i%nRooms],
		)
		residents[i] = a
		mgr.Add(a)
	}

	// Churn pool — one actor per slot, repeatedly Added and Removed by
	// the churn worker. Pre-allocated so we never construct actors
	// inside the hot loop (constructing under -race produces noise that
	// isn't representative).
	churn := make([]*connActor, nChurnSlots)
	for i := range churn {
		a, _ := newFakeActor(
			fmt.Sprintf("churn-c-%d", i),
			fmt.Sprintf("churn-p-%d", i),
			"acc-churn",
			fmt.Sprintf("Churn%d", i),
			rooms[i%nRooms],
		)
		churn[i] = a
	}

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var ops int64
	var wg sync.WaitGroup

	// Movers: each goroutine cycles a slice of residents through rooms.
	nMovers := max(runtime.GOMAXPROCS(0), 2)
	for m := 0; m < nMovers; m++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			i := seed
			for ctx.Err() == nil {
				a := residents[i%len(residents)]
				a.SetRoom(rooms[(i+seed)%nRooms])
				i++
				atomic.AddInt64(&ops, 1)
			}
		}(m)
	}

	// Broadcasters: SendToRoom + SendToAll. Writes are captured by
	// fakeConn so this also exercises the actor lock under the manager
	// snapshot path.
	for b := range 2 {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			i := seed
			for ctx.Err() == nil {
				mgr.SendToRoom(ctx, rooms[i%nRooms].ID, "ping")
				if i%4 == 0 {
					mgr.SendToAll(ctx, "ping-all")
				}
				i++
				atomic.AddInt64(&ops, 1)
			}
		}(b)
	}

	// Readers: lookups + Count. These take only the RLock so they
	// should not block writers; we still want them in the mix to catch
	// a read racing a moveRoom mutation.
	for r := range 2 {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			i := seed
			for ctx.Err() == nil {
				_, _ = mgr.GetByName(fmt.Sprintf("Resident%d", i%nResidentActors))
				_, _ = mgr.GetByPlayerID(fmt.Sprintf("res-p-%d", i%nResidentActors))
				_ = mgr.Count()
				i++
				atomic.AddInt64(&ops, 1)
			}
		}(r)
	}

	// Churn worker: repeatedly Add and Remove each churn-pool actor.
	// Mixed in alongside SetRoom on the same actor to exercise the
	// SetRoom-vs-Remove race guard in moveRoom.
	wg.Go(func() {
		i := 0
		for ctx.Err() == nil {
			a := churn[i%len(churn)]
			mgr.Add(a)
			a.SetRoom(rooms[i%nRooms])
			mgr.Remove(a)
			i++
			atomic.AddInt64(&ops, 1)
		}
	})

	wg.Wait()

	// Consistency: every resident still has byPlayerID, byName,
	// byAccount entries, and is present in exactly one byRoom bucket
	// that matches roomByPID.
	if got := mgr.Count(); got != nResidentActors {
		t.Errorf("post-stress Count=%d, want %d", got, nResidentActors)
	}
	for _, a := range residents {
		if got, ok := mgr.GetByPlayerID(a.playerID); !ok || got != a {
			t.Errorf("resident %s lost byPlayerID", a.playerID)
			continue
		}
		if _, ok := mgr.GetByName(a.PlayerName()); !ok {
			t.Errorf("resident %s lost byName", a.PlayerName())
		}
		mgr.mu.RLock()
		pidRoom, hasPidRoom := mgr.roomByPID[a.playerID]
		var inBucket bool
		if hasPidRoom {
			occ := mgr.byRoom[pidRoom]
			_, inBucket = occ[a.playerID]
		}
		mgr.mu.RUnlock()
		if !hasPidRoom {
			t.Errorf("resident %s missing roomByPID", a.playerID)
			continue
		}
		if !inBucket {
			t.Errorf("resident %s in roomByPID=%s but not in byRoom bucket", a.playerID, pidRoom)
		}
	}

	// Churn-pool actors must NOT be indexed (last op on each is Remove).
	for _, a := range churn {
		if _, ok := mgr.GetByPlayerID(a.playerID); ok {
			t.Errorf("churn actor %s still indexed after run", a.playerID)
		}
	}

	// byAccount for residents must hold exactly nResidentActors entries
	// (acc-res); acc-churn must be absent.
	if list := mgr.GetByAccountID("acc-res"); len(list) != nResidentActors {
		t.Errorf("byAccount[acc-res] = %d, want %d", len(list), nResidentActors)
	}
	if list := mgr.GetByAccountID("acc-churn"); len(list) != 0 {
		t.Errorf("byAccount[acc-churn] = %d, want 0", len(list))
	}

	t.Logf("stress: %d ops in %v", atomic.LoadInt64(&ops), duration)
}

// TestSession_StressConcurrentTakeoverClaims hammers performTakeover
// from many goroutines against the same actor. Even with N callers,
// exactly one must observe ok=true; the rest must see ok=false and
// touch nothing. Race detector + invariant assertions together cover
// the H2 contract under load.
func TestSession_StressConcurrentTakeoverClaims(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in -short")
	}

	const trials = 200
	for trial := range trials {
		mgr := NewManager()
		r := &world.Room{ID: "x:1"}
		a, fc := newFakeActor("c1", "p1", "acc1", "Alice", r)
		mgr.Add(a)
		cfg := Config{Manager: mgr}

		const N = 8
		var winners int64
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range N {
			wg.Go(func() {
				<-start
				if _, _, ok := performTakeover(context.Background(), cfg, a); ok {
					atomic.AddInt64(&winners, 1)
				}
			})
		}
		close(start)
		wg.Wait()

		if winners != 1 {
			t.Fatalf("trial %d: winners=%d, want 1", trial, winners)
		}
		if !fc.closed() {
			t.Fatalf("trial %d: winner did not close old conn", trial)
		}
		if _, ok := mgr.GetByPlayerID("p1"); ok {
			t.Fatalf("trial %d: byPlayerID still present after takeover", trial)
		}
	}
}
