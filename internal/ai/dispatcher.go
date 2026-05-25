package ai

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// Dispatcher walks the live mob set each tick and invokes each
// mob's registered behavior. Spec mobs-ai-spawning §4.3.
//
// The mob set comes from Store.GetByTag(entities.TagMob): every
// MobInstance carries the synthetic TagMob applied at instantiation,
// so iterating the read-side tag bucket is a single map lookup
// followed by a slice walk. No bus subscription, no parallel mob
// list — the store is already the source of truth.
//
// Active-vs-inactive area filtering (spec §4.1) is NOT implemented
// yet — every mob is dispatched every tick. With ≤10 mobs the perf
// cost is irrelevant; the filter lands when room renderer scaling
// or area-reset cycles arrive.
type Dispatcher struct {
	reg       *Registry
	deps      Deps
	evaluator *Evaluator // optional; non-nil clears per-tick dedup at tick start
}

// NewDispatcher wires the dispatcher with the registry it dispatches
// against and the shared dependency bundle behaviors will receive.
// Deps.Rand may be nil; the constructor supplies a process-wide
// seeded default so Tick never has to mutate dispatcher state on the
// hot path.
func NewDispatcher(reg *Registry, deps Deps) *Dispatcher {
	if deps.Rand == nil {
		// Seed from wall-clock nanos so two processes started back to
		// back don't see identical wander sequences. Two-arg PCG seed:
		// state and stream, distinct values for independence.
		now := uint64(time.Now().UnixNano())
		deps.Rand = rand.New(rand.NewPCG(now, now^0x9e3779b97f4a7c15))
	}
	return &Dispatcher{reg: reg, deps: deps}
}

// AttachEvaluator wires the disposition evaluator so Tick can reset
// its per-tick dedup cache at the top of each cadence (spec §5.2).
// Optional: passing nil is allowed (e.g. ai-only tests). Called by
// bootstrap after both objects exist.
func (d *Dispatcher) AttachEvaluator(e *Evaluator) {
	d.evaluator = e
}

// Tick is the per-cadence handler registered against tick.Loop. It
// reads the mob set from the store and invokes each mob's behavior.
// Errors are logged and ignored so one buggy behavior cannot stall
// the others (spec §4.3 implicit contract: "behavior failure is a
// warning, not a fatal").
//
// tickCount is unused today; later slices may gate cadence-sensitive
// behaviors on it. Tick does NOT mutate any Dispatcher field — the
// Rand source was decided at NewDispatcher time so this method is
// safe to call from any goroutine (in practice it's called only
// from the tick loop's single handler-pump goroutine, but the
// invariant is preserved against future re-entry).
func (d *Dispatcher) Tick(ctx context.Context, tickCount uint64) {
	logger := logging.From(ctx).With(slog.String("event", "ai.tick"))

	// Per-tick disposition dedup is cleared first so any reaction
	// fired during this tick (e.g. via OnMobEntered called from a
	// wander) starts from a clean slate. Spec mobs-ai-spawning §5.2
	// step 1.
	if d.evaluator != nil {
		d.evaluator.ResetTick()
	}

	mobs := d.deps.Store.GetByTag(entities.TagMob)
	for _, e := range mobs {
		m, ok := e.(*entities.MobInstance)
		if !ok {
			// Tag bucket should only ever hold MobInstance, but a
			// future tag-clash could land something here. Skip
			// rather than panic on the player-visible tick goroutine.
			continue
		}
		name, _ := m.Properties()[entities.PropBehavior].(string)
		if name == "" {
			continue
		}
		fn, err := d.reg.Get(name)
		if err != nil {
			logger.Warn("unknown behavior",
				slog.String("mob_id", string(m.ID())),
				slog.String("behavior", name))
			continue
		}
		if err := fn(ctx, m, d.deps); err != nil {
			logger.Warn("behavior error",
				slog.String("mob_id", string(m.ID())),
				slog.String("behavior", name),
				slog.Any("err", err))
		}
	}
}
