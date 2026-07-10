package pack

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// areaPropPack writes a minimal core pack with one area (given body) + a room.
func areaPropPack(t *testing.T, areaBody string) string {
	t.Helper()
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), areaBody)
	writeFile(t, filepath.Join(pack, "rooms/square.yaml"), `
id: square
area: town
name: The Square
description: stones
`)
	return root
}

// The area property bag round-trips: registered properties load and are readable
// through the typed accessors on world.Area.
func TestLoad_AreaPropertyBag_RoundTrips(t *testing.T) {
	root := areaPropPack(t, `
id: town
name: Town
properties:
  region: seattle
  security: AAA
  level_range: "1-10"
`)
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	area, err := regs.World.Area("tapestry-core:town")
	if err != nil {
		t.Fatalf("Area: %v", err)
	}
	for _, tc := range []struct{ key, want string }{
		{"region", "seattle"},
		{"security", "AAA"},
		{"level_range", "1-10"},
	} {
		if got, ok := area.PropertyString(tc.key); !ok || got != tc.want {
			t.Errorf("%s = %q (ok=%v), want %q", tc.key, got, ok, tc.want)
		}
	}
}

// An unregistered area property is a load error (same contract as rooms), naming
// the offending key so a content author can find it.
func TestLoad_AreaProperty_UnregisteredIsError(t *testing.T) {
	root := areaPropPack(t, `
id: town
name: Town
properties:
  not_a_real_property: nope
`)
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if err == nil {
		t.Fatal("expected a load error for an unregistered area property")
	}
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("error = %v, want ErrInvalidContent", err)
	}
	if !strings.Contains(err.Error(), "not_a_real_property") {
		t.Errorf("error %q should name the offending property", err)
	}
}

// A registered property authored with the wrong value type is a load error
// (security is a string; an int must bounce).
func TestLoad_AreaProperty_TypeMismatchIsError(t *testing.T) {
	root := areaPropPack(t, `
id: town
name: Town
properties:
  security: 5
`)
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if err == nil || !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("expected ErrInvalidContent for a type-mismatched area property, got %v", err)
	}
}
