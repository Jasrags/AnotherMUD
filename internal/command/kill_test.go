package command_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// killFixture builds a fully-wired environment: consider's room +
// store + placement, plus a combat.Manager whose locator carries
// the attacker, target mob, and any extra named combatants the test
// added.
type killFixture struct {
	*considerFixture
	mgr     *combat.Manager
	locator combat.MapLocator
}

func newKillFixture(t *testing.T) *killFixture {
	t.Helper()
	cf := newConsiderFixture(t)
	loc := combat.MapLocator{}
	// Pre-register the guard so Manager.Engage's name-lookup yields
	// a meaningful event payload.
	loc[cf.guard.CombatantID()] = cf.guard
	mgr := combat.NewManager(loc, nil)
	return &killFixture{considerFixture: cf, mgr: mgr, locator: loc}
}

func (f *killFixture) env() command.Env {
	e := f.considerFixture.env()
	e.Combat = f.mgr
	return e
}

// registerCombatant adds c to the locator so Manager can resolve its
// name. Used by tests that engage a second target.
func (f *killFixture) registerCombatant(c combat.Combatant) {
	f.locator[c.CombatantID()] = c
}

// dispatchActor is a kill-test variant of dispatch that takes a
// command.Actor directly — needed because kill's Combatant assertion
// fails when the inner *testActor is passed instead of the wrapping
// *combatActor.
func dispatchActor(t *testing.T, r *command.Registry, env command.Env, a command.Actor, line string) {
	t.Helper()
	if err := r.Dispatch(context.Background(), env, a, line); err != nil {
		t.Fatalf("dispatch %q: %v", line, err)
	}
}

func TestKill_NoArgPrompts(t *testing.T) {
	f := newKillFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	f.registerCombatant(a)
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "kill")
	if got := a.lastLine(); !strings.Contains(got, "Kill whom") {
		t.Errorf("no-arg kill = %q, want prompt", got)
	}
}

func TestKill_SelfRefused(t *testing.T) {
	f := newKillFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	f.registerCombatant(a)
	r := newRegistry(t)
	for _, target := range []string{"self", "me", "Alice"} {
		dispatchActor(t, r, f.env(), a, "kill "+target)
		if got := a.lastLine(); !strings.Contains(got, "yourself") {
			t.Errorf("kill %q = %q, want self-refusal", target, got)
		}
	}
	if f.mgr.InCombat(a.CombatantID()) {
		t.Error("self-kill engaged the actor")
	}
}

func TestKill_MobByKeyword_EngagesAndAnnounces(t *testing.T) {
	f := newKillFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	f.registerCombatant(a)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "kill guard")

	// First-person line to the attacker.
	if got := a.lastLine(); !strings.Contains(got, "You attack a village guard") {
		t.Errorf("attacker line = %q, want 'You attack a village guard!'", got)
	}
	// Manager state: both sides hold each other.
	if !f.mgr.InCombat(a.CombatantID()) {
		t.Error("attacker not in combat after kill")
	}
	opps := f.mgr.OpponentsOf(a.CombatantID())
	if len(opps) != 1 || opps[0] != f.guard.CombatantID() {
		t.Errorf("attacker opponents = %v, want [guard]", opps)
	}
	guardOpps := f.mgr.OpponentsOf(f.guard.CombatantID())
	if len(guardOpps) != 1 || guardOpps[0] != a.CombatantID() {
		t.Errorf("guard opponents = %v, want [alice]", guardOpps)
	}
}

func TestKill_MissingTargetMessage(t *testing.T) {
	f := newKillFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	f.registerCombatant(a)
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "kill dragon")
	if got := a.lastLine(); !strings.Contains(got, "don't see them") {
		t.Errorf("missing target = %q, want 'don't see them here'", got)
	}
	if f.mgr.InCombat(a.CombatantID()) {
		t.Error("attacker engaged something despite missing target")
	}
}

func TestKill_AlreadyEngagedMessage(t *testing.T) {
	f := newKillFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	f.registerCombatant(a)
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "kill guard")
	// Second attempt should surface the "already fighting" message,
	// not re-emit the attack lines.
	dispatchActor(t, r, f.env(), a, "kill guard")
	if got := a.lastLine(); !strings.Contains(got, "already fighting") {
		t.Errorf("repeat kill = %q, want already-fighting message", got)
	}
	// Manager still has exactly one engagement between them.
	if got := len(f.mgr.OpponentsOf(a.CombatantID())); got != 1 {
		t.Errorf("attacker opponents len = %d, want 1 (no duplicate)", got)
	}
}

func TestKill_PlayerViaLocator(t *testing.T) {
	f := newKillFixture(t)
	alice := newCombatActor("Alice", "p-1", f.room)
	bob := newCombatActor("Bob", "p-2", f.room)
	f.registerCombatant(alice)
	f.registerCombatant(bob)

	env := f.env()
	env.Locator = locatorFunc(func(_ world.RoomID, name string) command.Actor {
		if strings.EqualFold(name, "Bob") {
			return bob
		}
		return nil
	})
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, alice, "kill Bob"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "You attack Bob") {
		t.Errorf("attacker line = %q, want 'You attack Bob!'", got)
	}
	if !f.mgr.InCombat(alice.CombatantID()) || !f.mgr.InCombat(bob.CombatantID()) {
		t.Error("neither side ended in combat after kill Bob")
	}
}

func TestKill_NoCombatEnvRefuses(t *testing.T) {
	f := newKillFixture(t)
	a := newCombatActor("Alice", "p-1", f.room)
	env := f.env()
	env.Combat = nil
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "kill guard"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "can't attack right now") {
		t.Errorf("no-Combat env = %q, want refusal", got)
	}
}

// Pin that Engagement event payload reaches the sink with the
// correct names — the integration check that Manager + locator +
// command wiring agree end-to-end.
func TestKill_EmitsEngagementEvent(t *testing.T) {
	cf := newConsiderFixture(t)
	a := newCombatActor("Alice", "p-1", cf.room)
	loc := combat.MapLocator{
		cf.guard.CombatantID(): cf.guard,
		a.CombatantID():        a,
	}
	sink := &recordingSink{}
	mgr := combat.NewManager(loc, sink)

	env := cf.env()
	env.Combat = mgr
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "kill guard")

	if len(sink.engaged) != 1 {
		t.Fatalf("Engagement events = %d, want 1", len(sink.engaged))
	}
	e := sink.engaged[0]
	if e.AttackerName != "Alice" || e.TargetName != "a village guard" {
		t.Errorf("event names = (%q, %q), want (Alice, a village guard)",
			e.AttackerName, e.TargetName)
	}
	if e.RoomID != cf.room.ID {
		t.Errorf("event room = %s, want %s", e.RoomID, cf.room.ID)
	}
}

// recordingSink + locatorFunc reuse: locatorFunc lives in
// consider_test.go (this file's package). recordingSink is package-
// local to this file because combat.EventSink consumers belong with
// their tests. Mutex-guarded so any future test that dispatches
// concurrently doesn't trip -race on the slice append.
type recordingSink struct {
	mu      sync.Mutex
	engaged []combat.Engagement
	ended   []combat.CombatEnded
}

func (r *recordingSink) OnEngagement(_ context.Context, e combat.Engagement) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engaged = append(r.engaged, e)
}

func (r *recordingSink) OnCombatEnded(_ context.Context, e combat.CombatEnded) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ended = append(r.ended, e)
}

// M7.4 auto-attack events — the kill-command tests don't engage the
// heartbeat loop, so these stay as no-op recorders. Defined to satisfy
// the widened EventSink interface.
func (r *recordingSink) OnHit(context.Context, combat.Hit)                     {}
func (r *recordingSink) OnMiss(context.Context, combat.Miss)                   {}
func (r *recordingSink) OnEvade(context.Context, combat.Evade)                 {}
func (r *recordingSink) OnVitalDepleted(context.Context, combat.VitalDepleted) {}
