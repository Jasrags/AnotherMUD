package entities

import (
	"errors"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// fakeEntity is a minimal Entity for store-only tests that don't need
// the full ItemInstance lifecycle.
type fakeEntity struct {
	id   EntityID
	typ  string
	tags []string
}

func (f *fakeEntity) ID() EntityID  { return f.id }
func (f *fakeEntity) Type() string  { return f.typ }
func (f *fakeEntity) Tags() []string { return f.tags }

func TestStoreTrackAndUntrack(t *testing.T) {
	s := NewStore()
	e := &fakeEntity{id: "x", typ: "item", tags: []string{"weapon"}}
	if err := s.Track(e); err != nil {
		t.Fatalf("Track: %v", err)
	}
	if err := s.Track(e); !errors.Is(err, ErrAlreadyTracked) {
		t.Errorf("re-Track err = %v, want ErrAlreadyTracked", err)
	}
	if got, ok := s.GetByID("x"); !ok || got != e {
		t.Errorf("GetByID ok=%v got=%v", ok, got)
	}
	if err := s.Untrack("x"); err != nil {
		t.Errorf("Untrack: %v", err)
	}
	if err := s.Untrack("x"); !errors.Is(err, ErrNotTracked) {
		t.Errorf("double-Untrack err = %v, want ErrNotTracked", err)
	}
	if _, ok := s.GetByID("x"); ok {
		t.Error("entity still resolves after Untrack")
	}
}

func TestStoreGetByIDFallbackPromotes(t *testing.T) {
	// §4.2 step 2: when tracked misses, fall back to room scan and
	// opportunistically promote into the tracked index.
	s := NewStore()
	stray := &fakeEntity{id: "stray", typ: "item"}
	s.SetRoomScan(func(id EntityID) (Entity, bool) {
		if id == stray.ID() {
			return stray, true
		}
		return nil, false
	})

	got, ok := s.GetByID("stray")
	if !ok || got != stray {
		t.Fatalf("fallback miss: ok=%v got=%v", ok, got)
	}
	if s.Count() != 1 {
		t.Errorf("Count after promote = %d, want 1", s.Count())
	}
	// Subsequent lookups hit the tracked index directly. To prove that,
	// drop the fallback and look up again.
	s.SetRoomScan(nil)
	if _, ok := s.GetByID("stray"); !ok {
		t.Error("second lookup missed; was the entity not promoted?")
	}
}

func TestStoreGetByIDNoFallbackJustMisses(t *testing.T) {
	s := NewStore()
	if _, ok := s.GetByID("nope"); ok {
		t.Error("ok=true on nil-fallback miss")
	}
}

func TestStoreGetByTagDoubleBufferReadsLagOneTick(t *testing.T) {
	// §3.7 / §4.3 read consistency: queries within a tick see a
	// consistent snapshot; mutations land in the write side and become
	// visible only after SwapTagIndex.
	s := NewStore()
	a := &fakeEntity{id: "a", typ: "item", tags: []string{"weapon"}}

	if err := s.Track(a); err != nil {
		t.Fatalf("Track: %v", err)
	}
	// Track wrote to the write side. The read side is still empty
	// until the first SwapTagIndex.
	if got := s.GetByTag("weapon"); len(got) != 0 {
		t.Errorf("pre-swap GetByTag = %v, want empty (write side not yet published)", got)
	}
	s.SwapTagIndex()
	got := s.GetByTag("weapon")
	if len(got) != 1 || got[0] != a {
		t.Errorf("post-swap GetByTag = %v, want [a]", got)
	}

	// Untrack lands on write side; reads still see a until next swap.
	if err := s.Untrack(a.ID()); err != nil {
		t.Fatalf("Untrack: %v", err)
	}
	if got := s.GetByTag("weapon"); len(got) != 1 {
		t.Errorf("pre-swap (post-untrack) GetByTag len = %d, want 1 (still cached)", len(got))
	}
	s.SwapTagIndex()
	if got := s.GetByTag("weapon"); len(got) != 0 {
		t.Errorf("post-swap (post-untrack) GetByTag = %v, want empty", got)
	}
}

func TestStoreGetByTagSnapshotIsCopy(t *testing.T) {
	s := NewStore()
	a := &fakeEntity{id: "a", typ: "item", tags: []string{"weapon"}}
	_ = s.Track(a)
	s.SwapTagIndex()

	got := s.GetByTag("weapon")
	got[0] = nil // mutate the returned slice

	got2 := s.GetByTag("weapon")
	if len(got2) != 1 || got2[0] != a {
		t.Errorf("internal state corrupted by caller mutation: %v", got2)
	}
}

func TestStoreGetByTypeCaseInsensitive(t *testing.T) {
	s := NewStore()
	a := &fakeEntity{id: "a", typ: "Item"}
	b := &fakeEntity{id: "b", typ: "MOB"}
	c := &fakeEntity{id: "c", typ: "item"}
	for _, e := range []Entity{a, b, c} {
		if err := s.Track(e); err != nil {
			t.Fatalf("Track %s: %v", e.ID(), err)
		}
	}
	items := s.GetByType("item")
	if len(items) != 2 {
		t.Errorf("items len = %d, want 2", len(items))
	}
	mobs := s.GetByType("mob")
	if len(mobs) != 1 || mobs[0] != b {
		t.Errorf("mobs = %v, want [b]", mobs)
	}
	if got := s.GetByType("nope"); got != nil {
		t.Errorf("unknown type returned %v, want nil", got)
	}
}

func TestStoreSwapClearsRemovedTags(t *testing.T) {
	// Tag with no remaining members should be pruned from the read
	// side too.
	s := NewStore()
	a := &fakeEntity{id: "a", typ: "item", tags: []string{"unique"}}
	_ = s.Track(a)
	s.SwapTagIndex()
	if got := s.GetByTag("unique"); len(got) != 1 {
		t.Fatalf("setup: GetByTag unique = %v", got)
	}
	_ = s.Untrack("a")
	s.SwapTagIndex()
	// After two swaps with no members, the read side should be empty.
	if got := s.GetByTag("unique"); len(got) != 0 {
		t.Errorf("post-swap GetByTag unique = %v, want empty", got)
	}
}

func TestStoreSpawnTracks(t *testing.T) {
	s := NewStore()
	tpl := &item.Template{ID: "x", Name: "n", Type: "item", Tags: []string{"weapon"}}
	a, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if got, ok := s.GetByID(a.ID()); !ok || got != a {
		t.Errorf("Spawn did not Track: ok=%v got=%v", ok, got)
	}
	// Tag landed on write side, not read.
	if got := s.GetByTag("weapon"); len(got) != 0 {
		t.Errorf("pre-swap GetByTag = %v, want empty", got)
	}
	s.SwapTagIndex()
	if got := s.GetByTag("weapon"); len(got) != 1 || got[0] != a {
		t.Errorf("post-swap GetByTag = %v, want [a]", got)
	}
}

func TestStoreConcurrentSpawnAndQuery(t *testing.T) {
	// Race-detector smoke: many spawns and queries in parallel.
	s := NewStore()
	tpl := &item.Template{ID: "x", Name: "n", Type: "item", Tags: []string{"weapon"}}

	var wg sync.WaitGroup
	const writers, readers, swaps = 8, 8, 4
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if _, err := s.Spawn(tpl); err != nil {
					t.Errorf("Spawn: %v", err)
					return
				}
			}
		}()
	}
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = s.GetByTag("weapon")
				_ = s.GetByType("item")
			}
		}()
	}
	for i := 0; i < swaps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				s.SwapTagIndex()
			}
		}()
	}
	wg.Wait()

	if got, want := s.Count(), writers*100; got != want {
		t.Errorf("Count = %d, want %d", got, want)
	}
}
