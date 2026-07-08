package help

import (
	"strings"
	"testing"
)

func TestRenderTopicSections(t *testing.T) {
	full := &Topic{
		ID: "look", Title: "Look", Brief: "Examine your surroundings.",
		Body:   "Shows the current room.\nNames its exits.",
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

func TestRenderTopicSanitizesWrappedFields(t *testing.T) {
	// Title and Brief are wrapped in a semantic tag, so a value that
	// tries to close it early or inject a tag must have its angle
	// brackets stripped. Syntax/Body/See-also are emitted outside tags,
	// so <placeholder> notation and body color tags pass through.
	mal := &Topic{
		ID: "x", Title: "Foo</title><danger>evil", Brief: "b<subtle>x",
		Syntax: []string{"look <target>"}, SeeAlso: []string{"a"},
		Body: "<highlight>body tags allowed</highlight>",
	}
	out := RenderTopic(mal, 40)
	if strings.Contains(out, "Foo</title>") || strings.Contains(out, "<danger>") {
		t.Errorf("title not sanitized:\n%s", out)
	}
	if strings.Contains(out, "b<subtle>x") {
		t.Errorf("brief not sanitized:\n%s", out)
	}
	// syntax placeholder preserved verbatim.
	if !strings.Contains(out, "look <target>") {
		t.Errorf("syntax placeholder should pass through:\n%s", out)
	}
	// body keeps its tags.
	if !strings.Contains(out, "<highlight>body tags allowed</highlight>") {
		t.Errorf("body tags should be preserved:\n%s", out)
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
