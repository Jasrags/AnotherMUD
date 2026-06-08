package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/wizard"
)

// wizardGmcpSink bridges the wizard's structured StepEvent (character-
// creation §5) to a Char.Wizard GMCP frame on one connection. It is the
// consumer M12.3 deferred until a negotiated structured-data channel
// existed — GMCP landed in M16, so a rich client now gets every rendered
// step as structured data alongside the plain-text path and can draw an
// in-place creation panel without scraping prompts.
//
// The creation wizard runs pre-actor (no connActor exists yet), so the
// sink sends straight on the conn rather than through the per-actor
// flusher used by in-game packages. OnFlowStep re-checks GmcpActive
// defensively even though newWizardGmcpSink only returns a live sink.
type wizardGmcpSink struct {
	sender gmcpSender
}

// OnFlowStep marshals one StepEvent to a Char.Wizard frame. A send error
// is logged at debug and swallowed: a failed creation-panel frame must
// not abort creation (the plain-text path already rendered the step).
func (w *wizardGmcpSink) OnFlowStep(ctx context.Context, ev wizard.StepEvent) {
	if w == nil || w.sender == nil || !w.sender.GmcpActive() {
		return
	}
	payload := gmcp.CharWizardStep{
		Flow:   ev.FlowID,
		Step:   ev.StepID,
		Type:   ev.StepType,
		Prompt: ev.Prompt,
		Secret: ev.Secret,
	}
	if len(ev.Options) > 0 {
		payload.Options = make([]gmcp.WizardOption, len(ev.Options))
		for i, o := range ev.Options {
			payload.Options[i] = gmcp.WizardOption{Label: o.Label, Tag: o.Tag}
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := w.sender.SendGmcp(ctx, gmcp.PackageCharWizard, data); err != nil {
		logging.From(ctx).Debug("gmcp wizard step send failed",
			slog.String("flow", ev.FlowID),
			slog.String("step", ev.StepID),
			slog.Any("err", err))
	}
}

// newWizardGmcpSink returns a wizard.EventSink that emits Char.Wizard
// frames on c, or nil when c does not support GMCP or has not negotiated
// it. A nil sink is the discard the wizard already tolerates, so the
// plain-text path still renders every step for every client (§5). The
// return is a nil interface (untyped nil), not a typed nil pointer, so
// the wizard's nil check fires correctly.
func newWizardGmcpSink(c conn.Connection) wizard.EventSink {
	sender, ok := c.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return nil
	}
	return &wizardGmcpSink{sender: sender}
}
