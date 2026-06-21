package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// knockoutTestSink builds a minimal productionCombatSink for the OnVitalDepleted
// routing tests (subdual-damage §4): a real bus to observe Kill, a discard
// logger, and the knockOut hook wired to the supplied stub. mgr/locator/sessions
// stay nil — the death pipeline nil-guards them, and the test victim is a player
// with no attacker, so the killer-attribution + DisengageAll branches that would
// use them are skipped.
func knockoutTestSink(t *testing.T, knockOut func(context.Context, combat.VitalDepleted) bool) (*productionCombatSink, *int) {
	t.Helper()
	bus := eventbus.New()
	kills := 0
	bus.Subscribe(eventbus.EventKill, func(_ context.Context, ev eventbus.Event) {
		if _, ok := ev.(eventbus.Kill); ok {
			kills++
		}
	})
	sink := &productionCombatSink{
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		bus:      bus,
		knockOut: knockOut,
	}
	return sink, &kills
}

func subdualDepletion(subdual bool) combat.VitalDepleted {
	return combat.VitalDepleted{
		VictimID:   combat.NewPlayerCombatantID("victim"),
		VictimName: "the victim",
		Vital:      combat.VitalHP,
		Subdual:    subdual,
	}
}

// A SUBDUAL finishing blow routes to knockOut and suppresses the death pipeline:
// the knock-out runs and NO Kill is published (no corpse/credit).
func TestOnVitalDepleted_SubdualRoutesToKnockOut(t *testing.T) {
	knocked := 0
	sink, kills := knockoutTestSink(t, func(context.Context, combat.VitalDepleted) bool {
		knocked++
		return true
	})

	sink.OnVitalDepleted(context.Background(), subdualDepletion(true))

	if knocked != 1 {
		t.Fatalf("subdual finish should route to knockOut exactly once, got %d", knocked)
	}
	if *kills != 0 {
		t.Fatalf("a knock-out must publish no Kill, got %d", *kills)
	}
}

// A LETHAL finishing blow ignores knockOut and runs the ordinary death pipeline
// (Kill published).
func TestOnVitalDepleted_LethalRunsDeathPipeline(t *testing.T) {
	knocked := 0
	sink, kills := knockoutTestSink(t, func(context.Context, combat.VitalDepleted) bool {
		knocked++
		return true
	})

	sink.OnVitalDepleted(context.Background(), subdualDepletion(false))

	if knocked != 0 {
		t.Fatalf("a lethal finish must not call knockOut, got %d", knocked)
	}
	if *kills != 1 {
		t.Fatalf("a lethal finish must publish exactly one Kill, got %d", *kills)
	}
}

// The safe fallback (subdual-damage §4): when knockOut returns false (victim
// gone, or the unconscious template missing), a subdual finish falls through to
// the ordinary lethal death rather than leaving a 0-HP zombie.
func TestOnVitalDepleted_SubdualFallsThroughWhenKnockOutFails(t *testing.T) {
	sink, kills := knockoutTestSink(t, func(context.Context, combat.VitalDepleted) bool {
		return false // could not knock out
	})

	sink.OnVitalDepleted(context.Background(), subdualDepletion(true))

	if *kills != 1 {
		t.Fatalf("a failed knock-out must fall through to the lethal death (one Kill), got %d", *kills)
	}
}

// A nil knockOut hook (unwired/headless) leaves every death lethal — the mode is
// inert until wired.
func TestOnVitalDepleted_NilKnockOutHookIsLethal(t *testing.T) {
	sink, kills := knockoutTestSink(t, nil)

	sink.OnVitalDepleted(context.Background(), subdualDepletion(true))

	if *kills != 1 {
		t.Fatalf("with no knockOut hook a subdual finish stays lethal (one Kill), got %d", *kills)
	}
}
