package render

import (
	"strings"
	"sync"
)

// ColorRenderer translates markup into ANSI (RenderAnsi) or strips it
// to plain text (RenderPlain). Both modes share one structural scanner
// so the plain output is exactly the colored output with formatting
// removed (§4.1). Each mode memoizes input→output; the caches grow
// monotonically for the process lifetime (§4.2, no eviction) and are
// safe for concurrent reads/writes across sessions.
//
// A renderer is bound to a compiled ThemeRegistry. The theme should be
// compiled before the renderer serves traffic; entries added after are
// not visible until the next Compile (and stale cache entries for
// affected strings would persist — acceptable because themes are a
// boot-time concern).
type ColorRenderer struct {
	theme      *ThemeRegistry
	ansiCache  sync.Map // string → string
	plainCache sync.Map // string → string
}

// NewColorRenderer binds a renderer to a theme registry.
func NewColorRenderer(theme *ThemeRegistry) *ColorRenderer {
	return &ColorRenderer{theme: theme}
}

// RenderAnsi expands markup in s into ANSI SGR sequences.
func (r *ColorRenderer) RenderAnsi(s string) string {
	if v, ok := r.ansiCache.Load(s); ok {
		return v.(string)
	}
	out := r.render(s, true)
	r.ansiCache.Store(s, out)
	return out
}

// RenderPlain strips all markup from s, leaving visible text.
func (r *ColorRenderer) RenderPlain(s string) string {
	if v, ok := r.plainCache.Load(s); ok {
		return v.(string)
	}
	out := r.render(s, false)
	r.plainCache.Store(s, out)
	return out
}

// render is the single-pass scanner. ansi selects whether structural
// constructs emit SGR codes or nothing; in both modes the visible text
// is identical. Raw 0x1B bytes are dropped in both modes (security
// chokepoint — see package doc).
func (r *ColorRenderer) render(s string, ansi bool) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	openColor := false // an unreset color code was emitted (ansi mode)

	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == 0x1B:
			i++ // drop smuggled ESC
		case c == '<':
			consumed, opened := r.scanAngle(s, i, ansi, &b)
			if consumed == 0 {
				b.WriteByte('<') // unmatched '<' → literal, rescan rest
				i++
				continue
			}
			if ansi && opened != openNone {
				openColor = opened == openOpen
			}
			i += consumed
		case c == '{':
			consumed, opened := r.scanBrace(s, i, ansi, &b)
			if consumed == 0 {
				b.WriteByte('{')
				i++
				continue
			}
			if ansi && opened != openNone {
				openColor = opened == openOpen
			}
			i += consumed
		default:
			b.WriteByte(c)
			i++
		}
	}

	// Append a reset if color was opened and never closed, so styling
	// does not bleed into the next write. Mirrors internal/ansi.
	if ansi && openColor {
		b.WriteString(Reset)
	}
	return b.String()
}

// openState reports what a construct did to the color stream so the
// caller can track unterminated color for trailing-reset cleanup.
type openState int

const (
	openNone  openState = iota // emitted nothing color-affecting
	openOpen                   // opened a color that should later reset
	openClose                  // emitted a reset / close
)

// scanAngle handles a '<' at s[i]. Returns the number of bytes consumed
// (0 means "not a recognized construct; caller emits '<' literally and
// advances one") and the open-state effect. Recognizes, in order:
// closing tags (</name>), literal color (<color …>), and semantic tags
// (<name>). Unknown opening tags consume 0 so they pass through (§2.5).
func (r *ColorRenderer) scanAngle(s string, i int, ansi bool, b *strings.Builder) (int, openState) {
	end := strings.IndexByte(s[i:], '>')
	if end < 0 {
		return 0, openNone // unmatched '<'
	}
	end += i
	inner := s[i+1 : end] // between '<' and '>'
	total := end - i + 1  // includes '<' and '>'

	if strings.HasPrefix(inner, "/") {
		name := strings.ToLower(strings.TrimSpace(inner[1:]))
		if name == "color" || r.theme.IsKnown(name) {
			// Consume the close. Emit a reset only when the matching
			// open actually produced color: <color> always opens, and a
			// theme tag only when it resolves. A declared-but-color-less
			// tag is a pure passthrough wrapper, so its close emits
			// nothing (no stray reset around plain content).
			if ansi {
				_, hasColor := r.theme.Resolve(name)
				if name == "color" || hasColor {
					b.WriteString(Reset)
					return total, openClose
				}
			}
			return total, openNone
		}
		return 0, openNone // unknown closing tag passes through
	}

	// Opening tag. The tag name is the first whitespace-delimited token.
	name := inner
	if sp := strings.IndexByte(inner, ' '); sp >= 0 {
		name = inner[:sp]
	}
	lname := strings.ToLower(name)

	if lname == "color" {
		open := ""
		if ansi {
			open = ResolveFgColor(attrValue(inner, "fg")) + ResolveBgColor(attrValue(inner, "bg"))
			b.WriteString(open)
		}
		if open != "" {
			return total, openOpen
		}
		return total, openNone
	}

	if r.theme.IsKnown(lname) {
		if ansi {
			if pair, ok := r.theme.Resolve(lname); ok {
				b.WriteString(pair.Open)
				return total, openOpen
			}
		}
		return total, openNone // declared-but-color-less → emit content plain
	}

	return 0, openNone // unknown opening tag passes through (§2.5)
}

// scanBrace handles a '{' at s[i]. Returns bytes consumed (0 = not a
// recognized token; caller emits '{' literally) and the open-state.
func (r *ColorRenderer) scanBrace(s string, i int, ansi bool, b *strings.Builder) (int, openState) {
	// {{ → literal '{'
	if i+1 < len(s) && s[i+1] == '{' {
		b.WriteByte('{')
		return 2, openNone
	}
	end := strings.IndexByte(s[i:], '}')
	if end < 0 {
		return 0, openNone
	}
	end += i
	token := s[i+1 : end]
	code, isReset, ok := resolveBrace(token)
	if !ok {
		return 0, openNone // unknown brace token passes through (§13)
	}
	if ansi {
		b.WriteString(code)
		if isReset {
			return end - i + 1, openClose
		}
		return end - i + 1, openOpen
	}
	return end - i + 1, openNone
}

// attrValue extracts a quoted attribute value from a tag's inner text,
// e.g. attrValue(`color fg="red" bg="black"`, "fg") == "red". Regex-free,
// single-character quotes only (' or "). Returns "" when absent.
//
// The key match is anchored to a word boundary: the "key=" token must
// sit at the start of inner or be preceded by whitespace, so a longer
// attribute name ending in the key (e.g. "nofg=") does not match "fg=".
func attrValue(inner, key string) string {
	lower := strings.ToLower(inner)
	needle := strings.ToLower(key) + "="
	idx := -1
	for from := 0; ; {
		hit := strings.Index(lower[from:], needle)
		if hit < 0 {
			break
		}
		at := from + hit
		if at == 0 || lower[at-1] == ' ' {
			idx = at
			break
		}
		from = at + len(needle)
	}
	if idx < 0 {
		return ""
	}
	rest := inner[idx+len(key)+1:]
	if rest == "" {
		return ""
	}
	q := rest[0]
	if q != '"' && q != '\'' {
		return ""
	}
	close := strings.IndexByte(rest[1:], q)
	if close < 0 {
		return ""
	}
	return rest[1 : 1+close]
}
