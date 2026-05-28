package quest

import (
	"errors"
	"testing"
)

func validDef() *Definition {
	return &Definition{
		ID: "rescue", Name: "Rescue", Abandonable: true,
		Stages: []Stage{
			{ID: "find", Description: "Find the captive.", Objectives: []Objective{
				{Type: "kill", Target: "core:guard"},
				{Type: "collect", Target: "core:key", Count: 3},
			}},
			{Description: "Return home.", Objectives: []Objective{
				{Type: "visit", Target: "core:home"},
			}},
		},
	}
}

func TestRegisterValidation(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); !errors.Is(err, ErrNilDefinition) {
		t.Errorf("nil = %v", err)
	}
	if err := r.Register(&Definition{Stages: validDef().Stages}); !errors.Is(err, ErrMissingID) {
		t.Errorf("missing id = %v", err)
	}
	if err := r.Register(&Definition{ID: "x"}); !errors.Is(err, ErrNoStages) {
		t.Errorf("no stages = %v", err)
	}
	if err := r.Register(&Definition{ID: "x", Stages: []Stage{{}}}); !errors.Is(err, ErrNoObjectives) {
		t.Errorf("no objectives = %v", err)
	}
}

func TestRegisterNormalizesObjectives(t *testing.T) {
	r := NewRegistry()
	d := validDef()
	if err := r.Register(d); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Lookup("rescue")
	s0 := got.Stages[0]
	// generated ids from stage id + type + position
	if s0.Objectives[0].ID != "find-kill-0" {
		t.Errorf("obj0 id = %q, want find-kill-0", s0.Objectives[0].ID)
	}
	if s0.Objectives[1].ID != "find-collect-1" {
		t.Errorf("obj1 id = %q, want find-collect-1", s0.Objectives[1].ID)
	}
	// stage with no id falls back to stageN
	if got.Stages[1].Objectives[0].ID != "stage1-visit-0" {
		t.Errorf("stage1 obj id = %q, want stage1-visit-0", got.Stages[1].Objectives[0].ID)
	}
	// count defaults to 1, explicit count preserved
	if s0.Objectives[0].Count != 1 || s0.Objectives[1].Count != 3 {
		t.Errorf("counts = %d, %d", s0.Objectives[0].Count, s0.Objectives[1].Count)
	}
}

func TestObjectiveIDStableAcrossReloads(t *testing.T) {
	r1, r2 := NewRegistry(), NewRegistry()
	_ = r1.Register(validDef())
	_ = r2.Register(validDef())
	a, _ := r1.Lookup("rescue")
	b, _ := r2.Lookup("rescue")
	for s := range a.Stages {
		for o := range a.Stages[s].Objectives {
			if a.Stages[s].Objectives[o].ID != b.Stages[s].Objectives[o].ID {
				t.Errorf("unstable id at [%d][%d]: %q vs %q", s, o,
					a.Stages[s].Objectives[o].ID, b.Stages[s].Objectives[o].ID)
			}
		}
	}
}

func TestRegisterPreservesExplicitObjectiveID(t *testing.T) {
	r := NewRegistry()
	d := validDef()
	d.Stages[0].Objectives[0].ID = "custom-id"
	_ = r.Register(d)
	got, _ := r.Lookup("rescue")
	if got.Stages[0].Objectives[0].ID != "custom-id" {
		t.Errorf("explicit id overwritten: %q", got.Stages[0].Objectives[0].ID)
	}
}

func TestRegisterReplaces(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(validDef())
	d2 := validDef()
	d2.Name = "Rescue v2"
	_ = r.Register(d2)
	got, _ := r.Lookup("rescue")
	if got.Name != "Rescue v2" {
		t.Errorf("later registration should replace: %q", got.Name)
	}
	if r.Len() != 1 {
		t.Errorf("len = %d, want 1", r.Len())
	}
}

func TestLookupMissingAndAll(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Lookup("nope"); ok {
		t.Error("missing lookup should be false")
	}
	_ = r.Register(validDef())
	if all := r.All(); len(all) != 1 {
		t.Errorf("All() = %d, want 1", len(all))
	}
}
