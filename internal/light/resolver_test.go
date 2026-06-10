package light

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/gameclock"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

type fixedPeriod string

func (f fixedPeriod) CurrentPeriod() string { return string(f) }

func roomWithLight(level string) *world.Room {
	r := &world.Room{ID: "r", Properties: map[string]any{}}
	if level != "" {
		r.Properties[PropRoomLight] = level
	}
	return r
}

func TestOverrideFor(t *testing.T) {
	if _, ok := OverrideFor(nil); ok {
		t.Fatal("OverrideFor(nil) ok=true, want false")
	}
	if _, ok := OverrideFor(roomWithLight("")); ok {
		t.Fatal("OverrideFor(no property) ok=true, want false")
	}
	if lvl, ok := OverrideFor(roomWithLight("dim")); !ok || lvl != Dim {
		t.Fatalf("OverrideFor(dim) = (%v,%v), want (Dim,true)", lvl, ok)
	}
	// Garbage value → treated as no override (fail safe, never a
	// silent black pin).
	if _, ok := OverrideFor(roomWithLight("nonsense")); ok {
		t.Fatal("OverrideFor(garbage) ok=true, want false")
	}
}

func TestResolver_Effective_OutdoorsDayVsNight(t *testing.T) {
	cfg := DefaultConfig()
	day := NewResolver(cfg, fixedPeriod(gameclock.PeriodDay))
	night := NewResolver(cfg, fixedPeriod(gameclock.PeriodNight))
	room := &world.Room{ID: "road", Terrain: world.TerrainOutdoors}

	if got := day.Effective(room, Black, Black); got != Lit {
		t.Fatalf("outdoors day = %v, want Lit", got)
	}
	if got := night.Effective(room, Black, Black); got != Gloom {
		t.Fatalf("outdoors night = %v, want Gloom", got)
	}
}

func TestResolver_Effective_UndergroundIsBlackThenLitBySource(t *testing.T) {
	cfg := DefaultConfig()
	r := NewResolver(cfg, fixedPeriod(gameclock.PeriodDay))
	room := &world.Room{ID: "cave", Terrain: world.TerrainUnderground}

	if got := r.Effective(room, Black, Black); got != Black {
		t.Fatalf("underground no source = %v, want Black", got)
	}
	if got := r.Effective(room, Dim, Black); got != Dim {
		t.Fatalf("underground with carried source = %v, want Dim", got)
	}
}

func TestResolver_Effective_RoomOverrideWins(t *testing.T) {
	cfg := DefaultConfig()
	r := NewResolver(cfg, fixedPeriod(gameclock.PeriodNight))
	// A lamp-lit street pinned dim at night.
	room := &world.Room{ID: "street", Terrain: world.TerrainOutdoors,
		Properties: map[string]any{PropRoomLight: "dim"}}
	if got := r.Effective(room, Black, Black); got != Dim {
		t.Fatalf("pinned-dim street at night = %v, want Dim", got)
	}
}

func TestFloorFor(t *testing.T) {
	if _, ok := FloorFor(nil); ok {
		t.Fatal("FloorFor(nil) ok=true, want false")
	}
	bare := &world.Room{ID: "r", Properties: map[string]any{}}
	if _, ok := FloorFor(bare); ok {
		t.Fatal("FloorFor(no property) ok=true, want false")
	}
	lit := &world.Room{ID: "r", Properties: map[string]any{PropRoomLightFloor: "dim"}}
	if lvl, ok := FloorFor(lit); !ok || lvl != Dim {
		t.Fatalf("FloorFor(dim) = (%v,%v), want (Dim,true)", lvl, ok)
	}
	// Garbage value → no floor (fail safe, like the pin path).
	junk := &world.Room{ID: "r", Properties: map[string]any{PropRoomLightFloor: "nonsense"}}
	if _, ok := FloorFor(junk); ok {
		t.Fatal("FloorFor(garbage) ok=true, want false")
	}
}

func TestResolver_Effective_FloorLiftsNightNotDay(t *testing.T) {
	cfg := DefaultConfig()
	// A lamp-lit village street: light_floor dim, outdoors.
	room := &world.Room{ID: "green", Terrain: world.TerrainOutdoors,
		Properties: map[string]any{PropRoomLightFloor: "dim"}}

	night := NewResolver(cfg, fixedPeriod(gameclock.PeriodNight))
	if got := night.Effective(room, Black, Black); got != Dim {
		t.Fatalf("floor-lit street at night = %v, want Dim (lifted from gloom)", got)
	}
	day := NewResolver(cfg, fixedPeriod(gameclock.PeriodDay))
	if got := day.Effective(room, Black, Black); got != Lit {
		t.Fatalf("floor-lit street at noon = %v, want Lit (floor must not cap daylight)", got)
	}
}

func TestResolver_Effective_TwoViewersDiffer(t *testing.T) {
	// Same room, same instant: a darkvision viewer and a human get
	// different effective light (§4 / §2 per-viewer).
	cfg := DefaultConfig()
	r := NewResolver(cfg, fixedPeriod(gameclock.PeriodDay))
	room := &world.Room{ID: "cave", Terrain: world.TerrainUnderground}

	human := r.Effective(room, Black, Black)
	dwarf := r.Effective(room, Black, cfg.DarkvisionViewerFloor(true))
	if human != Black {
		t.Fatalf("human in cave = %v, want Black", human)
	}
	if dwarf != Gloom {
		t.Fatalf("darkvision viewer in cave = %v, want Gloom", dwarf)
	}
}

func TestResolver_NilClockFloorsToGloomOutdoors(t *testing.T) {
	// A nil clock → period "" → ambient floored at Gloom; outdoors
	// passes it through.
	r := NewResolver(DefaultConfig(), nil)
	room := &world.Room{ID: "road", Terrain: world.TerrainOutdoors}
	if got := r.Effective(room, Black, Black); got != Gloom {
		t.Fatalf("nil clock outdoors = %v, want Gloom", got)
	}
}

func TestResolver_ConfigAccessor(t *testing.T) {
	cfg := DefaultConfig()
	r := NewResolver(cfg, nil)
	if r.Config().IndoorCap != cfg.IndoorCap {
		t.Fatal("Config() did not round-trip IndoorCap")
	}
}
