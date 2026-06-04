package session

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

func TestManagerRoster_SnapshotFieldsAndMarkers(t *testing.T) {
	mgr := NewManager()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewManual(t0)

	// Active admin — acted "now".
	admin := &connActor{
		id: "c1", playerID: "p1",
		save:        &player.Save{Name: "Alice"},
		lastInputAt: t0,
		roles:       map[string]struct{}{"admin": {}},
	}
	// Idle ordinary player — last acted 5 minutes ago (> 60s threshold).
	bob := &connActor{
		id: "c2", playerID: "p2",
		save:        &player.Save{Name: "Bob"},
		lastInputAt: t0.Add(-5 * time.Minute),
	}
	mgr.Add(admin)
	mgr.Add(bob)

	entries := NewWhoRoster(mgr, clk, DefaultWhoConfig()).OnlineRoster()
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (both playing sessions)", len(entries))
	}
	byName := map[string]command.WhoEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if byName["Alice"].RoleMarker != "Admin" {
		t.Errorf("Alice marker = %q, want Admin", byName["Alice"].RoleMarker)
	}
	if byName["Alice"].Idle {
		t.Error("Alice should not be idle (acted now)")
	}
	if byName["Bob"].RoleMarker != "" {
		t.Errorf("Bob marker = %q, want none", byName["Bob"].RoleMarker)
	}
	if !byName["Bob"].Idle {
		t.Error("Bob should be idle (5m since input)")
	}
}

// Proves the snapshot lock discipline: OnlineRoster (m.mu + per-actor a.mu)
// runs concurrently with a writer mutating an actor field under a.mu. Must
// be clean under -race.
func TestManagerRoster_ConcurrentSnapshot(t *testing.T) {
	mgr := NewManager()
	clk := clock.NewManual(time.Unix(0, 0))
	a := &connActor{id: "c1", playerID: "p1", save: &player.Save{Name: "Alice"}}
	mgr.Add(a)
	roster := NewWhoRoster(mgr, clk, DefaultWhoConfig())

	done := make(chan struct{})
	go func() { // writer: touch lastInputAt under the actor lock
		for {
			select {
			case <-done:
				return
			default:
				a.mu.Lock()
				a.lastInputAt = a.lastInputAt.Add(time.Second)
				a.mu.Unlock()
				runtime.Gosched() // yield so readers aren't starved on GOMAXPROCS=1
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = roster.OnlineRoster()
			}
		}()
	}
	wg.Wait()
	close(done)
}
