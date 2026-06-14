package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// recordingCastNotifier captures OnCastInterrupted emissions for assertions.
type recordingCastNotifier struct {
	began       []progression.CastBeganEvent
	interrupted []progression.CastInterruptedEvent
}

func (n *recordingCastNotifier) OnCastBegan(_ context.Context, ev progression.CastBeganEvent) {
	n.began = append(n.began, ev)
}
func (n *recordingCastNotifier) OnCastInterrupted(_ context.Context, ev progression.CastInterruptedEvent) {
	n.interrupted = append(n.interrupted, ev)
}

func newInterruptSink(casts *progression.CastTracker, notify progression.CastNotifier) *productionCombatSink {
	return &productionCombatSink{
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		nameOf:     func(id string) string { return id },
		casts:      casts,
		castNotify: notify,
		// sessions/locator/entities left nil — tell/announce no-op without a
		// session manager, which is all this wiring test needs.
	}
}

// A blow landing on a target mid-weave aborts the cast and tells the caster.
func TestCombatSink_OnHitInterruptsMidCast(t *testing.T) {
	casts := progression.NewCastTracker()
	notify := &recordingCastNotifier{}
	sink := newInterruptSink(casts, notify)

	casts.Begin("alice", progression.Cast{AbilityID: "bonds-of-air", AbilityName: "Bonds of Air", Remaining: 2})

	sink.OnHit(context.Background(), combat.Hit{
		AttackerID: combat.NewMobCombatantID("boar-1"),
		TargetID:   combat.NewPlayerCombatantID("alice"),
		Damage:     4,
		DamageType: combat.DamageTypePhysical,
	})

	if casts.IsCasting("alice") {
		t.Fatal("a hit on the caster should have interrupted the in-flight weave")
	}
	if len(notify.interrupted) != 1 {
		t.Fatalf("want one interrupt notification, got %d", len(notify.interrupted))
	}
	got := notify.interrupted[0]
	if got.SourceID != "alice" || got.AbilityID != "bonds-of-air" || got.Cause != "hit" {
		t.Fatalf("interrupt event = %+v; want alice/bonds-of-air/hit", got)
	}
}

// interruptCast carries the disruption cause through to the notification, so
// the movement path ("moved") reads distinctly from a combat hit ("hit").
func TestCombatSink_InterruptCastCarriesCause(t *testing.T) {
	casts := progression.NewCastTracker()
	notify := &recordingCastNotifier{}
	sink := newInterruptSink(casts, notify)

	casts.Begin("alice", progression.Cast{AbilityID: "warding", AbilityName: "Warding", Remaining: 2})
	sink.interruptCast(context.Background(), combat.NewPlayerCombatantID("alice"), "moved")

	if casts.IsCasting("alice") {
		t.Fatal("interruptCast should clear the in-flight weave")
	}
	if len(notify.interrupted) != 1 || notify.interrupted[0].Cause != "moved" {
		t.Fatalf("interrupt cause = %+v; want one with cause \"moved\"", notify.interrupted)
	}
}

// A hit on a target who is NOT casting is a harmless no-op: nothing to
// interrupt, no spurious notification.
func TestCombatSink_OnHitNonCasterNoOp(t *testing.T) {
	casts := progression.NewCastTracker()
	notify := &recordingCastNotifier{}
	sink := newInterruptSink(casts, notify)

	sink.OnHit(context.Background(), combat.Hit{
		AttackerID: combat.NewMobCombatantID("boar-1"),
		TargetID:   combat.NewPlayerCombatantID("bob"),
		Damage:     3,
		DamageType: combat.DamageTypePhysical,
	})

	if len(notify.interrupted) != 0 {
		t.Fatalf("a hit on a non-caster must not emit an interrupt: %+v", notify.interrupted)
	}
}

// An incapacitating condition landing on a mid-cast entity drops the weave
// (cause "stunned"); a non-incapacitating one leaves it channeling.
func TestCombatSink_OnEffectAppliedInterruptsWhenIncapacitated(t *testing.T) {
	cases := []struct {
		name          string
		incapacitated bool
		wantInterrupt bool
	}{
		{"stunned breaks the weave", true, true},
		{"a non-incapacitating condition does not", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			casts := progression.NewCastTracker()
			notify := &recordingCastNotifier{}
			sink := newInterruptSink(casts, notify)
			sink.incapacitated = func(string) bool { return tc.incapacitated }

			casts.Begin("alice", progression.Cast{AbilityID: "firebolt", AbilityName: "Firebolt", Remaining: 2})
			sink.onEffectApplied(context.Background(), "alice")

			if casts.IsCasting("alice") == tc.wantInterrupt {
				t.Fatalf("incapacitated=%v: IsCasting=%v, wantInterrupt=%v", tc.incapacitated, casts.IsCasting("alice"), tc.wantInterrupt)
			}
			if tc.wantInterrupt {
				if len(notify.interrupted) != 1 || notify.interrupted[0].Cause != "stunned" {
					t.Fatalf("want one interrupt with cause \"stunned\", got %+v", notify.interrupted)
				}
			} else if len(notify.interrupted) != 0 {
				t.Fatalf("no interrupt expected, got %+v", notify.interrupted)
			}
		})
	}
}

// With no incapacitation predicate wired the stun-interrupt path is inert.
func TestCombatSink_OnEffectAppliedNilPredicateNoOp(t *testing.T) {
	casts := progression.NewCastTracker()
	notify := &recordingCastNotifier{}
	sink := newInterruptSink(casts, notify) // incapacitated left nil
	casts.Begin("alice", progression.Cast{AbilityID: "firebolt", AbilityName: "Firebolt", Remaining: 2})

	sink.onEffectApplied(context.Background(), "alice")

	if !casts.IsCasting("alice") {
		t.Fatal("a nil incapacitation predicate must not interrupt")
	}
}

// The caster who LANDS a hit (attacker) is not the one interrupted — only the
// victim's cast is at risk. A mid-cast attacker keeps weaving (their own swing
// shouldn't disrupt them).
func TestCombatSink_OnHitDoesNotInterruptAttacker(t *testing.T) {
	casts := progression.NewCastTracker()
	notify := &recordingCastNotifier{}
	sink := newInterruptSink(casts, notify)

	casts.Begin("alice", progression.Cast{AbilityID: "firebolt", AbilityName: "Firebolt", Remaining: 1})

	sink.OnHit(context.Background(), combat.Hit{
		AttackerID: combat.NewPlayerCombatantID("alice"), // alice is the attacker, mid-cast
		TargetID:   combat.NewMobCombatantID("boar-1"),
		Damage:     5,
		DamageType: combat.DamageTypePhysical,
	})

	if !casts.IsCasting("alice") {
		t.Fatal("the attacker's own hit must not interrupt their cast")
	}
	if len(notify.interrupted) != 0 {
		t.Fatalf("no interrupt expected when the caster is the attacker: %+v", notify.interrupted)
	}
}
