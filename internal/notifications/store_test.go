package notifications

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func tmpStore(t *testing.T, cap int) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	return NewStore(dir, cap), dir
}

func TestStore_LoadMissingFileReturnsEmptyQueue(t *testing.T) {
	s, _ := tmpStore(t, 50)
	q, err := s.Load(context.Background(), "ent-1", "Alice")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if q == nil {
		t.Fatalf("Load returned nil queue")
	}
	if q.Len() != 0 {
		t.Errorf("Len = %d, want 0 on missing file", q.Len())
	}
	if q.Cap() != 50 {
		t.Errorf("Cap = %d, want 50", q.Cap())
	}
}

func TestStore_RoundTrip(t *testing.T) {
	s, _ := tmpStore(t, 50)
	ctx := context.Background()

	// Load to populate the id→name cache, then publish-equivalent
	// inserts via Append.
	q, err := s.Load(ctx, "ent-1", "Alice")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	q.Append(mk("c1", PriorityChannel, 0))
	q.Append(mk("t1", PriorityTell, 1))
	q.Append(mk("s1", PrioritySystem, 2))

	if err := s.Save("ent-1", q); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// New store, same root → load and verify identical drain order.
	s2 := NewStore(filepath.Dir(s.root), 50)
	q2, err := s2.Load(ctx, "ent-1", "Alice")
	if err != nil {
		t.Fatalf("Load(2): %v", err)
	}
	got := q2.DrainAll()
	wantIDs := []string{"s1", "t1", "c1"}
	if len(got) != len(wantIDs) {
		t.Fatalf("loaded count = %d, want %d", len(got), len(wantIDs))
	}
	for i, n := range got {
		if n.ID != wantIDs[i] {
			t.Errorf("got[%d].ID = %q, want %q", i, n.ID, wantIDs[i])
		}
		// Spot-check the rest of the round-tripped fields.
		if len(n.Recipients) != 1 || n.Recipients[0] != "alice" {
			t.Errorf("got[%d].Recipients = %v, want [alice]", i, n.Recipients)
		}
	}
}

func TestStore_SaveWithoutLoadIsSkipped(t *testing.T) {
	s, _ := tmpStore(t, 50)
	q := NewQueue(50)
	q.Append(mk("a", PrioritySystem, 0))

	// No Load was called for "ent-2"; Save should be a no-op
	// (logged warn, no error).
	if err := s.Save("ent-2", q); err != nil {
		t.Errorf("Save unknown entity: err = %v, want nil", err)
	}
}

func TestStore_LoadEmptyFileTreatedAsNoBacklog(t *testing.T) {
	s, dir := tmpStore(t, 50)
	canonDir := filepath.Join(dir, "players", "alice")
	if err := os.MkdirAll(canonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonDir, "notifications.yaml"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	q, err := s.Load(context.Background(), "ent-3", "Alice")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if q.Len() != 0 {
		t.Errorf("empty file: Len = %d, want 0", q.Len())
	}
}

func TestStore_LoadUnknownPrioritySkipped(t *testing.T) {
	s, dir := tmpStore(t, 50)
	canonDir := filepath.Join(dir, "players", "alice")
	if err := os.MkdirAll(canonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `entries:
  - id: x
    priority: bogus
    kind: bogus
    text: should-skip
    published_at: 2026-05-30T12:00:00Z
  - id: y
    priority: tell
    kind: tell
    text: should-load
    published_at: 2026-05-30T12:00:01Z
`
	if err := os.WriteFile(filepath.Join(canonDir, "notifications.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	q, err := s.Load(context.Background(), "ent-4", "Alice")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := q.DrainAll()
	if len(got) != 1 || got[0].ID != "y" {
		t.Errorf("got %+v, want only [y]", got)
	}
}

func TestStore_LoadParseFailureReturnsEmpty(t *testing.T) {
	s, dir := tmpStore(t, 50)
	canonDir := filepath.Join(dir, "players", "alice")
	if err := os.MkdirAll(canonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(canonDir, "notifications.yaml"),
		[]byte("not: valid: yaml: ["), 0o600); err != nil {
		t.Fatal(err)
	}

	q, err := s.Load(context.Background(), "ent-5", "Alice")
	if err != nil {
		t.Errorf("Load: err = %v, want nil (errors are logged, not propagated)", err)
	}
	if q.Len() != 0 {
		t.Errorf("Len on parse failure = %d, want 0", q.Len())
	}
}

func TestStore_LoadFileExceedsCapTruncatesHighPriorityFirst(t *testing.T) {
	s, dir := tmpStore(t, 2)
	canonDir := filepath.Join(dir, "players", "alice")
	if err := os.MkdirAll(canonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// File holds 3 entries (cap is 2). Drain order on disk is
	// system > tell > channel; truncation should keep system and
	// tell, drop channel.
	body := `entries:
  - id: s1
    priority: system
    kind: system
    text: keep-system
    published_at: 2026-05-30T12:00:00Z
  - id: t1
    priority: tell
    kind: tell
    text: keep-tell
    published_at: 2026-05-30T12:00:01Z
  - id: c1
    priority: channel
    kind: channel
    text: drop-channel
    published_at: 2026-05-30T12:00:02Z
`
	if err := os.WriteFile(filepath.Join(canonDir, "notifications.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	q, err := s.Load(context.Background(), "ent-6", "Alice")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := q.DrainAll()
	if len(got) != 2 {
		t.Fatalf("loaded count = %d, want 2 (truncated to cap)", len(got))
	}
	if got[0].ID != "s1" || got[1].ID != "t1" {
		t.Errorf("kept = [%q,%q], want [s1,t1]", got[0].ID, got[1].ID)
	}
}

func TestStore_SaveEmptyQueueWritesFile(t *testing.T) {
	s, dir := tmpStore(t, 50)
	ctx := context.Background()
	q, err := s.Load(ctx, "ent-7", "Alice")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := s.Save("ent-7", q); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// File exists; subsequent load yields empty queue.
	p := filepath.Join(dir, "players", "alice", "notifications.yaml")
	if _, err := os.Stat(p); err != nil {
		t.Errorf("expected file at %q, stat err: %v", p, err)
	}

	s2 := NewStore(filepath.Dir(s.root), 50)
	q2, err := s2.Load(ctx, "ent-7", "Alice")
	if err != nil {
		t.Fatalf("Load(2): %v", err)
	}
	if q2.Len() != 0 {
		t.Errorf("post-empty-save Len = %d, want 0", q2.Len())
	}
}

func TestStore_Forget(t *testing.T) {
	s, _ := tmpStore(t, 50)
	ctx := context.Background()
	if _, err := s.Load(ctx, "ent-8", "Alice"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	s.Forget("ent-8")

	// Save without a cached name should now be a no-op.
	q := NewQueue(50)
	q.Append(mk("a", PriorityTell, 0))
	if err := s.Save("ent-8", q); err != nil {
		t.Errorf("Save after Forget: err = %v, want nil", err)
	}
}

func TestStore_LoadBadPathReturnsEmpty(t *testing.T) {
	s, _ := tmpStore(t, 50)
	// A traversal-attempt name should be rejected by SafeJoin and
	// yield an empty queue (errors logged, not propagated).
	q, err := s.Load(context.Background(), "ent-9", "../escape")
	if err != nil {
		t.Errorf("Load: err = %v, want nil", err)
	}
	if q == nil || q.Len() != 0 {
		t.Errorf("bad-path Load: q=%v Len=%d", q, q.Len())
	}
}
