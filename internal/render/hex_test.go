package render

import "testing"

func TestParseHex(t *testing.T) {
	cases := []struct {
		in      string
		r, g, b int
		ok      bool
	}{
		{"#FF0000", 255, 0, 0, true},
		{"ff0000", 255, 0, 0, true},
		{"  #00FF00  ", 0, 255, 0, true},
		{"#0000FF", 0, 0, 255, true},
		{"#FFFFFF", 255, 255, 255, true},
		{"#000000", 0, 0, 0, true},
		{"#12AbCd", 0x12, 0xAB, 0xCD, true},
		// Negatives
		{"", 0, 0, 0, false},
		{"#FFF", 0, 0, 0, false}, // short form not accepted
		{"#GGGGGG", 0, 0, 0, false},
		{"red", 0, 0, 0, false},
		{"#1234567", 0, 0, 0, false},
	}
	for _, c := range cases {
		r, g, b, ok := parseHex(c.in)
		if ok != c.ok || r != c.r || g != c.g || b != c.b {
			t.Errorf("parseHex(%q) = (%d,%d,%d,%v), want (%d,%d,%d,%v)",
				c.in, r, g, b, ok, c.r, c.g, c.b, c.ok)
		}
	}
}

func TestHexToTrueColorSGR(t *testing.T) {
	cases := []struct {
		hex  string
		isBg bool
		want string
	}{
		{"#FF0000", false, "\x1b[38;2;255;0;0m"},
		{"#FF0000", true, "\x1b[48;2;255;0;0m"},
		{"#00FF80", false, "\x1b[38;2;0;255;128m"},
		{"#000000", false, "\x1b[38;2;0;0;0m"},
		{"bogus", false, ""},
		{"", false, ""},
	}
	for _, c := range cases {
		got := hexToTrueColorSGR(c.hex, c.isBg)
		if got != c.want {
			t.Errorf("hexToTrueColorSGR(%q, %v) = %q, want %q", c.hex, c.isBg, got, c.want)
		}
	}
}

func TestNearestXterm256(t *testing.T) {
	cases := []struct {
		r, g, b int
		want    int
		desc    string
	}{
		// Cube corners — easy to verify by hand.
		{0, 0, 0, 16, "pure black → cube black"},
		{255, 0, 0, 196, "pure red → 16+36*5"},
		{0, 255, 0, 46, "pure green → 16+6*5"},
		{0, 0, 255, 21, "pure blue → 16+5"},
		// Grayscale ramp path — (r==g==b).
		{8, 8, 8, 232, "grayscale ramp start"},
		{128, 128, 128, 244, "grayscale mid (128-8)/10 = 12 → 232+12 = 244"},
		// Clamped grays.
		{0, 0, 0, 16, "pure black clamps to cube"},
		{255, 255, 255, 231, "near-white clamps high → cube white"},
		// Off-axis quantization.
		{120, 120, 120, 243, "exact gray uses ramp ((120-8)/10 = 11 → 243)"},
		// Regression: grayscale-ramp boundary. The formula
		// 232+(v-8)/10 overflows past index 255 for v >= 248;
		// these values must clamp to cube white (231) to stay
		// within the valid 256-color SGR range.
		{247, 247, 247, 255, "v=247 → last grayscale-ramp index (255)"},
		{248, 248, 248, 231, "v=248 → cube white (boundary clamp)"},
		{250, 250, 250, 231, "v=250 → cube white (clamped)"},
		{254, 254, 254, 231, "v=254 → cube white (clamped)"},
	}
	for _, c := range cases {
		got := nearestXterm256(c.r, c.g, c.b)
		if got != c.want {
			t.Errorf("%s: nearestXterm256(%d,%d,%d) = %d, want %d",
				c.desc, c.r, c.g, c.b, got, c.want)
		}
	}
}

func TestHexTo256SGR(t *testing.T) {
	cases := []struct {
		hex  string
		isBg bool
		want string
	}{
		{"#FF0000", false, "\x1b[38;5;196m"}, // pure red
		{"#FF0000", true, "\x1b[48;5;196m"},  // pure red bg
		{"#000000", false, "\x1b[38;5;16m"},  // black
		{"#FFFFFF", false, "\x1b[38;5;231m"}, // near-white clamps
		{"bad", false, ""},
		{"", false, ""},
	}
	for _, c := range cases {
		got := hexTo256SGR(c.hex, c.isBg)
		if got != c.want {
			t.Errorf("hexTo256SGR(%q, %v) = %q, want %q", c.hex, c.isBg, got, c.want)
		}
	}
}
