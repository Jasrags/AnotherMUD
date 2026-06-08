package recipe

import (
	"errors"
	"testing"
)

func sample(id RecipeID, discipline string) *Recipe {
	return &Recipe{
		ID:          id,
		DisplayName: "a thing",
		Discipline:  discipline,
		Inputs:      []Ingredient{{Template: "core:wood", Quantity: 2}},
		Output:      Output{Template: "core:plank", Quantity: 1},
	}
}

func TestRegistry_TryAddGetHas(t *testing.T) {
	reg := NewRegistry()
	r := sample("core:plank", "woodworking")
	if err := reg.TryAdd(r); err != nil {
		t.Fatalf("TryAdd: %v", err)
	}
	if !reg.Has("core:plank") {
		t.Error("Has(core:plank) = false, want true")
	}
	got, err := reg.Get("core:plank")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DisplayName != "a thing" {
		t.Errorf("DisplayName = %q", got.DisplayName)
	}
	if reg.Count() != 1 {
		t.Errorf("Count = %d, want 1", reg.Count())
	}
}

func TestRegistry_TryAddDuplicate(t *testing.T) {
	reg := NewRegistry()
	if err := reg.TryAdd(sample("core:plank", "woodworking")); err != nil {
		t.Fatalf("first TryAdd: %v", err)
	}
	err := reg.TryAdd(sample("core:plank", "woodworking"))
	if !errors.Is(err, ErrDuplicateID) {
		t.Errorf("second TryAdd err = %v, want ErrDuplicateID", err)
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	reg := NewRegistry()
	if _, err := reg.Get("core:nope"); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("Get(missing) err = %v, want ErrRecipeNotFound", err)
	}
	if reg.Has("core:nope") {
		t.Error("Has(missing) = true, want false")
	}
}

func TestRegistry_ByDiscipline(t *testing.T) {
	reg := NewRegistry()
	_ = reg.TryAdd(sample("core:plank", "woodworking"))
	_ = reg.TryAdd(sample("core:stew", "cooking"))
	_ = reg.TryAdd(sample("core:beam", "woodworking"))

	got := reg.ByDiscipline("WoodWorking") // case-insensitive
	if len(got) != 2 {
		t.Fatalf("ByDiscipline(woodworking) returned %d, want 2", len(got))
	}
	if len(reg.ByDiscipline("cooking")) != 1 {
		t.Errorf("ByDiscipline(cooking) returned %d, want 1", len(reg.ByDiscipline("cooking")))
	}
	if len(reg.ByDiscipline("alchemy")) != 0 {
		t.Errorf("ByDiscipline(alchemy) returned %d, want 0", len(reg.ByDiscipline("alchemy")))
	}
}

func TestParseAcquisitionTier(t *testing.T) {
	cases := []struct {
		in   string
		want AcquisitionTier
		ok   bool
	}{
		{"", AcqBaseline, true},
		{"baseline", AcqBaseline, true},
		{"Common", AcqCommon, true},
		{"  UNCOMMON ", AcqUncommon, true},
		{"rare", AcqRare, true},
		{"regional", AcqRegional, true},
		{"mythic", AcqBaseline, false},
	}
	for _, c := range cases {
		got, ok := ParseAcquisitionTier(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseAcquisitionTier(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}
