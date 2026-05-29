package wizard

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Step type tags carried in StepEvent.StepType (spec §3).
const (
	stepTypeInfo    = "info"
	stepTypeChoice  = "choice"
	stepTypeText    = "text"
	stepTypeConfirm = "confirm"
)

// skipFn is a step's optional skip predicate (§3.5). A nil predicate
// never skips.
type skipFn func(Entity) bool

func runSkip(fn skipFn, e Entity) bool {
	return fn != nil && fn(e)
}

// --- Info step (§3.1) -------------------------------------------------

// InfoStep renders text once and auto-advances. Non-interactive.
type InfoStep struct {
	ID   string
	Text string
	Skip skipFn
}

func (s *InfoStep) StepID() string           { return s.ID }
func (s *InfoStep) ShouldSkip(e Entity) bool { return runSkip(s.Skip, e) }
func (s *InfoStep) Interactive() bool        { return false }

func (s *InfoStep) Render(ctx context.Context, io IO) (StepEvent, error) {
	ev := StepEvent{StepID: s.ID, StepType: stepTypeInfo, Prompt: s.Text}
	return ev, io.Write(ctx, s.Text)
}

// Handle is never called for a non-interactive step; present to satisfy
// Step.
func (s *InfoStep) Handle(context.Context, IO, Entity, string) (stepResult, error) {
	return resultAdvance, nil
}

// --- Choice step (§3.2) -----------------------------------------------

// Option is one selectable choice (§3.2). Value is passed to OnSelect.
type Option struct {
	Label       string
	Description string
	Tag         string
	Value       any
}

// ChoiceStep renders a prompt + an ordered option list. Input is a
// 1-based index OR a unique case-insensitive prefix of an option label.
type ChoiceStep struct {
	ID       string
	Prompt   string
	Options  []Option
	OnSelect func(e Entity, value any)
	Skip     skipFn
}

func (s *ChoiceStep) StepID() string           { return s.ID }
func (s *ChoiceStep) ShouldSkip(e Entity) bool { return runSkip(s.Skip, e) }
func (s *ChoiceStep) Interactive() bool        { return true }

func (s *ChoiceStep) Render(ctx context.Context, io IO) (StepEvent, error) {
	var b strings.Builder
	b.WriteString(s.Prompt)
	for i, opt := range s.Options {
		b.WriteString(fmt.Sprintf("\n  %d) %s", i+1, opt.Label))
		if opt.Tag != "" {
			b.WriteString(" — " + opt.Tag)
		}
	}
	ev := StepEvent{StepID: s.ID, StepType: stepTypeChoice, Prompt: s.Prompt}
	for _, opt := range s.Options {
		ev.Options = append(ev.Options, StepOption{Label: opt.Label, Tag: opt.Tag})
	}
	return ev, io.Write(ctx, b.String())
}

func (s *ChoiceStep) Handle(ctx context.Context, io IO, e Entity, input string) (stepResult, error) {
	idx, ok := s.resolve(input)
	if !ok {
		return resultRepeat, nil
	}
	if s.OnSelect != nil {
		s.OnSelect(e, s.Options[idx].Value)
	}
	return resultAdvance, nil
}

// resolve maps input to an option index: a 1-based numeric index, or a
// unique case-insensitive prefix of exactly one option's label. Returns
// (idx, false) on no-match or ambiguous prefix (§3.2 "invalid input
// repeats the prompt").
func (s *ChoiceStep) resolve(input string) (int, bool) {
	in := strings.TrimSpace(input)
	if in == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(in); err == nil {
		if n >= 1 && n <= len(s.Options) {
			return n - 1, true
		}
		return 0, false
	}
	low := strings.ToLower(in)
	match, count := -1, 0
	for i, opt := range s.Options {
		if strings.HasPrefix(strings.ToLower(opt.Label), low) {
			match = i
			count++
		}
	}
	if count == 1 {
		return match, true
	}
	return 0, false // 0 → no match; >1 → ambiguous
}

// --- Text step (§3.3) -------------------------------------------------

// TextStep renders a prompt and accepts free-form input. An optional
// Validate predicate gates acceptance; Secret hides echo.
type TextStep struct {
	ID         string
	Prompt     string
	Secret     bool
	Validate   func(string) bool
	InvalidMsg string
	OnInput    func(e Entity, input string)
	Skip       skipFn
}

func (s *TextStep) StepID() string           { return s.ID }
func (s *TextStep) ShouldSkip(e Entity) bool { return runSkip(s.Skip, e) }
func (s *TextStep) Interactive() bool        { return true }

func (s *TextStep) Render(ctx context.Context, io IO) (StepEvent, error) {
	// Secret steps suppress echo BEFORE the prompt and rely on Handle to
	// restore it before any subsequent output (§3.3).
	if s.Secret {
		io.SetEcho(ctx, false)
	}
	ev := StepEvent{StepID: s.ID, StepType: stepTypeText, Prompt: s.Prompt, Secret: s.Secret}
	return ev, io.Write(ctx, s.Prompt)
}

func (s *TextStep) Handle(ctx context.Context, io IO, e Entity, input string) (stepResult, error) {
	if s.Validate != nil && !s.Validate(input) {
		// Restore echo before the invalid message + repeated prompt so
		// the rejection is visible (§3.3).
		if s.Secret {
			io.SetEcho(ctx, true)
		}
		if s.InvalidMsg != "" {
			if err := io.Write(ctx, s.InvalidMsg); err != nil {
				return resultRepeat, err
			}
		}
		return resultRepeat, nil
	}
	if s.Secret {
		io.SetEcho(ctx, true)
	}
	if s.OnInput != nil {
		s.OnInput(e, input)
	}
	return resultAdvance, nil
}

// --- Confirm step (§3.4) ----------------------------------------------

// ConfirmStep renders a yes/no prompt. Affirmative runs OnYes, negative
// OnNo; anything else repeats. Either branch advances the flow.
type ConfirmStep struct {
	ID     string
	Prompt string
	OnYes  func(Entity)
	OnNo   func(Entity)
	Skip   skipFn
}

func (s *ConfirmStep) StepID() string           { return s.ID }
func (s *ConfirmStep) ShouldSkip(e Entity) bool { return runSkip(s.Skip, e) }
func (s *ConfirmStep) Interactive() bool        { return true }

func (s *ConfirmStep) Render(ctx context.Context, io IO) (StepEvent, error) {
	ev := StepEvent{StepID: s.ID, StepType: stepTypeConfirm, Prompt: s.Prompt}
	return ev, io.Write(ctx, s.Prompt)
}

func (s *ConfirmStep) Handle(ctx context.Context, io IO, e Entity, input string) (stepResult, error) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "y", "yes":
		if s.OnYes != nil {
			s.OnYes(e)
		}
		return resultAdvance, nil
	case "n", "no":
		if s.OnNo != nil {
			s.OnNo(e)
		}
		return resultAdvance, nil
	default:
		return resultRepeat, nil
	}
}
