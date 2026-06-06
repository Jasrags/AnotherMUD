package slot

import (
	"errors"
	"testing"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Def{Name: "wield", Label: "wielded", Max: 1, Scope: EngineScope}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := r.Get("wield")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "wield" || got.Max != 1 || got.Scope != EngineScope {
		t.Errorf("Get(wield) = %+v", got)
	}
	if !r.Has("wield") {
		t.Error("Has(wield) = false")
	}
}

func TestRegistryRejectsInvalidName(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"", "Wield", "left-hand", "_hidden", "weird/slash"} {
		err := r.Register(Def{Name: name, Max: 1, Scope: EngineScope})
		if !errors.Is(err, ErrInvalidName) {
			t.Errorf("Register(%q) err = %v, want ErrInvalidName", name, err)
		}
	}
}

func TestRegistryRejectsNegativeMax(t *testing.T) {
	r := NewRegistry()
	err := r.Register(Def{Name: "wield", Max: -1, Scope: EngineScope})
	if !errors.Is(err, ErrInvalidMax) {
		t.Errorf("Register err = %v, want ErrInvalidMax", err)
	}
}

func TestRegistryDuplicateAcrossScopes(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Def{Name: "wield", Max: 1, Scope: EngineScope}); err != nil {
		t.Fatalf("first: %v", err)
	}
	err := r.Register(Def{Name: "wield", Max: 1, Scope: "extra-pack"})
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

func TestRegistryLookupIsCaseInsensitive(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Def{Name: "wield", Max: 1, Scope: EngineScope}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !r.Has("WIELD") {
		t.Error("Has(WIELD) = false")
	}
	got, err := r.Get("Wield")
	if err != nil {
		t.Fatalf("Get(Wield): %v", err)
	}
	if got.Name != "wield" {
		t.Errorf("normalized name = %q, want wield", got.Name)
	}
}

func TestRegistryGetUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRegistryAllPreservesRegistrationOrder(t *testing.T) {
	// §3.1: Iteration over all slots MUST preserve registration order.
	r := NewRegistry()
	names := []string{"head", "wield", "finger", "feet"}
	for _, n := range names {
		if err := r.Register(Def{Name: n, Max: 1, Scope: EngineScope}); err != nil {
			t.Fatalf("Register(%q): %v", n, err)
		}
	}
	all := r.All()
	if len(all) != len(names) {
		t.Fatalf("All() len = %d, want %d", len(all), len(names))
	}
	for i, d := range all {
		if d.Name != names[i] {
			t.Errorf("All()[%d].Name = %q, want %q", i, d.Name, names[i])
		}
	}
}

func TestRegistryAllReturnsSnapshot(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(Def{Name: "wield", Max: 1, Scope: EngineScope})

	// Slice-level mutation: replacing an entry must not affect later calls.
	snap := r.All()
	snap[0] = Def{Name: "spoofed", Max: 99}
	if r.Count() != 1 {
		t.Errorf("Count after snapshot mutation = %d, want 1", r.Count())
	}
	if again := r.All(); again[0].Name != "wield" {
		t.Errorf("registry corrupted by slice mutation: %+v", again[0])
	}

	// Field-level mutation: writing to a returned Def must not bleed
	// back into the registry.
	snap2 := r.All()
	snap2[0].Max = 999
	if d, _ := r.Get("wield"); d.Max != 1 {
		t.Errorf("registry corrupted by field mutation: Max=%d, want 1", d.Max)
	}
}

func TestRegistryGetReturnsCopy(t *testing.T) {
	// Writing to a Get-returned Def must not alias the live registry.
	r := NewRegistry()
	_ = r.Register(Def{Name: "wield", Max: 1, Scope: EngineScope})
	d, _ := r.Get("wield")
	d.Max = 999
	again, _ := r.Get("wield")
	if again.Max != 1 {
		t.Errorf("Get-returned Def aliases registry: Max=%d, want 1", again.Max)
	}
}

func TestRegisterEngineBaseline(t *testing.T) {
	r := NewRegistry()
	if err := RegisterEngineBaseline(r); err != nil {
		t.Fatalf("RegisterEngineBaseline: %v", err)
	}
	for _, name := range []string{"wield", "head", "finger", "light"} {
		if !r.Has(name) {
			t.Errorf("baseline missing %q", name)
		}
	}
	finger, _ := r.Get("finger")
	if finger.Max != 2 {
		t.Errorf("finger.Max = %d, want 2", finger.Max)
	}
	light, _ := r.Get("light")
	if light.Max != 1 {
		t.Errorf("light.Max = %d, want 1", light.Max)
	}
}

func TestRegisterEngineBaselineIdempotentFailure(t *testing.T) {
	// Calling twice surfaces a duplicate; baseline isn't idempotent
	// by design — callers should call it once at boot.
	r := NewRegistry()
	if err := RegisterEngineBaseline(r); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := RegisterEngineBaseline(r); !errors.Is(err, ErrDuplicate) {
		t.Errorf("second call err = %v, want ErrDuplicate", err)
	}
}
