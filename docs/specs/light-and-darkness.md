# Light and Darkness — Feature Specification

**Status:** Draft · **Scope:** A per-viewer **effective light level**
for every room, computed from time-of-day, terrain sky-exposure, a
per-room override, carried/equipped light sources, and a per-viewer
darkvision floor; the **real-friction** consequences of low light
(withheld room information, blocked examination, a combat accuracy
penalty, movement risk); light-source items with a lit/unlit state and
a fuel burn-down loop; light transitions felt as the day/night cycle
turns; and the **persistence of in-game time** so a restart does not
black out the world (resolving `docs/specs/time-and-clock.md` §3.6) ·
**Audience:** Anyone reimplementing or porting this feature in any
language.

This document describes *what* the light surface must do, not how to
implement it. Numeric values (ambient per period, indoor cap, combat
penalty, source levels, burn rate, darkvision cap) are **not** fixed
here — they live in the configuration surface (§11). The four ordinal
light levels and the four time-of-day periods are fixed vocabulary, the
way the period names are fixed in `time-and-clock.md`.

Design rationale and the decisions that steer this spec live in
`docs/proposals/light-and-darkness.md` (PD-1…PD-7). This spec inherits
those decisions; where it makes a finer choice the proposal left open,
it says so.

---

## 1. Overview

The world has a day/night cycle (`time-and-clock.md` §3): the in-game
clock advances an hour at a fixed cadence, classifies the hour into a
period (`night`/`dawn`/`day`/`dusk`), and emits `time.period.change`.
Today nothing reacts to it but ambience text. This feature makes
darkness *mean* something: what a character can see, examine, fight,
and safely walk into depends on how much light reaches them, where
light comes from time-of-day, terrain, room features, and the torch in
their hand.

### Core concepts

- **Effective light level** — an ordinal value in `{black, gloom, dim,
  lit}` (0–3) describing how well a *specific viewer* can see in a
  *specific room* right now. Computed, never stored. Per-viewer: two
  characters in the same room can have different effective light (one
  holds a torch, one is a dwarf, one is blind).
- **Ambient light** — the light the sky provides, a function of the
  current period. Daylight is full; twilight is reduced; night is
  reduced further but **never black** (starlight). The darkest natural
  sky is gloom, not black.
- **Terrain sky-gate** — whether ambient reaches a room at all, keyed
  to the room's terrain (`world-rooms-movement.md` §6.4:
  `outdoors`/`indoors`/`underground`). Outdoors gets full ambient;
  indoors gets ambient capped (windows); underground gets none. This is
  what makes a windowless cell black at noon and an open road merely
  gloomy at midnight.
- **Room light override** — an authored per-room floor/ceiling (a
  lamp-lit street pinned bright at night, a phosphorescent cave, a
  sealed vault pinned black). Content's single knob.
- **Light source** — an item (or, later, a luminous mob/effect) that
  contributes light to the room it is in. A *lit* source raises the
  room's effective light; a fuel-burning source consumes fuel while lit
  and gutters out when empty.
- **Viewer floor** — a per-viewer minimum effective light (racial
  darkvision, a light/sight effect) applied after everything else.
- **Friction** — the consequences of low light: withheld room
  information, blocked examination/reading, a combat accuracy penalty,
  and movement risk. Darkness is a mechanic, not a mood. Atmosphere
  (a dimmer-tinted room) is the *floor* of the effect, not the whole.

### Goals

- A pure, per-viewer **light resolver**: given a room and a viewer,
  return one of the four effective light levels.
- The day/night cycle, terrain, room features, and held light all
  visibly and mechanically matter.
- Carrying and maintaining a light source is a meaningful choice
  (a resource to spend, a slot to occupy, a tactical tradeoff).
- A restart never forces the world dark (in-game time persists).
- Content controls a room's light with one property; unaudited content
  fails **safe** (visible), never **dark**.
- Presentation extends the existing color/ambience pipeline and surfaces
  the light level to capable clients.
- An invariant guarantee that a character can never be trapped by
  darkness with no way out.

### Non-goals

- **Not** a continuous/lumen simulation — four ordinal levels, not a
  float.
- **Not** inter-room light propagation — a source lights the room it is
  in, never the room next door.
- **Not** a stealth/hiding system — darkness exposes hooks a future
  stealth feature can use (light-as-beacon, dark-aids-hiding), but this
  spec does not define hiding.
- **Not** weather-driven light in this version — overcast dimming
  daylight is a deferred refinement (§12).
- **Not** over-bright/blinding as a mechanic — `lit` is the ceiling.
- **Not** per-individual occupant concealment — gloom hides the
  occupant list coarsely; which specific entities a partial-light room
  reveals is a presentation rule (§5.2), not a per-entity fog.

---

## 2. Effective light level

### 2.1 The scale

Effective light is one of four ordinal levels. The names are fixed
vocabulary (events, GMCP, and content reference them):

| Level | Name | The viewer can… |
|---:|---|---|
| 3 | `lit` | see everything (full room render) |
| 2 | `dim` | see everything, presented muted |
| 1 | `gloom` | make out shapes and directions, not detail |
| 0 | `black` | see nothing |

### 2.2 Computation

Effective light for `(room, viewer)` is the maximum of the contributing
sources, gated by terrain and floored by the viewer, then clamped to the
scale:

```
ambient   = ambientFor(currentPeriod)          # lit / dim / gloom — NEVER black
exposed   = throughTerrain(ambient, room.terrain)
roomFloor = room.light                          # the authored override, when present
sources   = best light contributed by lit sources the viewer carries
            or holds, and by luminous items/mobs in the room
viewerCap = darkvision / light-effect floor for THIS viewer (default black)

effective = clamp( max(exposed, roomFloor, sources), viewerCap, black, lit )
```

- `ambientFor` maps the current period to an ambient level. Daylight
  maps to `lit`; twilight periods to a reduced level; night to a
  further-reduced level. It **never returns `black`** — the darkest
  natural sky is `gloom`. The exact mapping is configurable (§11).
- The combine is a **maximum**: any single bright contributor wins. A
  torch in a black cave makes the cave visible; a lamp-lit room
  override beats a dark night; daylight beats an unlit room with no
  ceiling override.
- The result is **never persisted** — it is recomputed on demand
  (look, move, render, combat swing).

### 2.3 The terrain sky-gate

`throughTerrain` decides how much ambient survives the room's enclosure,
keyed to terrain (`world-rooms-movement.md` §6.4, reusing the same
classifier and shielding rule the weather cascade uses):

| Terrain | Ambient reaching the room |
|---|---|
| `outdoors` (and the empty default) | full ambient |
| `indoors` | ambient capped at a configured ceiling (windows let some through) |
| `underground` | none (no sky reaches it) |

Consequences that must hold:

- **Night ≠ black.** An `outdoors` room at night is `gloom`
  (ambient floor), never `black`, with no source.
- **Sealed = black.** An `underground` room receives no ambient, so it
  is `black` unless a room override or a light source raises it —
  regardless of the hour.
- The terrain gate applies to *ambient only*. Room overrides and light
  sources are not gated by terrain (a torch works underground; a
  `light` override pins a value the sky can't reach).

### 2.4 The room light overrides (cascade)

A room's authored light comes in **two distinct directives**, because
"force this exact level" and "keep this room at least this bright" are
genuinely different needs:

- **Pin (`light`)** — names an exact level that **replaces** ambient
  entirely: it both floors *and* ceilings the room. A sealed vault pins
  `light: black` and stays black at noon; a windowless glowing hall pins
  `light: lit` and stays lit at midnight. The pin is time-independent —
  the sky no longer reaches the room at all.
- **Floor (`light_floor`)** — names a minimum level that **max-combines**
  with the terrain-gated ambient: it lifts a dark sky without capping a
  bright one. A lamp-lit village street sets `light_floor: dim` and is
  `lit` at noon (daylight beats the floor) but `dim` — never `gloom` — at
  midnight (the lamps lift the dark). This is the "settlement after dark
  is navigable, the open wilds are not" knob.

A pin outranks a floor when a room resolves to both (a sealed cellar
inside a lamp-lit village pins `light: black` and stays dark despite the
village floor). Neither directive is gated by terrain (§2.3) — a floor's
lamps reach an `indoors` room, a pin's value holds `underground`.

Resolution of each directive follows the same **room → area → zone**
cascade the weather messages use (`world-rooms-movement.md` §6):

- A room-level directive wins.
- Else an area-level default applies (an area's `light_floor` is the
  lamp-lit-village tier — it bakes onto member rooms at load).
- Else a zone/biome default applies (leaving a tier for a future
  `biomes.md` contribution — e.g. a "cavern" biome defaulting `black`).
- Else there is no override and ambient (through the terrain gate)
  governs.

**Acceptance criteria**

- [ ] Effective light is one of exactly four ordinal levels.
- [ ] Effective light is computed per-viewer and never stored.
- [ ] Ambient is derived from the current period and is never `black`.
- [ ] An `outdoors` room at the darkest period resolves to `gloom`
      with no source or override.
- [ ] An `underground` room resolves to `black` with no source or
      override, at any hour.
- [ ] An `indoors` room never exceeds the configured indoor ambient cap
      from ambient alone.
- [ ] A room `light` PIN both floors and ceilings the room's light
      (replaces ambient) regardless of terrain or period.
- [ ] A room `light_floor` FLOOR lifts a dark ambient (gloom → its
      level) but never caps a bright one (noon stays `lit`), regardless
      of terrain.
- [ ] A pin outranks a floor when a room resolves to both.
- [ ] A lit source carried by the viewer raises effective light even in
      an `underground`/`black` room.
- [ ] Each directive resolves room → area → zone, first match wins; an
      area `light_floor` applies to member rooms lacking their own.

---

## 3. Light sources

### 3.1 The light property and lit state

A light-source item carries a `light` property naming the level it
contributes when lit. Source light is a property on the item, read the
same way other reserved item properties are
(`inventory-equipment-items.md`); it is not a new item field.

A source has a **lit** state:

- A source contributes light **only while lit**.
- A source is lit/extinguished by an explicit action (a `light` /
  `extinguish` verb pair). Equipping a source MAY auto-light it
  (configurable, §11); extinguishing is always explicit (to conserve
  fuel).
- The lit state lives on the item instance (a reserved instance
  property), so it survives being picked up, dropped, given, or stored,
  and is admin-settable.

### 3.2 Fuel and burn-down

A source is either **fuel-burning** or **permanent**:

- A **fuel-burning** source (torch, oil lantern, candle) carries a fuel
  value. While lit, fuel decrements on a recurring tick at a configured
  cadence and amount — the same drain shape as sustenance
  (`economy-survival.md` §4.4). When fuel reaches zero the source
  **gutters out**: it becomes unlit, emits a notification to the holder
  (and a transition to the room if it was the room's light, §6), and
  stops contributing. A guttered source with no refuel path is spent.
- A **permanent** source (a glowing blade, an everburning lantern)
  carries no fuel and is always-on while lit; it never gutters.
- Refuelling (adding oil, a fresh torch) is content-defined and out of
  scope for the base mechanic; the spec only requires that fuel can be
  decremented to zero and that zero extinguishes.

### 3.3 The held light slot

The active light a viewer provides is held in a dedicated **light**
equipment slot (the same slot machinery as existing worn slots,
`inventory-equipment-items.md`). One light occupies the slot at a time.
This makes the active source visible in the equipment view, gives the
resolver an unambiguous "the viewer's light" to read, and makes
carrying light a tradeoff (whether the light slot contends with hands
holding a two-handed weapon or shield is content/equipment-model
defined — §12).

A source in inventory but **not** in the light slot does not light the
viewer (it is just cargo). Luminous items lying in the room, and
luminous mobs, contribute to that room's `sources` term independently of
any viewer's slot.

**Acceptance criteria**

- [ ] A source contributes its `light` level only while lit.
- [ ] A source is lit/extinguished by explicit verbs; lit state
      persists across pickup/drop/give/store.
- [ ] A lit fuel-burning source loses fuel on the configured cadence
      and gutters out (becomes unlit) at zero.
- [ ] Guttering notifies the holder and, when it was the room's light,
      transitions the room (§6).
- [ ] A permanent source never loses fuel and never gutters.
- [ ] Only the source in the light slot lights its bearer; inventory
      sources do not.
- [ ] Luminous items/mobs in a room contribute to that room's light
      regardless of any viewer's slot.

---

## 4. Per-viewer sight: darkvision and light effects

Effective light is per-viewer because some viewers see in the dark:

- **Racial darkvision** sets a per-viewer **floor** (a race flag read
  via `progression.md`). A viewer with darkvision treats any room as at
  least their floor level. Darkvision is **capped** — it floors the
  viewer to `gloom`, never to `lit` (it is monochrome / shape-only, an
  advantage, not daylight). The floor and cap are configurable (§11).
- **Light/sight effects** (a cast light, an infravision buff) raise the
  viewer's floor for a duration via the effect system
  (`abilities-and-effects.md`). They contribute to the same
  `viewerCap` term.
- Because the render and action paths are already per-viewer, no new
  plumbing is required — the resolver is called with the viewer in
  hand.

**Acceptance criteria**

- [ ] A darkvision viewer's effective light is floored to the
      configured darkvision level, never below.
- [ ] Darkvision never raises effective light above its configured cap
      (`gloom` by default) from darkvision alone.
- [ ] A light/sight effect raises the viewer's floor for its duration
      and stops on expiry.
- [ ] Two viewers in the same room can have different effective light.

---

## 5. What darkness costs (the friction)

This is the centre of the feature. Each consequence consults the
resolver at an existing chokepoint. Atmosphere (the muted tint at
`dim`, §8) is the floor of the effect; the rungs below are the friction.

### 5.1 Room information

The room render (`ui-rendering-help.md`, the per-viewer room view)
branches on the viewer's effective light **before** composing the body:

| Effective | Room render |
|---|---|
| `lit` | full render (unchanged) |
| `dim` | full render, presented muted (§8) |
| `gloom` | **obscured**: a terse "it is dark" description in place of the room's prose; **exits shown as bare directions** (no door/weather detail); **occupants shown coarsely** — presence/【kind】 without identity (names hidden) |
| `black` | **suppressed**: name, description, occupants all withheld → a single "you can see nothing" line |

- The terse `gloom` description and the `black` line are content/render
  strings, configurable.
- **Occupant coarsening at `gloom`** hides identity but reveals
  presence; the exact descriptor granularity (bare presence vs. count
  vs. coarse kind) is configurable (§11). It is a list-level rule, not
  per-entity concealment.

### 5.2 Examination and reading

- `look <target>` and any read action (signs, inscriptions, labels)
  require at least `dim`. At `gloom`/`black` they return a "too dark to
  make it out" response instead of the detail.
- Inventory inspection of items **held by the viewer** is not gated
  (you can feel what you carry); examining things **in the room** is.

### 5.3 Combat

- Darkness imposes a **to-hit penalty** scaled by how dark it is for the
  **attacker** (their effective light): the darker, the larger the
  penalty. The penalty degrades accuracy; it never makes combat
  impossible (no hard block). Magnitudes are configurable (§11).
- A lit attacker (effective `lit`) suffers no darkness penalty.
- Whether a **lit target** is easier to hit regardless of the
  attacker's own light (illuminating yourself as a combat liability) is
  a refinement left open (§12); the base rule keys the penalty to the
  attacker's effective light only.

### 5.4 Movement and the escape invariant

- Movement into a room the viewer cannot see is **allowed by default**
  (you stumble through; the destination is unseen until you arrive and
  it is rendered per §5.1). A room or zone MAY mark movement **blocked**
  in darkness for genuine hazards (a cliff path); this is content-
  defined and off by default.
- **Escape invariant (normative):** a character can never be trapped by
  darkness. This is guaranteed by the combination of:
  - `outdoors` rooms are never `black` (ambient floors them at
    `gloom`, where exits are visible per §5.1); and
  - in any room, **the direction the character entered from is always
    known to them** (they can retrace their steps), even at `black`.
  Other exits in a `black` room are hidden until the viewer raises the
  light. The engine MUST NOT produce a state where a legally-occupiable
  room offers a character no discoverable exit.

**Acceptance criteria**

- [ ] At `gloom` the room prose is replaced by a terse dark form, exits
      render as bare directions, and occupant identities are hidden.
- [ ] At `black` the room name, description, and occupants are withheld.
- [ ] `look <target>` and reading require at least `dim`; below that
      they return a too-dark response.
- [ ] Examining items the viewer holds is never gated by room light.
- [ ] Combat applies a to-hit penalty that grows as the attacker's
      effective light falls, and is zero at `lit`; combat is never
      blocked by darkness.
- [ ] Movement into an unseen room is allowed unless the room/zone marks
      darkness-blocked.
- [ ] No legally-occupiable room ever leaves a character with no
      discoverable exit (escape invariant): `outdoors` is never `black`,
      and the entry direction is always known.

---

## 6. Light transitions

Light is felt as it changes, not only when first entered.

- On `time.period.change` (`time-and-clock.md` §3.4), every **occupied,
  eligible** room whose effective light level changes for its occupants
  emits a **transition message** to those occupants ("Darkness falls."
  / "The first light of dawn touches the room."). This reuses the
  period-change broadcast seam the weather time-ambience already uses
  (`world-rooms-movement.md` §6). Transition messages are configurable
  per direction (darkening vs. brightening) and crossing.
- A light source **guttering out** (§3.2) transitions the room for its
  occupants the same way (the room may drop a level when the torch
  dies).
- Transitions are level-crossing events: a period change that does not
  change a room's effective level (e.g. dusk → night in a room pinned
  `lit`) emits nothing.
- Transitions are per-room and reflect each occupant's own effective
  level where it differs (a darkvision viewer may feel no transition a
  human does); the minimum required behavior is that occupants for whom
  the level crossed are notified.

**Acceptance criteria**

- [ ] A period change that lowers/raises a room's effective light
      notifies its occupants with a transition message.
- [ ] A period change that does not cross a level for a room emits
      nothing for that room.
- [ ] A source guttering out transitions the room when it drops the
      room's level.
- [ ] An empty room emits no transition (no occupants to notify).

---

## 7. Game-time persistence (resolves `time-and-clock.md` §3.6)

Because darkness now gates gameplay, the clock must not reset the world
to night on every restart (`time-and-clock.md` §3.6 is hereby resolved
in favour of persistence; PD-7).

- In-game time (`CurrentHour` and `DayCount`) is **persisted** and
  restored at boot, so the world resumes at the time it stopped.
- It is a **global** artifact (one clock for the world), written
  alongside other global state, not per-player (cf. the global channel-
  scrollback store, `chat-channels-and-tells.md`). It is **not** part of
  any player save.
- At boot the clock **seeds** `CurrentHour`/`DayCount` from the saved
  value when present; absent or unreadable saved time falls back to the
  documented initial state (`time-and-clock.md` §3.5: hour 0, day 0).
  A fresh world therefore still cold-starts deterministically.
- The saved time is updated at a cadence that bounds loss to at most a
  small, configurable amount of in-game time on an unclean shutdown
  (e.g. written on hour advance and/or on the autosave tick); a clean
  shutdown flushes the current time.
- Sub-hour position (the tick remainder inside the current hour) need
  not be preserved; restoring to the start of the saved hour is
  sufficient.

**Acceptance criteria**

- [ ] In-game `CurrentHour` and `DayCount` survive a clean restart.
- [ ] A world with no saved time cold-starts at the documented initial
      state.
- [ ] Unreadable/corrupt saved time falls back to the initial state, not
      a crash.
- [ ] Saved time is global, not stored on any player save.
- [ ] A restart never forces a room dark that was lit before it (subject
      to the same period/terrain rules).

---

## 8. Presentation and GMCP

- **Render states** map to the scale and reuse the color pipeline
  (`ui-rendering-help.md`): `lit` renders as today; `dim` renders the
  full body wrapped in a muted/night tint (a semantic theme tag, the
  same mechanism as the weather/time ambience tints); `gloom` renders
  the obscured form heavily subdued; `black` renders the single dark
  line. All degrade to clean text on no-color clients, because the
  tint is markup the renderer strips.
- **GMCP** surfaces the viewer's effective light level on the room-info
  package (`networking-protocols.md`), so capable clients can theme the
  viewport or swap a day/night map. The server sends the level; the
  client renders. The exact field name/shape is pinned at GMCP-slice
  time against a live client.
- A **probe verb** lets a player read the current light/time directly
  ("It is night; here it is dark.") rather than inferring it — a
  read-only convenience.

**Acceptance criteria**

- [ ] Each effective level produces its documented render state.
- [ ] All light presentation degrades to clean text with color
      disabled.
- [ ] The room-info GMCP package carries the viewer's effective light
      level.

---

## 9. Content authoring and migration

- Authors set a room's `light` property (and the existing `terrain`) to
  control its light. The property is registered in the engine property
  registry and validated at load like other room properties
  (`world-rooms-movement.md` §2.2 / the property registry).
- **Fail-safe default.** A room with no `light` override and no terrain
  follows the sky (`outdoors`): lit by day, gloom by night — visible.
  Unaudited content therefore fails **safe** (always at least
  navigable), never dark.
- **Migration obligation.** Introducing this feature makes every
  existing `indoors`/`underground` room darker than before — an
  `underground` room with no override becomes `black`. Existing content
  MUST be audited: rooms that should remain visible (a torch-lit
  dungeon corridor, an inn interior) get an explicit `light` override or
  a light source placed in them. The migration is a content pass, not an
  engine change; the engine's only obligation is the fail-safe default
  so the audit can be incremental.

**Acceptance criteria**

- [ ] The room `light` property is registered and load-validated.
- [ ] A room with neither override nor terrain renders as `outdoors`
      (sky-governed), never `black`.
- [ ] Authored `light` overrides take effect at load with no code
      change.

---

## 10. Observable events

| Event | Fires when | Carries |
|---|---|---|
| `light.source.extinguished` | a lit fuel source gutters out | the source, its holder/room |
| (existing) `time.period.change` | period boundary crossed | period, previous, hour — consumed here to drive §6 transitions |

Light-level *changes for a viewer standing still* are conveyed by §6
transition messages and by the next render, not by a dedicated
per-viewer event. A lit/extinguish action is an ordinary command result,
not a bus event, except for the gutter-out case above.

**Acceptance criteria**

- [ ] A fuel source guttering out emits `light.source.extinguished`.
- [ ] §6 transitions are driven by the existing `time.period.change`
      event, not a new periodic light event.

---

## 11. Configuration surface

The following are externally configurable and not fixed by this spec.

| Policy | Where it applies |
|---|---|
| Ambient level per period (day/dawn/dusk/night → light level) | §2.2 |
| Indoor ambient cap | §2.3 |
| Light-source levels (per item, content) | §3.1 |
| Auto-light on equip (on/off) | §3.1 |
| Fuel burn cadence and amount | §3.2 |
| Darkvision floor and cap | §4 |
| Light-effect floor and duration | §4 |
| Terse-`gloom` and `black` render strings | §5.1 |
| Occupant-coarsening granularity at `gloom` (presence / count / kind) | §5.1 |
| Examination/reading minimum level | §5.2 |
| Combat to-hit penalty per level below `lit` | §5.3 |
| Movement-blocked-in-dark (per room/zone) | §5.4 |
| Transition messages (darkening / brightening, per crossing) | §6 |
| Saved-time write cadence | §7 |
| Room `light` PIN (per room/area/zone, content) | §2.4 / §9 |
| Room/area `light_floor` (lamp-lit settlement floor, content) | §2.4 / §9 |

---

## 12. Open questions / future work

- **Lit target easier to hit (§5.3).** Should illuminating yourself make
  you easier to hit regardless of the attacker's light — a tactical
  liability that balances carrying a torch? The base rule keys the
  penalty to the attacker's light only; a target-illumination term is a
  natural refinement once combat feel is tuned.
- **Light as a beacon (stealth hook).** A lit source is, in principle,
  visible to others ("a torch bobs in the dark to the north") and
  should defeat the bearer's own concealment. This spec exposes the
  source-light data but defines no hiding; it belongs to a future
  stealth/`visibility.md` interaction.
- **Visibility-spec precedence.** Seeing a thing now has two gates:
  darkness (this spec) and concealment (`visibility.md`/
  `hidden-exits.md`). A hidden exit in a lit room stays hidden; a
  visible exit in a `black` room is hidden by darkness. The exact
  composition order should be pinned jointly with `visibility.md`.
- **Black-room exit discovery.** The escape invariant guarantees the
  entry direction is always retraceable, and `outdoors` is never black.
  Whether a character may also "feel for obvious exits" in a `black`
  room (a kinder carve-out) vs. needing light to find any non-entry
  exit is left to tuning.
- **Light slot contention.** Whether the light slot contends with hands
  (two-handed weapons / shields) — a richer tradeoff — or is a free
  non-contending slot, depends on the equipment model and is deferred to
  it.
- **Moonlight and weather-driven ambient.** Night ambient is a flat
  `gloom` today, independent of moon phase or sky cover — yet a full
  moon on a clear night is navigable without a torch, and a new moon or
  heavy overcast is not. The refinement makes night ambient a function
  of period **and** moon phase **and** cloud cover: `ambientFor(period,
  moonPhase, cloudCover)`, where the moon lifts the night floor (gloom →
  dim on a bright clear night) and clouds gate it (and gate daylight
  down — the "Weather dimming" item below is the same machinery, the
  opposite direction). Phase is a pure function of the in-game day
  (`gameclock.DayCount`), so no new persisted state — like the period is
  a pure function of the hour. It touches three specs (`time-and-clock`
  for the lunar calendar + phase vocabulary, this spec for the
  `ambientFor` signature, `weather`/`world-rooms-movement §6` for cloud
  cover as the gate). It composes cleanly with §2.4's `light_floor`: a
  lamp-lit village keeps its floor regardless of moon, while hamlets and
  wilds (no floor) gain moonlit navigability for free. Tracked as a
  greenfield slice in `docs/BACKLOG.md §2`; **subsumes** the
  "Weather dimming" item below.
- **Weather dimming.** Heavy overcast/storm could knock daylight down a
  level (the weather state is already on the area). Atmospheric and
  cheap, but a second-order input; deferred so the core ships first.
  *(Folded into "Moonlight and weather-driven ambient" above — same
  `ambientFor(period, moon, clouds)` machinery.)*
- **Biome light defaults.** A `biomes.md` tier could default a region's
  light (a cavern biome → `black`, a glowing forest → `dim`). §2.4
  leaves a cascade tier for it.
- **Refuelling and light economy.** This spec only requires fuel can
  reach zero and extinguish; refuel items/verbs, torch durability tiers,
  and the shop economy around them are content/economy work
  (`economy-survival.md`).
- **Per-viewer transition fidelity (§6).** The minimum is "notify
  occupants for whom the level crossed." Whether to compute and message
  every occupant's distinct crossing on every period change, or a
  coarser room-level approximation, is an efficiency/quality tradeoff
  for the implementation.
