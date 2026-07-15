package light

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/gameclock"
)

func TestDefaultConfig_AmbientByPeriod(t *testing.T) {
	c := DefaultConfig()
	cases := map[string]Level{
		gameclock.PeriodDay:   Lit,
		gameclock.PeriodDawn:  Dim,
		gameclock.PeriodDusk:  Dim,
		gameclock.PeriodNight: Gloom,
	}
	for period, want := range cases {
		if got := c.AmbientFor(period); got != want {
			t.Fatalf("AmbientFor(%q) = %v, want %v", period, got, want)
		}
	}
}

func TestAmbientFor_NeverBlack(t *testing.T) {
	// Unknown period and an explicitly-misconfigured below-gloom entry
	// both floor at Gloom — ambient is never black (§2.2).
	c := DefaultConfig()
	if got := c.AmbientFor("eclipse"); got != Gloom {
		t.Fatalf("AmbientFor(unknown) = %v, want Gloom", got)
	}
	c.AmbientByPeriod[gameclock.PeriodNight] = Black
	if got := c.AmbientFor(gameclock.PeriodNight); got != Gloom {
		t.Fatalf("AmbientFor with black-configured night = %v, want Gloom floor", got)
	}
}

func TestAmbientFor_EmptyPeriodFloors(t *testing.T) {
	// A nil clock resolves period to "" — must still floor at Gloom.
	c := DefaultConfig()
	if got := c.AmbientFor(""); got != Gloom {
		t.Fatalf("AmbientFor(\"\") = %v, want Gloom", got)
	}
}

func TestHitPenalty(t *testing.T) {
	c := DefaultConfig()
	cases := map[Level]int{Lit: 0, Dim: 1, Gloom: 2, Black: 4}
	for lvl, want := range cases {
		if got := c.HitPenalty(lvl); got != want {
			t.Fatalf("HitPenalty(%v) = %d, want %d", lvl, got, want)
		}
	}
	// Nil table → no penalty.
	empty := Config{}
	if got := empty.HitPenalty(Black); got != 0 {
		t.Fatalf("HitPenalty with nil table = %d, want 0", got)
	}
	// A negative configured value is clamped to 0 (never a to-hit bonus
	// from darkness).
	c.CombatHitPenalty[Lit] = -5
	if got := c.HitPenalty(Lit); got != 0 {
		t.Fatalf("HitPenalty with negative entry = %d, want 0", got)
	}
}

func TestDarkvisionViewerFloor(t *testing.T) {
	c := DefaultConfig()
	if got := c.DarkvisionViewerFloor(false); got != Black {
		t.Fatalf("no darkvision floor = %v, want Black", got)
	}
	if got := c.DarkvisionViewerFloor(true); got != Gloom {
		t.Fatalf("darkvision floor = %v, want Gloom", got)
	}
}

func TestDefaultConfig_VisionModeFloors(t *testing.T) {
	c := DefaultConfig()
	// Thermographic is an unconditional see-in-the-dark floor at Gloom,
	// combined through ViewerFloor's effect-flag path.
	if got := c.ViewerFloor(false, []string{ThermographicFlag}); got != Gloom {
		t.Fatalf("thermographic floor = %v, want Gloom", got)
	}
	// Low-light names its target level (Dim); the conditional lift lives at
	// the call site, this field only supplies the level.
	if c.LowLightFloor != Dim {
		t.Fatalf("LowLightFloor = %v, want Dim", c.LowLightFloor)
	}
}

func TestDarkvisionViewerFloor_CapBoundsFloor(t *testing.T) {
	// A floor configured above the cap is bounded down to the cap —
	// darkvision is never daylight (§4).
	c := DefaultConfig()
	c.DarkvisionFloor = Lit
	c.DarkvisionCap = Gloom
	if got := c.DarkvisionViewerFloor(true); got != Gloom {
		t.Fatalf("floor above cap = %v, want Gloom (capped)", got)
	}
}
