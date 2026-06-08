package recipe

import (
	"reflect"
	"testing"
)

func regWith(recipes ...*Recipe) *Registry {
	r := NewRegistry()
	for _, rec := range recipes {
		_ = r.TryAdd(rec)
	}
	return r
}

func TestKnownManager_LearnKnowsSnapshot(t *testing.T) {
	m := NewKnownManager(nil)
	if !m.Learn("e1", "core:a") {
		t.Error("first Learn = false, want true (newly learned)")
	}
	if m.Learn("e1", "core:a") {
		t.Error("re-Learn = true, want false (already known)")
	}
	if !m.Knows("e1", "core:a") {
		t.Error("Knows(core:a) = false")
	}
	if m.Knows("e1", "core:b") {
		t.Error("Knows(core:b) = true, want false")
	}
	_ = m.Learn("e1", "core:c")
	// Snapshot is sorted.
	if got := m.Snapshot("e1"); !reflect.DeepEqual(got, []string{"core:a", "core:c"}) {
		t.Errorf("Snapshot = %v, want [core:a core:c]", got)
	}
	// A fresh entity snapshots to nil (so the save omits the key).
	if got := m.Snapshot("nobody"); got != nil {
		t.Errorf("Snapshot(nobody) = %v, want nil", got)
	}
}

func TestKnownManager_RestoreDropsUnknown(t *testing.T) {
	// §9: a known id whose recipe is no longer in content is ignored.
	reg := regWith(sample("core:a", "cooking"))
	m := NewKnownManager(reg)
	m.Restore("e1", []string{"core:a", "core:gone", " ", "core:a"})
	got := m.Snapshot("e1")
	if !reflect.DeepEqual(got, []string{"core:a"}) {
		t.Errorf("Snapshot after Restore = %v, want [core:a] (unknown + blank dropped)", got)
	}
}

func TestKnownManager_RestoreNoRegistryAcceptsAll(t *testing.T) {
	m := NewKnownManager(nil)
	m.Restore("e1", []string{"core:x", "core:y"})
	if got := m.Snapshot("e1"); !reflect.DeepEqual(got, []string{"core:x", "core:y"}) {
		t.Errorf("Snapshot = %v, want [core:x core:y]", got)
	}
}

func TestKnownManager_RestoreEmptyClears(t *testing.T) {
	m := NewKnownManager(nil)
	m.Learn("e1", "core:a")
	m.Restore("e1", nil)
	if m.Knows("e1", "core:a") {
		t.Error("Restore(nil) did not clear the set")
	}
}

func TestKnownManager_Drop(t *testing.T) {
	m := NewKnownManager(nil)
	m.Learn("e1", "core:a")
	m.Drop("e1")
	if m.Knows("e1", "core:a") {
		t.Error("Drop did not clear the set")
	}
}

func TestKnownManager_GrantBaseline(t *testing.T) {
	baselineA := sample("core:a", "smithing")
	baselineA.Acquisition = AcqBaseline
	commonB := sample("core:b", "smithing")
	commonB.Acquisition = AcqCommon // not baseline — not granted
	otherC := sample("core:c", "cooking")
	otherC.Acquisition = AcqBaseline // different discipline — not granted
	reg := regWith(baselineA, commonB, otherC)
	m := NewKnownManager(reg)

	learned := m.GrantBaseline("e1", "smithing")
	if !reflect.DeepEqual(learned, []RecipeID{"core:a"}) {
		t.Errorf("GrantBaseline learned = %v, want [core:a]", learned)
	}
	if !m.Knows("e1", "core:a") {
		t.Error("baseline recipe not learned")
	}
	if m.Knows("e1", "core:b") {
		t.Error("non-baseline recipe was learned")
	}
	if m.Knows("e1", "core:c") {
		t.Error("other-discipline recipe was learned")
	}
	// Idempotent: re-granting learns nothing new.
	if again := m.GrantBaseline("e1", "smithing"); again != nil {
		t.Errorf("re-GrantBaseline learned = %v, want nil", again)
	}
}

func TestKnownManager_GrantBaselineNoRegistry(t *testing.T) {
	m := NewKnownManager(nil)
	if got := m.GrantBaseline("e1", "smithing"); got != nil {
		t.Errorf("GrantBaseline with nil registry = %v, want nil", got)
	}
}
