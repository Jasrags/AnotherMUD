package progression_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

func TestActionQueueManager_PushPeekPop(t *testing.T) {
	m := progression.NewActionQueueManager(progression.ActionQueueConfig{})
	if ok := m.Push("ent-1", progression.QueuedAction{AbilityID: "Slash"}); !ok {
		t.Fatal("Push returned false")
	}
	if ok := m.Push("ent-1", progression.QueuedAction{AbilityID: "kick", TargetEntityID: "MOB-7"}); !ok {
		t.Fatal("Push #2 returned false")
	}
	if got := m.Len("ent-1"); got != 2 {
		t.Fatalf("Len = %d, want 2", got)
	}
	head, ok := m.Peek("ent-1")
	if !ok || head.AbilityID != "slash" {
		t.Fatalf("Peek = (%+v,%v), want slash", head, ok)
	}
	if got := m.Len("ent-1"); got != 2 {
		t.Fatalf("Peek mutated Len: %d", got)
	}
	popped, ok := m.Pop("ent-1")
	if !ok || popped.AbilityID != "slash" {
		t.Fatalf("Pop = (%+v,%v)", popped, ok)
	}
	second, _ := m.Peek("ent-1")
	if second.AbilityID != "kick" || second.TargetEntityID != "mob-7" {
		t.Fatalf("after Pop, Peek = %+v; want {kick mob-7}", second)
	}
}

func TestActionQueueManager_PendingEntities(t *testing.T) {
	m := progression.NewActionQueueManager(progression.ActionQueueConfig{})
	if got := m.PendingEntities(); len(got) != 0 {
		t.Fatalf("empty manager: PendingEntities = %v, want none", got)
	}
	m.Push("p-1", progression.QueuedAction{AbilityID: "heal"})
	m.Push("p-2", progression.QueuedAction{AbilityID: "bless"})
	got := m.PendingEntities()
	set := map[string]bool{}
	for _, id := range got {
		set[id] = true
	}
	if len(got) != 2 || !set["p-1"] || !set["p-2"] {
		t.Fatalf("PendingEntities = %v, want p-1 + p-2", got)
	}
	// Draining an entity drops it from the pending set.
	m.Pop("p-1")
	got = m.PendingEntities()
	if len(got) != 1 || got[0] != "p-2" {
		t.Fatalf("after draining p-1: PendingEntities = %v, want [p-2]", got)
	}
}

func TestActionQueueManager_PushRejectsEmpty(t *testing.T) {
	m := progression.NewActionQueueManager(progression.ActionQueueConfig{})
	if m.Push("", progression.QueuedAction{AbilityID: "slash"}) {
		t.Error("Push with empty entityID returned true")
	}
	if m.Push("ent-1", progression.QueuedAction{}) {
		t.Error("Push with empty AbilityID returned true")
	}
	if m.Len("ent-1") != 0 {
		t.Error("rejected pushes mutated state")
	}
}

func TestActionQueueManager_LimitRejects(t *testing.T) {
	m := progression.NewActionQueueManager(progression.ActionQueueConfig{Limit: 2})
	if !m.Push("ent-1", progression.QueuedAction{AbilityID: "a"}) {
		t.Fatal("push 1")
	}
	if !m.Push("ent-1", progression.QueuedAction{AbilityID: "b"}) {
		t.Fatal("push 2")
	}
	if m.Push("ent-1", progression.QueuedAction{AbilityID: "c"}) {
		t.Fatal("push 3 should have been rejected by limit")
	}
	if m.Len("ent-1") != 2 {
		t.Fatalf("Len = %d, want 2", m.Len("ent-1"))
	}
}

func TestActionQueueManager_PopEmptyDeletesKey(t *testing.T) {
	m := progression.NewActionQueueManager(progression.ActionQueueConfig{})
	m.Push("ent-1", progression.QueuedAction{AbilityID: "a"})
	if _, ok := m.Pop("ent-1"); !ok {
		t.Fatal("Pop returned false on populated queue")
	}
	if m.Len("ent-1") != 0 {
		t.Errorf("Len after final Pop = %d", m.Len("ent-1"))
	}
	if _, ok := m.Pop("ent-1"); ok {
		t.Error("Pop on empty queue returned true")
	}
}

func TestActionQueueManager_SnapshotIsDeepCopy(t *testing.T) {
	m := progression.NewActionQueueManager(progression.ActionQueueConfig{})
	m.Push("ent-1", progression.QueuedAction{AbilityID: "a"})
	m.Push("ent-1", progression.QueuedAction{AbilityID: "b"})
	snap := m.Snapshot("ent-1")
	if len(snap) != 2 {
		t.Fatalf("Snapshot len = %d", len(snap))
	}
	snap[0].AbilityID = "mutated"
	if head, _ := m.Peek("ent-1"); head.AbilityID != "a" {
		t.Errorf("snapshot mutation leaked: front = %q", head.AbilityID)
	}
}

func TestActionQueueManager_Drop(t *testing.T) {
	m := progression.NewActionQueueManager(progression.ActionQueueConfig{})
	m.Push("ent-1", progression.QueuedAction{AbilityID: "a"})
	m.Push("ent-1", progression.QueuedAction{AbilityID: "b"})
	if n := m.Drop("ent-1"); n != 2 {
		t.Errorf("Drop = %d, want 2", n)
	}
	if m.Len("ent-1") != 0 {
		t.Error("Drop did not clear queue")
	}
	if n := m.Drop("ent-1"); n != 0 {
		t.Errorf("Drop on empty = %d, want 0", n)
	}
}
