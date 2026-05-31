package session

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// fakeCombatant is a minimal Combatant for the locator. Only the
// fields the flush path reads (Name + Vitals) carry meaningful
// data; Stats is unused by Char.Combat.
type fakeCombatant struct {
	id     combat.CombatantID
	name   string
	vitals *combat.Vitals
}

func (f *fakeCombatant) CombatantID() combat.CombatantID { return f.id }
func (f *fakeCombatant) Name() string                    { return f.name }
func (f *fakeCombatant) Vitals() *combat.Vitals          { return f.vitals }
func (f *fakeCombatant) Stats() combat.Stats             { return combat.Stats{} }

// newCombatGmcpActor builds an actor wired to a real combat
// manager + a MapLocator the test populates with the actor +
// target combatants.
func newCombatGmcpActor(t *testing.T, playerID, displayName string) (*connActor, *gmcpFakeConn, *combat.Manager, combat.MapLocator) {
	t.Helper()
	fc := &gmcpFakeConn{fakeConn: fakeConn{id: "test-" + playerID}}
	locator := combat.MapLocator{}
	mgr := combat.NewManager(locator, nil)
	room := &world.Room{ID: "test-room", Name: "Test"}
	a := &connActor{
		id:            fc.id,
		conn:          fc,
		playerID:      playerID,
		room:          room,
		combat:        mgr,
		combatLocator: locator,
		vitals:        combat.NewVitalsAt(50, 100),
		save:          &player.Save{ID: playerID, Name: displayName, Sustenance: 100},
	}
	a.sustenance = 100
	// Register the actor itself in the locator so the Manager's
	// own engage path can resolve it (it logs combatant names).
	locator[a.CombatantID()] = &fakeCombatant{
		id:     a.CombatantID(),
		name:   displayName,
		vitals: a.vitals,
	}
	return a, fc, mgr, locator
}

func combatFrames(t *testing.T, fc *gmcpFakeConn) []gmcp.CharCombat {
	t.Helper()
	raw := fc.framesSnapshot()
	out := make([]gmcp.CharCombat, 0, len(raw))
	for _, f := range raw {
		if f.pkg != gmcp.PackageCharCombat {
			continue
		}
		var c gmcp.CharCombat
		if err := json.Unmarshal(f.payload, &c); err != nil {
			t.Fatalf("payload unmarshal: %v (raw %s)", err, f.payload)
		}
		out = append(out, c)
	}
	return out
}

func TestFlushGmcpCombat_NoSendBeforeActivation(t *testing.T) {
	a, fc, _, _ := newCombatGmcpActor(t, "p-1", "Alice")
	a.flushGmcpCombat(context.Background())
	if got := len(fc.framesSnapshot()); got != 0 {
		t.Errorf("pre-activation emitted %d frames, want 0", got)
	}
}

func TestFlushGmcpCombat_FirstFlushSendsNotInCombat(t *testing.T) {
	// A fresh actor with no engagements still emits the first
	// frame so the panel knows the initial state.
	a, fc, _, _ := newCombatGmcpActor(t, "p-1", "Alice")
	fc.setActive(true)

	a.flushGmcpCombat(context.Background())

	frames := combatFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("first flush emitted %d frames, want 1", len(frames))
	}
	if frames[0].InCombat {
		t.Errorf("InCombat = true, want false")
	}
	if frames[0].Target != "" || frames[0].TargetID != "" {
		t.Errorf("target fields populated on fresh actor: %+v", frames[0])
	}
}

func TestFlushGmcpCombat_SendsOnEngage(t *testing.T) {
	a, fc, mgr, locator := newCombatGmcpActor(t, "p-1", "Alice")
	fc.setActive(true)

	target := &fakeCombatant{
		id:     "mob:e-1",
		name:   "a village guard",
		vitals: combat.NewVitalsAt(40, 60),
	}
	locator[target.id] = target

	a.flushGmcpCombat(context.Background()) // baseline (not in combat)

	if !mgr.Engage(context.Background(), a.CombatantID(), target.id, "test-room") {
		t.Fatal("Engage returned false")
	}
	a.flushGmcpCombat(context.Background())

	frames := combatFrames(t, fc)
	if len(frames) != 2 {
		t.Fatalf("post-engage flush count = %d, want 2", len(frames))
	}
	last := frames[1]
	if !last.InCombat {
		t.Errorf("InCombat = false, want true")
	}
	if last.Target != "a village guard" {
		t.Errorf("Target = %q, want a village guard", last.Target)
	}
	if last.TargetID != "mob:e-1" {
		t.Errorf("TargetID = %q", last.TargetID)
	}
	if last.TargetHP != 40 || last.TargetMaxHP != 60 {
		t.Errorf("TargetHP/Max = %d/%d, want 40/60", last.TargetHP, last.TargetMaxHP)
	}
	if last.TargetHPPercent != 66 { // 40/60 = 66
		t.Errorf("TargetHPPercent = %d, want 66", last.TargetHPPercent)
	}
}

func TestFlushGmcpCombat_NoRedundantSendWhenUnchanged(t *testing.T) {
	a, fc, mgr, locator := newCombatGmcpActor(t, "p-1", "Alice")
	fc.setActive(true)
	target := &fakeCombatant{
		id:     "mob:e-1",
		name:   "x",
		vitals: combat.NewVitalsAt(40, 60),
	}
	locator[target.id] = target
	mgr.Engage(context.Background(), a.CombatantID(), target.id, "test-room")

	a.flushGmcpCombat(context.Background()) // baseline
	preCount := len(combatFrames(t, fc))

	a.flushGmcpCombat(context.Background())
	a.flushGmcpCombat(context.Background())
	if got := len(combatFrames(t, fc)) - preCount; got != 0 {
		t.Errorf("redundant flushes added %d frames", got)
	}
}

func TestFlushGmcpCombat_SendsOnTargetHPChange(t *testing.T) {
	a, fc, mgr, locator := newCombatGmcpActor(t, "p-1", "Alice")
	fc.setActive(true)
	target := &fakeCombatant{
		id:     "mob:e-1",
		name:   "x",
		vitals: combat.NewVitalsAt(40, 60),
	}
	locator[target.id] = target
	mgr.Engage(context.Background(), a.CombatantID(), target.id, "test-room")
	a.flushGmcpCombat(context.Background()) // baseline

	target.vitals.ApplyDamage(10) // target HP 40 → 30
	a.flushGmcpCombat(context.Background())

	frames := combatFrames(t, fc)
	last := frames[len(frames)-1]
	if last.TargetHP != 30 {
		t.Errorf("TargetHP = %d, want 30", last.TargetHP)
	}
	if last.TargetHPPercent != 50 {
		t.Errorf("TargetHPPercent = %d, want 50", last.TargetHPPercent)
	}
}

func TestFlushGmcpCombat_SendsOnDisengage(t *testing.T) {
	a, fc, mgr, locator := newCombatGmcpActor(t, "p-1", "Alice")
	fc.setActive(true)
	target := &fakeCombatant{
		id:     "mob:e-1",
		name:   "x",
		vitals: combat.NewVitalsAt(40, 60),
	}
	locator[target.id] = target
	mgr.Engage(context.Background(), a.CombatantID(), target.id, "test-room")
	a.flushGmcpCombat(context.Background()) // baseline (in combat)

	mgr.DisengageAll(context.Background(), a.CombatantID(), "test-room")
	a.flushGmcpCombat(context.Background())

	frames := combatFrames(t, fc)
	last := frames[len(frames)-1]
	if last.InCombat {
		t.Errorf("post-disengage InCombat = true, want false")
	}
	if last.Target != "" || last.TargetID != "" {
		t.Errorf("post-disengage target fields populated: %+v", last)
	}
}

func TestFlushGmcpCombat_NilLocatorEmitsFlagOnly(t *testing.T) {
	// Compose without a locator: in_combat flag works, target
	// fields stay empty (because the resolver can't run).
	fc := &gmcpFakeConn{fakeConn: fakeConn{id: "test-p-1"}}
	mgr := combat.NewManager(combat.MapLocator{}, nil)
	room := &world.Room{ID: "test-room", Name: "Test"}
	a := &connActor{
		id:       fc.id,
		conn:     fc,
		playerID: "p-1",
		room:     room,
		combat:   mgr,
		// combatLocator intentionally nil
		vitals: combat.NewVitalsAt(50, 100),
		save:   &player.Save{ID: "p-1", Name: "Alice", Sustenance: 100},
	}
	a.sustenance = 100
	fc.setActive(true)

	// Have to use a locator-aware manager to engage; build one
	// just for the engage call.
	otherLoc := combat.MapLocator{a.CombatantID(): &fakeCombatant{id: a.CombatantID(), name: "Alice", vitals: a.vitals}}
	otherLoc["mob:t"] = &fakeCombatant{id: "mob:t", name: "x", vitals: combat.NewVitalsAt(10, 10)}
	otherMgr := combat.NewManager(otherLoc, nil)
	otherMgr.Engage(context.Background(), a.CombatantID(), "mob:t", "test-room")
	// Swap into the actor's manager so InCombat sees it.
	a.combat = otherMgr

	a.flushGmcpCombat(context.Background())
	frames := combatFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("flushed %d frames, want 1", len(frames))
	}
	if !frames[0].InCombat || frames[0].TargetID != "mob:t" {
		t.Errorf("nil-locator payload = %+v", frames[0])
	}
	if frames[0].Target != "" || frames[0].TargetHP != 0 {
		t.Errorf("nil-locator should not resolve name/HP, got %+v", frames[0])
	}
}

func TestFlushGmcpCombat_NoCombatManagerIsSilent(t *testing.T) {
	fc := &gmcpFakeConn{fakeConn: fakeConn{id: "test-x"}}
	room := &world.Room{ID: "r", Name: "R"}
	a := &connActor{
		id:       "x",
		conn:     fc,
		playerID: "p-x",
		room:     room,
		// combat: intentionally nil
		vitals: combat.NewVitalsAt(50, 100),
		save:   &player.Save{ID: "p-x", Sustenance: 100},
	}
	a.sustenance = 100
	fc.setActive(true)
	a.flushGmcpCombat(context.Background())
	if got := len(fc.framesSnapshot()); got != 0 {
		t.Errorf("nil-Manager emitted %d frames, want 0", got)
	}
}

func TestFlushGmcpCombat_ShadowResetForcesResend(t *testing.T) {
	a, fc, _, _ := newCombatGmcpActor(t, "p-1", "Alice")
	fc.setActive(true)
	a.flushGmcpCombat(context.Background())
	preCount := len(combatFrames(t, fc))

	a.resetGmcpCombatShadow()
	a.flushGmcpCombat(context.Background())

	if got := len(combatFrames(t, fc)) - preCount; got != 1 {
		t.Errorf("post-reset added %d frames, want 1", got)
	}
}

func TestManagerFlushGmcpCombat_FansOutToLiveActors(t *testing.T) {
	mgr := NewManager()
	a1, fc1, _, _ := newCombatGmcpActor(t, "p-1", "Alice")
	a2, fc2, _, _ := newCombatGmcpActor(t, "p-2", "Bob")
	fc1.setActive(true)
	fc2.setActive(true)
	mgr.Add(a1)
	mgr.Add(a2)

	mgr.FlushGmcpCombat(context.Background())

	if got := len(combatFrames(t, fc1)); got != 1 {
		t.Errorf("a1 frames = %d, want 1", got)
	}
	if got := len(combatFrames(t, fc2)); got != 1 {
		t.Errorf("a2 frames = %d, want 1", got)
	}
}

func TestFlushGmcpCombat_PayloadShape(t *testing.T) {
	a, fc, mgr, locator := newCombatGmcpActor(t, "p-1", "Alice")
	fc.setActive(true)
	target := &fakeCombatant{
		id:     "mob:e-1",
		name:   "a village guard",
		vitals: combat.NewVitalsAt(25, 50),
	}
	locator[target.id] = target
	mgr.Engage(context.Background(), a.CombatantID(), target.id, "test-room")

	a.flushGmcpCombat(context.Background())

	for _, f := range fc.framesSnapshot() {
		if f.pkg != gmcp.PackageCharCombat {
			continue
		}
		s := string(f.payload)
		if strings.Contains(s, `"in_combat":true`) &&
			strings.Contains(s, `"target":"a village guard"`) &&
			strings.Contains(s, `"target_hp_percent":50`) {
			return
		}
	}
	t.Errorf("no Char.Combat frame matched expected shape; frames=%v", fc.framesSnapshot())
}

func TestFlushGmcpCombat_TargetWithNilVitalsShipsNameOnly(t *testing.T) {
	// Defends the `if vit := target.Vitals(); vit != nil` guard:
	// a Combatant implementation that returns nil Vitals (rare
	// but possible — the staticCombatant test fake in combat
	// package does this) must not crash the flusher. The payload
	// ships in_combat + target name + target_id, with HP fields
	// staying zero (and omitting via omitempty on the wire).
	a, fc, mgr, locator := newCombatGmcpActor(t, "p-1", "Alice")
	fc.setActive(true)

	nilVitalsTarget := &fakeCombatant{
		id:     "mob:ghost",
		name:   "an ethereal shade",
		vitals: nil, // explicit nil — exercises the guard
	}
	locator[nilVitalsTarget.id] = nilVitalsTarget
	mgr.Engage(context.Background(), a.CombatantID(), nilVitalsTarget.id, "test-room")

	// Must not panic.
	a.flushGmcpCombat(context.Background())

	frames := combatFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("flushed %d frames, want 1", len(frames))
	}
	got := frames[0]
	if !got.InCombat {
		t.Errorf("InCombat = false, want true")
	}
	if got.Target != "an ethereal shade" {
		t.Errorf("Target = %q, want %q", got.Target, "an ethereal shade")
	}
	if got.TargetID != "mob:ghost" {
		t.Errorf("TargetID = %q", got.TargetID)
	}
	// HP fields must remain zero (the guard short-circuited the
	// Snapshot call) and serialize as omitted on the wire.
	if got.TargetHP != 0 || got.TargetMaxHP != 0 || got.TargetHPPercent != 0 {
		t.Errorf("HP fields populated despite nil Vitals: %+v", got)
	}
}
