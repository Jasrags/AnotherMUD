# Time and Clock — Feature Specification

**Status:** Draft · **Scope:** The real-time tick counter that
drives the simulation; the in-game time-of-day clock that runs on
top of it; the tick-handler scheduling primitive other features
use to attach periodic work; and the slow-tick observability seam ·
**Audience:** Anyone reimplementing or porting this feature in
any language.

This document describes *what* the time substrate must do, not
*how* to implement it. Specific tick rates, hours-per-day,
period boundaries, and weather-roll intervals are policy and
live outside this spec.

---

## 1. Overview

Tapestry has two related but distinct timekeeping mechanisms:

1. **The simulation tick** — a monotonic counter incremented
   once per game-loop iteration. Used for cooldowns, scheduled
   delays, spawn-tracking, autosave cadence, and anything else
   that thinks in "simulation seconds".
2. **The in-game clock** — a 24-hour cycle that advances one
   in-game hour every N simulation ticks (N is configurable).
   Used for time-of-day flavor: ambient messages, weather rolls,
   shop opening hours, nocturnal mob behavior.

Both run inside the same game loop, advanced by the same
tick-handler pipeline. The game loop exposes a primitive —
`RegisterTickHandler(name, intervalTicks, handler)` — that lets
every other feature attach cadence-driven work without
implementing its own timer.

### Core concepts

- **Tick** — one iteration of the game loop. The wall-clock
  duration of a tick is determined by the configured tick rate
  (today: 100 ms, i.e. 10 ticks per second).
- **Tick count** — the monotonic count of ticks since process
  start. Exposed via two services (§2.2) so consumers can pick
  the one that suits them.
- **In-game hour** — an integer 0..23 advancing on a configured
  tick interval. Wraps to 0 when it would exceed 23 and
  increments the day count.
- **In-game day count** — the number of times the hour has
  wrapped past 23 since process start. Not persisted today.
- **Time period** — a named bucket of the 24-hour cycle: Dawn,
  Day, Dusk, or Night. Determined by configured boundary hours.
- **Tick handler** — a registered `(name, intervalTicks,
  action)` triple that the game loop invokes when
  `currentTick % intervalTicks == 0`.
- **Slow tick** — a tick whose total elapsed wall time
  exceeded a configured threshold. Reported via an
  observability event so operators can investigate.

### Goals

1. Provide a single monotonic tick clock the engine and
   content can rely on.
2. Provide an in-game time-of-day clock layered on the tick
   clock with stable, observable transitions.
3. Provide a shared tick-handler primitive other features
   use; the loop owns the loop, not each feature.
4. Surface slow ticks as observable events for operators.

### Non-goals

- Wall-clock time / calendar dates. The day count is a
  monotonic integer; there is no "year", "month", or weekday
  concept in the engine.
- Real-time scheduling outside the tick loop (cron, intervals
  decoupled from simulation). Everything pauses if the loop
  pauses.
- Sub-tick precision. The smallest time unit the simulation
  reasons about is one tick.
- Catch-up after a stall. If the loop pauses for several
  seconds, no catch-up ticks are issued — the simulation
  resumes at the next scheduled tick.

---

## 2. The tick clock

### 2.1 Tick rate

The tick rate is configured as the number of milliseconds
between ticks. The game loop derives the ticks-per-second
value (`1000 / tickRateMs`) and uses it to build the
`TickTimer` (§2.2). The default value is 100 ms (10 Hz); this
sets the granularity for every cooldown and timer in the
system.

The tick rate is fixed at startup. Changing it at runtime is
out of scope.

### 2.2 Two services, one count

Two services expose the tick count:

- **`GameLoop.TickCount`** — set and incremented by the game
  loop itself. This is the canonical "what tick are we on"
  read.
- **`TickTimer`** — a thin wrapper carrying the configured
  `TicksPerSecond` and offering conversion helpers:
  - `SecondsToTicks(seconds)` — convert a wall-clock duration
    to ticks.
  - `TicksToSeconds(ticks)` — the inverse.
  - `Advance()` — manually advance the wrapper's internal
    counter (called by the loop's `tick-timer` handler each
    tick so its `CurrentTick` stays in sync with
    `GameLoop.TickCount`).
  - `CurrentTick` — the wrapper's counter.

Having two services is a historical artifact: the `TickTimer`
exists to expose conversion helpers as a small focused
dependency, while `GameLoop.TickCount` is the source of truth.
Consumers should prefer `TickTimer` when they need the helpers
and `GameLoop.TickCount` when they only need the count.

Both counts MUST be equal at the end of every tick. If a
feature reads `TickTimer.CurrentTick` mid-tick before the
`tick-timer` handler has run, it will see `GameLoop.TickCount
- 1`. This is currently unspecified — see open questions.

### 2.3 Conversion helpers

The conversion helpers exist so callers don't carry the
tick-rate constant themselves. The shape:

- `SecondsToTicks(0.5)` → ticks-per-half-second.
- `TicksToSeconds(300)` → wall-clock seconds for 300 ticks.

`SecondsToTicks` returns a `long`. Callers can compose:

```
fireTick = now + timer.SecondsToTicks(delaySeconds)
```

This is the canonical idiom for "wake me up in X seconds"
deadlines in the mob command queue, autosave cadence, etc.

**Acceptance criteria**

- [ ] Tick count is monotonic and never resets during a
      process lifetime.
- [ ] `TickTimer` and `GameLoop` tick counts converge to the
      same value at the end of every tick.
- [ ] `SecondsToTicks` and `TicksToSeconds` are exact inverses
      modulo integer truncation.
- [ ] The tick rate is fixed at startup; no runtime change
      mechanism is exposed.

---

## 3. The in-game clock

### 3.1 Hour advancement

The game clock holds an `int CurrentHour` initialized to 0 and
a `long DayCount` initialized to 0. On each call to its
`Tick()` method (driven by the loop's `game-clock` handler at
cadence 1, §4.2), the clock:

1. Increments its internal tick counter.
2. If the counter is not a multiple of the configured
   `TicksPerGameHour`, return without effect.
3. Otherwise:
   - Capture the current period (for comparison after the
     hour advances).
   - Increment `CurrentHour`. If it exceeds 23, wrap to 0 and
     increment `DayCount`.
   - Recompute `CurrentPeriod` from the new hour (§3.2).
   - If the new period differs from the captured one, emit a
     `time.period.change` event.
   - Always emit a `time.hour.change` event.

The clock advances exactly one hour per `TicksPerGameHour`
ticks. There is no concept of "minutes" — the hour is the
finest granularity the in-game clock exposes.

### 3.2 Period determination

The four time periods (Dawn, Day, Dusk, Night) are determined
by an ordered array of four boundary hours:

```
boundaries = [dawn_start, day_start, dusk_start, night_start]
```

Default values are `[5, 8, 18, 20]`, meaning:

- Night spans 20:00 - 04:59 (wrapping across midnight).
- Dawn spans 5:00 - 7:59.
- Day spans 8:00 - 17:59.
- Dusk spans 18:00 - 19:59.

Lookup logic walks the boundaries top-down:

1. If hour ≥ night_start → Night.
2. Else if hour ≥ dusk_start → Dusk.
3. Else if hour ≥ day_start → Day.
4. Else if hour ≥ dawn_start → Dawn.
5. Else → Night (the pre-dawn hours).

The boundaries MUST be strictly ascending and SHOULD cover
the 0..23 range. The clock does not validate them — content
ships sane defaults and operators override at their own risk.

### 3.3 Hour-change event

`time.hour.change` is emitted every time `CurrentHour`
advances, regardless of whether the period changed. The
payload carries:

- `hour` — the new hour (0..23).
- `period` — the new period name, lowercased.
- `dayCount` — the current day count.

Consumers include the weather service (rolling at a
configured hour interval; see `docs/specs/world-rooms-
movement.md` §6.2) and any content that wants per-hour flavor.

### 3.4 Period-change event

`time.period.change` is emitted ONLY on a real transition.
Two consecutive hour advances within the same period produce
two hour-change events and zero period-change events. The
payload:

- `period` — the new period name, lowercased.
- `previousPeriod` — the prior period name, lowercased.
- `hour` — the new hour.

Consumers include the weather service (ambient time messages
sent to exposed rooms) and any content that wants
sunrise / sunset / nightfall flavor.

### 3.5 Initial state

`CurrentHour` starts at 0 and `CurrentPeriod` is computed
from hour 0. With the default boundaries, hour 0 falls into
Night, so the clock boots in Night. The first
`time.period.change` event therefore fires when the clock
crosses into Dawn (default: at hour 5 of day 0).

`DayCount` starts at 0 and increments on the hour 23 → 0
wrap.

### 3.6 Persistence

In-game time is NOT persisted across server restarts. Every
restart begins at hour 0, day 0. This is a deliberate
simplification — flagged as an open question.

**Acceptance criteria**

- [ ] One in-game hour advances per `TicksPerGameHour`
      simulation ticks.
- [ ] `CurrentHour` wraps 23 → 0 and increments `DayCount` on
      the wrap.
- [ ] `time.hour.change` fires on every hour advance.
- [ ] `time.period.change` fires only on actual period
      transitions, never on hour advances within the same
      period.
- [ ] Period determination follows the boundary-array
      top-down rule.

---

## 4. Tick-handler scheduling

### 4.1 Registration

The game loop exposes:

- `RegisterTickHandler(name, intervalTicks, action)` —
  register a per-tick callback. The handler runs on every
  tick whose count is a multiple of `intervalTicks`. An
  interval of 1 means "every tick"; an interval of 10 means
  "every tenth tick"; etc.
- `SetPreTickAction(action)` — a single action that runs
  before any handlers each tick. Used by the world feature
  to swap its tag-index buffers (see `docs/specs/world-rooms-
  movement.md` §3.4).

Handler names MUST be unique per registration session. The
list is registration order; the loop iterates in that order.

### 4.2 Canonical handler set

The reference implementation registers this set of handlers
during bootstrap (in the documented order):

| Name | Interval | Owner |
|---|---:|---|
| (pre-tick) | n/a | world tag-buffer swap |
| `area-tick` | 1 | area-tick service (see world spec §6) |
| `game-clock` | 1 | this feature |
| `tick-timer` | 1 | this feature |
| `mob-ai` | 10 | mob AI manager |
| `mob-command-queue` | 1 | mob command queue |
| `heartbeat` | 1 | combat / ability heartbeat |
| `corpse-decay` | 30 | loot / corpse pipeline (loot-and-corpses §7) |
| `sustenance-drain` | configured | sustenance pipeline |
| `autosave` | configured | player persistence |
| `regen` | 30 | vital regen |
| `gmcp-vitals-flush` | 1 | GMCP outbound batching |

The order matters: the pre-tick swap MUST happen before any
handler reads the world's tag indices; `game-clock` MUST
advance before subscribers to its events fire (they fire
synchronously inside the handler call); `tick-timer.Advance`
MUST happen at a known position so its `CurrentTick` stays
in sync with the loop's count.

Adding a new handler does not require modifying this list
in spec; the contract is the registration API, not the
specific list. The list is informative — it tells operators
what their server actually runs.

### 4.3 Handler isolation

A handler's exception MUST be caught by the loop, logged
with the handler name, and not propagated. One misbehaving
handler must not stop the simulation. (The reference
implementation wraps each handler in a tracing span; an
exception thrown out of the action is caught at the loop
level.)

### 4.4 Cadence semantics

The cadence is "every Nth tick", computed as
`tickCount % N == 0`. This means:

- All handlers with `interval = N` fire on the same ticks
  (the Nth, 2Nth, 3Nth, …).
- They fire in registration order on those ticks.
- A handler registered after startup misses ticks 0..K
  where K is the registration moment; that's intentional
  (handlers register at boot, not during play).

The very first tick after registration MAY or MAY NOT fire
a handler depending on whether tick 0 is considered a
multiple of N. Implementations may differ; both are
acceptable as long as the cadence is stable thereafter.

**Acceptance criteria**

- [ ] Handlers register by name and interval.
- [ ] The pre-tick action runs before any handler each tick.
- [ ] Handlers fire when `tickCount % intervalTicks == 0`.
- [ ] Handler exceptions are caught and logged; the loop
      continues.
- [ ] The game-clock and tick-timer handlers are both at
      interval 1, registered before any other handler that
      reads `CurrentTick`.

---

## 5. Slow-tick observability

The game loop measures the wall-clock duration of each tick.
When the duration exceeds a configured threshold (today
sourced from `telemetry.admin_channel.slow_tick_threshold_ms`),
the loop fires an observability event:

`OnSlowTick(tickCount, totalMs, eventQueueMs, commandsMs,
handlersMs)`

The breakdown lets operators identify the offending phase:

- **Event-queue time** — how long the system-event queue took
  to drain.
- **Commands time** — how long player command routing took.
- **Handlers time** — total time across all tick handlers.

The reference implementation routes the event to the admin
channel (via the telemetry feature's tag-based routing) so
admins logged in to the channel see the report. It also
emits OpenTelemetry metrics for offline analysis.

Setting the threshold to a value the system can never breach
(very high) effectively disables the alert; setting it very
low produces frequent noise.

**Acceptance criteria**

- [ ] The threshold is configurable and consulted on every
      tick.
- [ ] A tick whose total exceeds the threshold fires the
      slow-tick event with the four-component breakdown.
- [ ] The slow-tick event fires independent of which phase
      caused the delay (the breakdown identifies it).

---

## 6. Observable events

| Event | When |
|---|---|
| time.hour.change | every in-game hour advance (§3.3) |
| time.period.change | every period transition (§3.4) |
| (callback) OnSlowTick | a tick exceeded the threshold (§5) |

`time.*` events are bus events consumable by any subscriber.
`OnSlowTick` is a direct callback on the game loop, not a bus
event; observers wire to it explicitly. (Today the admin
channel router wires it; bus-ifying would let scripts hook
it too.)

Tick handlers themselves are NOT observable as events —
they're synchronous callbacks. Features that want to publish
"I ran my pulse" do so from inside their handler (mob AI
publishes `mob.ai.tick`, for example).

**Acceptance criteria**

- [ ] The two `time.*` events fire with the documented
      payloads.
- [ ] `OnSlowTick` fires with all four timing components.

---

## 7. Configuration surface

The following are externally configurable and not fixed by
this spec.

| Policy | Where it applies |
|---|---|
| Tick rate in ms (default 100 → 10 Hz) | §2.1 |
| Ticks per in-game hour (default 600) | §3.1 |
| Period boundaries (default [5, 8, 18, 20]) | §3.2 |
| Slow-tick threshold in ms | §5 |
| Per-handler interval | §4.1 |

---

## 8. Open questions / future work

- **In-game time is not persisted.** Every restart begins at
  hour 0, day 0. Players who depend on time-of-day mechanics
  (nocturnal mob spawns, shop opening hours) effectively
  reset their world's clock on every restart. Persistence of
  `CurrentHour` and `DayCount` alongside player saves would
  be straightforward.
- **`TickTimer` lag relative to `GameLoop.TickCount`.** When
  the tick-timer handler hasn't run yet on the current tick,
  `TickTimer.CurrentTick` reads as `GameLoop.TickCount - 1`.
  Consumers that mix the two sources see off-by-one races.
  Removing the `TickTimer` counter (defer to
  `GameLoop.TickCount` for the "what tick are we on" read,
  keep `TickTimer` only for the conversion helpers) would
  eliminate the gap.
- **Period boundaries are not validated.** Configuration
  with non-ascending or out-of-range boundaries silently
  produces nonsense period assignments. A startup validation
  pass would fail-fast.
- **No catch-up after stall.** If the loop pauses for a
  human-scale duration (e.g. GC pause, OS hang, debugger),
  the next tick fires at wall-clock-now + tickRateMs. The
  intervening "missed" ticks are simply skipped. This is
  the right behavior for most things (you don't want to
  fire 10000 backed-up regen pulses at once) but it does
  mean an in-game clock running behind wall-clock realism
  during a stall is silently lost.
- **`OnSlowTick` is a callback, not a bus event.** Wiring it
  to the bus would let scripts and dashboards subscribe
  uniformly with the rest of the engine.
- **No "is it day / night" predicate.** Consumers compare
  `CurrentPeriod` to enum values themselves. A pair of
  helper queries (`IsDaytime` / `IsNighttime`) on the clock
  would be a small ergonomics win.
- **Day count is unbounded.** It grows forever and is never
  clamped or wrapped. After a long-running server it
  becomes a large number that some renderers (e.g. older
  GMCP clients) may not handle.
- **Period boundary array is positional.** The four values
  are interpreted by position (dawn_start, day_start, etc.).
  A named map would be clearer in YAML.

---

<!-- Generated: 2026-05-21 · Scope: GameClock + TickTimer + GameLoop tick-handler registration + TickHandlerModule wiring · Spec style: narrative + acceptance criteria · Detail level: behavior only -->
