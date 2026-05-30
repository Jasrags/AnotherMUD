package effect

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

func bless() progression.EffectTemplate {
	return progression.EffectTemplate{
		ID:        "bless",
		Duration:  300,
		Modifiers: []stats.Modifier{{Stat: "hit_mod", Value: 2}},
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(bless()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := r.Get("BLESS")
	if !ok {
		t.Fatalf("Get(BLESS) miss")
	}
	if got.ID != "bless" || got.Duration != 300 {
		t.Errorf("Get returned %+v", got)
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d, want 1", r.Len())
	}
}

func TestRegistry_RegisterCanonicalizesID(t *testing.T) {
	r := NewRegistry()
	tpl := bless()
	tpl.ID = "  Bless  " // whitespace + casing
	if err := r.Register(tpl); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, _ := r.Get("bless")
	if got.ID != "bless" {
		t.Errorf("canonical ID = %q, want %q", got.ID, "bless")
	}
}

func TestRegistry_RejectsDuplicateID(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(bless())
	err := r.Register(bless())
	if err == nil || !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("dup id: %v", err)
	}
}

func TestRegistry_RejectsEmptyID(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(progression.EffectTemplate{}); err == nil {
		t.Errorf("empty id: want error")
	}
}

func TestRegistry_AllInRegistrationOrder(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(progression.EffectTemplate{ID: "a"})
	_ = r.Register(progression.EffectTemplate{ID: "b"})
	_ = r.Register(progression.EffectTemplate{ID: "c"})
	got := r.All()
	if len(got) != 3 || got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "c" {
		ids := make([]string, len(got))
		for i, t := range got {
			ids[i] = t.ID
		}
		t.Errorf("All order = %v", ids)
	}
}

func TestRegistry_GetMissReturnsZero(t *testing.T) {
	r := NewRegistry()
	got, ok := r.Get("nothing")
	if ok || got.ID != "" {
		t.Errorf("Get miss = (%+v, %v), want zero", got, ok)
	}
}

func TestRegistry_GetEmptyKey(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get(""); ok {
		t.Errorf("Get empty: want miss")
	}
	if _, ok := r.Get("   "); ok {
		t.Errorf("Get whitespace: want miss")
	}
}

// TestRegistry_GetReturnsDeepCopy pins the post-review H1 fix:
// mutating slices on the returned EffectTemplate must not affect
// the registry's stored entry.
func TestRegistry_GetReturnsDeepCopy(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(bless()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, _ := r.Get("bless")
	if len(got.Modifiers) != 1 {
		t.Fatalf("Modifiers len = %d, want 1", len(got.Modifiers))
	}
	// Mutate the returned slice.
	got.Modifiers[0].Value = 9999
	// Re-fetch and confirm the stored entry is untouched.
	again, _ := r.Get("bless")
	if again.Modifiers[0].Value != 2 {
		t.Errorf("stored Modifiers[0].Value = %d after caller mutation; want 2 (defensive copy broken)",
			again.Modifiers[0].Value)
	}
}

// TestRegistry_AllReturnsDeepCopy pins the same H1 guarantee for All.
func TestRegistry_AllReturnsDeepCopy(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(bless())
	out := r.All()
	out[0].Flags = append(out[0].Flags, "tampered")
	out[0].Modifiers[0].Value = 9999

	got, _ := r.Get("bless")
	if got.Modifiers[0].Value != 2 {
		t.Errorf("stored Modifiers[0].Value = %d after caller mutation via All", got.Modifiers[0].Value)
	}
	for _, f := range got.Flags {
		if f == "tampered" {
			t.Error("stored Flags carries tampered entry")
		}
	}
}
