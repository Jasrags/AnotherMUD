package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// wimpyActor wraps testActor with the wimpyController surface (read
// + write threshold) so the verb's set/get paths exercise without
// needing a real session.connActor.
type wimpyActor struct {
	*testActor
	threshold int
}

func newWimpyActor(name, playerID string, room *world.Room) *wimpyActor {
	return &wimpyActor{testActor: newNamedTestActor(name, playerID, room)}
}

func (w *wimpyActor) WimpyThreshold() int       { return w.threshold }
func (w *wimpyActor) SetWimpyThreshold(pct int) { w.threshold = pct }

func TestWimpy_NoArgReportsOff(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newWimpyActor("Alice", "p-1", f.room)

	dispatchActor(t, r, f.env(), a, "wimpy")
	if got := a.lastLine(); !strings.Contains(got, "off") {
		t.Errorf("default report = %q, want 'off'", got)
	}
}

func TestWimpy_NoArgReportsCurrentValue(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newWimpyActor("Alice", "p-1", f.room)
	a.threshold = 35

	dispatchActor(t, r, f.env(), a, "wimpy")
	if got := a.lastLine(); !strings.Contains(got, "35") {
		t.Errorf("report = %q, want '35'", got)
	}
}

func TestWimpy_SetValueClamps(t *testing.T) {
	cases := []struct {
		name string
		arg  string
		want int
		msg  string
	}{
		{"set 30", "30", 30, "30%"},
		{"set 0 disables", "0", 0, "disabled"},
		{"off disables", "off", 0, "disabled"},
		{"none disables", "none", 0, "disabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newKillFixture(t)
			r := newRegistry(t)
			a := newWimpyActor("Alice", "p-1", f.room)
			a.threshold = 50 // start non-zero so set→0 is observable

			dispatchActor(t, r, f.env(), a, "wimpy "+tc.arg)
			if a.threshold != tc.want {
				t.Errorf("threshold = %d, want %d", a.threshold, tc.want)
			}
			if got := a.lastLine(); !strings.Contains(got, tc.msg) {
				t.Errorf("message = %q, want substring %q", got, tc.msg)
			}
		})
	}
}

func TestWimpy_RejectsOutOfRange(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newWimpyActor("Alice", "p-1", f.room)

	for _, arg := range []string{"-5", "150", "999"} {
		dispatchActor(t, r, f.env(), a, "wimpy "+arg)
		if a.threshold != 0 {
			t.Errorf("threshold accepted out-of-range %q: %d", arg, a.threshold)
		}
		if got := a.lastLine(); !strings.Contains(got, "between 0 and 100") {
			t.Errorf("message for %q = %q", arg, got)
		}
	}
}

func TestWimpy_RejectsNonNumeric(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newWimpyActor("Alice", "p-1", f.room)

	dispatchActor(t, r, f.env(), a, "wimpy banana")
	if got := a.lastLine(); !strings.Contains(got, "Usage") {
		t.Errorf("non-numeric arg message = %q", got)
	}
}

// A plain testActor (no wimpyController surface) gets a clean
// refusal rather than a nil-deref.
func TestWimpy_NonControllerRefuses(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newNamedTestActor("Plain", "p-1", f.room)

	dispatchActor(t, r, f.env(), a, "wimpy 50")
	if got := a.lastLine(); !strings.Contains(got, "able to flee") {
		t.Errorf("non-controller wimpy = %q", got)
	}
}

// Suppress unused-import — command is referenced via type assertion
// helpers above.
var _ = command.Env{}
