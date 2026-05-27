package session

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
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

// TestPersist_SyncsVitalsFromCombatTick confirms M7.5 vitals
// persistence: damage applied through the Combatant surface (which
// never goes through markDirtyLocked — combat doesn't know about
// session) still round-trips to disk via Persist's pre-dirty-check
// vitals sync.
func TestPersist_SyncsVitalsFromCombatTick(t *testing.T) {
	dir := t.TempDir()
	st, err := player.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	a := &connActor{
		id: "c1", players: st,
		save: &player.Save{
			Version: player.CurrentVersion, ID: "p1", AccountID: "a1",
			Name: "Wounded", Location: "x:1",
		},
		vitals: combat.NewVitals(combat.DefaultPlayerMaxHP),
	}
	// Damage applied combat-side does NOT flip session's dirty bit.
	a.vitals.ApplyDamage(7)

	if err := a.Persist(context.Background()); err != nil {
		t.Fatalf("Persist: %v", err)
	}

	got, err := st.Load(context.Background(), "Wounded")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Vitals == nil {
		t.Fatalf("Vitals nil after Persist; want HP/MaxHP populated")
	}
	wantHP := combat.DefaultPlayerMaxHP - 7
	if got.Vitals.HP != wantHP || got.Vitals.MaxHP != combat.DefaultPlayerMaxHP {
		t.Errorf("Vitals = %+v, want {HP:%d MaxHP:%d}", got.Vitals, wantHP, combat.DefaultPlayerMaxHP)
	}
}

// TestPersist_DroppedManagerDoesNotEraseAbilities pins the M9.1
// Drop/autosave race guard. If fullTeardown's Drop runs before an
// autosave-in-flight Persist (the snapshot returns empty), the
// guard must NOT overwrite the populated save block with empty.
// Regression coverage for the CRITICAL review finding.
func TestPersist_DroppedManagerDoesNotEraseAbilities(t *testing.T) {
	dir := t.TempDir()
	st, err := player.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	prof := progression.NewProficiencyManager(nil, progression.DefaultProficiencyConfig())
	a := &connActor{
		id: "c1", players: st, prof: prof,
		playerID: "p1",
		save: &player.Save{
			Version: player.CurrentVersion, ID: "p1", AccountID: "a1",
			Name: "Maevyn", Location: "x:1",
			Abilities: progression.AbilitySnapshot{
				Proficiency: map[string]int{"slash": 25},
				Cap:         map[string]int{"slash": 50},
			},
		},
	}
	// Manager already Dropped this entity (snapshot returns empty).
	// Mark dirty so Persist actually writes — we want to assert the
	// save-on-disk preserves the pre-Drop abilities.
	prof.Drop("p1")
	a.dirty = true
	if err := a.Persist(context.Background()); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	got, err := st.Load(context.Background(), "Maevyn")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Abilities.Proficiency["slash"] != 25 || got.Abilities.Cap["slash"] != 50 {
		t.Errorf("dropped-manager Persist erased abilities: got %+v", got.Abilities)
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
// for assertions and tracks whether Close has been invoked. Used by
// manager broadcast / idle tests that don't need the full network
// stack from session_test.go.
type fakeConn struct {
	id       string
	mu       sync.Mutex
	lines    []string
	closeHit bool
}

func (f *fakeConn) ID() string { return f.id }
func (f *fakeConn) Write(_ context.Context, p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines = append(f.lines, string(p))
	return len(p), nil
}
func (f *fakeConn) Read(_ context.Context) (string, error) { return "", nil }
func (f *fakeConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeHit = true
	return nil
}
func (f *fakeConn) RemoteAddr() string { return "fake" }

func (f *fakeConn) closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closeHit
}

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
		id:        connID,
		conn:      fc,
		playerID:  playerID,
		accountID: accountID,
		room:      room,
		save: &player.Save{
			Version:   player.CurrentVersion,
			ID:        playerID,
			AccountID: accountID,
			Name:      name,
			Location:  string(room.ID),
		},
		// M7.1: fakes get the same combat defaults as live actors so
		// tests that touch the Combatant surface (consider, future
		// combat manager) don't nil-deref on Vitals(). M8.1: the
		// combat block now derives from the progression StatBlock,
		// so stamp the engine-default base.
		vitals:    combat.NewVitals(combat.DefaultPlayerMaxHP),
		statBlock: progression.NewWithBase(progression.DefaultPlayerBase()),
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

// TestManager_FindInRoom resolves by name within the requested room,
// is case-insensitive, returns nil for unknown names, and does not
// match an actor who is in a different room.
func TestManager_FindInRoom(t *testing.T) {
	mgr := NewManager()
	r1 := &world.Room{ID: "x:1", Name: "Square"}
	r2 := &world.Room{ID: "x:2", Name: "Forge"}
	alice, _ := newFakeActor("c1", "p1", "acc1", "Alice", r1)
	bob, _ := newFakeActor("c2", "p2", "acc2", "Bob", r2)
	mgr.Add(alice)
	mgr.Add(bob)

	if got := mgr.FindInRoom(r1.ID, "alice"); got != alice {
		t.Errorf("FindInRoom(r1, alice) = %v, want alice", got)
	}
	if got := mgr.FindInRoom(r1.ID, "ALICE"); got != alice {
		t.Errorf("FindInRoom is not case-insensitive")
	}
	if got := mgr.FindInRoom(r1.ID, "bob"); got != nil {
		t.Errorf("FindInRoom(r1, bob) = %v, want nil (bob is in r2)", got)
	}
	if got := mgr.FindInRoom(r2.ID, "bob"); got != bob {
		t.Errorf("FindInRoom(r2, bob) = %v, want bob", got)
	}
	if got := mgr.FindInRoom(r1.ID, "ghost"); got != nil {
		t.Errorf("FindInRoom unknown name = %v, want nil", got)
	}
	if got := mgr.FindInRoom(r1.ID, ""); got != nil {
		t.Errorf("FindInRoom empty name = %v, want nil", got)
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

// TestManager_SetRoomRacingRemoveLeavesNoIndexLeak fires SetRoom and
// Remove from two goroutines for many trials. After the dust settles
// every byRoom map MUST be empty: no orphaned entry for a removed
// actor. The race detector also runs.
func TestManager_SetRoomRacingRemoveLeavesNoIndexLeak(t *testing.T) {
	const trials = 200
	r1 := &world.Room{ID: "x:1"}
	r2 := &world.Room{ID: "x:2"}

	for i := 0; i < trials; i++ {
		mgr := NewManager()
		a, _ := newFakeActor("c", "p", "acc", "Eve", r1)
		mgr.Add(a)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); a.SetRoom(r2) }()
		go func() { defer wg.Done(); mgr.Remove(a) }()
		wg.Wait()

		if mgr.Count() != 0 {
			t.Fatalf("trial %d: Count=%d after Remove", i, mgr.Count())
		}
		// Snapshot byRoom; both rooms must be absent from the index.
		mgr.mu.RLock()
		leaked := len(mgr.byRoom) > 0
		pidStuck := len(mgr.roomByPID) > 0
		mgr.mu.RUnlock()
		if leaked {
			t.Fatalf("trial %d: byRoom not empty after Remove: %v", i, mgr.byRoom)
		}
		if pidStuck {
			t.Fatalf("trial %d: roomByPID not empty after Remove: %v", i, mgr.roomByPID)
		}
	}
}

// TestManager_DuplicateAddIsNoOp verifies that a second Add of the
// same actor does not double-insert into byAccount (which would leak
// a dangling entry after Remove).
func TestManager_DuplicateAddIsNoOp(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:1"}
	a, _ := newFakeActor("c1", "p1", "acc1", "Frank", r)
	mgr.Add(a)
	mgr.Add(a)
	if list := mgr.GetByAccountID("acc1"); len(list) != 1 {
		t.Errorf("duplicate Add produced %d account entries, want 1", len(list))
	}
	mgr.Remove(a)
	if list := mgr.GetByAccountID("acc1"); len(list) != 0 {
		t.Errorf("after Remove of duplicate-Added actor: %d account entries, want 0", len(list))
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
