// Package wizard is the engine-side flow primitive that drives an
// ordered sequence of typed, interactive steps against an in-progress
// entity (spec character-creation §3-§5). It is content-agnostic: the
// steps, prompts, options, and handlers are supplied by the caller
// (packs / the composition root); the engine only sequences them,
// routes input, and emits the observable per-step event.
//
// The primitive is reusable for any interactive flow, not just
// character creation — creation is one Flow wired against the
// new-player trigger (M12.2/M12.3). This package has NO dependency on
// session, login, or telnet; it talks to the connection through the IO
// interface and surfaces structured events through EventSink.
//
// M12.1 scope: flow execution, the four step types, skip predicates,
// secret-echo handling, and the structured step-event seam. The
// completion pipeline (validation / restart / commit, §6-§7) and the
// session wiring (phase, input routing, help passthrough, disconnect)
// are the caller's responsibility — Start / Input report when a flow
// has completed and expose the assembled Entity so the driver can run
// that pipeline.
package wizard

import "context"

// Entity is the in-progress character a flow assembles. It is opaque to
// the engine: only content-supplied skip predicates and step handlers
// inspect or mutate it (they type-assert to the concrete entity). The
// engine passes it through unchanged.
type Entity = any

// IO is the connection surface a flow renders to and reads echo control
// from. The session supplies the real implementation (telnet conn);
// tests supply a fake. Input is NOT pulled through IO — the session
// pushes each line into Instance.Input, so the flow never blocks on a
// read.
type IO interface {
	// Write sends a line of output to the connection.
	Write(ctx context.Context, msg string) error
	// SetEcho toggles client echo. Used by secret text steps (§3.3) to
	// hide password-like input: off before the prompt, on before any
	// subsequent output.
	SetEcho(ctx context.Context, on bool)
}

// StepEvent is the structured per-step signal (spec §5). Every rendered
// step emits one so a rich client can render the step without parsing
// the text. M12.1 ships the event seam; the GMCP panel renderer that
// consumes it is deferred (no negotiated client channel yet) — see the
// M12 deferral notes.
type StepEvent struct {
	FlowID   string
	StepID   string
	StepType string // "info" | "choice" | "text" | "confirm"
	Prompt   string
	// Options is populated for choice steps only.
	Options []StepOption
	// Secret marks a text step whose input is hidden.
	Secret bool
}

// StepOption is one choice option in a StepEvent (label + optional tag
// line for richer rendering, §3.2).
type StepOption struct {
	Label string
	Tag   string
}

// EventSink receives a StepEvent for every rendered step. The
// composition root bridges it to the event bus; tests record. A nil
// sink is tolerated by NewInstance (becomes a discard).
type EventSink interface {
	OnFlowStep(ctx context.Context, ev StepEvent)
}

type nopSink struct{}

func (nopSink) OnFlowStep(context.Context, StepEvent) {}

// stepResult is the outcome of feeding input to an interactive step.
type stepResult int

const (
	// resultAdvance — input accepted; move to the next step.
	resultAdvance stepResult = iota
	// resultRepeat — input rejected; re-render the current step.
	resultRepeat
)

// Step is one unit of a flow. The four concrete types live in steps.go.
// The interface is the §3.6 extension seam: a pack-defined step type
// only needs to implement these methods. Render writes the text AND
// returns the structured event; Handle processes one input line for
// interactive steps (never called when Interactive reports false).
type Step interface {
	// StepID is the step's stable id (§3).
	StepID() string
	// ShouldSkip evaluates the skip predicate against the in-progress
	// entity, BEFORE rendering (§3.5).
	ShouldSkip(e Entity) bool
	// Interactive reports whether the step waits for input. Info steps
	// are non-interactive and auto-advance (§3.1).
	Interactive() bool
	// Render writes the step's text to io and returns its StepEvent. The
	// in-progress entity is supplied so a step may render content that depends
	// on earlier answers (a ChoiceStep with a dynamic OptionsFn); most steps
	// ignore it.
	Render(ctx context.Context, io IO, e Entity) (StepEvent, error)
	// Handle processes one input line against the entity, returning
	// whether to advance or repeat. Only called for interactive steps.
	Handle(ctx context.Context, io IO, e Entity, input string) (stepResult, error)
}

// Flow is an ordered, named sequence of steps with a single completion
// handler (spec §1 core concepts). It is immutable content; a live
// execution is an Instance.
type Flow struct {
	// ID is the flow's stable identifier (used by restart §7).
	ID string
	// Trigger is the event name that starts this flow (§2). The
	// registry resolves a trigger to a flow.
	Trigger string
	// Steps is the ordered step list.
	Steps []Step
	// WizardSteps is the optional progress-label list for rich clients
	// (§1, §5). Empty disables progress rendering.
	WizardSteps []string
	// Cancellable marks whether a cancel keyword escapes the flow (§4 /
	// §6.2). Character creation sets this false.
	Cancellable bool
	// OnComplete is the completion handler invoked by the driver (NOT by
	// the Instance) once the final step's handler returns (§6.3). It
	// validates the assembled entity and returns ok plus an optional
	// user-facing message. May be nil (treated as always-ok).
	OnComplete func(ctx context.Context, e Entity) (ok bool, msg string)
}

// Status is the result of advancing a flow (Start / Input).
type Status int

const (
	// StatusAwaitingInput — the flow rendered an interactive step and is
	// waiting for the next input line.
	StatusAwaitingInput Status = iota
	// StatusCompleted — every step has run; the driver should now invoke
	// OnComplete and the completion pipeline (§6).
	StatusCompleted
)

// Instance is a live execution of a Flow against one entity. Not safe
// for concurrent use — the session drives it from a single goroutine
// (the per-connection read loop), one input line at a time.
type Instance struct {
	flow   *Flow
	entity Entity
	io     IO
	sink   EventSink
	cur    int // index of the current step; -1 before Start
	done   bool
}

// NewInstance binds a flow to an entity, IO, and event sink. A nil sink
// becomes a discard. Call Start to render the first step.
func NewInstance(flow *Flow, entity Entity, io IO, sink EventSink) *Instance {
	if sink == nil {
		sink = nopSink{}
	}
	return &Instance{flow: flow, entity: entity, io: io, sink: sink, cur: -1}
}

// Flow returns the flow being executed (used by the driver for restart
// by id, §7).
func (in *Instance) Flow() *Flow { return in.flow }

// Entity returns the in-progress entity (used by the driver to run the
// completion pipeline, §6).
func (in *Instance) Entity() Entity { return in.entity }

// Done reports whether the flow has completed (every step run).
func (in *Instance) Done() bool { return in.done }

// Start renders the first non-skipped step. Returns StatusCompleted
// immediately if the flow has no interactive steps (e.g. a trigger with
// only info steps, or an empty flow — the §2 "no flow → immediate
// commit" case is the driver's, but an empty step list behaves the
// same here).
func (in *Instance) Start(ctx context.Context) (Status, error) {
	return in.advance(ctx)
}

// Input feeds one input line to the current interactive step. On accept
// the flow advances (auto-running info / skipped steps) to the next
// interactive step or to completion; on reject the current step is
// re-rendered. Calling Input after completion is a no-op returning
// StatusCompleted.
func (in *Instance) Input(ctx context.Context, line string) (Status, error) {
	if in.done {
		return StatusCompleted, nil
	}
	step := in.flow.Steps[in.cur]
	res, err := step.Handle(ctx, in.io, in.entity, line)
	if err != nil {
		return StatusAwaitingInput, err
	}
	switch res {
	case resultAdvance:
		return in.advance(ctx)
	default: // resultRepeat
		ev, err := step.Render(ctx, in.io, in.entity)
		if err != nil {
			return StatusAwaitingInput, err
		}
		in.sink.OnFlowStep(ctx, ev)
		return StatusAwaitingInput, nil
	}
}

// advance moves past the current step to the next renderable one,
// skipping steps whose predicate is true and auto-running
// non-interactive (info) steps, until it lands on an interactive step
// (StatusAwaitingInput) or exhausts the list (StatusCompleted).
func (in *Instance) advance(ctx context.Context) (Status, error) {
	for {
		in.cur++
		if in.cur >= len(in.flow.Steps) {
			in.done = true
			return StatusCompleted, nil
		}
		step := in.flow.Steps[in.cur]
		if step.ShouldSkip(in.entity) {
			continue
		}
		ev, err := step.Render(ctx, in.io, in.entity)
		if err != nil {
			return StatusAwaitingInput, err
		}
		in.sink.OnFlowStep(ctx, ev)
		if step.Interactive() {
			return StatusAwaitingInput, nil
		}
		// Non-interactive (info): auto-advance to the next step.
	}
}
