package telnet

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/render"
)

func TestIsKnownMudClient(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"", false},
		{"xterm-256color", false},
		{"xterm", false},
		{"vt100", false},
		{"Mudlet", true},
		{"mudlet 4.18", true},
		{"MUDLET", true},
		{"MUSHclient", true},
		{"MUSHclient/5.10", true},
		{"TinTin++", true},
		{"tintin++/2.02.32", true},
		{"tintin", true}, // bare tintin also matches
		{"ZMud", true},
		{"CMud", true},
		{"Atlantis", true},
		{"Potato", true},
		{"BlowTorch", true},
		{"KildClient", true},
		{"BeIP", true},
		{"GnomeMUD", true},
		// Negatives
		{"alacritty", false},
		{"kitty", false},
		{"iTerm2", false},
	}
	for _, c := range cases {
		got := isKnownMudClient(c.name)
		if got != c.want {
			t.Errorf("isKnownMudClient(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDeriveColorTier(t *testing.T) {
	cases := []struct {
		name        string
		ttype       string
		isMudClient bool
		want        render.ColorTier
	}{
		{"no ttype", "", false, render.ColorTierNone},
		{"truecolor wins", "XTERM-TRUECOLOR", false, render.ColorTierTrueColor},
		{"truecolor lowercase", "xterm-truecolor", false, render.ColorTierTrueColor},
		{"truecolor over known-client", "Mudlet-TRUECOLOR", true, render.ColorTierTrueColor},
		{"256color hint", "xterm-256color", false, render.ColorTierExtended},
		{"256color in mid", "screen-256color-bce", false, render.ColorTierExtended},
		{"known mud client no hint", "Mudlet", true, render.ColorTierExtended},
		{"unknown ttype unknown client", "xterm", false, render.ColorTierBasic},
		{"vt100", "vt100", false, render.ColorTierBasic},
	}
	for _, c := range cases {
		got := deriveColorTier(c.ttype, c.isMudClient)
		if got != c.want {
			t.Errorf("%s: deriveColorTier(%q, %v) = %v, want %v",
				c.name, c.ttype, c.isMudClient, got, c.want)
		}
	}
}

func TestColorTier_String(t *testing.T) {
	cases := []struct {
		tier render.ColorTier
		want string
	}{
		{render.ColorTierNone, "none"},
		{render.ColorTierBasic, "basic"},
		{render.ColorTierExtended, "extended"},
		{render.ColorTierTrueColor, "truecolor"},
		{render.ColorTier(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.tier.String(); got != c.want {
			t.Errorf("%v.String() = %q, want %q", c.tier, got, c.want)
		}
	}
}
