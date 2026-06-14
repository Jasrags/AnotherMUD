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
