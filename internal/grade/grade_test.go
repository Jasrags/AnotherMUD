package grade

import "testing"

func ladder() *Registry {
	r := NewRegistry()
	r.Register(Grade{Key: "masterwork", Order: 1, WeaponToHit: 1})
	r.Register(Grade{Key: "masterpiece", Order: 2, WeaponToHit: 2})
	r.Register(Grade{Key: "power-wrought", Order: 3, WeaponToHit: 3, WeaponDamage: 3, Unbreakable: true})
	return r
}

func TestRegister_RejectsInvalidKey(t *testing.T) {
	r := NewRegistry()
	if r.Register(Grade{Key: "  "}) {
		t.Error("blank key should be rejected")
	}
	if r.Register(Grade{Key: "two words"}) {
		t.Error("whitespace key should be rejected")
	}
	if r.Len() != 0 {
		t.Errorf("registry should be empty, has %d", r.Len())
	}
}

func TestGet_CaseInsensitiveAndUnknown(t *testing.T) {
	r := ladder()
	g, ok := r.Get("MASTERWORK")
	if !ok || g.WeaponToHit != 1 {
		t.Errorf("Get(MASTERWORK) = %+v,%v; want masterwork (+1 hit)", g, ok)
	}
	if _, ok := r.Get("legendary"); ok {
		t.Error("unknown grade should resolve (zero,false)")
	}
}

func TestAll_OrderedLowToHigh(t *testing.T) {
	got := ladder().All()
	want := []string{"masterwork", "masterpiece", "power-wrought"}
	if len(got) != len(want) {
		t.Fatalf("All len = %d, want %d", len(got), len(want))
	}
	for i, g := range got {
		if g.Key != want[i] {
			t.Errorf("All[%d] = %q, want %q", i, g.Key, want[i])
		}
	}
}

// The power-wrought grade records the unbreakable flag (masterwork §4) — an
// inert forward hook today (no durability system), carried by the grade.
func TestPowerWrought_RecordsUnbreakable(t *testing.T) {
	r := ladder()
	if g, ok := r.Get("power-wrought"); !ok || !g.Unbreakable {
		t.Errorf("power-wrought grade Unbreakable = %v,%v; want true", g.Unbreakable, ok)
	}
	if g, _ := r.Get("masterwork"); g.Unbreakable {
		t.Error("masterwork should not be unbreakable")
	}
}

func TestIsHigher(t *testing.T) {
	r := ladder()
	if !r.IsHigher("power-wrought", "masterwork") {
		t.Error("power-wrought should be finer than masterwork")
	}
	if r.IsHigher("masterwork", "masterpiece") {
		t.Error("masterwork should not be finer than masterpiece")
	}
	// An ungraded / unknown key is the floor.
	if !r.IsHigher("masterwork", "ungraded") {
		t.Error("any grade should be finer than an unknown/ungraded key")
	}
	if r.IsHigher("ungraded", "masterwork") {
		t.Error("an unknown key should never be finer than a real grade")
	}
}
