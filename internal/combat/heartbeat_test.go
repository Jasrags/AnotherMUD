package combat

import (
	"context"
	"sync"
	"testing"
)

// recordingPhase captures the (phase-name, combatant) pairs that
// reach it, in invocation order. Concurrency-safe so a heartbeat
// that runs in a tick goroutine and an assertion that runs on the
// test goroutine cannot race.
type recordingPhase struct {
	mu    sync.Mutex
	calls []phaseCall
}

type phaseCall struct {
	phase string
	c     CombatantID
}

func (r *recordingPhase) makeFunc(name string) PhaseFunc {
	return func(_ context.Context, c CombatantID, _ *Manager) {
		r.mu.Lock()
		r.calls = append(r.calls, phaseCall{phase: name, c: c})
		r.mu.Unlock()
	}
}

func (r *recordingPhase) snapshot() []phaseCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]phaseCall, len(r.calls))
	copy(out, r.calls)
	return out
}

func TestHeartbeatEmptyManagerNoOp(t *testing.T) {
	mgr, _, _ := makeRig(t)
	rec := &recordingPhase{}
	hb := NewHeartbeat(mgr, Phases{AutoAttack: rec.makeFunc("auto-attack")})

	hb.Tick(context.Background(), 1)

	if calls := rec.snapshot(); len(calls) != 0 {
		t.Fatalf("expected 0 phase calls on empty manager, got %d", len(calls))
	}
}

func TestHeartbeatNilPhasesAreSkipped(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b")
	mgr.Engage(context.Background(), ids[0], ids[1], testRoom)

	hb := NewHeartbeat(mgr, Phases{})
	// All phases nil — Tick must not panic.
	hb.Tick(context.Background(), 1)
}

func TestHeartbeatPhasesRunInSpecOrder(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b")
	mgr.Engage(context.Background(), ids[0], ids[1], testRoom)

	rec := &recordingPhase{}
	hb := NewHeartbeat(mgr, Phases{
		Ability:    rec.makeFunc("ability"),
		AutoAttack: rec.makeFunc("auto-attack"),
		Effects:    rec.makeFunc("effects"),
		Wimpy:      rec.makeFunc("wimpy"),
	})

	hb.Tick(context.Background(), 1)

	calls := rec.snapshot()
	// 4 phases × 2 combatants = 8 calls.
	if len(calls) != 8 {
		t.Fatalf("want 8 phase calls, got %d", len(calls))
	}

	// Per-combatant order within a phase is unspecified (map iteration
	// order is non-deterministic). What §3 guarantees is that the
	// phase index of every call is monotonically non-decreasing.
	phaseIndex := map[string]int{
		"ability": 0, "auto-attack": 1, "effects": 2, "wimpy": 3,
	}
	last := -1
	for i, c := range calls {
		idx, ok := phaseIndex[c.phase]
		if !ok {
			t.Fatalf("call %d: unknown phase %q", i, c.phase)
		}
		if idx < last {
			t.Fatalf("call %d (phase %q, idx %d) regresses from previous idx %d; calls=%+v",
				i, c.phase, idx, last, calls)
		}
		last = idx
	}
}

func TestHeartbeatSnapshotExcludesMidRoundEngages(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b", "c", "d")
	ctx := context.Background()
	mgr.Engage(ctx, ids[0], ids[1], testRoom)

	rec := &recordingPhase{}
	// First phase engages a new pair partway through. The snapshot was
	// taken at round start, so c/d MUST NOT receive a phase call this
	// round (spec §3 "iteration is over a snapshot").
	hb := NewHeartbeat(mgr, Phases{
		Ability: func(ctx context.Context, c CombatantID, m *Manager) {
			rec.makeFunc("ability")(ctx, c, m)
			if c == ids[0] {
				m.Engage(ctx, ids[2], ids[3], testRoom)
			}
		},
		AutoAttack: rec.makeFunc("auto-attack"),
	})

	hb.Tick(ctx, 1)

	calls := rec.snapshot()
	seen := map[CombatantID]bool{}
	for _, call := range calls {
		seen[call.c] = true
	}
	if seen[ids[2]] || seen[ids[3]] {
		t.Fatalf("mid-round engaged combatants should not receive phase calls this round; calls=%+v", calls)
	}
	// a and b should still receive both phases.
	if !seen[ids[0]] || !seen[ids[1]] {
		t.Fatalf("round-start combatants missing from phase calls: %+v", calls)
	}
}

func TestHeartbeatLivenessSkipsMidRoundDisengage(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b", "c", "d")
	ctx := context.Background()
	mgr.Engage(ctx, ids[0], ids[1], testRoom)
	mgr.Engage(ctx, ids[2], ids[3], testRoom)

	rec := &recordingPhase{}
	// In the first phase, when a runs, fully disengage c/d. They were
	// in the round-start snapshot but must be skipped now that they
	// are no longer InCombat.
	hb := NewHeartbeat(mgr, Phases{
		Ability: func(ctx context.Context, c CombatantID, m *Manager) {
			rec.makeFunc("ability")(ctx, c, m)
			if c == ids[0] {
				m.DisengageAll(ctx, ids[2], testRoom)
				m.DisengageAll(ctx, ids[3], testRoom)
			}
		},
		AutoAttack: rec.makeFunc("auto-attack"),
	})

	hb.Tick(ctx, 1)

	for _, call := range rec.snapshot() {
		if call.phase != "auto-attack" {
			continue
		}
		if call.c == ids[2] || call.c == ids[3] {
			t.Fatalf("disengaged combatant %q reached auto-attack phase: calls=%+v", call.c, rec.snapshot())
		}
	}
}

func TestHeartbeatPhasePanicDoesNotAbortRound(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b")
	ctx := context.Background()
	mgr.Engage(ctx, ids[0], ids[1], testRoom)

	rec := &recordingPhase{}
	hb := NewHeartbeat(mgr, Phases{
		Ability: func(ctx context.Context, c CombatantID, m *Manager) {
			if c == ids[0] {
				panic("test panic")
			}
			rec.makeFunc("ability")(ctx, c, m)
		},
		AutoAttack: rec.makeFunc("auto-attack"),
	})

	// Must not propagate the panic.
	hb.Tick(ctx, 1)

	calls := rec.snapshot()
	// ids[1] reached ability after ids[0] panicked; both reached auto-attack.
	var abilityB, attackA, attackB bool
	for _, c := range calls {
		switch {
		case c.phase == "ability" && c.c == ids[1]:
			abilityB = true
		case c.phase == "auto-attack" && c.c == ids[0]:
			attackA = true
		case c.phase == "auto-attack" && c.c == ids[1]:
			attackB = true
		}
	}
	if !abilityB || !attackA || !attackB {
		t.Fatalf("panic in one combatant's phase aborted others; calls=%+v", calls)
	}
}
