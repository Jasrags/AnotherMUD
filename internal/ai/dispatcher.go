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

// AttachCombat wires the combat-state gate so Tick skips behavior
// dispatch for mobs currently engaged. Called by bootstrap after
// both Dispatcher and combat.Manager exist — neither side can be
// constructed before the other without inverting another seam, so
// the late-attach pattern (mirroring AttachEvaluator) is the
// cleanest cut. Optional: passing nil disables the gate.
func (d *Dispatcher) AttachCombat(g CombatGate) {
	d.deps.Combat = g
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
		// Combat gate (M7.6 follow-up): mobs in combat are owned by
		// the round loop, not the AI tick. Skipping them here closes
		// the wander-during-fight bug where the AI tick moved the
		// mob between rounds and the auto-attack pre-flight then
		// disengaged on different-room. nil gate disables (tests
		// that don't wire combat).
		if d.deps.Combat != nil && d.deps.Combat.InCombat(m.CombatantID()) {
			// A mob already in a fight has had its grudge settled — drop any
			// lingering retaliation intent so it doesn't re-pursue after this
			// combat ends (ranged-combat §10 slice 2).
			if hasRetaliation(m) {
				clearRetaliation(m)
			}
			continue
		}

		// Retaliation (ranged-combat §10): a mob shot from the next room
		// pursues + engages its attacker, preempting its normal behavior
		// (so even a stationary or behavior-less mob comes after you).
		if tryRetaliate(ctx, m, d.deps) {
			continue
		}

		rawBehavior, _ := m.Property(entities.PropBehavior)
		name, _ := rawBehavior.(string)
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
