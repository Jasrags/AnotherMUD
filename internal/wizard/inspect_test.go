package wizard

import (
	"context"
	"strings"
	"testing"
)

// inspectFlow builds a single-choice flow whose options carry descriptions.
func inspectFlow() *Flow {
	return &Flow{
		ID: "create",
		Steps: []Step{
			&ChoiceStep{ID: "class", Prompt: "Choose your class:", Options: []Option{
				{Label: "Warrior", Tag: "steel and grit", Description: "A frontline fighter.", Value: "warrior"},
				{Label: "Mage", Tag: "fire and study", Description: "A wielder of arcane power.", Value: "mage"},
			}, OnSelect: func(e Entity, v any) { e.(*testEntity).class = v.(string) }},
		},
	}
}

// TestInspect_ByPrefix: `? <prefix>` writes the matched option's detail and
// re-renders the menu without advancing.
func TestInspect_ByPrefix(t *testing.T) {
	io := &fakeIO{}
	in := NewInstance(inspectFlow(), &testEntity{}, io, nil)
	if _, err := in.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	before := len(io.lines)

	handled, err := in.Inspect(context.Background(), "mage")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !handled {
		t.Fatalf("Inspect handled = false, want true for a matching prefix")
	}
	// A detail line (the description) plus a re-render of the menu.
	joined := strings.Join(io.lines[before:], "\n")
	if !strings.Contains(joined, "arcane power") {
		t.Errorf("inspect output missing the description: %q", joined)
	}
	if !strings.Contains(joined, "Choose your class:") {
		t.Errorf("inspect did not re-render the menu: %q", joined)
	}
	if in.Done() {
		t.Errorf("Inspect advanced the flow; it must not")
	}
}

// TestInspect_ByIndex matches by 1-based index too.
func TestInspect_ByIndex(t *testing.T) {
	io := &fakeIO{}
	in := NewInstance(inspectFlow(), &testEntity{}, io, nil)
	_, _ = in.Start(context.Background())
	handled, err := in.Inspect(context.Background(), "1")
	if err != nil || !handled {
		t.Fatalf("Inspect(\"1\") = (%v, %v), want (true, nil)", handled, err)
	}
	if !strings.Contains(strings.Join(io.lines, "\n"), "frontline fighter") {
		t.Errorf("index inspect missing description: %q", io.lines)
	}
}

// TestInspect_NoMatch returns handled=false so the caller can fall back to help.
func TestInspect_NoMatch(t *testing.T) {
	in := NewInstance(inspectFlow(), &testEntity{}, &fakeIO{}, nil)
	_, _ = in.Start(context.Background())
	if handled, err := in.Inspect(context.Background(), "nonsense"); handled || err != nil {
		t.Fatalf("Inspect(no match) = (%v, %v), want (false, nil)", handled, err)
	}
}

// TestInspect_NonInspectableStep: a step type that is not a ChoiceStep (here a
// TextStep) returns handled=false so the creation flow falls through to help.
func TestInspect_NonInspectableStep(t *testing.T) {
	flow := &Flow{
		ID: "create",
		Steps: []Step{
			&TextStep{ID: "name", Prompt: "Your name:", OnInput: func(e Entity, s string) { e.(*testEntity).name = s }},
		},
	}
	in := NewInstance(flow, &testEntity{}, &fakeIO{}, nil)
	_, _ = in.Start(context.Background())
	if handled, err := in.Inspect(context.Background(), "1"); handled || err != nil {
		t.Fatalf("Inspect on a TextStep = (%v, %v), want (false, nil)", handled, err)
	}
}

// TestRender_InspectHint: a menu with descriptions advertises the inspect hint.
func TestRender_InspectHint(t *testing.T) {
	io := &fakeIO{}
	in := NewInstance(inspectFlow(), &testEntity{}, io, nil)
	_, _ = in.Start(context.Background())
	if !strings.Contains(strings.Join(io.lines, "\n"), "? <number>") {
		t.Errorf("menu with descriptions did not show the inspect hint: %q", io.lines)
	}
}
