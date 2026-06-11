package combat

import (
	"context"
	"testing"
)

// wimpyCombatant is a staticCombatant with a tunable wimpy threshold
// and a real Vitals pointer so the phase's HP% computation has
// something to read.
type wimpyCombatant struct {
	id        CombatantID
	name      string
	vitals    *Vitals
	threshold int
}

func (w *wimpyCombatant) CombatantID() CombatantID { return w.id }
func (w *wimpyCombatant) Name() string             { return w.name }
func (w *wimpyCombatant) Vitals() *Vitals          { return w.vitals }
func (w *wimpyCombatant) Stats() Stats             { return Stats{} }
func (w *wimpyCombatant) WimpyThreshold() int      { return w.threshold }

func TestWimpyTriggersFleeAtOrBelowThreshold(t *testing.T) {
	cfg, bus, mover, roomLoc := buildFleeRig(t)
	id := NewMobCombatantID("rat")
	wc := &wimpyCombatant{
		id:        id,
		name:      "a rat",
		vitals:    NewVitalsAt(20, 100), // 20% HP
		threshold: 30,                   // flee at <= 30%
	}
	cfg.Locator.(MapLocator)[id] = wc
	roomLoc[id] = fromRoom
	// Put the combatant in combat so the heartbeat would iterate it
	// (the phase itself doesn't check InCombat — that's the loop's
	// job — but we want the world state to be realistic).
	other := NewPlayerCombatantID("hero")
	cfg.Locator.(MapLocator)[other] = staticCombatant{id: other, name: "Hero"}
	cfg.Mgr.Engage(context.Background(), id, other, fromRoom)

	phase := NewWimpy(cfg)
	phase(context.Background(), id, cfg.Mgr, 0)

	if len(bus.flees) != 1 {
		t.Fatalf("flee events = %d, want 1", len(bus.flees))
	}
	if len(mover.moves) != 1 {
		t.Errorf("Move calls = %d, want 1", len(mover.moves))
	}
}

func TestWimpyExactlyAtThresholdStillFlees(t *testing.T) {
	cfg, bus, _, roomLoc := buildFleeRig(t)
	id := NewMobCombatantID("rat")
	wc := &wimpyCombatant{
		id: id, name: "rat",
		vitals:    NewVitalsAt(30, 100), // exactly 30%
		threshold: 30,
	}
	cfg.Locator.(MapLocator)[id] = wc
	roomLoc[id] = fromRoom

	phase := NewWimpy(cfg)
	phase(context.Background(), id, cfg.Mgr, 0)
	if len(bus.flees) != 1 {
		t.Errorf("flee events = %d, want 1 (boundary inclusive)", len(bus.flees))
	}
}

func TestWimpyAboveThresholdNoFlee(t *testing.T) {
	cfg, bus, _, roomLoc := buildFleeRig(t)
	id := NewMobCombatantID("rat")
	wc := &wimpyCombatant{
		id: id, name: "rat",
		vitals:    NewVitalsAt(80, 100), // 80%
		threshold: 30,
	}
	cfg.Locator.(MapLocator)[id] = wc
	roomLoc[id] = fromRoom

	phase := NewWimpy(cfg)
	phase(context.Background(), id, cfg.Mgr, 0)
	if len(bus.flees) != 0 {
		t.Errorf("flee events = %d, want 0", len(bus.flees))
	}
}

func TestWimpyZeroThresholdDisables(t *testing.T) {
	cfg, bus, _, roomLoc := buildFleeRig(t)
	id := NewMobCombatantID("rat")
	wc := &wimpyCombatant{
		id: id, name: "rat",
		vitals:    NewVitalsAt(1, 100), // 1% — dramatic, but threshold is 0
		threshold: 0,
	}
	cfg.Locator.(MapLocator)[id] = wc
	roomLoc[id] = fromRoom

	phase := NewWimpy(cfg)
	phase(context.Background(), id, cfg.Mgr, 0)
	if len(bus.flees) != 0 {
		t.Errorf("zero threshold should disable wimpy; got %d flee events", len(bus.flees))
	}
}

// §5.1: skip dead combatants — death flow owns them.
func TestWimpySkipsDeadCombatant(t *testing.T) {
	cfg, bus, _, roomLoc := buildFleeRig(t)
	id := NewMobCombatantID("rat")
	wc := &wimpyCombatant{
		id: id, name: "rat",
		vitals:    NewVitalsAt(0, 100),
		threshold: 50,
	}
	cfg.Locator.(MapLocator)[id] = wc
	roomLoc[id] = fromRoom

	phase := NewWimpy(cfg)
	phase(context.Background(), id, cfg.Mgr, 0)
	if len(bus.flees) != 0 || len(bus.prevented) != 0 || len(bus.failed) != 0 {
		t.Errorf("wimpy fired on dead combatant: flees=%d prevented=%d failed=%d",
			len(bus.flees), len(bus.prevented), len(bus.failed))
	}
}

// Combatants that do not implement WimpyHolder are silently skipped
// rather than causing a phase panic. This matters because
// staticCombatant (the bulk of test fixtures) lacks the threshold.
func TestWimpyIgnoresNonHolder(t *testing.T) {
	cfg, bus, _, roomLoc := buildFleeRig(t)
	id := NewMobCombatantID("rat")
	cfg.Locator.(MapLocator)[id] = staticCombatant{id: id, name: "rat"}
	roomLoc[id] = fromRoom

	phase := NewWimpy(cfg)
	phase(context.Background(), id, cfg.Mgr, 0)
	if len(bus.flees) != 0 {
		t.Error("wimpy fired for non-WimpyHolder combatant")
	}
}

// conditions §5: ForceFlee makes a full-HP combatant flee regardless of the
// wimpy threshold (a frightened victim flees each round).
func TestWimpyForceFleeOverridesHealthyHP(t *testing.T) {
	cfg, bus, mover, roomLoc := buildFleeRig(t)
	id := NewMobCombatantID("rat")
	wc := &wimpyCombatant{
		id: id, name: "a rat",
		vitals:    NewVitalsAt(100, 100), // full HP — wimpy alone would NOT flee
		threshold: 0,                     // wimpy disabled
	}
	cfg.Locator.(MapLocator)[id] = wc
	roomLoc[id] = fromRoom
	cfg.ForceFlee = func(c CombatantID) bool { return c == id }

	phase := NewWimpy(cfg)
	phase(context.Background(), id, cfg.Mgr, 0)

	if len(bus.flees) != 1 {
		t.Fatalf("force-flee events = %d, want 1 (frightened flees at full HP)", len(bus.flees))
	}
	if len(mover.moves) != 1 {
		t.Errorf("Move calls = %d, want 1", len(mover.moves))
	}
}

func TestWimpyForceFleeNilLeavesWimpyUnchanged(t *testing.T) {
	cfg, bus, _, roomLoc := buildFleeRig(t)
	id := NewMobCombatantID("rat")
	wc := &wimpyCombatant{id: id, name: "rat", vitals: NewVitalsAt(80, 100), threshold: 30}
	cfg.Locator.(MapLocator)[id] = wc
	roomLoc[id] = fromRoom
	cfg.ForceFlee = nil // unset — only the HP rule applies

	phase := NewWimpy(cfg)
	phase(context.Background(), id, cfg.Mgr, 0)
	if len(bus.flees) != 0 {
		t.Errorf("flee events = %d, want 0 (healthy, no force-flee)", len(bus.flees))
	}
}
