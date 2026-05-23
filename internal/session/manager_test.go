package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
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
