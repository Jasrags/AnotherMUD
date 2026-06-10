package command

import "strings"

// mapCanvas is a sparse character grid addressed by (col, row), growing
// to fit whatever is set (negative addresses allowed). Each occupied
// cell holds a short tagged string — a glyph or a connector, possibly
// wrapped in a semantic color tag — that the renderer collapses to one
// visible column, so blank cells (one space) keep the grid aligned.
type mapCanvas struct {
	cells                  map[[2]int]string
	minC, minR, maxC, maxR int
	used                   bool
}

func newMapCanvas() *mapCanvas { return &mapCanvas{cells: make(map[[2]int]string)} }

// set writes s at (col, row), expanding the canvas bounds to include it.
func (g *mapCanvas) set(col, row int, s string) {
	if !g.used {
		g.minC, g.maxC, g.minR, g.maxR = col, col, row, row
		g.used = true
	}
	if col < g.minC {
		g.minC = col
	}
	if col > g.maxC {
		g.maxC = col
	}
	if row < g.minR {
		g.minR = row
	}
	if row > g.maxR {
		g.maxR = row
	}
	g.cells[[2]int{col, row}] = s
}

// render emits the canvas as newline-joined rows (top row first), blank
// cells as single spaces, with trailing blank space trimmed per row and
// trailing blank rows removed.
func (g *mapCanvas) render() string {
	if !g.used {
		return ""
	}
	var b strings.Builder
	for row := g.minR; row <= g.maxR; row++ {
		var line strings.Builder
		for col := g.minC; col <= g.maxC; col++ {
			if s, ok := g.cells[[2]int{col, row}]; ok {
				line.WriteString(s)
			} else {
				line.WriteByte(' ')
			}
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderFixed emits a constant-size square window of (2*half+1) cells
// per side, centred on the origin cell (0,0): every column and row from
// -half..+half, blank cells as single spaces, with NO trimming or
// blank-row removal so the box stays the same size and keeps the viewer
// dead centre no matter how much is explored (player-maps §4, the
// fixed-viewport minimap). Cells outside the window are clipped. half is
// in canvas units — twice the step radius, since rooms sit on even
// coordinates with connectors between.
func (g *mapCanvas) renderFixed(half int) string {
	var b strings.Builder
	for row := -half; row <= half; row++ {
		for col := -half; col <= half; col++ {
			if s, ok := g.cells[[2]int{col, row}]; ok {
				b.WriteString(s)
			} else {
				b.WriteByte(' ')
			}
		}
		if row < half {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
