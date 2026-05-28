// Package render is the M10 UI rendering pipeline: a content-driven
// theme registry, a color renderer that translates markup into ANSI
// SGR sequences (or strips it for color-disabled clients), and a tag
// stripper used for width-aware layout.
//
// It supersedes the minimal M2 internal/ansi renderer (single-letter
// brace codes only) while keeping that vocabulary working for
// back-compat: the brace shorthand here recognizes both the classic
// ROM single-letter codes ({r}, {x}, …) and the spec's full color
// names ({yellow}, {bright_red}, {bold}, {/}, …).
//
// Security: like internal/ansi, every render path drops raw 0x1B (ESC)
// bytes present in the input. Color content is author-supplied and may
// come from untrusted packs; this is the chokepoint that guarantees
// arbitrary SGR sequences cannot reach the wire by smuggling past the
// markup grammar. C0/C1 control characters other than \t, \n, \r pass
// through (multi-line room descriptions must keep their newlines).
//
// Spec: ui-rendering-help §2-§6.
package render

import "strings"

// Reset is the SGR sequence that resets all attributes.
const Reset = "\x1b[0m"

// foreground SGR codes by canonical color name. Bright variants use the
// 90-series. The canonical key is lower-case with both '-' and '_'
// normalized to '-' (see normalizeColor) so the spec's historical split
// — literal tags use "bright-red", brace shorthand uses "bright_red"
// (§2.3) — both resolve here. Accepting both forms everywhere is the
// lenient direction the spec's open question (§13) leans toward.
var fgCodes = map[string]string{
	"black": "\x1b[30m", "red": "\x1b[31m", "green": "\x1b[32m",
	"yellow": "\x1b[33m", "blue": "\x1b[34m", "magenta": "\x1b[35m",
	"cyan": "\x1b[36m", "white": "\x1b[37m",
	"bright-black": "\x1b[90m", "dark-gray": "\x1b[90m",
	"bright-red": "\x1b[91m", "bright-green": "\x1b[92m",
	"bright-yellow": "\x1b[93m", "bright-blue": "\x1b[94m",
	"bright-magenta": "\x1b[95m", "bright-cyan": "\x1b[96m",
	"bright-white": "\x1b[97m",
}

// background SGR codes by canonical color name (40-series, bright
// 100-series).
var bgCodes = map[string]string{
	"black": "\x1b[40m", "red": "\x1b[41m", "green": "\x1b[42m",
	"yellow": "\x1b[43m", "blue": "\x1b[44m", "magenta": "\x1b[45m",
	"cyan": "\x1b[46m", "white": "\x1b[47m",
	"bright-black": "\x1b[100m", "dark-gray": "\x1b[100m",
	"bright-red": "\x1b[101m", "bright-green": "\x1b[102m",
	"bright-yellow": "\x1b[103m", "bright-blue": "\x1b[104m",
	"bright-magenta": "\x1b[105m", "bright-cyan": "\x1b[106m",
	"bright-white": "\x1b[107m",
}

// romCodes maps the classic M2 single-letter brace codes to SGR so
// existing pack strings ({r}hi{x}) keep rendering. Lowercase = normal
// (30s), uppercase = bright (90s), x = reset.
var romCodes = map[string]string{
	"k": "\x1b[30m", "K": "\x1b[90m",
	"r": "\x1b[31m", "R": "\x1b[91m",
	"g": "\x1b[32m", "G": "\x1b[92m",
	"y": "\x1b[33m", "Y": "\x1b[93m",
	"b": "\x1b[34m", "B": "\x1b[94m",
	"m": "\x1b[35m", "M": "\x1b[95m",
	"c": "\x1b[36m", "C": "\x1b[96m",
	"w": "\x1b[37m", "W": "\x1b[97m",
	"x": Reset,
}

// normalizeColor lower-cases a color name and collapses '_' to '-' so
// both the hyphen (literal-tag) and underscore (brace) spellings of
// bright variants resolve to one key.
func normalizeColor(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), "_", "-")
}

// ResolveFgColor maps a color name (§2.3) to its foreground SGR code
// without consulting the theme. Returns "" for an unrecognized name.
// Static so features can emit raw color outside the renderer's cache.
func ResolveFgColor(name string) string {
	return fgCodes[normalizeColor(name)]
}

// ResolveBgColor maps a color name to its background SGR code without
// consulting the theme. Returns "" for an unrecognized name.
func ResolveBgColor(name string) string {
	return bgCodes[normalizeColor(name)]
}

// resolveBrace maps a brace-shorthand token to its SGR code. It accepts,
// in order: the special attributes bold/dim/reset/"/", the classic
// single-letter ROM codes (case-sensitive), then full color names
// (case-insensitive). Returns (code, true) on a match. The returned
// isReset reports whether the token closes color (so the renderer can
// track unterminated color for trailing-reset cleanup).
func resolveBrace(token string) (code string, isReset bool, ok bool) {
	switch strings.ToLower(token) {
	case "reset", "/":
		return Reset, true, true
	case "bold":
		return "\x1b[1m", false, true
	case "dim":
		return "\x1b[2m", false, true
	}
	// ROM single-letter codes are case-sensitive (r vs R).
	if c, found := romCodes[token]; found {
		return c, token == "x", true
	}
	if c := fgCodes[normalizeColor(token)]; c != "" {
		return c, false, true
	}
	return "", false, false
}
