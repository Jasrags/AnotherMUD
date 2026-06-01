package scripting

import (
	"context"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/logging"
	lua "github.com/yuin/gopher-lua"
)

// M17.4 — the schedule primitive. engine.schedule(delayTicks, fn)
// registers a one-shot callback to run after delayTicks engine ticks
// (tick = 100ms by default). The Runtime fires due callbacks from
// Tick, which the composition root drives off the tick loop at cadence
// one. Fire-and-forget: there is no cancel handle in v1; pending
// callbacks are dropped automatically on Reload / Close.

// makeScheduleFn returns the LGFunction backing engine.schedule. Lua
// signature: engine.schedule(delayTicks number, fn function). The
// callback's due tick is lastTick + delayTicks, so it fires on the
// first Tick at or after that point. A delay below 1 is rejected (a
// callback can't fire in the past or the current tick — by the time a
// script runs, this tick's schedule pump has already passed).
func (r *Runtime) makeScheduleFn(sb *Sandbox) lua.LGFunction {
	return func(L *lua.LState) int {
		delay := L.CheckInt(1)
		fn := L.CheckFunction(2)
		if delay < 1 {
			L.ArgError(1, "schedule delay must be >= 1 tick")
			return 0
		}
		r.schedMu.Lock()
		r.scheduled = append(r.scheduled, scheduledCall{
			dueTick: r.lastTick + uint64(delay),
			sb:      sb,
			fn:      fn,
		})
		r.schedMu.Unlock()
		return 0
	}
}

// Tick advances the schedule clock to tickCount and fires every
// callback whose due tick has arrived. It satisfies tick.Handler so
// the composition root can register it at cadence one. Due callbacks
// are collected under schedMu and then invoked OUTSIDE the lock — a
// callback may itself call engine.schedule (re-arm), and a slow
// callback must not stall the queue. A re-armed callback's due tick is
// lastTick(=tickCount) + delay, so it lands on a future tick and can't
// loop within this one.
//
// Each callback runs under its sandbox lock + per-call timeout, the
// same path as a bus handler; an error (including a closed sandbox
// after a concurrent reload) is logged and the pump continues.
func (r *Runtime) Tick(ctx context.Context, tickCount uint64) {
	r.schedMu.Lock()
	r.lastTick = tickCount
	if len(r.scheduled) == 0 {
		// Idle fast path — the common case (no pending callbacks).
		r.schedMu.Unlock()
		return
	}
	var due, kept []scheduledCall
	for _, c := range r.scheduled {
		if c.dueTick <= tickCount {
			due = append(due, c)
		} else {
			kept = append(kept, c)
		}
	}
	r.scheduled = kept
	r.schedMu.Unlock()

	for _, c := range due {
		if err := c.sb.Call(ctx, c.fn, 0); err != nil {
			logging.From(ctx).Warn("scripting schedule error",
				slog.String("event", "scripting.schedule.err"),
				slog.String("pack", c.sb.PackID()),
				slog.String("script", c.sb.ScriptPath()),
				slog.Uint64("tick", tickCount),
				slog.Any("err", err))
		}
	}
}
