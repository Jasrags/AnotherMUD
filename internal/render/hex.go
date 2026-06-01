package render

import (
	"strconv"
	"strings"
)

// parseHex parses a "#RRGGBB" or "RRGGBB" string into 0..255 RGB
// components. Returns ok=false for any other shape — short forms
// (#RGB), rgba(), and named colors are NOT accepted; the theme
// loader is the validation chokepoint and HTML strings in
// ThemeEntry are expected to be the canonical six-digit hex form.
func parseHex(hex string) (r, g, b int, ok bool) {
	s := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(n>>16) & 0xFF, int(n>>8) & 0xFF, int(n) & 0xFF, true
}

// hexToTrueColorSGR returns "\x1b[38;2;R;G;Bm" (fg) or
// "\x1b[48;2;R;G;Bm" (bg) for a "#RRGGBB" hex string. Returns ""
// for an unparseable hex — the caller falls back to ANSI-16.
func hexToTrueColorSGR(hex string, isBg bool) string {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return ""
	}
	prefix := "38"
	if isBg {
		prefix = "48"
	}
	var sb strings.Builder
	sb.WriteString("\x1b[")
	sb.WriteString(prefix)
	sb.WriteString(";2;")
	sb.WriteString(strconv.Itoa(r))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(g))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(b))
	sb.WriteByte('m')
	return sb.String()
}

// hexTo256SGR returns "\x1b[38;5;Nm" (fg) or "\x1b[48;5;Nm" (bg)
// for the nearest xterm-256 palette index. Returns "" for an
// unparseable hex.
//
// Mapping (xterm-256):
//   - For (r==g==b) values clearly on the grayscale ramp, use
//     the 24-step grayscale palette (indices 232-255).
//   - Otherwise, map each channel to the nearest of the 6-level
//     cube {0,95,135,175,215,255} and compute index =
//     16 + 36*r + 6*g + b (indices 16-231).
func hexTo256SGR(hex string, isBg bool) string {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return ""
	}
	n := nearestXterm256(r, g, b)
	prefix := "38"
	if isBg {
		prefix = "48"
	}
	var sb strings.Builder
	sb.WriteString("\x1b[")
	sb.WriteString(prefix)
	sb.WriteString(";5;")
	sb.WriteString(strconv.Itoa(n))
	sb.WriteByte('m')
	return sb.String()
}

// cubeLevels are the 6 RGB intensity steps the xterm-256 cube
// quantizes to. Each channel maps to its nearest level by
// absolute distance.
var cubeLevels = [6]int{0, 95, 135, 175, 215, 255}

// nearestCubeLevel returns the cube-level index (0..5) closest
// to v.
func nearestCubeLevel(v int) int {
	bestIdx := 0
	bestDist := 1 << 30
	for i, lvl := range cubeLevels {
		d := v - lvl
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	return bestIdx
}

// nearestXterm256 returns the palette index in the 256-color
// space closest to the input RGB.
//
// True grayscale (r==g==b) takes the grayscale-ramp path
// because that ramp is denser than the cube along the diagonal
// — 24 steps vs 6, so the perceptual approximation is much
// closer.
func nearestXterm256(r, g, b int) int {
	if r == g && g == b {
		// Grayscale ramp (232..255) covers v ∈ [8, 238] at 10-step
		// intervals. Clamp the ends so very-dark and very-light
		// hit black (16) and white (231) on the cube instead.
		if r < 8 {
			return 16 // cube black
		}
		// Valid grayscale-ramp indices are 232..255 (24 entries, gray
		// values 8..238 at 10-step intervals). For v >= 248 the formula
		// `232 + (v-8)/10` would compute 256, an out-of-range SGR
		// index; clamp to cube white (231) at the boundary.
		if r >= 248 {
			return 231 // cube white
		}
		return 232 + (r-8)/10
	}
	ri := nearestCubeLevel(r)
	gi := nearestCubeLevel(g)
	bi := nearestCubeLevel(b)
	return 16 + 36*ri + 6*gi + bi
}
