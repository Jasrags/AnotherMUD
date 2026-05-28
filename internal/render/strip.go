package render

import "strings"

// StripTags drops angle-bracket markup (<…>) from s, returning the
// visible text. It understands only angle constructs — semantic tags
// and literal color tags both reduce to their inner content — and does
// NOT interpret brace shorthand or literal-color attributes (§6). Panel
// content is emitted with semantic tags only, so this is sufficient for
// width math; brace shorthand is reserved for ad-hoc messages.
//
// A '<' with no matching '>' is treated as opening an indefinite tag:
// everything from '<' to the end of the string is dropped. This errs
// toward a shorter visible length over emitting raw '<' characters.
//
// StripTags operates on pre-render markup, not on already-rendered ANSI.
// It does NOT strip SGR escape sequences, so VisibleLength of a string
// that already went through RenderAnsi would overcount. Compute widths
// from the tagged source (or RenderPlain), never from RenderAnsi output.
func StripTags(s string) string {
	if !strings.ContainsRune(s, '<') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '<' {
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				break // unterminated tag: drop rest
			}
			i += end + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// VisibleLength returns the number of visible bytes in s after stripping
// angle markup. Fast path: a string with no '<' has length len(s).
func VisibleLength(s string) int {
	if !strings.ContainsRune(s, '<') {
		return len(s)
	}
	return len(StripTags(s))
}
