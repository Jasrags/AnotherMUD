# Theme C — World Depth (plan)

**Hook:** The world has state beyond rooms and exits — doors that
open, weather that changes, portals that expire, locations players
can recall to.

**Source:** `docs/THEME-AXIS-PLAN.md` §"Theme C — World Depth".
**Roadmap milestone:** M15 (see `docs/ROADMAP.md`).
**Status:** spec phase — most items have existing spec coverage,
recall needs a new spec.

---

## What the spec already says

Three of the four items have substantial existing spec coverage in
`docs/specs/world-rooms-movement.md`:

- **§5.1-§5.5 Doors + locks** — per-exit state with paired reverse-
  side sync, lock/unlock, key resolution, target text matching,
  area-reset behavior. Implementation-ready.
- **§5.6 Temporary keyword exits ("portals")** — runtime-only
  keyword exits with TTL, observable creation/expiry events, the
  cleanup tick handler. Implementation-ready.
- **§6 Weather + time ambience** — area-scoped weather zones,
  hour-driven rolls, per-state message tables, room-render
  integration. Implementation-ready (but see PD-2 below — the spec
  already picks per-area; the theme-axis PD was a checkpoint).

**Recall has no spec.** A new `docs/specs/recall.md` (or a §7
section in `world-rooms-movement.md`) needs to land before M15.3
implementation.

---

## The four items

### M15.1 — Doors + locks

**Spec:** `world-rooms-movement.md` §5.1-§5.5.
**Gap matrix:** §1.8.

Per-exit state: closed/open, locked/unlocked, key entity reference,
reverse-side paired with the matching exit on the destination room.
`open` / `close` / `lock` / `unlock` verbs. Movement attempt against
a closed door emits a "door blocked" event and fails. Area reset
restores doors to their template state.

**Shape:** medium. Spec is complete; implementation is mostly new
substrate + four verbs + room-render integration (exit listing
shows door state). ~1 week.

### M15.2 — Portals (temporary keyword exits)

**Spec:** `world-rooms-movement.md` §5.6.
**Gap matrix:** §3 "Portals / temporary exits".

Runtime-only keyword exits with TTL. Creation surface (initially
admin-only or content-only; scripting comes later). Tick handler
sweeps expired portals and emits a portal.expired event. Movement
through a portal uses the keyword exit map (room.go already
distinguishes direction-keyed from keyword-keyed exits).

**Shape:** small. ~3-5 days.

### M15.3 — Recall / return-home  ✅ SHIPPED

**Spec:** `docs/specs/recall.md` (written for this milestone).
**Gap matrix:** §3 "Recall / return-home".

Per-character return-address service: tracks the last `set recall`
point. The `recall` verb teleports to that point.

**What landed:**
- Player save v14 with the `recall` string field; v13→v14
  migration is a no-op (legacy saves load with no recall point
  bound, the documented default).
- `connActor.Recall()` / `SetRecall()` getter/setter pair
  hydrated from save at construction.
- `set recall` and `recall` verbs in `internal/command/recall.go`.
- Five bus events: `recall.set`, `recall.before` (cancellable),
  `recall.after`, `recall.no_point`, `recall.unresolved`.
- Source / destination room broadcasts ("vanishes" / "appears
  in a swirl of light"), suppressed for the same-room no-op.
- Unresolved saved id (content drift) is non-destructive: the
  save field is left alone so a future content patch can
  restore the address.

**Pre-decisions in §"Open pre-decisions" below.**

### M15.4a — Weather substrate  ✅ SHIPPED

`internal/weather` package (Zone, Registry, Service) +
`world.Room.Terrain` / `WeatherExposed` / `TimeExposed` +
`world.Area.WeatherZone` + `weather.changed` bus event. The
service exposes `HourChanged(ctx, hour)` and
`PeriodChanged(ctx, period)` as the seams the future in-game
clock subscriber will call. State machine, weighted-pick
transition, message cascade (zone-by-terrain today; room +
area overrides plug into the same resolver in M15.4b), and
eligibility gate (indoors/underground shielded unless
exposure flag flipped) all in place. Tests cover roll,
identical-state no-op, off-interval skip, SetWeather force +
no-op, period delivery, and shielding/exposure permutations.

**Out of scope here** (lands in M15.4b alongside the real
clock): YAML loader extensions, composition-root wiring of
`time.hour.change` → `Service.HourChanged`, room-render
integration with current weather state.

### M15.4 — Weather  ✅ SHIPPED (closes Theme C)

**Spec:** `world-rooms-movement.md` §6 + `time-and-clock.md` §3.
**Gap matrix:** §3 "Weather".

Shipped across four slices (M15.4a substrate already noted above;
M15.4b₁/b₂a/b₂b below):

- **M15.4b₁ — In-game clock.** `internal/gameclock` (CurrentHour,
  DayCount, TicksPerGameHour cadence, period boundary lookup,
  `time.hour.change` + `time.period.change` events with
  period-fires-before-hour ordering).
- **M15.4b₂a — Loader + composition wiring.** Pack loader
  (`weather_zones/*.yaml` schema with terrain-keyed message
  tables, area `weather_zone`, room `terrain` / `weather_exposed` /
  `time_exposed`). Composition root: `game-clock` tick handler
  + bus subscribers binding both clock events to
  `weather.Service`. Starter `temperate` zone in `content/core`.
- **M15.4b₂b — Render integration.** `Service.Ambience(room)`
  cascade-resolves the current state's `ongoing` message for
  a room. `RenderRoom` grows an ambience callback; `command.Env`
  + `command.Context` + `session.Config` thread it; composition
  binds `weatherSvc.Ambience`. `look` and movement renders now
  show weather in eligible rooms.

Side-benefit: `WorldRooms` interface gained `Area(id)` for O(1)
zone lookup, closing the M15.4a-review O(n) deferral.

---

## Suggested sequence

```
M15.1 — Doors + locks    (smallest, most contained, well-spec'd)
   ↓
M15.2 — Portals          (extends exits with TTL; reuses door-render patterns)
   ↓
M15.3 — Recall           (small; requires new spec before impl)
   ↓
M15.4 — Weather          (largest; touches the clock + render hooks)
```

Why this order:
- Doors are the simplest and exercise the per-exit state model
  Portals will piggyback on.
- Portals reuse exits with TTL — the cleanup tick pattern is novel
  but the data model is familiar.
- Recall is tiny; punching it through before weather keeps the
  large-and-cross-cutting item last.
- Weather closes the theme with the highest visual payoff (rooms
  describe their weather state) but also the highest blast radius
  (touches clock, render, area-reset).

---

## Pre-decisions (locked 2026-05-30)

All five pre-decisions resolved before implementation begins.
Headlines:

| ID | Decision |
|---|---|
| PD-1 | Door state lives on `world.Exit` (field, not service) |
| PD-2 | Weather is area-scoped (closed by spec §6) |
| PD-3 | Recall ships with full event hooks (cancellable before + post) but no built-in cooldown/cost |
| PD-4 | Key entities use `key_for: <door-id>` item property (registered as TypeString) |
| PD-5 | Portals creatable via BOTH content YAML AND an admin verb |

### PD-1 — Door state home

**Locked: field on the Exit struct.** `world.Exit` gains a
`Door *DoorState` field. Paired reverse-side sync walks both
rooms' exits on every state change; the cost is bounded
(at most one extra lookup per open/close) and keeps the data
model simple. A future migration to a separate service is a
refactor away if scale demands it.

### PD-2 — Weather granularity

**Locked by spec.** Spec §6 chooses area-scoped weather zones.
Per-room weather with neighbor influence is explicitly a non-goal
("coupling that the world feature must not impose"). Closed.

### PD-3 — Recall surface

**Locked: full event hooks, no built-in cooldown/cost.** The
verbs ship with:

- `set recall` — save the actor's current room as their recall
  point. Persisted on `player.yaml`.
- `recall` — publish `recall.before` (cancellable), teleport to
  the stored point if not cancelled, publish `recall.after`.
- No engine-level cooldown / sustenance cost / item charge in
  v1. Packs / admins can layer those on by subscribing to
  `recall.before` and cancelling, OR by subscribing to
  `recall.after` and applying a hunger debit.

Why this and not "verb-only": picking the event hooks up-front
makes the recall surface extensible without an API break later.
Cooldown/cost can be content-layer concerns rather than engine
ones, which keeps the engine substrate clean.

### PD-4 — Door key entities

**Locked: item property `key_for: <door-id>`.** The unlock verb
walks the actor's inventory, reads each item's `key_for`
property, matches against the locked door's id. Registers
`key_for` as a TypeString engine property in the M14.4 registry
so the property is discoverable in tooling and validated at
content load.

### PD-5 — Portal creator surface

**Locked: BOTH content YAML AND an admin verb.** Two parallel
creation paths:

- **Content path:** area YAML declares portals at boot. Includes
  `keyword`, `target`, optional `ttl` (omit for permanent).
  Loaded by `pack.Load` post-pass alongside item/mob placements.
- **Admin verb path:** `portal <keyword> <target> [ttl]` — admin-
  tagged actors can create portals at runtime. Depends on the
  role-tag system reaching production usability (m6-5 deferral).
  Until then, a config flag (e.g., `cfg.AdminPortalsEnabled`)
  gates the verb.

The cleanup tick handler doesn't care which path created the
portal — both end up in the same in-memory portal store.
Scripting (Theme D) layers on the same surface later via a
script-callable `engine.portal(...)` binding.

---

## Shape estimate

4-6 weeks per the theme plan.

| Item | Estimate |
|---|---|
| M15.1 doors + locks impl | ~1 week |
| M15.2 portals impl | ~3-5 days |
| M15.3 recall spec + impl | ~half a day + ~3-5 days |
| M15.4 weather impl | ~1-2 weeks |

Touches `internal/world` (substantial), new `internal/door` and
possibly `internal/portal` / `internal/recall` / `internal/weather`
(or a single `internal/worldfx` umbrella per the theme-axis plan
suggestion), `internal/command` for the verbs, and the room render
path for door state + weather text.

---

## Demo target

A locked door between two rooms; player uses a key item to unlock;
weather in the outer zone shifts from clear to storm on a tick;
player sets recall, walks around, recalls back to the saved
location.

---

## Tracking

- This file owns the live sequence + current step.
- `docs/ROADMAP.md` M15 heading carries the standard `[ ]/[x]` exit
  criteria.
- `docs/TAPESTRY-GAP-MATRIX.md` §1.8 + §3 (portals/weather/recall)
  entries get struck as each item closes.

When M15 ends:
1. Strike the closed items from `docs/TAPESTRY-GAP-MATRIX.md`.
2. Archive this file or leave for history.
3. Pick the next theme via the rubric. With A / C / E done, the
   remaining choices are B (Modern Client) and D (Content
   Authoring) — likely B unless content authors have arrived.
