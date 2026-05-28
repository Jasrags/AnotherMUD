package help

import (
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/render"
)

// DefaultHelpWidth is the column width help output is laid out for.
const DefaultHelpWidth = 78

// RenderTopic builds the full topic display (§10.1). Output carries
// semantic tags (<title>, <subtle>) for the color renderer downstream.
// The Syntax and See-also sections appear only when the topic declares
// them.
func RenderTopic(t *Topic, width int) string {
	if width <= 0 {
		width = DefaultHelpWidth
	}
	rule := strings.Repeat("=", width)
	var b strings.Builder
	b.WriteString(rule)
	b.WriteString("\r\n")
	// Title and Brief are wrapped in a semantic tag, so a pack value
	// containing angle brackets could close the tag early or inject a
	// new one — strip brackets from those two. Syntax, Body, and
	// See-also are emitted OUTSIDE any tag, where worst case is cosmetic
	// colorization (raw ESC is dropped downstream), and `<target>`-style
	// placeholders are legitimate — so those pass through verbatim. Body
	// may also carry color tags by design (spec §9.1).
	b.WriteString(center("<title>"+sanitizeContent(t.Title)+"</title>", width))
	b.WriteString("\r\n")
	b.WriteString(rule)
	b.WriteString("\r\n")
	if t.Brief != "" {
		b.WriteString("<subtle>" + sanitizeContent(t.Brief) + "</subtle>\r\n")
	}
	if len(t.Syntax) > 0 {
		b.WriteString("\r\n<subtle>Syntax:</subtle>\r\n")
		for _, s := range t.Syntax {
			b.WriteString("  " + s + "\r\n")
		}
	}
	if t.Body != "" {
		b.WriteString("\r\n")
		for _, line := range strings.Split(t.Body, "\n") {
			b.WriteString("  " + strings.TrimRight(line, "\r") + "\r\n")
		}
	}
	if len(t.SeeAlso) > 0 {
		b.WriteString("\r\n<subtle>See also:</subtle> " + strings.Join(t.SeeAlso, ", ") + "\r\n")
	}
	b.WriteString(rule)
	return b.String()
}

// RenderDisambiguation lists the matches for an ambiguous term (§10.2),
// aligning ids in a column.
func RenderDisambiguation(term string, matches []Summary, width int) string {
	if width <= 0 {
		width = DefaultHelpWidth
	}
	rule := strings.Repeat("=", width)
	idWidth := 0
	for _, m := range matches {
		if l := len(m.ID); l > idWidth {
			idWidth = l
		}
	}
	var b strings.Builder
	b.WriteString(rule)
	b.WriteString("\r\n")
	b.WriteString(center("<title>help: "+sanitizeTerm(term)+"</title>", width))
	b.WriteString("\r\n")
	b.WriteString(rule)
	b.WriteString("\r\nMultiple matches found:\r\n")
	for _, m := range matches {
		// Match cells are emitted outside any tag, so no framing-break
		// risk; ids/titles pass through verbatim.
		b.WriteString(fmt.Sprintf("  %-*s    %s\r\n", idWidth, m.ID, m.Title))
	}
	b.WriteString("<subtle>Type help [topic] for details.</subtle>\r\n")
	b.WriteString(rule)
	return b.String()
}

// RenderNoMatch is the single-line miss (§10.3). The term is sanitized so
// a query containing angle brackets cannot inject color tags.
func RenderNoMatch(term string) string {
	return fmt.Sprintf("No help found for '%s'.", sanitizeTerm(term))
}

// center pads s to width with the visible content centered. Width math
// uses VisibleLength so embedded tags don't skew the centering. Like the
// panel renderer, VisibleLength is byte-based, so centering of titles
// with multi-byte UTF-8 runes is approximate — fine for the ASCII help
// content the engine ships.
func center(s string, width int) string {
	pad := width - render.VisibleLength(s)
	if pad <= 0 {
		return s
	}
	left := pad / 2
	return strings.Repeat(" ", left) + s
}

var angleStripper = strings.NewReplacer("<", "", ">", "")

// sanitizeTerm strips angle brackets from a user-supplied term so it
// cannot be interpreted as markup by the color renderer downstream.
func sanitizeTerm(term string) string { return angleStripper.Replace(term) }

// sanitizeContent strips angle brackets from a pack-authored value that
// gets wrapped in a semantic tag (title, brief, syntax, see-also), so
// the value can't close the tag early or inject a new one. The topic
// body is exempt — spec §9.1 allows color tags there.
func sanitizeContent(s string) string { return angleStripper.Replace(s) }
