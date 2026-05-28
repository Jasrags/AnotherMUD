package help

import (
	"strings"
	"testing"
)

func TestRenderTopicSections(t *testing.T) {
	full := &Topic{
		ID: "look", Title: "Look", Brief: "Examine your surroundings.",
		Body: "Shows the current room.\nNames its exits.",
		Syntax: []string{"look", "look <target>"}, SeeAlso: []string{"examine", "scan"},
	}
	out := RenderTopic(full, 40)
	for _, want := range []string{"<title>Look</title>", "Examine your surroundings", "Syntax:", "look <target>", "Shows the current room.", "See also:", "examine, scan"} {
		if !strings.Contains(out, want) {
			t.Errorf("topic render missing %q in:\n%s", want, out)
		}
	}

	// Syntax + See-also omitted when absent.
	bare := &Topic{ID: "x", Title: "X", Body: "body"}
	out = RenderTopic(bare, 40)
	if strings.Contains(out, "Syntax:") {
		t.Error("Syntax section should be omitted when no syntax")
	}
	if strings.Contains(out, "See also:") {
		t.Error("See-also should be omitted when no references")
	}
}

func TestRenderDisambiguationColumn(t *testing.T) {
	out := RenderDisambiguation("magic", []Summary{
		{ID: "cast", Title: "Cast Spell"},
		{ID: "spells", Title: "Spell List"},
	}, 50)
	if !strings.Contains(out, "Multiple matches found:") {
		t.Error("missing header")
	}
	if !strings.Contains(out, "Type help [topic] for details.") {
		t.Error("missing footer hint")
	}
	// ids aligned: "cast" padded to width of "spells" (6).
	if !strings.Contains(out, "cast      Cast Spell") {
		t.Errorf("id column not aligned:\n%s", out)
	}
}

func TestRenderNoMatchSanitizesTerm(t *testing.T) {
	out := RenderNoMatch("<danger>evil</danger>")
	if strings.ContainsAny(out, "<>") {
		t.Errorf("term not sanitized: %q", out)
	}
	if !strings.Contains(out, "No help found for") {
		t.Errorf("unexpected no-match: %q", out)
	}
}
