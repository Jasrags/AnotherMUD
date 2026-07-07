package entities

import "testing"

// AddTag appends a gameplay tag once (admin-verbs §4 `set tag`); a second add
// of the same tag is idempotent and reports no change.
func TestMobInstance_AddTagIdempotent(t *testing.T) {
	s := NewStore()
	m, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if !m.AddTag("cursed") {
		t.Fatal("AddTag(cursed) = false, want true (first add changes the set)")
	}
	if m.AddTag("cursed") {
		t.Error("AddTag(cursed) second time = true, want false (already present)")
	}
	if !m.HasTag("cursed") {
		t.Error("HasTag(cursed) = false after AddTag")
	}
	// A tag already contributed by the template is not double-added.
	if m.AddTag("guard") {
		t.Error("AddTag(guard) = true, want false (template already carries it)")
	}
}

// RemoveTag drops a present tag and reports the change; removing an absent tag
// is a no-op. Template-derived tags are removable through this path too (the
// command layer, not MobInstance, decides what is off-limits).
func TestMobInstance_RemoveTag(t *testing.T) {
	s := NewStore()
	m, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	m.AddTag("cursed")
	if !m.RemoveTag("cursed") {
		t.Fatal("RemoveTag(cursed) = false, want true")
	}
	if m.HasTag("cursed") {
		t.Error("HasTag(cursed) = true after RemoveTag")
	}
	if m.RemoveTag("cursed") {
		t.Error("RemoveTag(cursed) again = true, want false (already gone)")
	}
}

// After AddTag on a tracked mob the store index reflects the new tag once
// SwapTagIndex publishes the write-side buckets (the in-place mutation needs a
// Retag + swap to reach GetByTag — the caller in set.go drives Retag).
func TestMobInstance_AddTagReindexesViaRetag(t *testing.T) {
	s := NewStore()
	m, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	m.AddTag("cursed")
	if err := s.Retag(m.ID()); err != nil {
		t.Fatalf("Retag: %v", err)
	}
	s.SwapTagIndex()
	got := s.GetByTag("cursed")
	if len(got) != 1 || got[0].ID() != m.ID() {
		t.Errorf("GetByTag(cursed) = %v, want [%q]", got, m.ID())
	}
}
