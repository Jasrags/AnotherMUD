# Biomes — Feature Specification

**Status:** Draft · **Scope:** The ecological classification of a
room — a pack-registered **Biome** definition that the room's
existing `terrain` property keys into, generalizing today's
hardcoded weather/time shielding into biome metadata and adding
the data biomes drive: weather shielding, idle **ambience**, an
optional **spawn table**, and the **forage / node resource
tables** the gathering feature consumes · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the biome layer must do, not
*how*. Biome names, ambience cadence, and message policy live in
the configuration-surface table at §7.

Biomes are the **fourth item in the Gameplay Systems cluster**
(`BACKLOG.md` §2), designed in one pass with
[gathering](gathering.md) (the fifth), which consumes the
resource tables defined here. Biomes are **not** a brand-new
classification axis: the room `terrain` property
(`world-rooms-movement.md` §6.4) already defaults to `outdoors`,
already carries values like `forest` / `mountain`, and already
gates weather/time ambience. This feature **promotes that
property from a bare string into a registered definition** (PD-1)
— richer, but fully backward-compatible.

---

## 1. Overview

A **biome** is the ecological character of a room: forest,
mountain, swamp, cavern, town-street. The room's existing
`terrain` property is the **biome id**; a registered Biome
definition behind that id carries what the biome *drives*.

### 1.1 Richer terrain, one axis (PD-1)

Rather than add a second `biome` property alongside `terrain`,
this feature makes `terrain` point at a **Biome registry**:

- The property name stays **`terrain`** — weather/time
  (`world-rooms-movement.md` §6) already reference it, and
  existing content authored as `terrain: forest` keeps working.
- A `terrain` value that **matches a registered biome** gains
  that biome's behavior (shielding, ambience, spawn/resource
  tables).
- A `terrain` value with **no registered biome** behaves exactly
  as today — a bare string, default `outdoors` semantics, eligible
  for weather/time, no resources (full backward compatibility,
  §2.3).

One axis, extended — consistent with how faction reused
alignment's architecture rather than proliferating a parallel
system. The one shape this cannot express — a *sheltered* room
(indoors) that is *also* ecologically a swamp — is the rejected
two-axis model, noted in §8.

### 1.2 What biomes are *not*

- **Not a new room property.** No `biome` field is added; the
  existing `terrain` property is reused (§1.1).
- **Not a refactor of weather.** Weather zones and the message
  cascade (`world-rooms-movement.md` §6) are untouched; biomes
  only *supply* the shielding flag that §6.4 already consults,
  now as biome metadata instead of two hardcoded strings (§3).
- **Not a spawn-system rewrite.** Mob spawning stays
  area-driven (`mobs-ai-spawning.md`); a biome may *offer* a
  spawn table, but how/whether the spawner consumes it is that
  feature's call (§5).
- **Not persisted.** Biome definitions are content; biome
  ambience state is ephemeral (§6).

### 1.3 Pre-decisions

| ID | Decision | Status |
|---|---|---|
| PD-1 | Biome = the registered definition behind the existing `terrain` property (richer terrain, one axis). No new property; unregistered terrain values behave exactly as today. | Decided |
| PD-2 | The engine-known shielding terrains (`indoors`, `underground`) become **registered biomes that declare shielding**, generalizing the hardcoded `world-rooms-movement.md` §6.4 list into biome metadata. | Defaulted |
| PD-3 | Biomes register through the same namespaced/engine-scope registry mechanism as tags/slots/properties (`scripting-and-packs.md` §4), so core biomes (`outdoors`, `forest`) resolve unprefixed and existing bare `terrain` strings still match. | Defaulted |

---

## 2. The biome definition

A **Biome** is a pack-registered definition (a new content
registry) keyed by a `terrain` value. It carries, all optional
except the id:

- **id** — the `terrain` string it matches. Engine-scope ids
  (`outdoors`, `forest`) resolve unprefixed; pack-scope ids are
  namespaced (PD-3, `scripting-and-packs.md` §4).
- **display name / description** — presentation (e.g. for
  `look`'s terrain hint, if content surfaces it).
- **weather-shielded / time-shielded** — booleans generalizing
  §6.4 (see §3).
- **ambience** — a pool of idle flavor lines delivered
  periodically to rooms of this biome (§4).
- **spawn table** — an optional mob spawn table this biome
  contributes (§5).
- **forage table** — the ambient-forage resource table
  ([gathering](gathering.md) §2) for rooms of this biome.
- **node spawn table** — which harvestable resource nodes
  ([gathering](gathering.md) §3) spawn into rooms of this biome.

### 2.1 Resolution

A room's biome is resolved by looking up its `terrain` value in
the Biome registry. Missing property → the default biome
(`outdoors`, §7). Value present but unregistered → the
backward-compat bare-string behavior (§2.3).

### 2.2 Acceptance — definition

- [ ] A biome registers under its `terrain`-value id (engine- or
      pack-scope) and is resolvable from a room's `terrain`
      property.
- [ ] A room with no `terrain` property resolves to the default
      biome.
- [ ] Each optional field defaults to "absent / no effect" when
      the biome omits it.

### 2.3 Backward compatibility

- [ ] A room whose `terrain` value has **no** registered biome
      behaves exactly as before this feature: eligible for
      weather/time unless its value is `indoors`/`underground`,
      no ambience, no resources.
- [ ] Existing content authored with bare `terrain` strings
      requires no change to keep its current behavior.

---

## 3. Weather / time shielding (generalized)

`world-rooms-movement.md` §6.4 today hardcodes two shielding
terrains: `indoors` and `underground` rooms do not receive
weather/time ambience unless their exposed flag is set. This
feature **generalizes** that:

- A biome declares `weather-shielded` and/or `time-shielded`.
  The §6.4 eligibility check reads the biome's flags instead of
  the two hardcoded strings.
- `indoors` and `underground` ship as **registered biomes**
  with the shielding flags set (PD-2), preserving today's exact
  behavior. New shielding biomes (a sealed vault, a thick
  canopy) are now content, not a config/engine change — closing
  the §6.4 "new shielding terrains require a configuration
  change" gap.
- The per-room weather-exposed / time-exposed override flags
  (`world-rooms-movement.md` §6) are unchanged and still win.

### 3.1 Acceptance — shielding

- [ ] `indoors` / `underground` rooms remain shielded with no
      content change (the shipped biomes carry the flags).
- [ ] A content-defined biome with `weather-shielded` set
      shields its rooms from weather ambience.
- [ ] A room's explicit weather-exposed / time-exposed flag
      still overrides its biome's shielding.

---

## 4. Biome ambience

Biome ambience is **idle ecological flavor** — birdsong in a
forest, dripping water in a cavern — distinct from the two
existing ambience sources it sits beside:

- **Weather** ambience is weather-state-driven
  (`world-rooms-movement.md` §6.2).
- **Time** ambience is period-change-driven (§6.5).
- **Biome** ambience is **time-interval-driven**: at a configured
  cadence, the engine picks a random line from the biome's
  ambience pool and delivers it to occupied rooms of that biome.

### 4.1 Delivery

- A biome-ambience tick handler (cadence in §7) iterates biomes
  that declare an ambience pool, selects rooms of that biome that
  are **occupied** (no point flavoring an empty room), and emits
  one randomly-chosen line as a one-shot room message.
- Like time ambience (§6.5), biome ambience emits **no engine
  event** — it is pure presentation. Missing/empty pools are
  silently skipped.
- Shielding (§3) does **not** suppress biome ambience — a
  sheltered cavern still drips. Biome ambience is independent of
  the weather/time eligibility gate (it is the biome's *own*
  flavor, not sky/weather leaking in). Content that wants a
  silent biome simply omits the pool.

### 4.2 Acceptance — ambience

- [ ] A biome with an ambience pool delivers a random line to its
      occupied rooms at the configured cadence.
- [ ] Empty rooms and biomes without a pool receive/emit nothing.
- [ ] Biome ambience emits no bus event and is independent of the
      weather/time shielding gate.

---

## 5. What biomes drive (integration)

Beyond shielding (§3) and ambience (§4), a biome is the keying
layer two other systems read. As with faction's consumer table,
the **capability** is defined here; *which* consumers wire it in
v1 is milestone scope.

| Consumer | How it reads the biome | Spec |
|---|---|---|
| Gathering — ambient forage | the room's biome **forage table** is what `forage` rolls against | [gathering](gathering.md) §2 |
| Gathering — resource nodes | the room's biome **node spawn table** decides which harvestable nodes spawn there | [gathering](gathering.md) §3 |
| Mob spawning (optional) | a biome may offer a **spawn table**; the area spawn scheduler MAY draw from each room's biome in addition to area-level spawns. Mob spawning stays area-driven (`mobs-ai-spawning.md`); this is an additive source, not a rewrite (§1.2). | `mobs-ai-spawning.md` §3 |
| Intrinsic hazard (optional) | a biome may declare an **ambient hazard** — a payload (typed damage ± condition) applied to everyone in a room of that terrain, gated by a carried/worn **protection key** (a `toxic` zone's radiation, a `vacuum` zone's pressure). Derived from the biome, not persisted; the damage layer composes with §3 shielding + §4 ambience. | [area-effects](area-effects.md) §4.6 |

### 5.1 Acceptance — integration

- [ ] A biome exposes its forage table, node spawn table, and
      (optional) mob spawn table to consumers by id.
- [ ] Biome integration adds no mandatory coupling: a biome that
      declares none of these tables still functions (shielding +
      ambience only).

---

## 6. Persistence

Nothing in this feature persists:

- **Biome definitions** are content (the registry), loaded with
  packs.
- A room's **`terrain`** value is content (existing room data).
- **Biome ambience** state (which room was last flavored) is
  ephemeral, like weather and time ambience — not saved.

No player- or world-save field is added; no migration is needed.

### 6.1 Acceptance — persistence

- [ ] No new save field is introduced by this feature.
- [ ] A restart reloads biome definitions from content and loses
      no persisted biome state (there is none).

---

## 7. Configuration surface

| Setting | Default | Meaning |
|---|---|---|
| Default biome | `outdoors` | §2.1 biome for a room with no `terrain`. |
| Shipped shielding biomes | `indoors`, `underground` (both shielded) | §3 backward-compat for §6.4's hardcoded list. |
| Biome-ambience cadence | configured interval | §4 how often biome flavor fires. |
| Biome-ambience occupied-only | on | §4 skip empty rooms. |
| Biome registry scope | engine-scope for core, pack-scope namespaced otherwise | §2 / PD-3. |
| Terrain hint in `look` | off | whether `look` surfaces the biome display name (presentation). |

No biome *names* are hardcoded by the engine; all biomes are
content.

---

## 8. Open questions / future work

- **Two-axis model.** The rejected PD-1 alternative: a separate
  `biome` property distinct from `terrain` (exposure), so a
  sheltered-but-ecological room (indoor swamp hut) is expressible.
  Revisit only if such rooms become common; the one-axis model
  covers the overwhelming majority.
- **`biome.entered` event.** v1 has no biome-transition bus event
  (a "you enter dense forest" line, if wanted, is a render-layer
  message on movement). A `biome.entered` event would give quests
  a "reach the swamp" hook; deferred until a consumer needs it,
  paralleling how `exit.discovered` was added for quests in
  `hidden-exits.md`.
- **Biome-driven spawn depth.** §5 defines the capability; full
  integration with the area spawner (weighting, biome-vs-area
  precedence) is milestone scope when mob spawning adopts it.
- **Dynamic / seasonal biomes.** A biome shifting with season or
  weather (a marsh that freezes in winter) would derive from the
  game clock; out of scope, non-persisted like weather when it
  lands.
- **Biome-gated movement / hazards.** Impassable biomes (deep water
  needs swimming, lava) overlap movement and are not modeled here.
  The **damaging** half is now specced — an intrinsic ambient hazard
  a biome declares (radiation, pressure), gated by carried/worn
  protection — as [area-effects](area-effects.md) §4.6 (§5 table
  above); build pending with the rest of that spec.

---

## Cross-references

- `world-rooms-movement` — §6.4 the terrain/shielding logic this
  feature generalizes; §6 weather/time ambience biomes sit beside
  (§3, §4); the `terrain` property reused as the biome id (§1.1).
- `gathering` — the coupled fifth cluster item: consumes the
  forage table (§2) and node spawn table (§3) defined here.
- `mobs-ai-spawning` — §3 the area spawner that may additively
  consume a biome spawn table (§5).
- `scripting-and-packs` — §4 the registry scoping biomes use
  (PD-3); the new Biome registry.
- `time-and-clock` — the ambience tick cadence (§4) and the
  deferred seasonal-biome hook (§8).
- `persistence` — unchanged; no save state added (§6).
- `docs/specs/README.md` — reading-order placement (layer 2,
  beside world-rooms-movement), the registry table (Biome), the
  tick-handlers table (biome-ambience), and the unchanged
  NOT-persisted surface.
- `BACKLOG.md` — §2 Gameplay Systems cluster, fourth item;
  designed with gathering (fifth).
