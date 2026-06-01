package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/render"
)

// promptActor is a namedActor plus the promptController surface so the
// `prompt` verb resolves and exercises read/write of the stored template
// without a real session.connActor.
type promptActor struct {
	*namedActor
	template string
}

func newPromptActor() *promptActor {
	return &promptActor{
		namedActor: &namedActor{testActor: newTestActor(nil), name: "Alice", playerID: "p-1"},
	}
}

func (p *promptActor) PromptTemplate() string     { return p.template }
func (p *promptActor) SetPromptTemplate(t string) { p.template = t }

func dispatchPrompt(t *testing.T, a command.Actor, input string) {
	t.Helper()
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), command.Env{}, a, input); err != nil {
		t.Fatalf("dispatch %q: %v", input, err)
	}
}

// `prompt <template>` stores the rest-of-line verbatim — internal spacing
// and color tags survive (token-rejoining c.Args would collapse spaces).
func TestPrompt_SetStoresVerbatim(t *testing.T) {
	a := newPromptActor()
	want := "<hp>[HP {hp}/{maxhp}]</hp>  ready>" // note the double space
	dispatchPrompt(t, a, "prompt "+want)

	if a.template != want {
		t.Errorf("template = %q, want %q (verbatim, internal spacing preserved)", a.template, want)
	}
	if got := a.lastLine(); !strings.Contains(got, "Prompt updated") {
		t.Errorf("confirmation = %q, want it to contain 'Prompt updated'", got)
	}
}

// Bare `prompt` with nothing set shows the default and says it's the default.
func TestPrompt_ShowDefaultWhenUnset(t *testing.T) {
	a := newPromptActor()
	dispatchPrompt(t, a, "prompt")

	got := a.lastLine()
	if !strings.Contains(strings.ToLower(got), "default") {
		t.Errorf("output %q should identify the default", got)
	}
	if !strings.Contains(got, render.DefaultPromptTemplate) {
		t.Errorf("output %q should include the default template", got)
	}
}

// Bare `prompt` with a template set shows that template (for editing).
func TestPrompt_ShowsCurrentWhenSet(t *testing.T) {
	a := newPromptActor()
	a.template = "my {hp} prompt"
	dispatchPrompt(t, a, "prompt")

	if got := a.lastLine(); !strings.Contains(got, "my {hp} prompt") {
		t.Errorf("output %q should echo the current template", got)
	}
}

// `prompt default` and `prompt reset` both clear the template.
func TestPrompt_ResetClearsTemplate(t *testing.T) {
	for _, kw := range []string{"default", "reset", "DEFAULT"} {
		a := newPromptActor()
		a.template = "something"
		dispatchPrompt(t, a, "prompt "+kw)

		if a.template != "" {
			t.Errorf("%q: template = %q, want cleared", kw, a.template)
		}
		if got := a.lastLine(); !strings.Contains(got, "reset to the default") {
			t.Errorf("%q: confirmation = %q", kw, got)
		}
	}
}

// `prompt default` when already on the default is a friendly no-op.
func TestPrompt_ResetWhenAlreadyDefault(t *testing.T) {
	a := newPromptActor() // template ""
	dispatchPrompt(t, a, "prompt default")

	if got := a.lastLine(); !strings.Contains(got, "already the default") {
		t.Errorf("output = %q, want 'already the default'", got)
	}
}

// An over-length template is rejected and the stored template is unchanged.
func TestPrompt_RejectsOverLength(t *testing.T) {
	a := newPromptActor()
	a.template = "keep"
	long := strings.Repeat("x", command.MaxPromptTemplateLen+1)
	dispatchPrompt(t, a, "prompt "+long)

	if a.template != "keep" {
		t.Errorf("template = %q, want unchanged 'keep' (over-length rejected)", a.template)
	}
	if got := a.lastLine(); !strings.Contains(got, "too long") {
		t.Errorf("output = %q, want 'too long'", got)
	}
}

// A template at exactly the cap is accepted.
func TestPrompt_AcceptsAtCap(t *testing.T) {
	a := newPromptActor()
	atCap := strings.Repeat("x", command.MaxPromptTemplateLen)
	dispatchPrompt(t, a, "prompt "+atCap)

	if a.template != atCap {
		t.Errorf("template at cap (%d) should be accepted", command.MaxPromptTemplateLen)
	}
}

// An actor that doesn't expose the prompt-control surface is refused.
func TestPrompt_ActorWithoutControllerRefused(t *testing.T) {
	a := newTestActor(nil) // plain testActor: no promptController
	dispatchPrompt(t, a, "prompt foo")

	if got := a.lastLine(); !strings.Contains(got, "can't change your prompt") {
		t.Errorf("output = %q, want refusal", got)
	}
}
