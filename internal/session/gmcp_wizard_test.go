package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/wizard"
)

// TestWizardGmcpSink_EmitsChoiceStep proves the §5 bridge marshals a
// choice StepEvent into one Char.Wizard frame with its options.
func TestWizardGmcpSink_EmitsChoiceStep(t *testing.T) {
	fc := &gmcpFakeConn{}
	fc.setActive(true)

	sink := newWizardGmcpSink(fc)
	if sink == nil {
		t.Fatal("active GMCP conn should yield a sink")
	}
	sink.OnFlowStep(context.Background(), wizard.StepEvent{
		FlowID: "creation", StepID: "race", StepType: "choice", Prompt: "Choose:",
		Options: []wizard.StepOption{{Label: "Human", Tag: "versatile"}, {Label: "Dwarf"}},
	})

	frames := fc.framesSnapshot()
	if len(frames) != 1 {
		t.Fatalf("emitted %d frames, want 1", len(frames))
	}
	if frames[0].pkg != gmcp.PackageCharWizard {
		t.Errorf("pkg = %q, want %q", frames[0].pkg, gmcp.PackageCharWizard)
	}
	var got gmcp.CharWizardStep
	if err := json.Unmarshal(frames[0].payload, &got); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if got.Type != "choice" || got.Step != "race" || got.Prompt != "Choose:" {
		t.Errorf("payload = %+v", got)
	}
	if len(got.Options) != 2 || got.Options[0].Label != "Human" || got.Options[0].Tag != "versatile" {
		t.Errorf("options = %+v", got.Options)
	}
}

// TestWizardGmcpSink_NilForPlainConn: a conn that does not implement
// GMCP gets no sink, so the wizard falls back to its text-only path.
func TestWizardGmcpSink_NilForPlainConn(t *testing.T) {
	if sink := newWizardGmcpSink(&fakeConn{}); sink != nil {
		t.Errorf("plain conn should yield nil sink, got %#v", sink)
	}
}

// TestWizardGmcpSink_NilWhenInactive: a GMCP-capable conn that has not
// negotiated GMCP yet also gets no sink (creation runs early, before
// some clients finish negotiation — the text path covers them).
func TestWizardGmcpSink_NilWhenInactive(t *testing.T) {
	fc := &gmcpFakeConn{} // active defaults false
	if sink := newWizardGmcpSink(fc); sink != nil {
		t.Errorf("inactive GMCP conn should yield nil sink, got %#v", sink)
	}
}
