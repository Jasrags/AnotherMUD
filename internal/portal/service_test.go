package portal

import (
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// captureSink records every OnPortalOpened / OnPortalClosed call
// so tests can assert event emission.
type captureSink struct {
	mu     sync.Mutex
	opened []Portal
	closed []Portal
}

func (s *captureSink) OnPortalOpened(p Portal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opened = append(s.opened, p)
}

func (s *captureSink) OnPortalClosed(p Portal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = append(s.closed, p)
}

func threeRoomWorld(t *testing.T) *world.World {
	t.Helper()
	w := world.New()
	w.AddRoom(&world.Room{ID: "town:square", Name: "Square"})
	w.AddRoom(&world.Room{ID: "town:gate", Name: "Gate"})
	w.AddRoom(&world.Room{ID: "wilds:hollow", Name: "Hollow"})
	return w
}

func TestCreate_HappyPath(t *testing.T) {
	w := threeRoomWorld(t)
	sink := &captureSink{}
	s := NewService(w, sink)

	id := s.Create("town:square", "Gate", "town:gate", 100, "ornate gate")
	if id == "" {
		t.Fatal("Create returned empty id")
	}

	// World-side registration happened (case-folded keyword).
	dst, err := w.MoveByKeyword("town:square", "GATE")
	if err != nil {
		t.Fatalf("MoveByKeyword: %v", err)
	}
	if dst.ID != "town:gate" {
		t.Errorf("dst = %q, want town:gate", dst.ID)
	}

	// Sink saw the open.
	if len(sink.opened) != 1 || sink.opened[0].Keyword != "gate" {
		t.Errorf("OnPortalOpened: %+v", sink.opened)
	}
}

func TestCreate_RejectsKeywordCollision(t *testing.T) {
	w := threeRoomWorld(t)
	s := NewService(w, nil)

	if s.Create("town:square", "gate", "town:gate", 0, "") == "" {
		t.Fatal("first create failed")
	}
	if id := s.Create("town:square", "gate", "wilds:hollow", 0, ""); id != "" {
		t.Errorf("collision: want empty id, got %q", id)
	}
}

func TestCreate_RejectsMissingTarget(t *testing.T) {
	w := threeRoomWorld(t)
	s := NewService(w, nil)
	if id := s.Create("town:square", "void", "no:such:room", 0, ""); id != "" {
		t.Errorf("missing target: want empty id, got %q", id)
	}
}

func TestCreate_RejectsEmptyKeyword(t *testing.T) {
	w := threeRoomWorld(t)
	s := NewService(w, nil)
	if id := s.Create("town:square", "  ", "town:gate", 0, ""); id != "" {
		t.Errorf("empty keyword: want empty id, got %q", id)
	}
}

func TestRemove_TearsDownWorldAndEmits(t *testing.T) {
	w := threeRoomWorld(t)
	sink := &captureSink{}
	s := NewService(w, sink)
	id := s.Create("town:square", "gate", "town:gate", 0, "")
	if !s.Remove(id) {
		t.Fatal("Remove returned false")
	}
	if w.HasKeywordExit("town:square", "gate") {
		t.Error("world keyword exit still present after Remove")
	}
	if len(sink.closed) != 1 {
		t.Errorf("OnPortalClosed: %d, want 1", len(sink.closed))
	}
}

func TestRemove_Idempotent(t *testing.T) {
	w := threeRoomWorld(t)
	s := NewService(w, nil)
	id := s.Create("town:square", "gate", "town:gate", 0, "")
	s.Remove(id)
	if s.Remove(id) {
		t.Error("second Remove returned true; want false")
	}
}

func TestCreatePaired_BothSidesAndCrossReference(t *testing.T) {
	w := threeRoomWorld(t)
	sink := &captureSink{}
	s := NewService(w, sink)

	primary, partner := s.CreatePaired("town:square", "gate", "wilds:hollow", "south", 100, "stone arch")
	if primary == "" || partner == "" {
		t.Fatalf("CreatePaired: primary=%q partner=%q", primary, partner)
	}

	// Both world-side exits registered.
	if !w.HasKeywordExit("town:square", "gate") || !w.HasKeywordExit("wilds:hollow", "south") {
		t.Error("paired exits not both registered")
	}

	// One portal.opened event fires (for primary), not two.
	if len(sink.opened) != 1 {
		t.Errorf("paired emit count = %d, want 1", len(sink.opened))
	}

	// Cross-reference.
	p, _ := s.Get(primary)
	if p.PairedID != partner {
		t.Errorf("primary.PairedID = %q, want %q", p.PairedID, partner)
	}
	q, _ := s.Get(partner)
	if q.PairedID != primary {
		t.Errorf("partner.PairedID = %q, want %q", q.PairedID, primary)
	}
}

func TestCreatePaired_RollsBackOnPartnerFailure(t *testing.T) {
	w := threeRoomWorld(t)
	s := NewService(w, nil)

	// Pre-claim the target keyword so the partner registration
	// fails. The primary-side registration must NOT leak.
	if !w.AddKeywordExit("wilds:hollow", "south", "town:square") {
		t.Fatal("pre-claim failed")
	}

	primary, partner := s.CreatePaired("town:square", "gate", "wilds:hollow", "south", 0, "")
	if primary != "" || partner != "" {
		t.Errorf("CreatePaired should have refused; got %q / %q", primary, partner)
	}
	if w.HasKeywordExit("town:square", "gate") {
		t.Error("primary-side registration leaked on partner failure")
	}
}

func TestRemove_PairedPartnerToo(t *testing.T) {
	w := threeRoomWorld(t)
	sink := &captureSink{}
	s := NewService(w, sink)
	primary, partner := s.CreatePaired("town:square", "gate", "wilds:hollow", "south", 0, "")

	if !s.Remove(primary) {
		t.Fatal("Remove returned false")
	}
	// Both keyword exits gone.
	if w.HasKeywordExit("town:square", "gate") || w.HasKeywordExit("wilds:hollow", "south") {
		t.Error("paired removal left a world-side keyword exit behind")
	}
	// Partner record gone from the registry.
	if _, ok := s.Get(partner); ok {
		t.Error("partner record still present after primary removal")
	}
	// Only ONE portal.closed event fires per pair (spec §5.6).
	if len(sink.closed) != 1 {
		t.Errorf("paired close emit count = %d, want 1", len(sink.closed))
	}
}

func TestExpireUpTo_RemovesExpiredInArea(t *testing.T) {
	w := threeRoomWorld(t)
	sink := &captureSink{}
	s := NewService(w, sink)

	s.Create("town:square", "alpha", "town:gate", 100, "")
	s.Create("town:square", "beta", "town:gate", 200, "")
	s.Create("wilds:hollow", "back", "town:square", 50, "")

	// Tick=120 in area town: alpha expires (100<=120), beta does
	// not (200>120). The wilds portal is not in scope (different
	// area).
	if n := s.ExpireUpTo("town", 120); n != 1 {
		t.Errorf("ExpireUpTo(town, 120) = %d, want 1", n)
	}
	if w.HasKeywordExit("town:square", "alpha") {
		t.Error("alpha not removed")
	}
	if !w.HasKeywordExit("town:square", "beta") {
		t.Error("beta should still be present")
	}
	// The wilds portal stays (different area).
	if !w.HasKeywordExit("wilds:hollow", "back") {
		t.Error("wilds:back should still be present (different area)")
	}
	if len(sink.closed) != 1 {
		t.Errorf("portal.closed emit count = %d, want 1", len(sink.closed))
	}
}

func TestExpireUpTo_ZeroExpiryIsPermanent(t *testing.T) {
	w := threeRoomWorld(t)
	s := NewService(w, nil)
	s.Create("town:square", "gate", "town:gate", 0, "")
	if n := s.ExpireUpTo("town", 99999); n != 0 {
		t.Errorf("zero-expiry portals should never expire; n=%d", n)
	}
}

func TestExpireUpTo_PairedAcrossAreasOnePass(t *testing.T) {
	w := threeRoomWorld(t)
	sink := &captureSink{}
	s := NewService(w, sink)
	// Pair town:square <-> wilds:hollow with expiryTick=100 on both.
	s.CreatePaired("town:square", "gate", "wilds:hollow", "south", 100, "")

	// Ticking just `town` should reap the pair (spec §5.6:
	// paired partners removed regardless of partner's area).
	if n := s.ExpireUpTo("town", 100); n != 1 {
		t.Errorf("paired expire: n = %d, want 1 (one primary)", n)
	}
	if w.HasKeywordExit("town:square", "gate") || w.HasKeywordExit("wilds:hollow", "south") {
		t.Error("paired expire left a side behind")
	}
	// Exactly one portal.closed (for the primary).
	if len(sink.closed) != 1 {
		t.Errorf("emit count = %d, want 1", len(sink.closed))
	}
}

func TestExpireUpTo_PairedInSameAreaCountedOnce(t *testing.T) {
	w := world.New()
	w.AddRoom(&world.Room{ID: "town:a"})
	w.AddRoom(&world.Room{ID: "town:b"})
	sink := &captureSink{}
	s := NewService(w, sink)
	// Both ends of the pair in the same area; tick reaps once.
	s.CreatePaired("town:a", "north", "town:b", "south", 50, "")
	if n := s.ExpireUpTo("town", 50); n != 1 {
		t.Errorf("same-area paired: n = %d, want 1", n)
	}
}

func TestService_LenAndAll(t *testing.T) {
	w := threeRoomWorld(t)
	s := NewService(w, nil)
	s.Create("town:square", "alpha", "town:gate", 0, "")
	s.Create("town:square", "beta", "town:gate", 0, "")
	if s.Len() != 2 {
		t.Errorf("Len = %d, want 2", s.Len())
	}
	if len(s.All()) != 2 {
		t.Errorf("All len = %d, want 2", len(s.All()))
	}
}

func TestService_ConcurrentCreateRemoveRaceClean(t *testing.T) {
	w := threeRoomWorld(t)
	s := NewService(w, nil)

	const n = 30
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		i := i
		go func() {
			defer wg.Done()
			kw := keywordFor(i)
			id := s.Create("town:square", kw, "town:gate", 0, "")
			if id != "" {
				s.Remove(id)
			}
		}()
	}
	wg.Wait()
	if s.Len() != 0 {
		t.Errorf("post-race Len = %d, want 0", s.Len())
	}
}

func keywordFor(i int) string {
	// produce 30 distinct lower-case keywords without overlapping
	// the room-namespace ":" character.
	return "p" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26))
}
