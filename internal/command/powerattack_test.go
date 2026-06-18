package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// powerAttackActor wraps testActor with the powerAttackController surface so the
// verb's read/set/feat-gate paths exercise without a real session.connActor.
type powerAttackActor struct {
	*testActor
	active  bool
	hasFeat bool
}

func newPowerAttackActor(name, playerID string, room *world.Room, hasFeat bool) *powerAttackActor {
	return &powerAttackActor{testActor: newNamedTestActor(name, playerID, room), hasFeat: hasFeat}
}

func (p *powerAttackActor) PowerAttackActive() bool  { return p.active }
func (p *powerAttackActor) HasPowerAttackFeat() bool { return p.hasFeat }
func (p *powerAttackActor) SetPowerAttack(on bool)   { p.active = on }

func TestPowerAttack_NoArgReportsState(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newPowerAttackActor("Alice", "p-1", f.room, true)

	dispatchActor(t, r, f.env(), a, "powerattack")
	if got := a.lastLine(); !strings.Contains(strings.ToLower(got), "off") {
		t.Errorf("default report = %q, want 'off'", got)
	}

	a.active = true
	dispatchActor(t, r, f.env(), a, "powerattack")
	if got := a.lastLine(); !strings.Contains(strings.ToLower(got), "on") {
		t.Errorf("active report = %q, want 'on'", got)
	}
}

func TestPowerAttack_OnEntersStance(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newPowerAttackActor("Alice", "p-1", f.room, true)

	dispatchActor(t, r, f.env(), a, "powerattack on")
	if !a.active {
		t.Error("powerattack on did not set the stance")
	}
}

func TestPowerAttack_OnRefusedWithoutFeat(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newPowerAttackActor("Alice", "p-1", f.room, false) // no feat

	dispatchActor(t, r, f.env(), a, "powerattack on")
	if a.active {
		t.Error("stance entered without the Power Attack feat")
	}
	if got := a.lastLine(); !strings.Contains(got, "don't know") {
		t.Errorf("refusal message = %q", got)
	}
}

func TestPowerAttack_OffLeavesStance(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newPowerAttackActor("Alice", "p-1", f.room, true)
	a.active = true

	dispatchActor(t, r, f.env(), a, "powerattack off")
	if a.active {
		t.Error("powerattack off did not clear the stance")
	}
}

func TestPowerAttack_BadArgShowsUsage(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newPowerAttackActor("Alice", "p-1", f.room, true)

	dispatchActor(t, r, f.env(), a, "powerattack banana")
	if got := a.lastLine(); !strings.Contains(got, "Usage") {
		t.Errorf("bad-arg message = %q, want usage", got)
	}
}

// A plain testActor (no controller surface) gets a clean refusal.
func TestPowerAttack_NonControllerRefuses(t *testing.T) {
	f := newKillFixture(t)
	r := newRegistry(t)
	a := newNamedTestActor("Plain", "p-1", f.room)

	dispatchActor(t, r, f.env(), a, "powerattack on")
	if got := a.lastLine(); !strings.Contains(got, "can't fight") {
		t.Errorf("non-controller message = %q", got)
	}
}
