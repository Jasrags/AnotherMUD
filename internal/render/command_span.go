package render

import "strings"

// maxCommandSpan bounds how much text a `…` backtick span may enclose before
// it is treated as literal prose rather than a command highlight. A suggested
// command is short — a verb plus a few tokens ("ask rook about") — so a long
// run between backticks is almost certainly incidental punctuation, not markup.
const maxCommandSpan = 40

// commandSpanEnd reports the index of the closing backtick of a command
// highlight that opens at s[i] (which must be a backtick), or -1 when the
// span is not a well-formed command reference. A command span is the
// content convention `verb` used across dialogue, quest text, help, and
// room prose (ui-rendering-help §2.6): the color renderer paints it in the
// `cmd` color and DimWrap leaves it bare so it pops against faint prose.
//
// The inner text must be non-empty, stay on one line, hold no other markup
// delimiter (backtick, '<', '{'), carry no raw ESC (0x1B), and not exceed
// maxCommandSpan. These rules keep the transform from swallowing ordinary
// prose that happens to contain a stray backtick, and from colliding with the
// tag/brace scanners.
//
// The ESC exclusion is a SECURITY guard, not cosmetics: the renderer copies a
// matched span verbatim in one WriteString (renderer.go), which is the only
// path that bulk-copies input rather than draining it byte-by-byte through the
// ESC-drop in renderTier. Rejecting a span that contains ESC forces such input
// back onto the per-byte path, where the smuggled ESC is stripped — closing the
// terminal-escape-injection hole for player-supplied text (say/tell) that might
// wrap an ESC in backticks.
func commandSpanEnd(s string, i int) int {
	if i < 0 || i >= len(s) || s[i] != '`' {
		return -1
	}
	rel := strings.IndexByte(s[i+1:], '`')
	if rel <= 0 { // no close, or empty ``
		return -1
	}
	end := i + 1 + rel
	inner := s[i+1 : end]
	if len(inner) > maxCommandSpan {
		return -1
	}
	if strings.ContainsAny(inner, "\n<{\x1b") {
		return -1
	}
	return end
}
