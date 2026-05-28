package queststore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/quest"
)

func regWith(ids ...string) *quest.Registry {
	r := quest.NewRegistry()
	for _, id := range ids {
		_ = r.Register(&quest.Definition{ID: id, Stages: []quest.Stage{
			{Objectives: []quest.Objective{{Type: "kill", Target: "x"}}},
		}})
	}
	return r
}

func sampleState() *quest.State {
	return &quest.State{
		Active: []quest.ActiveQuest{
			{QuestID: "q1", StageIndex: 1, Objectives: []quest.ObjectiveProgress{
				{ObjectiveID: "s1-kill-0", Current: 2, Required: 5},
			}},
		},
		Completed: []string{"q0"},
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, regWith("q0", "q1"))
	ctx := context.Background()

	// Load caches the name even though no file exists yet.
	if _, ok := st.Load(ctx, "p1", "Alice"); ok {
		t.Fatal("expected no file on first load")
	}
	st.Save("p1", sampleState())

	// File written under players/alice/quests.yaml.
	if _, err := os.Stat(filepath.Join(dir, "players", "alice", "quests.yaml")); err != nil {
		t.Fatalf("quests.yaml not written: %v", err)
	}

	got, ok := st.Load(ctx, "p1", "Alice")
	if !ok {
		t.Fatal("expected file on second load")
	}
	if len(got.Active) != 1 || got.Active[0].QuestID != "q1" || got.Active[0].StageIndex != 1 {
		t.Errorf("active round-trip wrong: %+v", got.Active)
	}
	if got.Active[0].Objectives[0].Current != 2 || got.Active[0].Objectives[0].Required != 5 {
		t.Errorf("objective round-trip wrong: %+v", got.Active[0].Objectives)
	}
	if len(got.Completed) != 1 || got.Completed[0] != "q0" {
		t.Errorf("completed round-trip wrong: %+v", got.Completed)
	}
}

func TestSaveSkippedWithoutCachedName(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, regWith("q1"))
	// No Load → no cached name → Save is a no-op (no panic, no file).
	st.Save("ghost", sampleState())
	if entries, _ := os.ReadDir(filepath.Join(dir, "players")); len(entries) != 0 {
		t.Errorf("save without cached name wrote files: %v", entries)
	}
}

func TestOrphanFilterDropsUnknown(t *testing.T) {
	dir := t.TempDir()
	// Registry knows only q1; q0 (completed) and q9 (active) are orphans.
	st := NewStore(dir, regWith("q1"))
	ctx := context.Background()
	st.Load(ctx, "p1", "Bob") // cache name
	orphaned := &quest.State{
		Active: []quest.ActiveQuest{
			{QuestID: "q1"}, {QuestID: "q9"},
		},
		Completed: []string{"q0", "q1"},
	}
	st.Save("p1", orphaned)

	got, _ := st.Load(ctx, "p1", "Bob")
	if len(got.Active) != 1 || got.Active[0].QuestID != "q1" {
		t.Errorf("active orphan not filtered: %+v", got.Active)
	}
	if len(got.Completed) != 1 || got.Completed[0] != "q1" {
		t.Errorf("completed orphan not filtered: %+v", got.Completed)
	}
}

func TestOrphanFilterSkippedWhenRegistryEmpty(t *testing.T) {
	dir := t.TempDir()
	// Empty registry → §6.4 exception: do NOT filter (would wipe history).
	full := NewStore(dir, regWith("q1"))
	ctx := context.Background()
	full.Load(ctx, "p1", "Cara")
	full.Save("p1", &quest.State{
		Active:    []quest.ActiveQuest{{QuestID: "q1"}, {QuestID: "q9"}},
		Completed: []string{"q0"},
	})

	empty := NewStore(dir, quest.NewRegistry())
	got, ok := empty.Load(ctx, "p1", "Cara")
	if !ok {
		t.Fatal("expected file")
	}
	if len(got.Active) != 2 || len(got.Completed) != 1 {
		t.Errorf("empty registry should skip orphan filter: active=%d completed=%d", len(got.Active), len(got.Completed))
	}
}

func TestLoadMissingFileReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, regWith("q1"))
	if got, ok := st.Load(context.Background(), "p1", "Nobody"); ok || got != nil {
		t.Errorf("missing file = (%v, %v), want (nil, false)", got, ok)
	}
}

func TestForgetDropsNameCache(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, regWith("q1"))
	st.Load(context.Background(), "p1", "Dan")
	st.Forget("p1")
	// After Forget, Save can't resolve the path → no file written.
	st.Save("p1", sampleState())
	if _, err := os.Stat(filepath.Join(dir, "players", "dan", "quests.yaml")); !os.IsNotExist(err) {
		t.Errorf("save after forget wrote a file: %v", err)
	}
}

func TestLoadCorruptYAMLReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, regWith("q1"))
	p := filepath.Join(dir, "players", "eve", "quests.yaml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("active: [unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := st.Load(context.Background(), "p1", "Eve"); ok || got != nil {
		t.Errorf("corrupt yaml = (%v,%v), want (nil,false)", got, ok)
	}
}

func TestLoadUnsafeNameReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, regWith("q1"))
	// An empty/whitespace name canonicalizes to "" → SafeJoin rejects.
	if got, ok := st.Load(context.Background(), "p1", "   "); ok || got != nil {
		t.Errorf("unsafe name = (%v,%v), want (nil,false)", got, ok)
	}
}
