package progression_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

func TestPulseDelayTracker_RecordAndIsCoolingDown(t *testing.T) {
	pd := progression.NewPulseDelayTracker()
	pd.Record("ent-1", "kick", 100)
	if !pd.IsCoolingDown("ent-1", "kick", 99) {
		t.Errorf("IsCoolingDown(99) = false, want true (readyAt=100)")
	}
	if pd.IsCoolingDown("ent-1", "kick", 100) {
		t.Errorf("IsCoolingDown(100) = true, want false (readyAt=100 means ready THIS pulse)")
	}
	if pd.IsCoolingDown("ent-1", "kick", 101) {
		t.Errorf("IsCoolingDown(101) = true, want false")
	}
}

func TestPulseDelayTracker_MissingEntries(t *testing.T) {
	pd := progression.NewPulseDelayTracker()
	if pd.IsCoolingDown("ent-x", "kick", 50) {
		t.Error("unrecorded entity should not be cooling down")
	}
	if _, ok := pd.ReadyAt("ent-x", "kick"); ok {
		t.Error("ReadyAt on unrecorded should be (_, false)")
	}
}

func TestPulseDelayTracker_RecordNonPositiveClears(t *testing.T) {
	pd := progression.NewPulseDelayTracker()
	pd.Record("ent-1", "kick", 100)
	pd.Record("ent-1", "kick", 0)
	if _, ok := pd.ReadyAt("ent-1", "kick"); ok {
		t.Error("Record(0) should clear the entry")
	}
}

func TestPulseDelayTracker_SweepEvictsStale(t *testing.T) {
	pd := progression.NewPulseDelayTracker()
	pd.Record("ent-1", "a", 50)
	pd.Record("ent-1", "b", 200)
	if n := pd.Sweep("ent-1", 100); n != 1 {
		t.Errorf("Sweep(100) = %d, want 1 (only 'a' is stale)", n)
	}
	if _, ok := pd.ReadyAt("ent-1", "a"); ok {
		t.Error("Sweep should have removed 'a'")
	}
	if _, ok := pd.ReadyAt("ent-1", "b"); !ok {
		t.Error("Sweep should have left 'b' intact")
	}
}

func TestPulseDelayTracker_Drop(t *testing.T) {
	pd := progression.NewPulseDelayTracker()
	pd.Record("ent-1", "a", 100)
	pd.Drop("ent-1")
	if _, ok := pd.ReadyAt("ent-1", "a"); ok {
		t.Error("Drop did not clear entries")
	}
}

func TestPulseDelayTracker_CaseInsensitive(t *testing.T) {
	pd := progression.NewPulseDelayTracker()
	pd.Record("ENT-1", "Kick", 50)
	if !pd.IsCoolingDown("ent-1", "kick", 10) {
		t.Error("case-insensitive lookup failed")
	}
}
