package render

import (
	"errors"
	"strings"
)

// Panel rendering (ui-rendering-help §8). A Panel is a structured value
// the renderer turns into a framed, width-aware multi-line string. Width
// math uses VisibleLength so a cell with embedded color tags lines up
// with a plain one beside it. The output uses \r\n line endings.

// DefaultPanelWidth is the visible width of a panel whose Width is unset.
const DefaultPanelWidth = 80

// ErrPanelTitleOverflow is returned by Panel.Render when a title row's
// right side alone exceeds the inner width — an authoring error the
// renderer refuses rather than silently mangling (§8.3).
var ErrPanelTitleOverflow = errors.New("render: panel title right side exceeds inner width")

// RuleStyle is the horizontal separator drawn above a section.
type RuleStyle int

const (
	RuleNone  RuleStyle = iota // no rule
	RuleMinor                  // light separator (---)
	RuleMajor                  // strong separator (===)
)

// Align controls horizontal placement of content within its width.
type Align int

const (
	AlignLeft Align = iota
	AlignRight
	AlignCenter
)

// Cell is one horizontal unit in a CellRow. Width is Fixed(Width) unless
// Fill is set, in which case the cell consumes an equal share of the
// row's leftover width. A progress cell renders a value/max bar across
// its width instead of Content.
type Cell struct {
	Content  string
	Width    int // fixed visible width; ignored when Fill is true
	Fill     bool
	Align    Align
	Progress bool
	Value    int
	Max      int
}

type rowKind int

const (
	kindEmpty rowKind = iota
	kindTitle
	kindText
	kindCells
	kindFooter
)

// Row is one entry in a Section. Construct rows with the EmptyRow /
// TitleRow / TextRow / CellRow / FooterRow helpers.
type Row struct {
	kind     rowKind
	left     string
	right    string
	content  string
	align    Align
	wrap     bool
	cells    []Cell
	dividers bool
}

// EmptyRow is a blank line within the panel frame.
func EmptyRow() Row { return Row{kind: kindEmpty} }

// TitleRow renders left as a <title> and the optional right as a
// <subtle>, padded apart to the panel width. Pass "" for right to omit.
func TitleRow(left, right string) Row {
	return Row{kind: kindTitle, left: left, right: right}
}

// TextRow renders a single string with the given alignment; when wrap is
// true, content too wide for the panel word-wraps onto further lines.
func TextRow(content string, align Align, wrap bool) Row {
	return Row{kind: kindText, content: content, align: align, wrap: wrap}
}

// CellRow renders a horizontal row of cells; dividers draws a frame bar
// between adjacent cells.
func CellRow(cells []Cell, dividers bool) Row {
	return Row{kind: kindCells, cells: cells, dividers: dividers}
}

// FooterRow renders a single subtle-styled footer line.
func FooterRow(content string) Row { return Row{kind: kindFooter, content: content} }

// Section is an ordered group of rows with a separator style drawn above
// it (suppressed for the first section, which sits under the top rule).
type Section struct {
	SeparatorAbove RuleStyle
	Rows           []Row
}

// Panel is a width plus an ordered list of sections.
type Panel struct {
	Width    int
	Sections []Section
}

// Render produces the framed multi-line string. Every line has the same
// visible width. The first and last rule are always Major regardless of
// section config (§8.2). Returns ErrPanelTitleOverflow if a title row's
// right side alone exceeds the inner width.
func (p Panel) Render() (string, error) {
	width := p.Width
	if width <= 0 {
		width = DefaultPanelWidth
	}
	inner := width - 2 // two frame columns
	if inner < 1 {
		inner = 1
	}

	lines := []string{majorRule(inner)}
	for si, sec := range p.Sections {
		if si > 0 {
			switch sec.SeparatorAbove {
			case RuleMajor:
				lines = append(lines, majorRule(inner))
			case RuleMinor:
				lines = append(lines, minorRule(inner))
			case RuleNone:
				// no rule
			}
		}
		for _, row := range sec.Rows {
			rendered, err := renderRow(row, inner)
			if err != nil {
				return "", err
			}
			lines = append(lines, rendered...)
		}
	}
	lines = append(lines, majorRule(inner))
	return strings.Join(lines, "\r\n") + "\r\n", nil
}

func majorRule(inner int) string {
	return "<frame>|" + strings.Repeat("=", inner) + "|</frame>"
}

func minorRule(inner int) string {
	return "<frame>|" + strings.Repeat("-", inner) + "|</frame>"
}

// frameLine wraps content (already sized to the inner width) in the
// vertical frame columns. It fits content to exactly inner visible
// columns as a safety net so the width invariant holds even if a row
// builder miscounts.
func frameLine(content string, inner int) string {
	return "<frame>|</frame>" + fitVisible(content, inner) + "<frame>|</frame>"
}

func renderRow(r Row, inner int) ([]string, error) {
	switch r.kind {
	case kindEmpty:
		return []string{frameLine("", inner)}, nil
	case kindTitle:
		return renderTitle(r, inner)
	case kindText:
		return renderText(r.content, r.align, r.wrap, inner), nil
	case kindFooter:
		return []string{frameLine(alignWithin("<subtle>"+r.content+"</subtle>", inner, AlignLeft), inner)}, nil
	case kindCells:
		return []string{frameLine(renderCells(r.cells, r.dividers, inner), inner)}, nil
	default:
		return []string{frameLine("", inner)}, nil
	}
}

func renderTitle(r Row, inner int) ([]string, error) {
	visRight := VisibleLength(r.right)
	if visRight > inner {
		return nil, ErrPanelTitleOverflow
	}
	leftBudget := inner - visRight
	left := truncateVisible(r.left, leftBudget, true)
	gap := inner - VisibleLength(left) - visRight
	if gap < 0 {
		gap = 0
	}
	var b strings.Builder
	b.WriteString("<title>")
	b.WriteString(left)
	b.WriteString("</title>")
	b.WriteString(strings.Repeat(" ", gap))
	if r.right != "" {
		b.WriteString("<subtle>")
		b.WriteString(r.right)
		b.WriteString("</subtle>")
	}
	return []string{frameLine(b.String(), inner)}, nil
}

func renderText(content string, align Align, wrap bool, inner int) []string {
	var rows []string
	if wrap {
		for _, line := range wordWrap(content, inner) {
			rows = append(rows, frameLine(alignWithin(line, inner, align), inner))
		}
	} else {
		line := truncateVisible(content, inner, false)
		rows = append(rows, frameLine(alignWithin(line, inner, align), inner))
	}
	if len(rows) == 0 {
		rows = append(rows, frameLine("", inner))
	}
	return rows
}

// renderCells lays out cells across the inner width. Fixed cells take
// their width; Fill cells split the remainder evenly (leftover columns
// go to the earliest fills). Cells render single-line, truncated/padded
// to their computed width. A divider is a single frame column.
func renderCells(cells []Cell, dividers bool, inner int) string {
	n := len(cells)
	if n == 0 {
		return ""
	}
	dividerCols := 0
	if dividers && n > 1 {
		dividerCols = n - 1
	}
	avail := inner - dividerCols
	if avail < 0 {
		avail = 0
	}

	fixedSum, fills := 0, 0
	for _, c := range cells {
		if c.Fill {
			fills++
		} else {
			fixedSum += c.Width
		}
	}
	fillTotal := avail - fixedSum
	if fillTotal < 0 {
		fillTotal = 0
	}
	base, extra := 0, 0
	if fills > 0 {
		base = fillTotal / fills
		extra = fillTotal % fills
	}

	parts := make([]string, 0, n)
	for _, c := range cells {
		w := c.Width
		if c.Fill {
			w = base
			if extra > 0 {
				w++
				extra--
			}
		}
		if w < 0 {
			w = 0
		}
		parts = append(parts, renderCell(c, w))
	}

	divider := ""
	if dividers {
		divider = "<frame>|</frame>"
	}
	return strings.Join(parts, divider)
}

func renderCell(c Cell, width int) string {
	if c.Progress {
		return progressBar(c.Value, c.Max, width)
	}
	return alignWithin(truncateVisible(c.Content, width, false), width, c.Align)
}

// progressBar renders a value/max bar exactly width columns wide using
// ASCII so VisibleLength (byte-based) stays correct. Empty width or
// non-positive max yields all-empty.
func progressBar(value, max, width int) string {
	if width <= 0 {
		return ""
	}
	filled := 0
	if max > 0 {
		if value < 0 {
			value = 0
		}
		if value > max {
			value = max
		}
		filled = value * width / max
	}
	return strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
}

// alignWithin pads s (measured by visible length) to width. Content
// wider than width is returned unchanged — callers pre-truncate.
func alignWithin(s string, width int, align Align) string {
	pad := width - VisibleLength(s)
	if pad <= 0 {
		return s
	}
	switch align {
	case AlignRight:
		return strings.Repeat(" ", pad) + s
	case AlignCenter:
		l := pad / 2
		return strings.Repeat(" ", l) + s + strings.Repeat(" ", pad-l)
	default:
		return s + strings.Repeat(" ", pad)
	}
}

// fitVisible returns s padded right or truncated to exactly width visible
// columns. The width safety net for frame lines.
func fitVisible(s string, width int) string {
	v := VisibleLength(s)
	if v == width {
		return s
	}
	if v < width {
		return s + strings.Repeat(" ", width-v)
	}
	return truncateVisible(s, width, false)
}

// truncateVisible returns s with at most max visible columns. Angle-tag
// markup (<...>) is copied through without counting so colors survive;
// only visible characters count toward the limit. When ellipsis is true
// and truncation occurs with room for it, the result ends with "...".
func truncateVisible(s string, max int, ellipsis bool) string {
	if max <= 0 {
		return ""
	}
	if VisibleLength(s) <= max {
		return s
	}
	limit := max
	if ellipsis && max >= 3 {
		limit = max - 3
	}
	var b strings.Builder
	visible := 0
	i := 0
	for i < len(s) && visible < limit {
		if s[i] == '<' {
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				break
			}
			b.WriteString(s[i : i+end+1])
			i += end + 1
			continue
		}
		b.WriteByte(s[i])
		visible++
		i++
	}
	if ellipsis && max >= 3 {
		b.WriteString("...")
	}
	return b.String()
}

// wordWrap greedily wraps content to lines of at most width visible
// columns, breaking on spaces. A single word wider than width is left on
// its own line (frameLine fits it). Tags are treated as part of their
// word for measurement via VisibleLength.
func wordWrap(content string, width int) []string {
	words := strings.Fields(content)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	var cur strings.Builder
	curLen := 0
	for _, w := range words {
		wl := VisibleLength(w)
		if curLen == 0 {
			cur.WriteString(w)
			curLen = wl
			continue
		}
		if curLen+1+wl > width {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(w)
			curLen = wl
			continue
		}
		cur.WriteByte(' ')
		cur.WriteString(w)
		curLen += 1 + wl
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}
