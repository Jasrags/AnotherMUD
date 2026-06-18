package session

import (
	"strconv"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// drainLiveMounts must be safe to race against the stable-path mutators
// (TrackLiveMount / UntrackLiveMount) on the same actor — the snapshot-and-clear
// is the guard that stops a logout from double-removing a mount a concurrent
// `stable` verb is also removing (mounts.md §9). Run under -race; the contract
// is "no race, no panic, and every id is drained or untracked exactly once".
func TestConnActor_DrainLiveMountsConcurrent(t *testing.T) {
	a := &connActor{}
	const n = 200
	ids := make([]entities.EntityID, n)
	for i := range ids {
		ids[i] = entities.EntityID("m-" + strconv.Itoa(i))
		a.TrackLiveMount(ids[i], "test:steed")
	}

	var wg sync.WaitGroup
	// One goroutine drains (the logout path); others untrack individual ids
	// (the stable path) concurrently. Whichever wins a given id, it is removed
	// once — the maps never see a double-delete panic.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = a.drainLiveMounts()
	}()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id entities.EntityID) {
			defer wg.Done()
			a.UntrackLiveMount(id)
		}(ids[i])
	}
	wg.Wait()

	// After both paths run, the live set is empty either way.
	if got := a.LiveMountTemplates(); len(got) != 0 {
		t.Errorf("live mounts after concurrent drain+untrack = %d, want 0", len(got))
	}
}
