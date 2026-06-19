package wizard

import (
	"context"
	"strings"
	"testing"
)

// fakeIO records writes and echo toggles.
type fakeIO struct {
	lines  []string
	echoes []bool
}

func (f *fakeIO) Write(_ context.Context, msg string) error {
	f.lines = append(f.lines, msg)
	return nil
}
func (f *fakeIO) SetEcho(_ context.Context, on bool) { f.echoes = append(f.echoes, on) }

// recordSink captures emitted step events.
type recordSink struct{ events []StepEvent }

func (r *recordSink) OnFlowStep(_ context.Context, ev StepEvent) { r.events = append(r.events, ev) }

// testEntity is a concrete in-progress entity the handlers mutate.
type testEntity struct {
	race    string
	class   string
	name    string
	confirm string
}

func ctx() context.Context { return context.Background() }

func TestInfoAutoAdvancesToChoice(t *testing.T) {
	e := &testEntity{}
	io, sink := &fakeIO{}, &recordSink{}
	flow := &Flow{
		ID: "create",
		Steps: []Step{
			&InfoStep{ID: "welcome", Text: "Welcome!"},
			&ChoiceStep{ID: "race", Prompt: "Pick a race:", Options: []Option{
				{Label: "Human", Value: "human"},
				{Label: "Elf", Value: "elf"},
			}, OnSelect: func(en Entity, v any) { en.(*testEntity).race = v.(string) }},
		},
	}
	in := NewInstance(flow, e, io, sink)

	st, err := in.Start(ctx())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Info renders + auto-advances; we land awaiting input on the choice.
	if st != StatusAwaitingInput {
		t.Fatalf("status = %v, want AwaitingInput", st)
	}
	if len(sink.events) != 2 || sink.events[0].StepType != "info" || sink.events[1].StepType != "choice" {
		t.Fatalf("events = %+v, want info then choice", sink.events)
	}
	// The choice event carries options for rich clients.
	if len(sink.events[1].Options) != 2 || sink.events[1].Options[0].Label != "Human" {
		t.Fatalf("choice options = %+v", sink.events[1].Options)
	}

	st, _ = in.Input(ctx(), "2") // pick Elf by index
	if st != StatusCompleted {
		t.Fatalf("status after final choice = %v, want Completed", st)
	}
	if e.race != "elf" {
		t.Errorf("race = %q, want elf", e.race)
	}
	if !in.Done() {
		t.Error("instance should be Done")
	}
}

func TestChoicePrefixAndInvalidRepeat(t *testing.T) {
	e := &testEntity{}
	io, sink := &fakeIO{}, &recordSink{}
	flow := &Flow{ID: "f", Steps: []Step{
		&ChoiceStep{ID: "race", Prompt: "Race?", Options: []Option{
			{Label: "Human", Value: "human"},
			{Label: "Halfling", Value: "halfling"},
			{Label: "Elf", Value: "elf"},
		}, OnSelect: func(en Entity, v any) { en.(*testEntity).race = v.(string) }},
	}}
	in := NewInstance(flow, e, io, sink)
	in.Start(ctx())

	// "hal" is a unique prefix → Halfling. "h" is ambiguous (Human/
	// Halfling) → repeat. "elf" exact → Elf.
	if st, _ := in.Input(ctx(), "h"); st != StatusAwaitingInput {
		t.Fatalf("ambiguous prefix should repeat, got %v", st)
	}
	if e.race != "" {
		t.Errorf("ambiguous prefix selected %q, want none", e.race)
	}
	if st, _ := in.Input(ctx(), "hal"); st != StatusCompleted {
		t.Fatalf("unique prefix should complete, got %v", st)
	}
	if e.race != "halfling" {
		t.Errorf("race = %q, want halfling", e.race)
	}
}

func TestChoiceOutOfRangeIndexRepeats(t *testing.T) {
	e := &testEntity{}
	flow := &Flow{ID: "f", Steps: []Step{
		&ChoiceStep{ID: "c", Prompt: "?", Options: []Option{{Label: "A", Value: "a"}}},
	}}
	in := NewInstance(flow, e, &fakeIO{}, nil)
	in.Start(ctx())
	if st, _ := in.Input(ctx(), "5"); st != StatusAwaitingInput {
		t.Errorf("index 5 of 1 option should repeat, got %v", st)
	}
	if st, _ := in.Input(ctx(), "0"); st != StatusAwaitingInput {
		t.Errorf("index 0 (not 1-based) should repeat, got %v", st)
	}
}

func TestTextStepValidationRepeats(t *testing.T) {
	e := &testEntity{}
	io := &fakeIO{}
	flow := &Flow{ID: "f", Steps: []Step{
		&TextStep{
			ID: "name", Prompt: "Name?",
			Validate:   func(s string) bool { return len(s) >= 3 },
			InvalidMsg: "Too short.",
			OnInput:    func(en Entity, s string) { en.(*testEntity).name = s },
		},
	}}
	in := NewInstance(flow, e, io, nil)
	in.Start(ctx())

	if st, _ := in.Input(ctx(), "ab"); st != StatusAwaitingInput {
		t.Fatalf("short input should repeat, got %v", st)
	}
	if e.name != "" {
		t.Errorf("rejected input set name %q", e.name)
	}
	// The invalid message + repeated prompt were written.
	if got := io.lines; len(got) < 2 || got[len(got)-2] != "Too short." {
		t.Errorf("expected invalid message then re-prompt, got %v", got)
	}
	if st, _ := in.Input(ctx(), "Gandalf"); st != StatusCompleted {
		t.Fatalf("valid input should complete, got %v", st)
	}
	if e.name != "Gandalf" {
		t.Errorf("name = %q, want Gandalf", e.name)
	}
}

func TestSecretTextTogglesEcho(t *testing.T) {
	io := &fakeIO{}
	flow := &Flow{ID: "f", Steps: []Step{
		&TextStep{ID: "pw", Prompt: "Password:", Secret: true, OnInput: func(Entity, string) {}},
	}}
	in := NewInstance(flow, &testEntity{}, io, nil)
	in.Start(ctx()) // render → echo off
	if len(io.echoes) != 1 || io.echoes[0] != false {
		t.Fatalf("expected echo off at render, got %v", io.echoes)
	}
	in.Input(ctx(), "hunter2") // accept → echo restored before advancing
	if len(io.echoes) != 2 || io.echoes[1] != true {
		t.Fatalf("expected echo restored after input, got %v", io.echoes)
	}
}

func TestSecretTextRestoresEchoOnReject(t *testing.T) {
	io := &fakeIO{}
	flow := &Flow{ID: "f", Steps: []Step{
		&TextStep{ID: "pw", Prompt: "Password:", Secret: true,
			Validate: func(s string) bool { return s != "" }},
	}}
	in := NewInstance(flow, &testEntity{}, io, nil)
	in.Start(ctx())     // echo off
	in.Input(ctx(), "") // reject → echo restored, then re-render off again
	// Sequence: off (render), on (reject restore), off (re-render).
	if len(io.echoes) != 3 || io.echoes[0] || !io.echoes[1] || io.echoes[2] {
		t.Fatalf("echo sequence = %v, want [off on off]", io.echoes)
	}
}

func TestConfirmYesNoAndReject(t *testing.T) {
	for _, tc := range []struct {
		in        string
		wantYes   bool
		wantNo    bool
		completes bool
	}{
		{"y", true, false, true},
		{"YES", true, false, true},
		{"n", false, true, true},
		{"no", false, true, true},
		{"maybe", false, false, false},
	} {
		e := &testEntity{}
		flow := &Flow{ID: "f", Steps: []Step{
			&ConfirmStep{ID: "ok", Prompt: "Sure?",
				OnYes: func(en Entity) { en.(*testEntity).confirm = "yes" },
				OnNo:  func(en Entity) { en.(*testEntity).confirm = "no" }},
		}}
		in := NewInstance(flow, e, &fakeIO{}, nil)
		in.Start(ctx())
		st, _ := in.Input(ctx(), tc.in)
		if tc.completes && st != StatusCompleted {
			t.Errorf("%q: status = %v, want Completed", tc.in, st)
		}
		if !tc.completes && st != StatusAwaitingInput {
			t.Errorf("%q: status = %v, want AwaitingInput (repeat)", tc.in, st)
		}
		if tc.wantYes && e.confirm != "yes" {
			t.Errorf("%q: confirm = %q, want yes", tc.in, e.confirm)
		}
		if tc.wantNo && e.confirm != "no" {
			t.Errorf("%q: confirm = %q, want no", tc.in, e.confirm)
		}
	}
}

func TestSkipPredicateBypassesStepAndHandler(t *testing.T) {
	e := &testEntity{class: "preset"} // class already set
	io, sink := &fakeIO{}, &recordSink{}
	ran := false
	flow := &Flow{ID: "f", Steps: []Step{
		&ChoiceStep{ID: "class", Prompt: "Class?",
			Options:  []Option{{Label: "Fighter", Value: "fighter"}},
			OnSelect: func(Entity, any) { ran = true },
			Skip:     func(en Entity) bool { return en.(*testEntity).class != "" }},
		&InfoStep{ID: "done", Text: "Set."},
	}}
	in := NewInstance(flow, e, io, sink)
	st, _ := in.Start(ctx())

	if st != StatusCompleted {
		t.Fatalf("status = %v, want Completed (choice skipped, info auto-advances)", st)
	}
	if ran {
		t.Error("skipped choice's OnSelect must not run")
	}
	// Only the info step rendered; the skipped choice did not.
	if len(sink.events) != 1 || sink.events[0].StepID != "done" {
		t.Errorf("events = %+v, want only the info step", sink.events)
	}
}

func TestEmptyFlowCompletesImmediately(t *testing.T) {
	in := NewInstance(&Flow{ID: "empty"}, &testEntity{}, &fakeIO{}, nil)
	st, err := in.Start(ctx())
	if err != nil || st != StatusCompleted {
		t.Fatalf("empty flow Start = (%v, %v), want (Completed, nil)", st, err)
	}
}

func TestInputAfterCompletionIsNoop(t *testing.T) {
	in := NewInstance(&Flow{ID: "empty"}, &testEntity{}, &fakeIO{}, nil)
	in.Start(ctx())
	if st, _ := in.Input(ctx(), "anything"); st != StatusCompleted {
		t.Errorf("input after completion = %v, want Completed", st)
	}
}

// A ChoiceStep with a dynamic OptionsFn renders options that depend on the
// in-progress entity (the seam the background chooser + eligibility filter use).
func TestChoiceOptionsFn_DynamicByEntity(t *testing.T) {
	io, sink := &fakeIO{}, &recordSink{}
	e := &testEntity{race: "human"} // OptionsFn keys off this

	var picked string
	step := &ChoiceStep{
		ID:     "class",
		Prompt: "Choose your class:",
		OptionsFn: func(ent Entity) []Option {
			if ent.(*testEntity).race == "human" {
				return []Option{{Label: "Soldier", Value: "soldier"}, {Label: "Scholar", Value: "scholar"}}
			}
			return []Option{{Label: "Outsider", Value: "outsider"}}
		},
		OnSelect: func(_ Entity, v any) { picked = v.(string) },
	}
	flow := &Flow{ID: "f", Steps: []Step{step}}
	in := NewInstance(flow, e, io, sink)
	if _, err := in.Start(ctx()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// The human entity sees Soldier/Scholar; a prefix pick resolves against them.
	if st, err := in.Input(ctx(), "scho"); err != nil || st != StatusCompleted {
		t.Fatalf("Input = (%v, %v)", st, err)
	}
	if picked != "scholar" {
		t.Errorf("picked = %q, want scholar (resolved against the dynamic options)", picked)
	}
	// The rendered prompt listed both dynamic options.
	joined := strings.Join(io.lines, "")
	if !strings.Contains(joined, "Soldier") || !strings.Contains(joined, "Scholar") {
		t.Errorf("dynamic options not rendered; writes = %q", joined)
	}
}
