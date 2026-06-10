package command

import (
	"strings"
	"testing"
)

func TestRoomColumnWidth(t *testing.T) {
	const mapW = 9 // a typical bordered minimap width
	cases := []struct {
		name      string
		termWidth int
		mapWidth  int
		want      int
	}{
		{"unknown width keeps default", 0, mapW, defaultRoomColumnWidth},
		{"negative width keeps default", -5, mapW, defaultRoomColumnWidth},
		{"narrow terminal clamps up to default", 60, mapW, defaultRoomColumnWidth},
		{"wide terminal fills available", 80, mapW, 80 - mapW - minimapGap},
		{"very wide terminal clamps to max", 222, mapW, maxRoomColumnWidth},
		{"exact-fit boundary", defaultRoomColumnWidth + mapW + minimapGap, mapW, defaultRoomColumnWidth},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := roomColumnWidth(c.termWidth, c.mapWidth); got != c.want {
				t.Errorf("roomColumnWidth(%d, %d) = %d, want %d", c.termWidth, c.mapWidth, got, c.want)
			}
		})
	}
}

func TestBlockWidth(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"single line", "abc", 3},
		{"widest line wins", "a\nabcde\nab", 5},
		{"markup discounted", "<frame>+---+</frame>", 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := blockWidth(c.in); got != c.want {
				t.Errorf("blockWidth(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestMarkupWidth(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"<title>The Square</title>", 10}, // angle tags zero-width
		{"{G}Gate{x}", 4},                 // single-letter ROM codes
		{"{dim}muted{/}", 5},              // attribute tokens
		{"{yellow}Gate{/}", 4},            // full color name (regression)
		{"a{{b", 3},                       // escaped literal brace stays visible
		{"the {key} fits", 14},            // unknown token passes through literally
		{"a — b", 5},                      // em-dash is one visible column, not 3 bytes
		{"west → there", 12},              // the way-back arrow counts as one column
	}
	for _, c := range cases {
		if got := markupWidth(c.in); got != c.want {
			t.Errorf("markupWidth(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestWrapMarkupLine(t *testing.T) {
	lines := wrapMarkupLine("one two three four five", 9)
	for _, ln := range lines {
		if markupWidth(ln) > 9 {
			t.Errorf("wrapped line %q exceeds width 9 (%d)", ln, markupWidth(ln))
		}
	}
	if strings.Join(lines, " ") != "one two three four five" {
		t.Errorf("wrap lost/reordered words: %q", lines)
	}
}

// joinBeside aligns the right block at a fixed column even when left
// lines carry zero-width markup, and resets color at the boundary.
func TestJoinBeside_AlignsAndResets(t *testing.T) {
	left := "<subtle>AB</subtle>\nABCDE" // visible widths 2 and 5 (angle-only)
	right := "R1\nR2"
	out := joinBeside(left, right, 6, 2)
	rows := strings.Split(out, "\n")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2:\n%s", len(rows), out)
	}
	// Each row: left padded to visible width 6, a {x} reset, 2-space gap,
	// then the right block — so the right block starts at the same visible
	// column on every row.
	for i, want := range []string{"R1", "R2"} {
		if !strings.HasSuffix(rows[i], "  "+want) {
			t.Errorf("row %d %q should end with the gap+right %q", i, rows[i], want)
		}
		if !strings.Contains(rows[i], "{x}") {
			t.Errorf("row %d missing the boundary color reset", i)
		}
		// Visible width of everything left of the reset is exactly 6.
		leftPart := rows[i][:strings.Index(rows[i], "{x}")]
		if markupWidth(leftPart) != 6 {
			t.Errorf("row %d left column width = %d, want 6 (%q)", i, markupWidth(leftPart), leftPart)
		}
	}
}

// A left line carrying a multi-byte glyph (em-dash) must still pad to the
// correct VISIBLE width, so the right block (minimap border) stays in its
// column — the bug where one prose row shifted the map sideways.
func TestJoinBeside_MultibyteLeftStaysAligned(t *testing.T) {
	left := "plain line here\nwork — hinges — done" // second line has two em-dashes
	right := "| map |\n| row |"
	out := joinBeside(left, right, 24, 2)
	rows := strings.Split(out, "\n")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	for i := range rows {
		leftPart := rows[i][:strings.Index(rows[i], "{x}")]
		if w := markupWidth(leftPart); w != 24 {
			t.Errorf("row %d left visible width = %d, want 24 — multibyte broke the pad (%q)", i, w, leftPart)
		}
	}
}

// A room-only row (the minimap is shorter than the room body) is emitted
// trimmed, with no trailing pad or stray reset.
func TestJoinBeside_RoomOnlyRowsTrimmed(t *testing.T) {
	out := joinBeside("L1\nL2\nL3", "R1", 6, 2)
	rows := strings.Split(out, "\n")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[1] != "L2" || rows[2] != "L3" {
		t.Errorf("room-only rows should be trimmed, got %q / %q", rows[1], rows[2])
	}
}
