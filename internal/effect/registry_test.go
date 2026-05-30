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
