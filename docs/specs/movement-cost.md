# Movement Cost (Movement Points) — Feature Specification

**Status:** Draft · **Scope:** The movement-point pool a character
spends to travel; the per-step cost gate layered on player-volition
movement; terrain-weighted step cost; the never-strand safety rule;
and the terrain-difficulty hint · **Audience:** Anyone reimplementing
or porting this feature in any language.

This document describes *what* the movement-cost feature must do, not
*how* to implement it. Specific pool sizes, per-terrain costs, the flat
default, and regen rates are policy that lives in configuration or
content (see §8).

This feature is the home of the movement-cost concern that
[world-rooms-movement](world-rooms-movement.md) deliberately declares a
**non-goal** (§1, §3.3): that spec keeps the move *primitive*
unconditional on resource availability; this spec owns the cost gate the
player-volition layer builds on top of it.

---

## 1. Overview

Travel is not free. A character carries a renewable **movement pool**;
walking from room to room spends it, and a depleted pool stops the
character until it regenerates. Rough terrain costs more than open
ground, so the pool also models the relative effort of crossing
different country.

The cost is a **precondition on player-volition movement only**. The
underlying move primitive stays unconditional and composable
(world-rooms-movement §3.3), so mob AI, flight, scripted teleports, and
administrative moves relocate without paying — only a player choosing to
walk is metered.

### Core concepts

- **Movement pool** — a per-character renewable resource with a
  `current` and a `max`. The max derives from a base attribute
  ([progression](progression.md)); the current is spent by travel and
  refilled by out-of-combat regen ([economy-survival](economy-survival.md)
  §4–§5). It is one instance of the generalized resource-pool model the
  character also uses for other spendable pools; this spec constrains
  only its movement use.
- **Step cost** — the number of movement points required to *enter* a
  destination room, derived from that room's terrain.
- **The cost gate** — the precondition that checks the pool, charges the
  step, and refuses an under-funded step. It sits in the player movement
  command, not the move primitive.
- **Never-strand rule** — a guarantee that a character with no movement
  pool, or facing a cost larger than the pool could ever hold, moves for
  free rather than being trapped.
- **Difficulty hint** — a brief, non-blocking cue when a step crosses
  onto rougher terrain than the room just left.

### Goals

1. Make sustained travel a metered, renewable resource rather than free.
2. Let content express relative terrain difficulty as a per-step cost.
3. Keep the move primitive unconditional, so only player-volition
   travel pays and every other mover composes the primitive unchanged.
4. Never trap a character because of movement cost.

### Non-goals

- The move primitive itself. Resolving the exit, the door-closed check,
  and relocating the entity belong to
  [world-rooms-movement](world-rooms-movement.md) §3.3; this feature is a
  precondition layered above it.
- The pool substrate and regen mechanics. The max attribute is owned by
  [progression](progression.md); the out-of-combat regen heartbeat is the
  regen tick handler, modulated by the sustenance/rest multipliers
  [economy-survival](economy-survival.md) exposes (§4.3, §5.5). This spec
  only specifies how movement *spends* the pool.
- Mounts and fatigue conditions. Mounted travel and
  exhaustion-as-a-condition are future couplings (§9). Carry-weight-driven
  cost (encumbrance) **is** in scope — see §4.4.
- Pathfinding or auto-travel. Cost is charged one player-issued step at
  a time; multi-room routing is out of scope.

---

## 2. The movement pool

### 2.1 Shape

A character's movement pool exposes a `current` and a `max`:

- `max` derives from a base attribute that content and progression set
  (a fresh character starts with a configured baseline, §8). A character
  with a zero `max` has **no pool** for the purposes of this feature.
- `current` is bounded to `[0, max]`. Spending floors at zero; it is
  never negative.
- `current` persists across sessions; `max` is re-derived from the base
  attribute on load (see §6).

### 2.2 Regen

The pool regenerates outside of combat on the **regen tick handler**, by
a configured amount per regen tick, independent of hit-point fullness,
and modulated by the sustenance/rest regen multipliers
[economy-survival](economy-survival.md) exposes (§4.3, §5.5). A dead or
disconnected character does not regenerate. This spec does not define the
heartbeat or the multipliers; it only relies on them to make the pool
renewable rather than a one-time budget.

**Acceptance criteria**

- [ ] The pool current is clamped to `[0, max]`; spending never drives
      it negative.
- [ ] A character with `max == 0` is treated as having no movement pool.
- [ ] The pool current is restored from the persisted save on login and
      the max is re-derived from the base attribute.
- [ ] The pool refills over time out of combat without further player
      action.

---

## 3. The cost gate

### 3.1 Placement

The cost gate runs **only in the player-volition movement command**, as
one of the "may I take this step" preconditions layered over the move
primitive. It MUST:

- run **after** the other step preconditions a player faces
  (knowledge-gated hidden exits, a closed door, a darkness hazard) and
  **before** any observable side effect of the move (the departure
  announcement, the relocation, the arrival announcement); so an
  under-funded step aborts cleanly with no partial effect;
- charge the step exactly once on success;
- **never** run inside the move primitive. Mob AI, flight, scripted
  teleports, recall, portals, and administrative moves invoke the
  primitive (or relocate directly) and MUST NOT be charged or refused by
  this gate.

### 3.2 Charging a step

Given the resolved cost of entering the destination (§4), the gate:

1. If the mover has no movement pool, allow the step and charge nothing
   (§3.3).
2. If the cost is non-positive, or exceeds what the pool could ever hold
   at full, allow the step and charge nothing (§3.3).
3. Otherwise, if the current pool is at least the cost, deduct the cost
   and allow the step.
4. Otherwise refuse the step (§3.4).

A step is considered **charged** only in case 3 — when a deduction
actually occurs. The charged/uncharged distinction is observable to the
difficulty hint (§5).

### 3.3 The never-strand rule

The gate MUST NOT trap a character. Two cases move for free rather than
being blocked:

- a mover with **no movement pool** (`max == 0`) — e.g. a creature the
  feature does not meter; and
- a cost **greater than the pool's maximum**, which a full pool could
  never afford — a content/config edge that must degrade to free
  passage, not a permanent wall.

In both cases the step proceeds and nothing is deducted.

### 3.4 Refusal

When the pool is non-empty-capable but the current is short of the cost,
the gate refuses:

- the character does not move (no relocation, no announcements, no
  events);
- a clear, user-facing message explains the refusal (the character is
  too winded / must recover);
- the refusal is silent to other observers — like other movement
  refusals, it emits no bus event.

**Acceptance criteria**

- [ ] The gate is exercised only by player-volition movement; a move
      through the primitive by AI / flee / script / admin is never
      charged or refused.
- [ ] The gate runs after the hidden-exit, door, and darkness
      preconditions and before any departure/relocation/arrival effect.
- [ ] A successful step deducts the cost exactly once.
- [ ] A mover with no pool, or a cost above pool capacity, passes free
      and unblocked.
- [ ] A short pool refuses the step with a clear message and no side
      effects; the character stays put.

---

## 4. Step cost

### 4.1 Terrain-weighted cost

The cost of a step is the cost of **entering the destination room**,
derived from that room's terrain via its [biome](biomes.md): a biome may
declare a movement cost, and rougher country declares a higher one. A
biome that declares no cost contributes none (it falls through to the
default, §4.2).

### 4.2 The default cost

When the destination terrain declares no per-biome cost — including the
backward-compatible case of terrain with no registered biome — the gate
charges a single configured **flat default** (§8). A server with no
per-biome costs authored therefore charges a uniform cost per step.

### 4.3 Destination-only

Cost is the price of **entering** the destination terrain. Leaving a
room costs nothing on its own, and the source room's terrain affects
only the difficulty hint (§5), not the amount charged. (A
higher-of-source-or-destination or leaving-cost model is considered in
§9 and deliberately not adopted here.)

### 4.4 Encumbrance surcharge

A loaded character pays more per step. The step cost is the terrain cost
(§4.1–§4.2) **plus an encumbrance surcharge** derived from how full the
character's **carry capacity** is — the same load the carry-weight pickup
limit measures (summed inventory weight). The surcharge is tiered: below a
**burdened** threshold the load is free; at or above it the surcharge
steps up by tier as the load nears capacity. Thresholds and per-tier
surcharges are policy (§8).

**Carry capacity** is a single quantity shared by the encumbrance
surcharge and the carry-weight pickup limit, resolved as: a **negative**
carry-capacity attribute is the explicit "unlimited" opt-out (no limit —
for a pack-mule or admin character); a **positive** attribute is a content
cap; otherwise capacity is **derived from Strength** (a configured
weight-per-Strength, §8). Only a character with the unlimited sentinel, no
stat surface, or non-positive Strength has no capacity ("no limit"); every
ordinary character therefore has a real capacity, so both the surcharge
and the pickup limit are **live**.

The surcharge depends only on the mover, not the room, so it adds equally
to a step's source and destination cost and therefore does **not** affect
the terrain difficulty hint (§5) — that hint stays purely terrain-driven.

**Acceptance criteria**

- [ ] A destination biome that declares a movement cost charges that
      cost, overriding the flat default.
- [ ] A destination with no per-biome cost (including unregistered
      terrain) charges the configured flat default.
- [ ] The terrain contribution depends only on the destination terrain,
      not the source.
- [ ] Carrying load at or above the burdened threshold adds the tier
      surcharge on top of the terrain cost; heavier load adds more.
- [ ] Carry capacity comes from an explicit attribute when set, else from
      Strength; the same capacity gates both the surcharge and pickup.
- [ ] A character with no stat surface / non-positive Strength carries no
      surcharge, and the surcharge never affects the §5 hint.

---

## 5. The difficulty hint

When a step crosses onto rougher ground, the feature surfaces a brief,
non-blocking cue so the larger spend is not a silent mystery. The hint
fires only when **both**:

- the step was actually **charged** (§3.2 case 3) — an unmetered or
  free mover never sees it; and
- the destination cost is **strictly greater** than the source-room
  cost — the terrain genuinely roughened.

Because the trigger is the *transition* to costlier ground, walking
within a single terrain stays silent (no per-step repetition), and there
is intentionally no message for stepping onto *easier* ground. The hint
is presented as subtle, secondary text below the room description, and
its absence never changes the outcome of the move.

**Acceptance criteria**

- [ ] Entering rougher terrain than the room just left, on a charged
      step, shows the hint once.
- [ ] Walking within one terrain (equal cost) shows no hint.
- [ ] An unmetered or free move (§3.3) shows no hint even onto rougher
      terrain.
- [ ] The hint is informational only; it never blocks or alters the
      move.

---

## 6. Persistence

The pool `current` persists per character so a character who logs out
mid-journey returns partly spent; the `max` is **not** persisted as pool
state — it is re-derived from the character's persisted base attribute
on load. A character whose persisted base predates this feature acquires
the baseline max from the engine default at load (the construction
default is merged under the persisted base), and a save migration makes
that value explicit on disk. See [progression](progression.md) for the
base-attribute semantics, and [persistence](persistence.md) for the
persisted base block and the versioned migration that backfills it.

**Acceptance criteria**

- [ ] A character's movement `current` round-trips across logout/login.
- [ ] A character created before this feature loads with the baseline
      max and is charged for movement like any other character.

---

## 7. Observable events

This feature introduces **no new bus events**. A successful charged step
rides the existing player-movement event the command layer already emits
(world-rooms-movement §8); a refused step is silent, consistent with
other movement refusals. The pool's depletion/refill is observed through
the existing resource-pool surfacing (the status prompt and character
sheet), not a movement-cost-specific event.

---

## 8. Configuration surface

The following are externally configurable and not fixed by this spec.

| Policy | Where it applies |
|---|---|
| Baseline movement-pool max for a new character | §2.1 |
| Out-of-combat movement regen amount and cadence | §2.2 (the regen tick handler) |
| Sustenance/rest regen multipliers applied to the pool | §2.2 (owned by economy-survival §4.3, §5.5) |
| Flat default per-step cost | §4.2 |
| Per-biome movement cost | §4.1 (content) |
| Carry-capacity derivation from Strength (weight per point) | §4.4 |
| Encumbrance tier thresholds (as a fraction of carry capacity) | §4.4 |
| Per-tier encumbrance surcharge | §4.4 |
| The user-facing refusal and difficulty-hint copy | §3.4, §5 |

---

## 9. Open questions / future work

- **Balance — first pass done, playtest pending.** The numbers were
  tuned once to a **moderate travel-friction** target against the shipped
  (small) room graph: step costs scaled so a round-trip across the map
  meaningfully drains the pool while local errands stay free, and
  movement regen brisked up so recovery is a short pause. This was a
  judgment without live playtest data; the values remain configuration
  (§8) and content, open to retuning once real play informs them. The
  pool size itself was deliberately left fixed (re-sizing it would need a
  save migration, like the one that backfilled it) — friction is tuned
  via cost and regen instead.
- **Encumbrance balance.** The surcharge (§4.4) is **live** — carry
  capacity now derives from Strength, so both it and the carry-weight
  pickup limit apply to every ordinary character. The shipped
  weight-per-Strength, tier thresholds, and surcharges are starting
  figures (a light traveler unburdened, a loot-laden one burdened) chosen
  without playtest data; tuning them against real item weights and travel
  patterns is the remaining work.
- **Encumbrance model breadth.** The surcharge is an additive,
  inventory-only, tiered term. Equipped-item weight is excluded (matching
  the pickup limit), and a multiplicative model (load *amplifies* rough
  terrain rather than adding a flat surcharge) was considered and not
  adopted for the first cut; revisit if balance asks.
- **Cost model breadth.** Cost is destination-only today. A
  higher-of-source-or-destination model (crossing a rough/easy boundary
  costs the worse of the two) or an explicit leaving-cost were
  considered and not adopted; revisit only if content design asks for
  the richer rule.
- **Mover policy for flight and the rest.** Flight, mob travel, and
  scripted moves pay nothing by construction (§3.1). Whether *flight*
  specifically should cost movement (a metered escape rather than a free
  one) is a balance question left open.
- **Hint completeness.** The difficulty hint is transition-only: a
  character who begins inside rough terrain gets no cue until they cross
  a boundary, and there is no "the going eases" message leaving rough
  ground. Both are intentional omissions that content may want revisited.
- **Client surfacing.** Step cost and terrain difficulty are not exposed
  to a structured client channel today; a future mapper/HUD integration
  may want them.
