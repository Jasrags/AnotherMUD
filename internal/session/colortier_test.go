package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/render"
)

// tierFakeConn extends fakeConn with a configurable ColorTier
// accessor so readColorTier picks up the per-test value.
type tierFakeConn struct {
	fakeConn
	tier render.ColorTier
}

func (t *tierFakeConn) ColorTier() render.ColorTier { return t.tier }

func TestReadColorTier_AccessorWins(t *testing.T) {
	cases := []struct {
		name string
		tier render.ColorTier
	}{
		{"none", render.ColorTierNone},
		{"basic", render.ColorTierBasic},
		{"extended", render.ColorTierExtended},
		{"truecolor", render.ColorTierTrueColor},
	}
	for _, c := range cases {
		fc := &tierFakeConn{
			fakeConn: fakeConn{id: "test-" + c.name},
			tier:     c.tier,
		}
		got := readColorTier(fc)
		if got != c.tier {
			t.Errorf("%s: readColorTier = %v, want %v", c.name, got, c.tier)
		}
	}
}

func TestReadColorTier_FakeConnDefaultsToBasic(t *testing.T) {
	// A conn that does NOT implement colorTierSource (the bare
	// fakeConn used by older session tests) falls back to Basic so
	// the M0-era ANSI-16 behavior is preserved.
	fc := &fakeConn{id: "legacy"}
	var c conn.Connection = fc
	if got := readColorTier(c); got != render.ColorTierBasic {
		t.Errorf("legacy fakeConn tier = %v, want Basic", got)
	}
}

func TestConnActor_ColorTier_CapturedFromConn(t *testing.T) {
	// The actor's colorTier is set once at construction; subsequent
	// changes to the conn's reported tier don't leak through (the
	// field is immutable post-construction by design).
	fc := &tierFakeConn{
		fakeConn: fakeConn{id: "actor-test"},
		tier:     render.ColorTierTrueColor,
	}
	a := &connActor{
		id:        fc.id,
		conn:      fc,
		colorTier: readColorTier(fc),
	}
	if got := a.ColorTier(); got != render.ColorTierTrueColor {
		t.Errorf("actor ColorTier = %v, want TrueColor", got)
	}

	// Even if the conn's tier "changes", the actor's frozen value
	// stays put.
	fc.tier = render.ColorTierNone
	if got := a.ColorTier(); got != render.ColorTierTrueColor {
		t.Errorf("actor ColorTier after conn change = %v, want TrueColor (frozen)", got)
	}
}
