package render

import (
	"errors"
	"strings"
	"testing"
)

// panelVisibleLines renders the panel, strips its color tags, and splits
// into lines so width invariants can be asserted on visible text.
func panelVisibleLines(t *testing.T, p Panel) []string {
	t.Helper()
	out, err := p.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out = strings.TrimSuffix(out, "\r\n")
	raw := strings.Split(out, "\r\n")
	lines := make([]string, len(raw))
	for i, l := range raw {
		lines[i] = StripTags(l)
	}
	return lines
}

func TestPanelAllLinesSameWidth(t *testing.T) {
	p := Panel{
		Width: 30,
		Sections: []Section{
			{Rows: []Row{
				TitleRow("Inventory", "3 items"),
				EmptyRow(),
				TextRow("a short line", AlignLeft, false),
				TextRow("<highlight>colored</highlight> cell text here that wraps around the panel", AlignLeft, true),
			}},
			{SeparatorAbove: RuleMinor, Rows: []Row{
				FooterRow("done"),
			}},
		},
	}
	lines := panelVisibleLines(t, p)
	for i, l := range lines {
		if len(l) != 30 {
			t.Errorf("line %d width = %d (%q), want 30", i, len(l), l)
		}
	}
}

func TestPanelTopBottomMajorRules(t *testing.T) {
	p := Panel{Width: 12, Sections: []Section{{Rows: []Row{EmptyRow()}}}}
	lines := panelVisibleLines(t, p)
	first, last := lines[0], lines[len(lines)-1]
	if !strings.Contains(first, "=") || !strings.Contains(last, "=") {
		t.Errorf("top/bottom must be Major (===): first=%q last=%q", first, last)
	}
	// first section separator is suppressed: line[1] is the empty row,
	// not another rule.
	if strings.Contains(lines[1], "===") {
		t.Errorf("first section separator should be suppressed, got rule %q", lines[1])
	}
}

func TestPanelSectionSeparators(t *testing.T) {
	p := Panel{
		Width: 12,
		Sections: []Section{
			{Rows: []Row{TextRow("a", AlignLeft, false)}},
			{SeparatorAbove: RuleMinor, Rows: []Row{TextRow("b", AlignLeft, false)}},
			{SeparatorAbove: RuleNone, Rows: []Row{TextRow("c", AlignLeft, false)}},
		},
	}
	lines := panelVisibleLines(t, p)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "|---") {
		t.Error("expected a Minor rule between section 1 and 2")
	}
	// section 3 has RuleNone: b and c should be adjacent with no rule.
	bIdx, cIdx := -1, -1
	for i, l := range lines {
		if strings.HasPrefix(l, "|b") {
			bIdx = i
		}
		if strings.HasPrefix(l, "|c") {
			cIdx = i
		}
	}
	if bIdx < 0 || cIdx < 0 || cIdx != bIdx+1 {
		t.Errorf("RuleNone should place c immediately after b: b=%d c=%d", bIdx, cIdx)
	}
}

func TestPanelTitleRightOverflowErrors(t *testing.T) {
	p := Panel{Width: 10, Sections: []Section{{Rows: []Row{
		TitleRow("x", "this right side is way too long"),
	}}}}
	if _, err := p.Render(); !errors.Is(err, ErrPanelTitleOverflow) {
		t.Errorf("err = %v, want ErrPanelTitleOverflow", err)
	}
}

func TestPanelTitleCombinedTruncatesLeft(t *testing.T) {
	// inner = 18; right "v2" is 2 visible; left budget 16. A long left
	// should truncate with an ellipsis.
	p := Panel{Width: 20, Sections: []Section{{Rows: []Row{
		TitleRow("A Very Long Inventory Title", "v2"),
	}}}}
	lines := panelVisibleLines(t, p)
	title := lines[1]
	if len(title) != 20 {
		t.Fatalf("title width = %d, want 20", len(title))
	}
	if !strings.Contains(title, "...") {
		t.Errorf("expected ellipsis in truncated title: %q", title)
	}
	if !strings.HasSuffix(strings.TrimRight(title, " |"), "v2") && !strings.Contains(title, "v2") {
		t.Errorf("right side missing: %q", title)
	}
}

func TestPanelCellsFillSplit(t *testing.T) {
	// inner = 18. Two fill cells split evenly → 9 each.
	p := Panel{Width: 20, Sections: []Section{{Rows: []Row{
		CellRow([]Cell{
			{Content: "L", Fill: true, Align: AlignLeft},
			{Content: "R", Fill: true, Align: AlignRight},
		}, false),
	}}}}
	lines := panelVisibleLines(t, p)
	row := lines[1]
	if len(row) != 20 {
		t.Fatalf("cell row width = %d, want 20", len(row))
	}
	inner := strings.TrimPrefix(strings.TrimSuffix(row, "|"), "|")
	if !strings.HasPrefix(inner, "L") || !strings.HasSuffix(inner, "R") {
		t.Errorf("fill cells misaligned: %q", inner)
	}
}

func TestPanelProgressCell(t *testing.T) {
	// width 10 fixed progress at 50% → 5 '#' + 5 '-'.
	p := Panel{Width: 14, Sections: []Section{{Rows: []Row{
		CellRow([]Cell{{Progress: true, Value: 5, Max: 10, Width: 10, Fill: false}}, false),
	}}}}
	lines := panelVisibleLines(t, p)
	row := lines[1]
	if !strings.Contains(row, "#####-----") {
		t.Errorf("progress bar = %q, want to contain #####-----", row)
	}
}

func TestPanelDefaultWidth(t *testing.T) {
	p := Panel{Sections: []Section{{Rows: []Row{EmptyRow()}}}}
	lines := panelVisibleLines(t, p)
	if len(lines[0]) != DefaultPanelWidth {
		t.Errorf("default width = %d, want %d", len(lines[0]), DefaultPanelWidth)
	}
}

func TestPanelColoredCellAlignsWithPlain(t *testing.T) {
	// A colored cell and a plain cell of the same fixed width must yield
	// the same visible width — width math uses VisibleLength.
	p := Panel{Width: 24, Sections: []Section{{Rows: []Row{
		CellRow([]Cell{
			{Content: "<highlight>HP</highlight>", Width: 10},
			{Content: "MP", Width: 10},
		}, true),
	}}}}
	lines := panelVisibleLines(t, p)
	if len(lines[1]) != 24 {
		t.Errorf("colored cell row width = %d, want 24", len(lines[1]))
	}
}

func TestPanelTextAlignmentAndTruncate(t *testing.T) {
	// Right and center alignment, plus non-wrap truncation.
	p := Panel{
		Width: 12, // inner 10
		Sections: []Section{{Rows: []Row{
			TextRow("hi", AlignRight, false),
			TextRow("hi", AlignCenter, false),
			TextRow("way too long to fit in ten", AlignLeft, false),
		}}},
	}
	lines := panelVisibleLines(t, p)
	right := strings.TrimSuffix(strings.TrimPrefix(lines[1], "|"), "|")
	center := strings.TrimSuffix(strings.TrimPrefix(lines[2], "|"), "|")
	trunc := strings.TrimSuffix(strings.TrimPrefix(lines[3], "|"), "|")
	if !strings.HasSuffix(right, "hi") {
		t.Errorf("right align = %q", right)
	}
	if !strings.HasPrefix(center, " ") || !strings.HasSuffix(center, " ") || !strings.Contains(center, "hi") {
		t.Errorf("center align = %q", center)
	}
	if len(trunc) != 10 {
		t.Errorf("truncated text inner width = %d, want 10", len(trunc))
	}
}

func TestProgressBarEdges(t *testing.T) {
	if got := progressBar(5, 10, 0); got != "" {
		t.Errorf("zero width = %q, want empty", got)
	}
	if got := progressBar(5, 0, 4); got != "----" {
		t.Errorf("zero max = %q, want ----", got)
	}
	if got := progressBar(-3, 10, 4); got != "----" {
		t.Errorf("negative value = %q, want ----", got)
	}
	if got := progressBar(99, 10, 4); got != "####" {
		t.Errorf("over max = %q, want ####", got)
	}
}

func TestPanelEmptyCells(t *testing.T) {
	p := Panel{Width: 10, Sections: []Section{{Rows: []Row{CellRow(nil, false)}}}}
	lines := panelVisibleLines(t, p)
	if len(lines[1]) != 10 {
		t.Errorf("empty cell row width = %d, want 10", len(lines[1]))
	}
}

func TestTruncateVisibleUTF8Safe(t *testing.T) {
	// "café" is 5 bytes (é = 0xC3 0xA9). VisibleLength is byte-based, so
	// "café" measures 5. Truncating to 3 bytes must NOT split the é —
	// the result must be valid UTF-8.
	got := truncateVisible("café", 3, false)
	if !utf8ValidString(got) {
		t.Errorf("truncated %q is not valid UTF-8 (% x)", got, got)
	}
	// 3 bytes budget: "caf" fits (3 ascii), é would push to 5 → stop.
	if got != "caf" {
		t.Errorf("truncateVisible(café,3) = %q, want caf", got)
	}
	// 4 bytes budget: "caf" (3) then é needs 2 → 5 > 4 → stop at caf.
	if got := truncateVisible("café", 4, false); !utf8ValidString(got) {
		t.Errorf("4-byte truncate not valid UTF-8: %q", got)
	}
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}
