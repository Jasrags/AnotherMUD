package command

import (
	"strings"
	"unicode/utf8"

	"github.com/Jasrags/AnotherMUD/internal/render"
)

// defaultRoomColumnWidth is the fallback left-column width the room view
// wraps to when the active minimap renders beside it (player-maps §4,
// §10 policy) and the client's terminal width is unknown (no NAWS) —
// kept narrow so the room column + a gap + the minimap still fit a
// standard 80-column terminal.
const defaultRoomColumnWidth = 50

// maxRoomColumnWidth caps the room column on wide terminals: prose stays
// readable at roughly this many columns, so a 200-column window widens
// the description toward this ceiling instead of sprawling edge to edge.
const maxRoomColumnWidth = 80

// minimapGap is the blank columns between the room column and the
// minimap (player-maps §10 policy).
const minimapGap = 3

// roomColumnWidth picks the left-column width for the side-by-side room
// view. With an unknown terminal width (termWidth <= 0) it keeps the
// narrow default. Otherwise it gives the room body all the space the
// minimap and gap leave, clamped into [defaultRoomColumnWidth,
// maxRoomColumnWidth] so it neither crowds nor sprawls.
func roomColumnWidth(termWidth, mapWidth int) int {
	if termWidth <= 0 {
		return defaultRoomColumnWidth
	}
	avail := termWidth - mapWidth - minimapGap
	switch {
	case avail < defaultRoomColumnWidth:
		return defaultRoomColumnWidth
	case avail > maxRoomColumnWidth:
		return maxRoomColumnWidth
	default:
		return avail
	}
}

// markupWidth returns the rendered column width of a markup line,
// discounting both <angle> semantic tags and {brace} color shorthand —
// each renders to zero visible width, so both must be removed for the
// side-by-side columns to align. Delegates to the render package, which
// owns the authoritative markup grammar (so a full color name like
// {yellow} is measured the same way the renderer collapses it).
//
// Counts RUNES, not bytes: room prose carries multi-byte glyphs (the
// em-dash `—`, the `→` in the way-back note), and a byte count would
// over-measure them — under-padding the left column and shifting the
// minimap border for that one row. Rune count equals visible columns for
// the Latin + punctuation content here (it does not handle double-width
// CJK, which the content does not use).
func markupWidth(s string) int {
	return utf8.RuneCountInString(render.StripBraces(render.StripTags(s)))
}

// markupReset is the reset token appended to close a wrapped line whose color
// run continues onto the next line, so every wrapped line is self-contained.
const markupReset = "{x}"

// wrapMarkupLine word-wraps one line to width visible columns, keeping each
// word's markup attached. Width math discounts markup. A single word wider than
// width is left whole (no mid-word break).
//
// The wrap is COLOR-RUN AWARE: when a break falls inside an open color run (a
// `{code}`/`<tag>` not yet closed), the current line is closed with a reset and
// the continuation line RE-OPENS the active markup. Without this, a name or a
// description wrapped across lines loses its color on every line after the
// first — visibly so under the side-by-side minimap (joinBeside appends a reset
// per line) and in any per-line-framed client. Re-opening makes each wrapped
// line stand on its own, correct regardless of how it's later concatenated.
func wrapMarkupLine(line string, width int) []string {
	if markupWidth(line) <= width {
		return []string{line}
	}
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	var open []string // active (unclosed) color markers, in emit order
	cur, curW := "", 0

	// flush appends the current line, closing an open color run so the line is
	// self-contained (harmless when nothing is open).
	flush := func() {
		if cur == "" {
			return
		}
		if len(open) > 0 {
			cur += markupReset
		}
		lines = append(lines, cur)
	}
	// start begins a new line with the active color markers re-emitted (zero
	// visible width) so a run split across the break keeps its color.
	start := func(word string, ww int) {
		cur = strings.Join(open, "") + word
		curW = ww
	}

	for _, w := range words {
		ww := markupWidth(w)
		switch {
		case cur == "":
			start(w, ww)
		case curW+1+ww > width:
			flush()
			start(w, ww)
		default:
			cur += " " + w
			curW += 1 + ww
		}
		trackOpenMarkup(w, &open) // update AFTER placing the word
	}
	flush()
	return lines
}

// trackOpenMarkup updates the stack of active (unclosed) color markers by
// scanning one word's markup tokens: a `{code}` or opening `<tag>` pushes, a
// closing `</tag>` pops, and the `{x}` reset clears all (it is a full SGR reset,
// matching the renderer). Only color state is tracked — the markers are re-
// emitted verbatim on a continuation line, so the renderer reproduces the state.
func trackOpenMarkup(word string, open *[]string) {
	for i := 0; i < len(word); {
		switch word[i] {
		case '{':
			j := strings.IndexByte(word[i:], '}')
			if j < 0 {
				return
			}
			tok := word[i : i+j+1]
			if strings.EqualFold(tok, markupReset) {
				*open = (*open)[:0] // reset clears everything
			} else {
				*open = append(*open, tok)
			}
			i += j + 1
		case '<':
			j := strings.IndexByte(word[i:], '>')
			if j < 0 {
				return
			}
			tok := word[i : i+j+1]
			if strings.HasPrefix(tok, "</") {
				if len(*open) > 0 {
					*open = (*open)[:len(*open)-1]
				}
			} else {
				*open = append(*open, tok)
			}
			i += j + 1
		default:
			i++
		}
	}
}

// padRight pads s with spaces to width visible columns (no truncation;
// callers wrap first).
func padRight(s string, width int) string {
	if pad := width - markupWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// joinBeside renders right to the right of left, top-aligned: left is
// wrapped to leftWidth and padded so right starts at a fixed column, with
// a `{x}` reset at the boundary so any open color from the left text
// can't bleed into the right block. A room-only row (no right content)
// is emitted left-trimmed; a map-only row keeps the left column blank so
// the map stays in its column.
func joinBeside(left, right string, leftWidth, gap int) string {
	var leftLines []string
	for ln := range strings.SplitSeq(left, "\n") {
		leftLines = append(leftLines, wrapMarkupLine(ln, leftWidth)...)
	}
	rightLines := strings.Split(right, "\n")

	rows := max(len(rightLines), len(leftLines))
	gapStr := strings.Repeat(" ", gap)
	var b strings.Builder
	for i := 0; i < rows; i++ {
		l, r := "", ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		if r == "" {
			b.WriteString(strings.TrimRight(l, " "))
		} else {
			b.WriteString(padRight(l, leftWidth))
			b.WriteString("{x}") // reset so left color can't bleed into the map
			b.WriteString(gapStr)
			b.WriteString(r)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
