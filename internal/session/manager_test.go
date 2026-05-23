package session

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TestManager_SaveAllPersistsDirtyActors confirms that the autosave
// path writes every dirty actor through to the store.
func TestManager_SaveAllPersistsDirtyActors(t *testing.T) {
	dir := t.TempDir()
	st, err := player.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := st.Save(context.Background(), &player.Save{
		Version: player.CurrentVersion, ID: "p1", AccountID: "a1",
		Name: "Dirty", Location: "old:room",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	mgr := NewManager()
	a := &connActor{
		id: "c1", players: st,
		save:  &player.Save{Version: player.CurrentVersion, ID: "p1", AccountID: "a1", Name: "Dirty", Location: "new:room"},
		dirty: true,
	}
	mgr.Add(a)

	mgr.SaveAll(context.Background())

	got, err := st.Load(context.Background(), "Dirty")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Location != "new:room" {
		t.Errorf("location = %q, want new:room", got.Location)
	}

	// Second SaveAll: actor is clean now. Overwrite the file by hand;
	// SaveAll should NOT re-write it.
	manualPath := filepath.Join(dir, "players", "dirty", "player.yaml")
	if err := os.WriteFile(manualPath, []byte("SENTINEL\n"), 0o600); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	mgr.SaveAll(context.Background())
	data, err := os.ReadFile(manualPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "SENTINEL\n" {
		t.Errorf("SaveAll re-wrote a clean actor; got %q", data)
	}
}

func TestManager_SaveAllIsolatesErrors(t *testing.T) {
	st, err := player.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	mgr := NewManager()

	bad := &connActor{
		id: "bad", players: st,
		save:  &player.Save{Version: 1, ID: "b", AccountID: "a", Name: "../etc/passwd", Location: "x"},
		dirty: true,
	}
	good := &connActor{
		id: "good", players: st,
		save:  &player.Save{Version: 1, ID: "g", AccountID: "a", Name: "Good", Location: "x"},
		dirty: true,
	}
	mgr.Add(bad)
	mgr.Add(good)

	mgr.SaveAll(context.Background())

	if _, err := st.Load(context.Background(), "Good"); err != nil {
		t.Errorf("good save not written despite bad neighbor: %v", err)
	}
	// Confirm the bad-name save did NOT land on disk. SafeJoin should
	// have rejected the traversal during write, and the Load call
	// likewise refuses to resolve the path — so neither should
	// produce a valid record.
	if _, err := st.Load(context.Background(), "../etc/passwd"); err == nil {
		t.Error("bad-name Load succeeded; path traversal not blocked")
	}
}

// fakeConn is a no-op conn.Connection stand-in that records every Write
// for assertions. Used by manager broadcast tests that don't need the
// full network stack from session_test.go.
type fakeConn struct {
	id    string
	mu    sync.Mutex
	lines []string
}

func (f *fakeConn) ID() string { return f.id }
func (f *fakeConn) Write(_ context.Context, p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines = append(f.lines, string(p))
	return len(p), nil
}
func (f *fakeConn) Read(_ context.Context) (string, error) { return "", nil }
func (f *fakeConn) Close() error                            { return nil }
func (f *fakeConn) RemoteAddr() string                      { return "fake" }

func (f *fakeConn) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.lines))
	copy(out, f.lines)
	return out
}

func newFakeActor(connID, playerID, accountID, name string, room *world.Room) (*connActor, *fakeConn) {
	fc := &fakeConn{id: connID}
	return &connActor{
		id:   connID,
		conn: fc,
		room: room,
		save: &player.Save{
			Version:   player.CurrentVersion,
			ID:        playerID,
			AccountID: accountID,
			Name:      name,
			Location:  string(room.ID),
		},
	}, fc
}

// TestManager_IndicesPopulated checks Add/Remove maintain every index.
func TestManager_IndicesPopulated(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:1", Name: "X"}
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", r)
	b, _ := newFakeActor("c2", "p2", "acc1", "Bob", r)

	mgr.Add(a)
	mgr.Add(b)

	if got, _ := mgr.GetByName("ALICE"); got != a {
		t.Errorf("GetByName(ALICE) did not return Alice")
	}
	if got, _ := mgr.GetByPlayerID("p2"); got != b {
		t.Errorf("GetByPlayerID(p2) did not return Bob")
	}
	if list := mgr.GetByAccountID("acc1"); len(list) != 2 {
		t.Errorf("GetByAccountID(acc1) returned %d, want 2", len(list))
	}
	if mgr.Count() != 2 {
		t.Errorf("Count = %d, want 2", mgr.Count())
	}

	mgr.Remove(a)
	if _, ok := mgr.GetByName("Alice"); ok {
		t.Errorf("Alice still indexed after Remove")
	}
	if list := mgr.GetByAccountID("acc1"); len(list) != 1 || list[0] != b {
		t.Errorf("account list after Remove(Alice) = %v, want [Bob]", list)
	}
}

// TestManager_SendToRoomDeliversToOccupantsExcludesSender stages two
// actors in the same room and verifies SendToRoom hits the right one.
func TestManager_SendToRoomDeliversToOccupantsExcludesSender(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:1", Name: "X"}
	a, fa := newFakeActor("c1", "p1", "acc1", "Alice", r)
	b, fb := newFakeActor("c2", "p2", "acc2", "Bob", r)
	mgr.Add(a)
	mgr.Add(b)

	mgr.SendToRoom(context.Background(), r.ID, "Carol has arrived.", "pCarol")

	if len(fa.writes()) != 1 || len(fb.writes()) != 1 {
		t.Fatalf("expected both to receive 1 line; alice=%d bob=%d",
			len(fa.writes()), len(fb.writes()))
	}

	// Exclude Alice: only Bob should hear it.
	fa.lines, fb.lines = nil, nil
	mgr.SendToRoom(context.Background(), r.ID, "Alice waves.", "p1")
	if len(fa.writes()) != 0 {
		t.Errorf("Alice should be excluded; got %v", fa.writes())
	}
	if len(fb.writes()) != 1 {
		t.Errorf("Bob should have received; got %d lines", len(fb.writes()))
	}
}

// TestManager_SetRoomUpdatesByRoomIndex verifies that connActor.SetRoom
// migrates an actor between rooms in the manager's broadcast index.
func TestManager_SetRoomUpdatesByRoomIndex(t *testing.T) {
	mgr := NewManager()
	r1 := &world.Room{ID: "x:1", Name: "Source"}
	r2 := &world.Room{ID: "x:2", Name: "Dest"}
	a, fa := newFakeActor("c1", "p1", "acc1", "Alice", r1)
	b, fb := newFakeActor("c2", "p2", "acc1", "Bob", r1)
	c, fc := newFakeActor("c3", "p3", "acc1", "Carol", r2)
	mgr.Add(a)
	mgr.Add(b)
	mgr.Add(c)

	// Move Alice from r1 to r2.
	a.SetRoom(r2)

	// A broadcast to r1 must reach only Bob now.
	fa.lines, fb.lines, fc.lines = nil, nil, nil
	mgr.SendToRoom(context.Background(), r1.ID, "ping r1")
	if got := fb.writes(); len(got) != 1 {
		t.Errorf("Bob (r1 occupant) writes = %d, want 1", len(got))
	}
	if got := fa.writes(); len(got) != 0 {
		t.Errorf("Alice (moved out) writes = %d, want 0", len(got))
	}

	// A broadcast to r2 must now reach Alice and Carol.
	fa.lines, fb.lines, fc.lines = nil, nil, nil
	mgr.SendToRoom(context.Background(), r2.ID, "ping r2")
	if len(fa.writes()) != 1 || len(fc.writes()) != 1 {
		t.Errorf("ping r2 alice=%d carol=%d (want 1,1)",
			len(fa.writes()), len(fc.writes()))
	}
}

// TestManager_ConcurrentAddRemoveSendToRoom drives Add/Remove and
// SendToRoom from multiple goroutines for the race detector. The test
// asserts no panic and that the final indices are consistent.
func TestManager_ConcurrentAddRemoveSendToRoom(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:1"}

	const n = 50
	actors := make([]*connActor, n)
	for i := 0; i < n; i++ {
		a, _ := newFakeActor(
			"c"+itoa(i), "p"+itoa(i), "acc1", "Alice"+itoa(i), r)
		actors[i] = a
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.Add(actors[i])
		}()
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.SendToRoom(context.Background(), r.ID, "hi")
		}()
	}
	wg.Wait()

	if mgr.Count() != n {
		t.Errorf("after concurrent Add, Count = %d, want %d", mgr.Count(), n)
	}

	wg = sync.WaitGroup{}
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.Remove(actors[i])
		}()
	}
	wg.Wait()
	if mgr.Count() != 0 {
		t.Errorf("after concurrent Remove, Count = %d, want 0", mgr.Count())
	}
}

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{digits[i%10]}, b...)
		i /= 10
	}
	return string(b)
}
