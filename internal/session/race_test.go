package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// makeRaceRegistry builds a registry with two races for the
// resolution-path tests. Mirrors what the pack loader would build
// from content/core/races/.
func makeRaceRegistry(t *testing.T) *progression.RaceRegistry {
	t.Helper()
	reg := progression.NewRaceRegistry()
	if err := reg.Register(&progression.Race{
		ID:          "human",
		DisplayName: "Human",
		RacialFlags: []string{"common-tongue"},
	}); err != nil {
		t.Fatalf("Register human: %v", err)
	}
	if err := reg.Register(&progression.Race{
		ID:                "dwarf",
		DisplayName:       "Dwarf",
		RacialFlags:       []string{"common-tongue", "darkvision"},
		StartingAlignment: 50,
	}); err != nil {
		t.Fatalf("Register dwarf: %v", err)
	}
	return reg
}

func TestApplyRace_SavedRaceWinsOverDefault(t *testing.T) {
	a := &connActor{}
	cfg := &Config{Races: makeRaceRegistry(t), DefaultRace: "human"}
	applyRace(a, cfg, "dwarf")

	if a.RaceID() != "dwarf" {
		t.Errorf("RaceID = %q, want dwarf (saved overrides default)", a.RaceID())
	}
	tags := a.Tags()
	if len(tags) != 2 || tags[0] != "common-tongue" || tags[1] != "darkvision" {
		t.Errorf("Tags = %v, want [common-tongue darkvision]", tags)
	}
}

func TestApplyRace_EmptySavedFallsBackToDefault(t *testing.T) {
	a := &connActor{}
	cfg := &Config{Races: makeRaceRegistry(t), DefaultRace: "human"}
	applyRace(a, cfg, "")

	if a.RaceID() != "human" {
		t.Errorf("RaceID = %q, want human (default fallback)", a.RaceID())
	}
	tags := a.Tags()
	if len(tags) != 1 || tags[0] != "common-tongue" {
		t.Errorf("Tags = %v, want [common-tongue]", tags)
	}
}

func TestApplyRace_UnknownIDLeavesRaceless(t *testing.T) {
	a := &connActor{}
	cfg := &Config{Races: makeRaceRegistry(t), DefaultRace: "human"}
	applyRace(a, cfg, "extinct-species")

	if a.RaceID() != "" {
		t.Errorf("RaceID = %q, want empty (unknown race must not stick)", a.RaceID())
	}
	if len(a.Tags()) != 0 {
		t.Errorf("Tags = %v, want empty for unknown race", a.Tags())
	}
}

func TestApplyRace_NilRegistryNoOp(t *testing.T) {
	a := &connActor{}
	cfg := &Config{DefaultRace: "human"}
	applyRace(a, cfg, "dwarf")

	if a.RaceID() != "" {
		t.Errorf("RaceID = %q, want empty (nil registry)", a.RaceID())
	}
}

func TestApplyRace_CaseInsensitive(t *testing.T) {
	a := &connActor{}
	cfg := &Config{Races: makeRaceRegistry(t), DefaultRace: "human"}
	applyRace(a, cfg, "DWARF")
	if a.RaceID() != "dwarf" {
		t.Errorf("RaceID = %q, want dwarf (lookup must be case-insensitive)", a.RaceID())
	}
}

func TestTagsReturnsCopy(t *testing.T) {
	a := &connActor{}
	cfg := &Config{Races: makeRaceRegistry(t), DefaultRace: "human"}
	applyRace(a, cfg, "dwarf")

	tags := a.Tags()
	tags[0] = "tampered"

	again := a.Tags()
	if again[0] == "tampered" {
		t.Error("Tags() returned aliased slice; mutation by caller bled into actor state")
	}
}
