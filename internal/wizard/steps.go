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
	// TextFn, when set, supplies the info text dynamically from the in-progress
	// entity — the same late-binding seam ChoiceStep.OptionsFn uses for options.
	// It takes precedence over the static Text. The character-creation review
	// step uses it to render a recap of the choices made so far; a static info
	// screen leaves it nil and sets Text.
	TextFn func(e Entity) string
	Skip   skipFn
}

// text returns the live info text: the dynamic TextFn when set, else Text.
func (s *InfoStep) text(e Entity) string {
	if s.TextFn != nil {
		return s.TextFn(e)
	}
	return s.Text
}

func (s *InfoStep) StepID() string           { return s.ID }
func (s *InfoStep) ShouldSkip(e Entity) bool { return runSkip(s.Skip, e) }
func (s *InfoStep) Interactive() bool        { return false }

func (s *InfoStep) Render(ctx context.Context, io IO, e Entity) (StepEvent, error) {
	txt := s.text(e)
	ev := StepEvent{StepID: s.ID, StepType: stepTypeInfo, Prompt: txt}
	return ev, io.Write(ctx, txt)
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
//
// Options are usually static (the Options slice). A step whose choices depend
// on an earlier answer sets OptionsFn instead — it is called with the
// in-progress entity at render AND resolve time and MUST be deterministic
// across that pair (it reads only already-committed earlier answers, so it is).
// OptionsFn takes precedence over Options when set.
type ChoiceStep struct {
	ID        string
	Prompt    string
	Options   []Option
	OptionsFn func(e Entity) []Option
	OnSelect  func(e Entity, value any)
	Skip      skipFn
}

func (s *ChoiceStep) StepID() string           { return s.ID }
func (s *ChoiceStep) ShouldSkip(e Entity) bool { return runSkip(s.Skip, e) }
func (s *ChoiceStep) Interactive() bool        { return true }

// opts returns the live option list for the entity: the dynamic OptionsFn when
// set, else the static Options.
func (s *ChoiceStep) opts(e Entity) []Option {
	if s.OptionsFn != nil {
		return s.OptionsFn(e)
	}
	return s.Options
}

func (s *ChoiceStep) Render(ctx context.Context, io IO, e Entity) (StepEvent, error) {
	options := s.opts(e)
	var b strings.Builder
	b.WriteString(s.Prompt)
	hasDetail := false
	for i, opt := range options {
		b.WriteString(fmt.Sprintf("\n  %d) %s", i+1, opt.Label))
		if opt.Tag != "" {
			b.WriteString(" — " + opt.Tag)
		}
		if opt.Description != "" {
			hasDetail = true
		}
	}
	// When any option carries a longer Description, advertise the
	// non-committal inspect affordance (§3.2): `? <number>` shows the detail
	// and re-displays the menu without spending the choice.
	if hasDetail {
		b.WriteString("\n(Type '? <number>' to inspect a choice before picking.)")
	}
	ev := StepEvent{StepID: s.ID, StepType: stepTypeChoice, Prompt: s.Prompt}
	for _, opt := range options {
		ev.Options = append(ev.Options, StepOption{Label: opt.Label, Tag: opt.Tag})
	}
	return ev, io.Write(ctx, b.String())
}

// inspectOption implements the inspectable seam: given a token (a 1-based
// index or a unique case-insensitive label prefix, the same grammar the
// choice itself accepts), it returns the matched option's detail block and
// whether a unique option matched. No match (or ambiguous prefix) returns
// ok=false so the caller can fall back to its help handler. The entity is
// passed so a dynamic OptionsFn resolves against the live list.
func (s *ChoiceStep) inspectOption(e Entity, token string) (string, bool) {
	options := s.opts(e)
	idx, ok := resolveChoice(options, token)
	if !ok {
		return "", false
	}
	opt := options[idx]
	var b strings.Builder
	b.WriteString(opt.Label)
	if opt.Tag != "" {
		b.WriteString(" — " + opt.Tag)
	}
	if opt.Description != "" {
		b.WriteString("\n" + opt.Description)
	}
	return b.String(), true
}

func (s *ChoiceStep) Handle(ctx context.Context, io IO, e Entity, input string) (stepResult, error) {
	options := s.opts(e)
	idx, ok := resolveChoice(options, input)
	if !ok {
		return resultRepeat, nil
	}
	if s.OnSelect != nil {
		s.OnSelect(e, options[idx].Value)
	}
	return resultAdvance, nil
}

// resolveChoice maps input to an option index in the supplied list: a 1-based
// numeric index, or a unique case-insensitive prefix of exactly one option's
// label. Returns (idx, false) on no-match or ambiguous prefix (§3.2 "invalid
// input repeats the prompt"). A free function (not a method) so it resolves
// against the live option list — static Options or a dynamic OptionsFn result.
func resolveChoice(options []Option, input string) (int, bool) {
	in := strings.TrimSpace(input)
	if in == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(in); err == nil {
		if n >= 1 && n <= len(options) {
			return n - 1, true
		}
		return 0, false
	}
	low := strings.ToLower(in)
	match, count := -1, 0
	for i, opt := range options {
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

func (s *TextStep) Render(ctx context.Context, io IO, _ Entity) (StepEvent, error) {
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

func (s *ConfirmStep) Render(ctx context.Context, io IO, _ Entity) (StepEvent, error) {
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
