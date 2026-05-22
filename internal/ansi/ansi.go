// Package ansi expands a small brace-markup syntax in pack-authored
// strings into ANSI-16 SGR escape sequences, or strips it for clients
// that have color disabled.
//
// Markup is the classic ROM/Smaug/CircleMUD style:
//
//	{k}{r}{g}{y}{b}{m}{c}{w}   normal foreground (SGR 30–37)
//	{K}{R}{G}{Y}{B}{M}{C}{W}   bright foreground (SGR 90–97)
//	{x}                        reset (SGR 0)
//	{{                         literal '{'
//
// Unknown codes (e.g. {z} or {ab}) pass through literally so authoring
// typos surface visibly instead of silently disappearing.
//
// This package owns ONLY the markup syntax and the SGR mapping. It
// does not look at terminal capabilities, env vars, or per-session
// state — callers decide whether to render colored or plain.
//
// Spec: ui-rendering-help (color subset for M2).
package ansi

import "strings"

// Reset is the SGR sequence that resets all attributes. Exposed so
// callers can append it explicitly when stitching output together.
const Reset = "\x1b[0m"

// codes maps the single-letter markup code to its SGR escape sequence.
// Uppercase letters are bright variants (90s); lowercase are normal (30s).
var codes = map[byte]string{
	'k': "\x1b[30m", 'K': "\x1b[90m",
	'r': "\x1b[31m", 'R': "\x1b[91m",
	'g': "\x1b[32m", 'G': "\x1b[92m",
	'y': "\x1b[33m", 'Y': "\x1b[93m",
	'b': "\x1b[34m", 'B': "\x1b[94m",
	'm': "\x1b[35m", 'M': "\x1b[95m",
	'c': "\x1b[36m", 'C': "\x1b[96m",
	'w': "\x1b[37m", 'W': "\x1b[97m",
	'x': Reset,
}

// Render expands markup in s. If enabled is true, codes are replaced
// by their SGR sequences; a trailing Reset is appended only when a
// color code was opened and not subsequently closed by an explicit
// {x}, so authors can write "{r}hi{x} done" without producing a
// redundant trailing reset. If enabled is false, every {code} is
// dropped from the output but the surrounding text is preserved
// verbatim.
//
// In BOTH modes, raw 0x1B (ESC) bytes already present in s are
// dropped. Color content is author-supplied and may come from
// untrusted packs; this is the single chokepoint that guarantees
// arbitrary SGR sequences cannot reach the wire by smuggling past
// the markup grammar. C0/C1 control characters other than tab,
// newline, and carriage return are passed through (the existing
// session log path sanitizes inbound; outbound text intentionally
// preserves \n / \r so multi-line room descriptions render).
//
// "{{" always renders as a literal "{" in both modes — authors need
// a way to write a brace without ambiguity.
func Render(s string, enabled bool) string {
	if enabled {
		return renderColor(s)
	}
	return strip(s)
}

func renderColor(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	// openColor tracks whether a color code was emitted that has not
	// yet been closed by a Reset. When the string ends with openColor
	// still true, we append a Reset so unclosed colors do not bleed
	// across writes; if the author already closed with {x}, no
	// trailing Reset is emitted.
	openColor := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1B {
			// Drop smuggled raw ESC bytes — see Render doc comment.
			continue
		}
		if c != '{' {
			b.WriteByte(c)
			continue
		}
		// {{ → literal '{'
		if i+1 < len(s) && s[i+1] == '{' {
			b.WriteByte('{')
			i++
			continue
		}
		// {X} where X is a recognized single byte code.
		if i+2 < len(s) && s[i+2] == '}' {
			if esc, ok := codes[s[i+1]]; ok {
				b.WriteString(esc)
				openColor = s[i+1] != 'x'
				i += 2
				continue
			}
		}
		// Anything else: emit the '{' literally so authors see their typo.
		b.WriteByte('{')
	}

	if openColor {
		b.WriteString(Reset)
	}
	return b.String()
}

func strip(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1B {
			// Drop smuggled raw ESC bytes — see Render doc comment.
			continue
		}
		if c != '{' {
			b.WriteByte(c)
			continue
		}
		if i+1 < len(s) && s[i+1] == '{' {
			b.WriteByte('{')
			i++
			continue
		}
		if i+2 < len(s) && s[i+2] == '}' {
			if _, ok := codes[s[i+1]]; ok {
				i += 2
				continue
			}
		}
		b.WriteByte('{')
	}
	return b.String()
}
