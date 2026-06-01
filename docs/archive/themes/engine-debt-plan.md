> **ARCHIVED 2026-06-01 — Theme E — Engine Debt (M14) shipped.** This per-theme execution plan is
> historical; its "Status: spec phase" header predates the build. Superseded by
> [`docs/BACKLOG.md`](../../BACKLOG.md) for open work and [`ROADMAP.md`](../../ROADMAP.md)
> for what shipped. Kept for the original sequencing and design reasoning.

---

# Theme E — Engine Debt (plan)

**Hook:** Close the deferrals that have been quietly accumulating across
M8-M11. The import cycle is already gone (cluster 1, `af94b0c`). What
remains is a small set of "spec says this should work; code path is
half-wired" items that block real features inside future themes.

**Source:** `docs/archive/THEME-AXIS-PLAN.md` §"Theme E — Engine Debt".
**Roadmap milestone:** M14 (see `docs/ROADMAP.md`).
**Status:** spec phase complete — every item already has a spec home,
this theme just wires consumers.

---

## What's left (after the m8-1 cluster-1 closure)

Six concrete items remain. Three are independent; three chain through
the property registry (E4 → E5 → E6). Pre-decisions in §"Open
pre-decisions" below.

### E1 — Vital re-clamp on max-affecting stat changes

**Spec:** `progression` / `combat`.
**Gap matrix:** §progression "Vital re-clamp under max-affecting stat
recompute".
**Memory:** `m8-1-deferred-fixes`.

Today `combat.Vitals` and `progression.StatBlock` max can diverge
silently. If an effect raises a player's CON (lifting `hp_max` from
40 to 50), nothing observes that and bumps the current-HP ceiling.
If max drops below current, current is never clamped down.

**Shape:** small. Add a listener seam to the stats recompute path;
on max-affecting recompute, call into Vitals.SetMax and let Vitals
clamp current as needed. ~1-2 days.

### E2 — Mob stat derivation from race + class at spawn

**Spec:** `mobs-ai-spawning` §3.2.
**Gap matrix:** §mobs-ai-spawning "mob stat derivation from race+class".
**Memory:** `m6-2` (deferred at M6.2 with note "no consumer yet").

Mob templates today declare a static `stats:` block; the race / class
fields are read but their progression modifiers don't actually flow
into the spawned mob's StatBlock. Mobs of race `human` class `fighter`
should inherit the same base stats players of that race/class would.

**Shape:** medium. Wire the race+class lookups into the mob spawn
path (Store.SpawnMob), apply modifiers to StatBlock at instantiation
under sourcekey `race:<id>` and `class:<id>`. Need to decide one-shot
vs. lazy (see pre-decisions). ~3-5 days.

### E3 — Consumable EffectTemplate registry

**Spec:** `economy-survival` / `abilities-and-effects`.
**Gap matrix:** §economy-survival "Effect-id registry for consumables".
**Memory:** `m11-5-deferred-fixes`.

`item.consumed` events carry `effect_id`, `effect_duration`,
`effect_data` — verified by tests. But there is no
`effect_id → EffectTemplate` lookup, so no subscriber can construct
an active effect to apply. Today `EffectTemplate` lives inline on
abilities (`progression/ability.go`); a separate id-keyed registry
is needed for the consumable path.

**Shape:** medium. Build a small `internal/effect.Registry` (or
extend an existing surface — see pre-decisions); pack-load templates
from `<pack>/effects/*.yaml`; subscribe to `item.consumed` and call
`effectMgr.Apply` with the resolved template. ~3-5 days.

### E4 — Property registry on persistence

**Spec:** `persistence` §2, §4.4.
**Gap matrix:** §persistence "Property registry + tagged-value envelope".

Today save state is flat: every persisted field has a hardcoded YAML
tag. The spec calls for a registered property surface where features
can declare typed properties (string/int/bool/list/map) and the
persistence layer rehydrates them into a tagged-value envelope on
disk. Unblocks E5 and E6.

**Shape:** medium. New `internal/property.Registry` + tagged-value
envelope codec; integrate with player.Save, mob/item instance
properties (already typed in m6-4 / m11-5 work). ~5-7 days.

### E5 — `world.Room.Property` bag

**Spec:** `world-rooms-movement` §2.2.
**Gap matrix:** §quests "no `world.Room.Property` bag (shares m7-6 gap)".

Depends on E4. Today rooms have hardcoded fields (description,
exits, items, mobs, tags, healing_rate, …); arbitrary content-
declared properties have no home. Adding the property bag is what
lets quest content say "this room grants quest X on entry."

**Shape:** small after E4. ~1-2 days.

### E6 — `quest_grant` on room

**Spec:** `quests` (watcher) + `world-rooms-movement` (property bag).
**Gap matrix:** §quests "`quest_grant` on item or room property".
**Memory:** `m10-9-deferred-fixes`.

Depends on E5. Once rooms carry a property bag, the quest watcher's
existing grant-trigger logic extends to read `quest_grant` on room
entry the same way it already reads it from items. The item-side is
already done in the M10 sweep.

**Shape:** small after E5. ~1 day.

---

## Suggested sequence

```
E1 ──┐
     ├── independent; ship in any order
E2 ──┤
     │
E3 ──┘

E4 ── E5 ── E6     chained; E4 first, then E5 unblocks E6
```

The independent block (E1/E2/E3) goes first — fastest user-visible
correctness wins. Then the chained block ships the property bag and
its first real consumer.

**Lean order:** E1 → E3 → E2 → E4 → E5 → E6.

- E1 is smallest and closes a known silent-divergence bug.
- E3 is the most user-visible (consumable effects actually apply).
- E2 is mostly correctness for content authors who set up race/class
  on mobs expecting it to mean something.
- E4 → E5 → E6 ships the property bag arc with its first quest
  consumer.

---

## Pre-decisions (locked 2026-05-30)

All six pre-decisions resolved before implementation begins. Headlines:

| ID | Decision |
|---|---|
| PD-1 | Property registry lives in new `internal/property` package |
| PD-2 | Tagged-value envelope: minimal types with explicit `kind:` field |
| PD-3 | Vital re-clamp: direct callback on StatBlock (Vitals registers at construction) |
| PD-4 | Mob stat derivation: at spawn, one-shot |
| PD-5 | EffectTemplate registry lives in new `internal/effect` package |
| PD-6 | Pack manifest: add `effects:` glob only (properties stay code-declared) |

### PD-1 — Property registry package shape

**Locked: `internal/property`** (new package). Clean separation;
mirrors `internal/slot`, `internal/srckey`, `internal/notifications`.
Persistence stays focused on file I/O.

### PD-2 — Tagged-value envelope type system

**Locked: minimal types + explicit `kind:` field.** Supported types:
string, int, bool, float, list (of same kind), map[string]any. Each
on-disk value carries an explicit `kind:` discriminator
(`kind: int`, `kind: bool`, …) so YAML's number-vs-string ambiguity
isn't load-bearing. Forward-compatible: new kinds extend the
enumeration without rewriting old files.

### PD-3 — Vital re-clamp mechanism

**Locked: direct callback on StatBlock.** Vitals registers a
handler (`onMaxChange func(stat string, oldMax, newMax int)`) at
construction. StatBlock invokes it after every max-affecting
recompute. Simpler than the bus path; matches Vitals' 1:1
relationship with its StatBlock owner.

### PD-4 — Mob stat derivation timing

**Locked: at spawn, one-shot.** `Store.SpawnMob` computes the
derived StatBlock once and stores it on the MobInstance. Matches
the player-side timing (character creation derives once, never
re-derives mid-session). Race/class are immutable post-spawn.

### PD-5 — EffectTemplate registry location

**Locked: `internal/effect`** (new package). Neutral home;
abilities (via `progression.AbilityRegistry`) and consumables
(via the new pack glob) both target effects, so neither owns the
registry. Ability code that constructs inline `EffectTemplate`
values keeps doing so; the new registry is for `effect_id`-keyed
references.

### PD-6 — Pack manifest content globs

**Locked: add `effects:` only.** `pack.ContentPaths` gains an
`Effects []string` field; the loader decodes
`<pack>/effects/*.yaml` into the new registry. Properties stay
code-declared — features register their typed property keys with
`internal/property` at boot, not via content YAML. This avoids
designing a content schema for property declarations before any
content needs one.

---

## Shape estimate

3-4 weeks per the theme-axis plan. Independent block + chained block
together. No user-visible demo by design — the wins are internal:

- Stat changes round-trip to current vitals (E1).
- Mobs honor their declared race/class (E2).
- Consumable potions actually do something (E3).
- The first content-declared room property works (E4 → E5 → E6).

Each item lands as its own commit; the theme closes when all six
ship.

---

## Tracking

- This file owns the live sequence + current step.
- `docs/ROADMAP.md` M14 heading carries the standard `[ ]/[x]` exit
  criteria.
- `docs/archive/TAPESTRY-GAP-MATRIX.md` entries under §progression,
  §mobs-ai-spawning, §economy-survival, §persistence, §quests get
  struck as each item closes.
- Per-item commits should close their corresponding memory entry
  by adding a `[RESOLVED M14.<n>]` annotation.

When M14 ends:
1. Strike the closed items from `docs/archive/TAPESTRY-GAP-MATRIX.md`.
2. Archive this file or leave for history.
3. Pick the next theme via the rubric in `docs/archive/THEME-AXIS-PLAN.md`.
   With Theme E closed, the rubric points hardest at Theme A
   (since Theme A is already done that means whichever has the
   strongest "yes" remains).
