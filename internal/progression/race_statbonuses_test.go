package progression_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// TestRaceRegistryStatBonusesCloned mirrors TestRaceRegistryStatCapsCloned:
// mutating the caller-side StatBonuses map after Register MUST NOT affect the
// registered entry (the registry deep-copies map fields). StatBonuses is the
// per-metatype starting-attribute grant (sr-m3c-deferred-fixes) applied once at
// creation via ApplyStartingStats, the same seam as Class.StartingStats.
func TestRaceRegistryStatBonusesCloned(t *testing.T) {
	bonuses := map[progression.StatType]int{progression.StatSTR: 2}
	r := progression.NewRaceRegistry()
	_ = r.Register(&progression.Race{ID: "ork", StatBonuses: bonuses})
	bonuses[progression.StatSTR] = 99 // tamper after registration

	got, _ := r.Get("ork")
	if got.StatBonuses[progression.StatSTR] != 2 {
		t.Errorf("StatBonuses[STR] = %d, want 2 (registry must clone)", got.StatBonuses[progression.StatSTR])
	}
}
