package entities

import (
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// ClaimLooted is the single-claim gate for the rob verb: under a stampede of
// concurrent robbers, exactly one wins (the coin-dupe guard). Runs under -race
// to prove the propsMu-guarded claim is data-race-free too.
func TestClaimLooted_AtomicSingleWinner(t *testing.T) {
	store := NewStore()
	m, err := store.SpawnMob(&mob.Template{ID: "core:goblin", Name: "a goblin", Type: "npc"})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if m.IsLooted() {
		t.Fatal("fresh mob should not be looted")
	}

	const robbers = 64
	var wins int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range robbers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if m.ClaimLooted() {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if wins != 1 {
		t.Errorf("ClaimLooted winners = %d, want exactly 1", wins)
	}
	if !m.IsLooted() {
		t.Error("mob should be looted after a successful claim")
	}
}
