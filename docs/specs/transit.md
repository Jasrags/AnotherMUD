# Transit: Elevators, Subways, and Conveyances — Feature Specification

**Status:** Draft (spec) · **elevator + subway SHIPPED** (2026-07-15 —
`internal/transit` service + car state machine + both call policies, in the
Shadowrun pack; live-verified). **On-demand** = the ACHE express elevator
(summon with `call`, pick a floor with `press <code>`). **Scheduled** = the
Downtown Metro, a subway whose train follows a fixed timetable (ping-ponging the
stop list) regardless of riders — you board during its dwell at a platform, no
calls. The landing→car doorway is a **real directional door** (a `world.Door`
the service keeps closed+locked while the car is away and unlocks+opens on
arrival) — riders **walk through the open doors** to board and `out` to alight.
An `axis`/`car_noun` reskins the motion prose (an elevator ascends/descends; a
train pulls out / rushes through). Still pending:
fares, multi-car lines, the crush hazard (§11),
the **hold** action (§6.1 — a rider can't yet extend dwell / re-open closing
doors; `DOORS_CLOSING` always proceeds), and the **active never-strand deposit**
(§6.2 — v1 relies on the boot-reseed-open-doors invariant, so `safe_landing` is
parsed but currently inert; the shutdown / line-reset / invalid-trip deposit
paths are unbuilt) · **Scope:** A **conveyance** that
carries riders between a fixed, ordered set of **stops** faster than walking the
room graph — a car (or train) you **ride inside** while it moves. Covers the
line/stop/car model, the doorway that re-binds to the current stop, the
call-and-ride cycle, the on-demand vs. scheduled call policy, door dwell and the
never-strand guarantee, the free-ride vs. stairs movement contract, and the
derived-not-persisted state model. The **elevator** is the v1 instance (a short
vertical line, one car, on-demand); the **subway/monorail** is the same machine
with a scheduled policy over a horizontal line · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the transit feature must do, not *how* to
implement it. Specific dwell times, per-stop travel durations, default stops,
line rosters, and fares are policy that lives in configuration or content
(see §11).

Transit is a **greenfield system**: nothing rideable-between-stops exists in
code today. It layers on [world-rooms-movement](world-rooms-movement.md) (rooms,
doors, and the temporary keyword-exit / "portal" retargeting primitive the car
doorway reuses), [movement-cost](movement-cost.md) (the free-ride vs. stairs
contract), [time-and-clock](time-and-clock.md) (the tick that drives car
motion), [session-lifecycle](session-lifecycle.md) (the never-strand-a-rider
deposit on shutdown, mirroring [mounts](mounts.md) §6), and optionally
[economy-survival](economy-survival.md) (fares, an open question in §11).

---

## 1. Overview

A **conveyance** carries riders between a fixed set of **stops** without walking
the intervening room graph. A rider **boards** the car at a stop, the car
**travels** (the rider is inside it while it moves), and the rider **alights** at
a later stop. The car is a real place: while aboard, a rider can look around,
talk, fight, and be caught by the closing doors — the ride is simulated, not an
instant teleport.

The elevator and the subway are the same machine. They differ only in **axis**
(the elevator's stops are floors reached vertically; the subway's are stations
reached horizontally) and **call policy** (the elevator is summoned on demand by
a button; the subway runs a timetable). One model expresses both, plus
monorails, mag-levs, funiculars, and any future "ride between fixed stops"
conveyance.

### Core concepts

- **Line** — an ordered sequence of **stops** served by one or more cars, with a
  **call policy** (on-demand or scheduled) and travel timing. A line is content:
  an elevator shaft is a short vertical line; a subway route is a longer
  horizontal line. The ordering defines adjacency and therefore per-hop travel
  time; it does not imply the car may only move one stop at a time (§4).
- **Stop** — one position on a line. A stop binds a **landing** (a permanent,
  normal room on the world graph — a floor lobby, a station platform) and a
  **call control** (the button or turnstile through which a rider requests the
  car). A landing exists and is fully playable whether or not the car is present;
  riders wait, fight, and idle there.
- **Car** — the conveyance itself: an **orphan room** reachable only through its
  own **doorway**, carrying a `Door`, a **current stop**, a **state** (§5), and a
  **request queue** (§4). Riders stand inside the car; when it moves, they move
  with it. A line may have more than one car (an open question for v1; §11), but
  the model is written so a car is the unit that holds riders and state.
- **The car doorway** — the single keyword exit binding the car's interior to a
  landing. It **re-binds** on every arrival to point at the current stop's
  landing, and **unbinds** while the car is in transit (so a rider cannot step
  out into a shaft). This is the same retarget-a-keyword-exit operation the
  temporary-exit ("portal") system already performs
  ([world-rooms-movement](world-rooms-movement.md) §3.6); transit reuses that
  primitive rather than inventing a second one.
- **Call policy** — how the request queue is fed. **On-demand:** a rider presses
  a call control, enqueueing a stop; the car services requests and idles when the
  queue empties (the elevator). **Scheduled:** the queue is a fixed timetable the
  car follows regardless of riders (the subway). The ride cycle (§5) is identical
  under both; only the queue's source differs.
- **The ride** — the transient relationship binding a rider to a car. Unlike a
  mount, the car is not owned and there is no single controller: any rider at a
  stop may call the car, and any rider aboard may request a stop. Many riders
  share one car.
- **Free ride vs. stairs** — riding spends **no** movement resource; it is the
  fast, effortless path. A building that also offers **stairs** (ordinary
  `up`/`down` exits, walked floor-by-floor at the normal per-step movement cost)
  gives two deliberately different ways to travel the same vertical space (§7).

### Goals

1. Let a rider travel between fixed stops **faster than walking the graph**, by
   riding inside a car that is a real, simulated place — not a teleport.
2. Express elevators, subways, monorails, and mag-levs as **content differences**
   (axis, stop roster, call policy, timing) over **one** conveyance model.
3. Reuse existing primitives — the temporary keyword-exit retarget for the
   doorway, the door primitive for the car doors, the tick for motion, the
   movement-cost pool for the free-ride contract — rather than reinventing them.
4. Make the car's motion **observable**: chimes, a floor/station indicator,
   doors opening and closing, so a rich client and a plain telnet client both
   read the ride clearly.
5. **Never strand a rider** — inside a car at shutdown, caught between stops, or
   aboard a car whose line is reset, a rider always ends on a safe, reachable
   landing.

### Non-goals

- **The move primitive.** Boarding and alighting are ordinary moves through the
  car doorway; resolving the exit, the closed-door check, and relocating the
  entity remain [world-rooms-movement](world-rooms-movement.md) §3.3. Transit
  re-points *where the doorway leads* (§3) and drives *when* it is traversable
  (§5); it does not change how a move resolves.
- **A second movement-metering model.** The free ride is the **absence** of a
  movement-cost charge, not a new resource ([movement-cost](movement-cost.md)
  remains the only travel-resource mechanism; §7).
- **Vehicles you drive.** A player-piloted car, throttle control, collisions, and
  steering are out of scope; a conveyance follows its line and policy, it is not
  driven (§11 notes a driver seat as future work).
- **Crush/door-trap damage.** v1 door dwell is cosmetic — a warning and a re-open
  affordance, no hit-point consequence for being caught. A crush hazard belongs
  to [area-effects](area-effects.md) and is deferred (§11).
- **Fares and turnstiles.** Charging to ride is a natural
  [economy-survival](economy-survival.md) hook but is out of v1 scope; the call
  control is written so a fare gate slots in later without reshaping the model
  (§11).
- **Multi-Z world coordinates.** The elevator does not require a real vertical
  coordinate axis; a line's ordering is its own adjacency. Whether stops also pin
  [room-coordinates](room-coordinates.md) is independent and out of scope here.

---

## 2. The line and its stops

### 2.1 What a line is

A **line** is content: an ordered list of stops, a call policy, one or more car
templates, and the timing that governs travel and dwell. The order of stops
defines which stops are **adjacent** (one hop apart) and therefore the travel
time between any two stops (a function of the hops between them; §5.3). A line is
identified by a stable, namespaced id like every other content id.

A line's axis is descriptive flavor, not mechanism: an elevator line is labelled
vertical and its stops named by floor; a subway line is labelled horizontal and
its stops named by station. The engine does not derive behavior from the axis —
it drives everything from the stop order and the policy.

**Acceptance criteria:**

- [ ] A line declares an **ordered** list of stops; the order is stable across
      boot and defines stop adjacency.
- [ ] A line declares exactly one **call policy** (§4): on-demand or scheduled.
- [ ] A line declares one or more car templates (v1 may constrain to exactly
      one car; §11).
- [ ] A line is identified by a stable namespaced id; unqualified stop/landing
      references resolve against the line's pack namespace.
- [ ] The axis label is presentational only; no behavior branches on it.

### 2.2 What a stop is

A **stop** binds:

- a **landing room** — a permanent room already in the world graph, where riders
  wait, board, and alight; and
- a **call control** — the in-room affordance (a button, a panel, later a
  turnstile) a rider uses to request the car (§4.1).

A landing is a fully ordinary room: it has its own description, exits, entities,
weather/light exposure, and combat. Its only transit-specific feature is the call
control and the fact that the car doorway will appear there when the car is
parked at that stop. A single room may be the landing for more than one line
(a station served by two subway routes; a lobby with two elevator banks) —
each line's car doorway and call control are distinct and keyword-addressable.

**Acceptance criteria:**

- [ ] Each stop names one landing room that exists in the world at line-load
      time; an unknown landing is a load-time error, not a silent drop.
- [ ] Each stop carries a call control addressable by keyword at its landing.
- [ ] A landing room is playable independent of car presence (wait, look, fight,
      idle) — nothing about transit gates ordinary room behavior.
- [ ] One landing room may serve multiple lines; each line's control and doorway
      are independently addressable and do not collide.

---

## 3. The car and its doorway

### 3.1 The car as an orphan room

A **car** is a room that is **not** reachable from the compass graph: it has no
`north`/`up`/etc. exit leading into it, and no room's fixed exits lead out of it.
Its only connection to the world is its **doorway** (§3.2). A car carries the
usual room surface (name, description, entity list, tags, properties) plus
transit state: its current stop, its motion state (§5), its request queue (§4),
and a floor/station **indicator** exposed as a room property (§9).

Riders inside the car are ordinary occupants of that room. They appear to one
another, can interact and fight, and are moved *with* the car (they do not move
between rooms as the car travels — the room they are in is the thing that
relocates its doorway).

**Acceptance criteria:**

- [ ] A car room has no fixed compass/vertical exits into or out of it; it is
      reachable only via its doorway.
- [ ] A car carries its current stop, motion state, and request queue as runtime
      state on the room (or a service keyed to it), not as content that must be
      re-authored.
- [ ] Occupants of a car are ordinary room occupants for look/social/combat.
- [ ] The car exposes a rider-visible indicator (current stop, direction of
      travel, door state) as a room property (§9).

### 3.2 The doorway binding

The **car doorway** is a keyword exit that connects the car interior to a
landing, in both directions:

- **From the landing side**, a keyword exit on the *current landing* leads into
  the car (the open elevator doors you step through).
- **From the car side**, a keyword exit leads out to the *current landing*.

Both sides are **re-bound on every arrival** (§5.4) to point at the new current
stop's landing, and **torn down (unbound) while the car is in transit** (§5.3) so
that neither a rider inside nor a bystander at a landing can traverse the doorway
mid-motion. This bind/unbind is exactly the create/retarget/remove of a temporary
keyword exit the world already supports; transit performs it through that same
primitive, preserving its lock order (world mutation happens under the world
lock, held after the transit service's own lock).

While in transit the doorway is **closed and unbound**: an attempt to leave the
car reports the doors are closed (it does not error, it narrates), and a bystander
at a former landing sees no doorway.

**Acceptance criteria:**

- [ ] When the car is parked (state IDLE, §5) at stop *S*, a keyword exit on
      *S*'s landing leads into the car, and a keyword exit in the car leads to
      *S*'s landing.
- [ ] On arrival at a new stop, **both** doorway halves re-bind atomically to the
      new landing; no half-bound state is observable between ticks.
- [ ] While the car is IN_TRANSIT, **both** doorway halves are unbound; no room
      shows a doorway into or out of the car.
- [ ] A traversal attempt through a closed/absent doorway narrates ("the doors
      are closed") and never relocates the entity into a shaft or a stale
      landing.
- [ ] Doorway bind/unbind uses the temporary keyword-exit primitive and its
      established lock order (no inverted lock acquisition; cf.
      [world-rooms-movement](world-rooms-movement.md) §3.6 and the portal
      service).

---

## 4. Calling and the request queue

Every line has a **request queue** of stops the car should service. What feeds
the queue is the line's **call policy**; how the car drains it is common to both
policies (§5).

### 4.1 On-demand policy (the elevator)

A rider at a landing uses the **call control** to request the car come to that
landing; a rider inside the car uses the interior control to request a
destination stop. Both actions enqueue a stop. The car services the queue and,
when it empties, **idles** at its last stop with doors open (or closed after
dwell; §6). No riders, no motion.

- A call for a stop already in the queue (or the current stop, doors open) is a
  no-op with feedback, not a duplicate enqueue.
- A rider need not specify anything but the target stop; the car's routing across
  intervening stops is the model's business (§5.3), not the rider's.

**Acceptance criteria:**

- [ ] A landing call control enqueues that landing's stop and gives the caller
      feedback (summoned / already here / already coming).
- [ ] An interior control enqueues the chosen destination stop with feedback.
- [ ] A duplicate call (queued stop, or current stop with doors open) is an
      idempotent no-op with feedback, not a second enqueue.
- [ ] With an empty queue the car idles at its current stop and does not move.

### 4.2 Scheduled policy (the subway)

The queue is a **timetable**: a repeating sequence of stops the car visits on a
cadence, independent of whether anyone is aboard or waiting. A call control under
this policy does not summon the car (it cannot be summoned off-schedule); it may
display the next arrival, and later gate a fare (§11). Riders board and alight
only while the car dwells at a stop (§6).

**Acceptance criteria:**

- [ ] Under scheduled policy the car advances through its timetable on the
      configured cadence regardless of riders or calls.
- [ ] A call control under scheduled policy does not alter the queue; it may
      report next-arrival information.
- [ ] Boarding/alighting occurs only during a stop's dwell window (§6); a rider
      who misses the window waits for the next arrival.

### 4.3 One cycle, two sources

The ride cycle (§5) consumes the queue identically under both policies. The only
difference is the queue's origin: rider actions (on-demand) or a timetable
(scheduled). This is the seam that makes an elevator and a subway one feature.

**Acceptance criteria:**

- [ ] The state machine in §5 references only "the next requested stop," never
      the policy; swapping the policy changes the queue source and nothing else.

---

## 5. The ride cycle and state machine

A car is always in exactly one **motion state**. A single tick handler advances
the state on the line's cadence.

```
IDLE          — parked at a stop, doors open, doorway bound. Awaiting a request.
DOORS_CLOSING — a request exists (or dwell elapsed); a warning has fired; the
                doors are closing. Last chance to hold (§6).
IN_TRANSIT    — doors closed, doorway unbound, car moving toward the next
                requested stop. Riders are sealed inside.
ARRIVING      — the car has reached the target stop; doors open, doorway
                re-binds, arrival is announced. Transitions to IDLE.
```

### 5.1 IDLE → DOORS_CLOSING

The car leaves IDLE when there is a next requested stop **and** the dwell window
has elapsed (§6), or when a scheduled departure is due. On entering
DOORS_CLOSING it fires a closing warning to both the car interior and the current
landing.

**Acceptance criteria:**

- [ ] The car departs IDLE only with a next requested stop (on-demand) or a due
      scheduled departure.
- [ ] Entering DOORS_CLOSING emits a closing warning to the car and the current
      landing.

### 5.2 DOORS_CLOSING → IN_TRANSIT (or back to IDLE)

If the doors finish closing uninterrupted, the car unbinds its doorway (§3.2) and
enters IN_TRANSIT. A **hold** action (§6) during DOORS_CLOSING re-opens the doors
and returns the car to IDLE (resetting dwell).

**Acceptance criteria:**

- [ ] Uninterrupted door closure unbinds both doorway halves, then enters
      IN_TRANSIT.
- [ ] A hold during DOORS_CLOSING re-opens the doors and returns to IDLE with
      dwell reset; the queued request survives (the car will try again).

### 5.3 IN_TRANSIT

The car moves toward the next requested stop, spending the configured travel time
**per hop** between adjacent stops (so a farther stop takes proportionally
longer). Intervening stops the queue also requests may be serviced en route
(routing across the ordered line is the model's concern, not the rider's). During
transit the indicator (§9) updates as the car passes each intervening stop.
Riders may not leave; the doorway is unbound.

**Acceptance criteria:**

- [ ] Travel time scales with the number of hops between the current and target
      stop, per the per-hop configuration (§11).
- [ ] The indicator updates as the car passes each intervening stop.
- [ ] No rider can leave the car during IN_TRANSIT; a leave attempt narrates
      closed doors (§3.2).
- [ ] En-route stops present in the queue may be serviced without returning to
      IDLE first (the car need not fully stop-and-restart per queued stop unless
      a stop is a requested destination).

### 5.4 ARRIVING → IDLE

On reaching a target stop the car re-binds both doorway halves to the new
landing (§3.2), opens the doors, announces arrival with a chime to both the car
interior and the new landing, and enters IDLE with a fresh dwell window (§6).

**Acceptance criteria:**

- [ ] On arrival both doorway halves re-bind atomically to the new landing before
      the doors are reported open.
- [ ] Arrival emits a chime/announcement to the car interior and the new landing.
- [ ] The car enters IDLE with dwell reset; the serviced stop is removed from the
      queue.

---

## 6. Doors, dwell, and the never-strand guarantee

### 6.1 Dwell and hold

On arrival the doors stay open for a configured **dwell** window before the car
will depart. A **hold** action (a rider inside or at the landing) extends the
dwell (re-opening the doors if in DOORS_CLOSING). Dwell is the boarding window:
riders board and alight freely while the doors are open (states IDLE and, until
the doors finish, DOORS_CLOSING).

v1 door behavior is **cosmetic**: a rider "caught" as the doors close is simply
sealed in and rides to the next stop; there is no damage. (A crush hazard is
deferred to [area-effects](area-effects.md); §11.)

**Acceptance criteria:**

- [ ] Doors remain open for the configured dwell before departure.
- [ ] A hold extends dwell and re-opens closing doors; repeated holds are allowed
      (subject to any configured cap; §11).
- [ ] Boarding and alighting succeed whenever the doors are open; no rider takes
      damage from the doors in v1.

### 6.2 Never strand a rider

A rider must always be able to end on a safe, reachable landing:

- **At shutdown**, any rider inside a car is **deposited** on a configured safe
  default landing for that line (mirroring [mounts](mounts.md) §6's
  never-strand deposit and the final-flush on SIGINT in
  [session-lifecycle](session-lifecycle.md)).
- **On line reset / reload**, riders aboard are deposited on the safe default
  landing before the car's runtime state is rebuilt.
- **On a stuck or cancelled trip** (a target stop that has become invalid), the
  car returns to a safe default stop and opens its doors rather than holding a
  rider in limbo.
- A rider is **never** left in a car whose doorway is unbound with no pending
  arrival.

**Acceptance criteria:**

- [ ] At clean shutdown every rider inside a car is relocated to the line's safe
      default landing and their session state committed.
- [ ] A line reset/reload deposits current riders on the safe default landing
      before rebuilding car state.
- [ ] A cancelled/invalid trip returns the car to a safe default stop with doors
      open; no rider remains in an unbound car with no pending arrival.

---

## 7. Movement-cost interaction: the free ride vs. the stairs

Riding is **free** of the movement-cost resource: boarding, riding, and alighting
spend no movement points. The ride's "cost" is **time** (dwell + travel), not the
rider's stamina — that is the entire point of fast transportation.

Where a building or route also offers **stairs** (or a walked service corridor) —
ordinary `up`/`down` (or compass) exits between the same landings — walking them
is normal movement: floor-by-floor, each step charged at the usual per-step
movement cost under [movement-cost](movement-cost.md), gated by encumbrance. This
gives two deliberately different paths over the same vertical/horizontal space:
**fast-and-effortless** (ride, but wait for the car and its schedule) vs.
**slow-and-free-form** (walk, but pay stamina and time per step, and reach
intermediate floors the car may skip).

**Acceptance criteria:**

- [ ] Boarding, riding, and alighting spend **no** movement-cost resource.
- [ ] Walking stairs/corridors between the same landings is charged the normal
      per-step movement cost and is subject to encumbrance gating.
- [ ] Nothing in transit adds a second travel-resource; the free ride is the
      absence of a charge, not a new pool.

---

## 8. Boarding, riding, and alighting (rider operations)

From the rider's side the whole feature is three ordinary interactions:

- **Board** — traverse the open car doorway from a landing (an ordinary move;
  §Non-goals). Succeeds while the doors are open; narrates closed doors
  otherwise.
- **Call / choose a stop** — use a call control (landing) or interior control
  (car) to enqueue a stop (§4). Under scheduled policy the interior may only
  *read* the timetable, not alter it (§4.2).
- **Alight** — traverse the open car doorway from the car to the current landing
  (an ordinary move). Succeeds while the doors are open at a stop.

Look, social, and combat inside the car are unchanged from any other room. The
rider-facing surface adds only the call/choose control and the indicator (§9);
everything else is existing room behavior.

**Acceptance criteria:**

- [ ] Boarding/alighting are ordinary moves through the doorway, honoring the
      door-closed check; no transit-specific move path exists.
- [ ] The call/choose control is keyword-addressable at both the landing and the
      car interior, with clear feedback.
- [ ] Look/social/combat inside the car behave exactly as in any room.

---

## 9. Observable events and rich-client surface

Every meaningful transition is observable so that a plain telnet client reads the
ride in prose and a rich client (GMCP) can render it structurally:

- **Chime / arrival** at a stop — to the car interior and the arriving landing.
- **Doors opening / closing / held** — to the car interior and the current
  landing.
- **Departure** — to the car interior and the departed landing.
- **Next stop** — the on-board destination cue, to the car interior as the doors
  close (a subway PA call, an elevator panel light).
- **Approaching** — to the *destination* landing one beat before arrival, so
  riders waiting there see the conveyance coming (only on legs long enough to
  announce; a single-step hop arrives too fast).
- **Indicator update** — current stop, direction of travel, and door state,
  exposed as a car room property and pushed to riders as it changes (the
  floor/station display). A landing may expose a next-arrival indicator under
  scheduled policy (§4.2).

Consistent with the rest of the engine, each transition emits exactly one
structured log line with the standard fields (add `line`, `car`, `stop` to the
usual `room_id`, `tick`, `event`).

**Acceptance criteria:**

- [ ] Chime, door-state change, departure, and arrival each emit to both the car
      interior and the relevant landing.
- [ ] The car indicator (stop, direction, door state) is exposed as a room
      property and updates on every transition; riders receive the update.
- [ ] Under scheduled policy a landing can expose next-arrival info.
- [ ] Every transition emits exactly one structured log line with `line`, `car`,
      `stop` alongside the standard fields.

---

## 10. Persistence

Transit runtime state is **derived-not-persisted**, matching weather and
temporary exits ([world-rooms-movement](world-rooms-movement.md) and the
README save/load surface):

- A car's **current stop, motion state, and request queue are not saved**. At
  boot each line seeds its cars at a configured **default stop**, IDLE, doors
  open, empty queue.
- The car doorway bindings are runtime keyword exits and are **not persisted**
  (they are rebuilt from the seeded state).
- No player save field is added for "in a car": the never-strand deposit (§6.2)
  guarantees no player is *in* a car across a restart — they are relocated to a
  landing first, and the landing is their ordinary persisted `location`.

**Acceptance criteria:**

- [ ] At boot each line seeds its car(s) at the configured default stop, IDLE,
      doors open, empty queue; no transit state is read from disk.
- [ ] No new player save version/field is required for transit; a returning
      player never loads *inside* a car (they were deposited on a landing at
      shutdown, §6.2).
- [ ] Doorway bindings are rebuilt from seeded state, not persisted.

---

## 11. Configuration surface

The following are externally configurable and not fixed by this spec.

| Policy | Where it applies |
|---|---|
| Line roster — ordered stop list, axis label, car template(s) per line | §2 (content) |
| Per-stop button **code** (the `press <code>` short label — "G", "C") | §4.1, §8 (content) |
| Landing-door **direction** + display name (the walk-through doorway) | §3.2, §8 (content) |
| Call policy per line — on-demand vs. scheduled | §4 (content) |
| Scheduled timetable — stop sequence and cadence | §4.2 (content) |
| Motion prose — `axis` (vertical/horizontal) + `car_noun` (car/train) | §5.3, §9 (content) |
| Per-hop travel time between adjacent stops | §5.3 |
| Door dwell window (open duration before departure) | §6.1 |
| Hold behavior — dwell extension amount and any repeat/hold cap | §6.1 |
| Closing-warning lead time before doors seal | §5.1, §6.1 |
| Default seed stop per line (boot state) | §10 |
| Safe default landing per line (never-strand deposit target) | §6.2 |
| Call-control and indicator keywords per stop/car | §2.2, §8 |
| User-facing copy — chimes, door messages, closed-doors narration, indicator labels | §5, §8, §9 |

---

## 12. Open questions / future work

- **Multiple cars per line.** v1 may constrain a line to one car. A bank of
  elevators (or a subway with several trains) needs a routing/assignment policy
  (which car answers a call, how cars avoid colliding on a shared line) — a
  clean extension of the queue model but out of v1 scope.
- **Fares and turnstiles.** A call control that charges nuyen/gold (or checks a
  pass/SIN) before admitting a rider is a natural
  [economy-survival](economy-survival.md) hook. The control is written so a fare
  gate slots in without reshaping the model; the fare *policy* (price, pass
  types, evasion) is deferred.
- **Keyed / access-restricted destinations** *(design decided 2026-07-15; build
  pending)*. Some stops are not public: a rider may **select** (send the car to) a
  restricted floor only while carrying a credential that clears it — the hotel
  model, where a card reaches the public floors and your own guest floor but
  nothing else. **Decided design:**
  - **Credential = a dedicated `keycard` item** (distinct from, but generalizing,
    a plain door key), carrying the access it grants **and a security rating** —
    the difficulty a later forge/hack must beat. A keycard clears a stop **either**
    by a specific **key id** **or** by an access **tag / clearance**, and a stop
    may require either — so one "corp-sec" card opens a whole class of floors while
    a guest card opens exactly one (**hybrid** from the start).
  - **Enforced at selection only** (`press`/`call`), never at alighting: you can't
    send the car to a floor you can't clear, but you can ride a car an authorized
    rider (or the schedule) sent there and step out. **Tailgating a badge-holder
    is a feature**, not a leak.
  - **Bypass:** a **master keycard** item and/or a **security role** clears every
    stop (the stealable maintenance spike; an admin force-open).
  - Scoped to **on-demand elevators first**; a scheduled subway stops everywhere,
    so restricting one is an alighting/turnstile concern that rides in with
    **Fares** above.

  **Gaps to fill:** (1) a per-stop access declaration — `Stop` gains a required
  key id and/or access tag (public when unset); (2) a **`keycard` item type**
  (granted key ids + access tags, plus a security rating), distinct from the
  door-key it generalizes; (3) the check needs the actor's **inventory + roles,
  which the transit service lacks** — the `press` command builds an access oracle
  (holds a clearing keycard? a master card? the security role?) and passes it into
  `Service.Press`, keeping the tick loop inventory-blind; (4) keep this
  **credential gate distinct from the presence-lock** the landing door already
  carries (that lock only stops boarding an *absent* car — orthogonal to who may
  ride where); (5) denied-selection feedback + a panel marking restricted floors
  (`[40] Penthouse [LOCKED]`). **No new save shape** — keycards are ordinary
  carried items; stop access and card grants are content. The keycard's security
  rating is the substrate for later **forged cards** (an illicitly-crafted keycard
  — a crafting/economy hook) and **hacking / decking** (a skill or Matrix action
  that beats the rating to bypass or grant access — layers on a future decking
  system + the skill-check primitive); pairs with the out-of-service lockdown
  below (revokes all access at once).
- **Express vs. local.** Skip-stop service (an express that passes intermediate
  stops without stopping) is pure content once the queue exists — a per-request
  "non-stop to *S*" flag — but the rider-facing selection UI and the interaction
  with on-demand calls at skipped stops want design.
- **Crush / door-trap hazard.** A rider caught in closing doors taking damage (or
  being shoved back out) belongs to [area-effects](area-effects.md); v1 doors are
  cosmetic (§6.1).
- **A driver / operator seat.** A player- or NPC-operated conveyance (a piloted
  tram, a bribed elevator operator, a hijacked train) would layer a controller
  relationship over the car, closer to [mounts](mounts.md)' ownership than to the
  ownerless v1 car.
- **Out-of-service / breakdown state.** A car that is broken, locked, key-carded,
  or powered-down (a security lockdown sealing a corp tower's express elevator)
  is a compelling content lever and a natural fifth motion state; deferred until
  a content need drives its exact semantics.
- **Following into a car.** How [follow](follow.md)/[grouping](grouping.md)
  interacts with boarding — does a follower auto-board when the leader does, and
  what happens if the doors close between them — reuses the mounted-travel
  follow questions and is deferred to a combined pass.
- **Room-coordinate integration.** Whether elevator floors pin real
  [room-coordinates](room-coordinates.md) Z-levels (so the `map` renders a
  tower in section) is independent of transit mechanics and deferred.
