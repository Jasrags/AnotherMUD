package economy

import (
	"context"
	"testing"
)

// fakeRestEntity is a minimal RestEntity for service tests.
type fakeRestEntity struct {
	id         string
	state      string
	target     string
	sleepStart uint64
}

func (f *fakeRestEntity) ID() string                { return f.id }
func (f *fakeRestEntity) RestState() string         { return f.state }
func (f *fakeRestEntity) SetRestState(s string)     { f.state = s }
func (f *fakeRestEntity) SetRestTarget(id string)   { f.target = id }
func (f *fakeRestEntity) SetSleepStart(tick uint64) { f.sleepStart = tick }

// recordingRestSink captures emitted events and can veto.
type recordingRestSink struct {
	events []restEvent
	veto   bool
}

type restEvent struct {
	id, old, next, reason string
}

func (r *recordingRestSink) OnRestStateChange(_ context.Context, id string, old, next RestState, reason string) bool {
	r.events = append(r.events, restEvent{id, string(old), string(next), reason})
	return r.veto
}

func TestGetRestMultiplier(t *testing.T) {
	cfg := DefaultRestConfig()
	cases := map[RestState]float64{
		StateAwake:    1.0,
		StateResting:  2.0,
		StateSleeping: 3.0,
		RestState(""): 1.0, // unset normalizes to awake elsewhere; raw "" → 1.0
	}
	for state, want := range cases {
		if got := cfg.GetRestMultiplier(state); got != want {
			t.Errorf("GetRestMultiplier(%q) = %v, want %v", state, got, want)
		}
	}
}

func TestSetRestState_DefaultsToAwake(t *testing.T) {
	svc := NewRestService(DefaultRestConfig(), nil, nil)
	e := &fakeRestEntity{id: "p1"} // unset state
	if got := svc.State(e); got != StateAwake {
		t.Fatalf("unset state = %q, want awake", got)
	}
	// Requesting awake while already (implicitly) awake is a no-op.
	ok, reason := svc.SetRestState(context.Background(), e, StateAwake, "")
	if ok || reason != "already_in_state" {
		t.Fatalf("awake→awake = (%v, %q), want (false, already_in_state)", ok, reason)
	}
}

func TestSetRestState_RecordsSleepStart(t *testing.T) {
	sink := &recordingRestSink{}
	svc := NewRestService(DefaultRestConfig(), sink, func() uint64 { return 4242 })
	e := &fakeRestEntity{id: "p1"}

	ok, reason := svc.SetRestState(context.Background(), e, StateSleeping, "")
	if !ok || reason != "" {
		t.Fatalf("→sleeping = (%v, %q), want (true, \"\")", ok, reason)
	}
	if e.state != string(StateSleeping) {
		t.Errorf("state = %q, want sleeping", e.state)
	}
	if e.sleepStart != 4242 {
		t.Errorf("sleepStart = %d, want 4242", e.sleepStart)
	}
	if len(sink.events) != 1 || sink.events[0].next != "sleeping" || sink.events[0].reason != "" {
		t.Errorf("events = %+v, want one awake→sleeping with empty reason", sink.events)
	}
}

func TestSetRestState_SetsAndClearsTarget(t *testing.T) {
	svc := NewRestService(DefaultRestConfig(), nil, nil)
	e := &fakeRestEntity{id: "p1"}

	if ok, _ := svc.SetRestState(context.Background(), e, StateResting, "chair-1"); !ok {
		t.Fatal("→resting failed")
	}
	if e.target != "chair-1" {
		t.Errorf("rest target = %q, want chair-1", e.target)
	}
	// Returning to awake clears the target.
	if ok, _ := svc.SetRestState(context.Background(), e, StateAwake, ""); !ok {
		t.Fatal("→awake failed")
	}
	if e.target != "" {
		t.Errorf("rest target after wake = %q, want empty", e.target)
	}
}

func TestSetRestState_CancelledVetoes(t *testing.T) {
	sink := &recordingRestSink{veto: true}
	svc := NewRestService(DefaultRestConfig(), sink, nil)
	e := &fakeRestEntity{id: "p1"}

	ok, reason := svc.SetRestState(context.Background(), e, StateResting, "")
	if ok || reason != "cancelled" {
		t.Fatalf("vetoed transition = (%v, %q), want (false, cancelled)", ok, reason)
	}
	if e.state != "" {
		t.Errorf("state should be unchanged on veto, got %q", e.state)
	}
}

func TestSetRestState_InvalidAndNil(t *testing.T) {
	svc := NewRestService(DefaultRestConfig(), nil, nil)
	if ok, reason := svc.SetRestState(context.Background(), nil, StateResting, ""); ok || reason != "entity_not_found" {
		t.Errorf("nil entity = (%v, %q), want (false, entity_not_found)", ok, reason)
	}
	e := &fakeRestEntity{id: "p1"}
	if ok, reason := svc.SetRestState(context.Background(), e, RestState("dozing"), ""); ok || reason != "invalid_state" {
		t.Errorf("invalid state = (%v, %q), want (false, invalid_state)", ok, reason)
	}
}

func TestForceAwake_WakesAndBypassesVeto(t *testing.T) {
	sink := &recordingRestSink{veto: true} // would veto a normal change
	svc := NewRestService(DefaultRestConfig(), sink, func() uint64 { return 10 })
	e := &fakeRestEntity{id: "p1", state: string(StateSleeping), target: "bed-1"}

	if woke := svc.ForceAwake(context.Background(), e, "combat"); !woke {
		t.Fatal("ForceAwake on a sleeper should return true")
	}
	if e.state != string(StateAwake) || e.target != "" {
		t.Errorf("after ForceAwake state=%q target=%q, want awake + empty", e.state, e.target)
	}
	if len(sink.events) != 1 || sink.events[0].reason != "combat" {
		t.Errorf("events = %+v, want one with reason=combat", sink.events)
	}

	// Already awake → no-op, no event.
	sink.events = nil
	if woke := svc.ForceAwake(context.Background(), e, "combat"); woke {
		t.Error("ForceAwake on an awake entity should return false")
	}
	if len(sink.events) != 0 {
		t.Errorf("no event expected for already-awake, got %+v", sink.events)
	}
}
