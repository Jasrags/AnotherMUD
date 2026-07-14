package session

import (
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// TestGuideOverlay_SetHasDrain covers the connActor live-guide overlay
// (onboarding-guide.md): set records the guide, Has/LiveGuideID read it, and
// DrainLiveGuide reads-and-clears exactly once.
func TestGuideOverlay_SetHasDrain(t *testing.T) {
	a := &connActor{}

	if a.HasLiveGuide() {
		t.Fatal("fresh actor should have no live guide")
	}
	if _, ok := a.LiveGuideID(); ok {
		t.Fatal("LiveGuideID should report absent on a fresh actor")
	}
	if _, ok := a.DrainLiveGuide(); ok {
		t.Fatal("DrainLiveGuide should report absent on a fresh actor")
	}

	a.SetLiveGuide(entities.EntityID("entity-7"))
	if !a.HasLiveGuide() {
		t.Fatal("HasLiveGuide should be true after SetLiveGuide")
	}
	if id, ok := a.LiveGuideID(); !ok || id != "entity-7" {
		t.Fatalf("LiveGuideID = (%q,%v), want (entity-7,true)", id, ok)
	}

	// LiveGuideID does NOT clear; the guide is still live after a read.
	if !a.HasLiveGuide() {
		t.Fatal("LiveGuideID must not clear the guide")
	}

	id, ok := a.DrainLiveGuide()
	if !ok || id != "entity-7" {
		t.Fatalf("DrainLiveGuide = (%q,%v), want (entity-7,true)", id, ok)
	}
	if a.HasLiveGuide() {
		t.Fatal("DrainLiveGuide must clear the guide")
	}
}

// TestGuideOverlay_DrainOnce proves the atomic drain is race-safe: with N
// goroutines draining concurrently, exactly ONE observes the guide id (so the
// three teardown paths — shoo, graduation, logout — can never double-dematerialize).
func TestGuideOverlay_DrainOnce(t *testing.T) {
	a := &connActor{}
	a.SetLiveGuide(entities.EntityID("entity-42"))

	const workers = 32
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if _, ok := a.DrainLiveGuide(); ok {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if wins != 1 {
		t.Fatalf("concurrent DrainLiveGuide winners = %d, want exactly 1", wins)
	}
	if a.HasLiveGuide() {
		t.Fatal("guide should be cleared after the drain race")
	}
}
