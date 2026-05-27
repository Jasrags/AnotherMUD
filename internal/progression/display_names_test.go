package progression_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

func TestStatDisplayNamesFallthrough(t *testing.T) {
	r := progression.NewStatDisplayNames()

	// Default — no override registered.
	if got := r.Lookup("str"); got != "Strength" {
		t.Fatalf("default Lookup(str) = %q, want Strength", got)
	}
	if got := r.Lookup("resource"); got != "Mana" {
		t.Fatalf("default Lookup(resource) = %q, want Mana", got)
	}

	// Override beats default.
	r.Set("resource", "Essence")
	if got := r.Lookup("resource"); got != "Essence" {
		t.Fatalf("after override, Lookup(resource) = %q, want Essence", got)
	}

	// Unknown name falls through to raw.
	if got := r.Lookup("perception"); got != "perception" {
		t.Fatalf("unknown Lookup = %q, want raw name", got)
	}

	// Empty input is empty out.
	if got := r.Lookup(""); got != "" {
		t.Fatalf("empty Lookup = %q, want empty", got)
	}

	// Lookup is case-insensitive.
	if got := r.Lookup("STR"); got != "Strength" {
		t.Fatalf("Lookup(STR) = %q, want Strength (case-insensitive)", got)
	}
}

func TestDefaultStatDisplayName(t *testing.T) {
	tests := map[string]string{
		"str":          "Strength",
		"hp":           "HP",
		"hp_max":       "Max HP",
		"resource_max": "Max Mana",
		"ac":           "AC",
		"hit_mod":      "Hit",
	}
	for in, want := range tests {
		got, ok := progression.DefaultStatDisplayName(in)
		if !ok {
			t.Errorf("DefaultStatDisplayName(%q) = (_, false), want present", in)
			continue
		}
		if got != want {
			t.Errorf("DefaultStatDisplayName(%q) = %q, want %q", in, got, want)
		}
	}
	if _, ok := progression.DefaultStatDisplayName("nonexistent_stat"); ok {
		t.Error("DefaultStatDisplayName(nonexistent_stat) = (_, true), want false")
	}
}
