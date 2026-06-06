package light

import "testing"

// mutableSource is a Source that also supports SetProperty/DecrementInt,
// so it satisfies FuelSource for Burn tests.
type mutableSource map[string]any

func (m mutableSource) Property(key string) (any, bool) {
	v, ok := m[key]
	return v, ok
}

func (m mutableSource) SetProperty(key string, value any) { m[key] = value }

func (m mutableSource) DecrementInt(key string, amount int) (int, bool) {
	v, _ := m[key].(int)
	v -= amount
	hitZero := false
	if v <= 0 {
		v = 0
		hitZero = true
	}
	m[key] = v
	return v, hitZero
}

func TestDefaultFuelConfig(t *testing.T) {
	c := DefaultFuelConfig()
	if c.BurnAmount != 1 || c.BurnCadence != 300 {
		t.Fatalf("DefaultFuelConfig = %+v, want {1, 300}", c)
	}
}

func TestBurn_DecrementsLitFuelSource(t *testing.T) {
	src := mutableSource{PropItemLight: "gloom", PropItemLit: true, PropItemFuel: 5}
	rem, guttered, burned := Burn(src, 2)
	if rem != 3 || guttered || !burned {
		t.Fatalf("Burn = (%d,%v,%v), want (3,false,true)", rem, guttered, burned)
	}
	if !IsLit(src) {
		t.Fatal("source should still be lit after a partial burn")
	}
}

func TestBurn_GuttersAtZero(t *testing.T) {
	src := mutableSource{PropItemLight: "gloom", PropItemLit: true, PropItemFuel: 1}
	rem, guttered, burned := Burn(src, 1)
	if rem != 0 || !guttered || !burned {
		t.Fatalf("Burn = (%d,%v,%v), want (0,true,true)", rem, guttered, burned)
	}
	if IsLit(src) {
		t.Fatal("source should be unlit after guttering")
	}
}

func TestBurn_UnlitSourceUntouched(t *testing.T) {
	src := mutableSource{PropItemLight: "gloom", PropItemLit: false, PropItemFuel: 5}
	rem, guttered, burned := Burn(src, 1)
	if rem != 0 || guttered || burned {
		t.Fatalf("Burn on unlit = (%d,%v,%v), want (0,false,false)", rem, guttered, burned)
	}
	if fuel, _ := src.Property(PropItemFuel); fuel.(int) != 5 {
		t.Fatalf("unlit source fuel changed to %v, want 5", fuel)
	}
}

func TestBurn_PermanentSourceNeverGutters(t *testing.T) {
	// No fuel property → permanent. Lit, but Burn leaves it alone.
	src := mutableSource{PropItemLight: "dim", PropItemLit: true}
	rem, guttered, burned := Burn(src, 1)
	if rem != 0 || guttered || burned {
		t.Fatalf("Burn on permanent = (%d,%v,%v), want (0,false,false)", rem, guttered, burned)
	}
	if !IsLit(src) {
		t.Fatal("permanent source should stay lit")
	}
	if _, ok := src.Property(PropItemFuel); ok {
		t.Fatal("Burn must not create a fuel property on a permanent source")
	}
}

func TestBurn_NilSource(t *testing.T) {
	rem, guttered, burned := Burn(nil, 1)
	if rem != 0 || guttered || burned {
		t.Fatalf("Burn(nil) = (%d,%v,%v), want zeros", rem, guttered, burned)
	}
}
