package command

import (
	"strings"

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

// blockWidth returns the widest visible column count across a multi-line
// markup block — used to measure the rendered minimap so the room column
// can claim the rest of the terminal.
func blockWidth(s string) int {
	max := 0
	for _, ln := range strings.Split(s, "\n") {
		if w := markupWidth(ln); w > max {
			max = w
		}
	}
	return max
}

// markupWidth returns the rendered column width of a markup line,
// discounting both <angle> semantic tags and {brace} color shorthand —
// each renders to zero visible width, so both must be removed for the
// side-by-side columns to align. Delegates to the render package, which
// owns the authoritative markup grammar (so a full color name like
// {yellow} is measured the same way the renderer collapses it).
func markupWidth(s string) int {
	return len(render.StripBraces(render.StripTags(s)))
}

// wrapMarkupLine word-wraps one line to width visible columns, keeping
// each word's markup attached. Width math discounts markup. A single
// word wider than width is left whole (no mid-word break).
func wrapMarkupLine(line string, width int) []string {
	if markupWidth(line) <= width {
		return []string{line}
	}
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur, curW := "", 0
	for _, w := range words {
		ww := markupWidth(w)
		switch {
		case cur == "":
			cur, curW = w, ww
		case curW+1+ww > width:
			lines = append(lines, cur)
			cur, curW = w, ww
		default:
			cur += " " + w
			curW += 1 + ww
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
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
	for _, ln := range strings.Split(left, "\n") {
		leftLines = append(leftLines, wrapMarkupLine(ln, leftWidth)...)
	}
	rightLines := strings.Split(right, "\n")

	rows := len(leftLines)
	if len(rightLines) > rows {
		rows = len(rightLines)
	}
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
