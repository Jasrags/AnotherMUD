# Area Effects: Grenadelike Weapons, Room Hazards, and Biome Hazards — Feature Specification

**Status:** Draft · **§4.6 biome hazards SHIPPED** (2026-07-13 — `internal/biome` HazardService + protection-key immunity + attacker-less environmental death; Shadowrun Glow City / Puyallup ash flats content; players-only, raw damage, no persistence per §5). **Implementation note:** the protection key is **wear-only** (an equipped item), a deliberate narrowing of §4.6(b)'s "carry or wear" — a sealed suit seals nothing in a backpack, so gearing up is a real decision. Grenades (§3), placed hazards (§4.1–4.5), and the placed-hazard world store remain build-pending. · **Scope:** The engine's first
**multi-target attack** — a shared *area-effect primitive* (a payload of typed
damage and/or a condition applied to every eligible creature in a region) and
its consumers: **grenadelike weapons** (thrown acid / oil / fireworks with
direct + splash damage and an ignition state), **room hazards** (placed,
persistent caltrops / oil pools that trigger on whoever enters or lingers), and
**biome hazards** (intrinsic, unplaced environmental damage — a `toxic` zone's
radiation, a `vacuum` zone's pressure — gated by carried/worn protection; §4.6) ·
**Audience:** Anyone reimplementing or porting this feature in any language.

This document describes *what* the feature must do, not *how* to implement it.
Specific damage, splash sizes, ignition chances, durations, and prices are policy
that lives in configuration or content (see §7).

Both halves are **greenfield**: every other attack in the engine is **one
attacker against one defender** ([combat](combat.md) §4). Nothing applies a
payload to *everyone in a region*, and nothing leaves a persistent, triggering
effect on a room. The two systems are specced together because they share that
single primitive (§2): a grenade applies it once on impact; a hazard applies it
repeatedly to whoever enters. The lit oil pool is the bridge — a thrown oil flask
that ignites *becomes* a room hazard (§3.4). This feature layers on
[combat](combat.md) (the damage step), [weapon-identity](weapon-identity.md) +
[armor-depth](armor-depth.md) (damage types + resistance), [conditions](conditions.md)
(the condition payload), [saves](saves.md) (the Reflex-to-avoid), [ranged-combat](ranged-combat.md)
(the throw), [economy-survival](economy-survival.md) (the consumable item),
[world-rooms-movement](world-rooms-movement.md) (room-attached hazard state), and
[visibility](visibility.md) (the hidden-trap hook).

---

## 1. Overview

Two long-requested capabilities — throwing a flask of acid into a knot of
enemies, scattering caltrops across a doorway — need the same thing the engine has
never had: a way to hit **more than one target at once**, defined by *where they
are* rather than *who you targeted*. This spec defines that shared **area-effect
primitive** and the two systems that consume it.

### Core concepts

- **Payload** — what an area effect *does* to a creature it catches: an amount of
  typed **damage** (reusing the [weapon-identity](weapon-identity.md) damage
  types and [armor-depth](armor-depth.md) per-type resistance) and/or a
  **condition** (reusing the [conditions](conditions.md) flagged-effect model —
  e.g. the caltrops speed-halve). A payload may carry an **avoidance save**
  (a Reflex save, [saves](saves.md)) that negates or halves it.
- **Region** — the set of creatures an area effect can reach. In a world with no
  sub-room positioning, the v1 region is **the room** (optionally minus the
  source) or, for a thrown weapon, the room resolved through the
  [ranged-combat](ranged-combat.md) engagement (§3.2). The region is *who is
  here*, not a measured radius.
- **Friend-or-foe rule** — whether the payload spares the **source** (the
  thrower, the caltrop-layer) and any future allies. The engine's first
  not-just-1v1 targeting decision (§2.3).
- **The area-effect primitive** — apply a payload to every creature in the
  region the friend-or-foe rule admits, resolving each one's save and resistance
  independently, and announce the multi-target result. The shared core of §2.
- **Grenadelike weapon** — a **thrown consumable** that fires the primitive once
  on impact: a **direct** hit on a primary target plus **splash** on the rest of
  the region, optionally leaving an **ignition** state (§3).
- **Room hazard** — a **placed, persistent** payload-emitter attached to a room
  that fires the primitive on a **trigger** (a creature entering, or lingering /
  fighting in the room) until it is cleared or expires (§4).
- **Biome hazard** — an **intrinsic, unplaced** hazard declared on a *biome* (or a
  single room) rather than laid down by an actor: a `toxic` zone's radiation, a
  `vacuum` zone's pressure. Same trigger-fires-the-primitive model as a room hazard,
  with three differences — it has no placer (environmental), it is gated by a
  carried/worn **protection key** (a sealed suit, rad gear) that grants immunity, and
  it is **derived from content, not persisted** (like weather and biome ambience). The
  "you can't go there without the right gear" layer (§4.6).

### Goals

1. Give the engine a single, reusable **multi-target attack** primitive and build
   both grenades and hazards on it rather than two bespoke paths.
2. Reuse the existing damage, resistance, condition, and save systems for the
   payload — an area effect is a *delivery mechanism*, not a new damage model.
3. Let content express thrown grenadelike weapons (acid, oil, fireworks) and
   placed hazards (caltrops, oil pools) as data over the one primitive.
4. Make a placed hazard **persist** across a restart (the explicit ask — a
   scattered caltrop field is durable world state), without making *all* room
   state persistent.
5. Compose: a thrown weapon that ignites becomes a hazard (§3.4), demonstrating
   the shared primitive end to end.

### Non-goals

- **A new damage or condition model.** Damage typing/resistance is
  [armor-depth](armor-depth.md); conditions are [conditions](conditions.md);
  saves are [saves](saves.md). This spec only *delivers* those to many targets.
- **Sub-room positioning / measured radii.** There is no grid; "within 5 ft of
  the landing point" becomes "in the region" (§2.2). Geometric splash is out of
  scope.
- **Friendly-fire by party.** Ally/party exemption needs a grouping system the
  engine lacks; v1's friend-or-foe rule is source-vs-everyone-else (§2.3).
- **The rocket-stack demolition mechanic.** The fireworks-as-siege-charge rule
  (stacked rockets, architecture-gated multipliers) is deferred (§8).
- **A general trap/snare system.** A hidden, armed, mechanical trap is a richer
  thing; v1 ships visible placed hazards with a *hook* for concealment via
  [visibility](visibility.md) (§4.4), not the full trap system (§8).
- **Cross-room area effects.** A grenade and a hazard act within one room; lobbing
  into an adjacent room reuses (and is bounded by) [ranged-combat](ranged-combat.md)
  Model C's cross-room limits and is deferred here (§8).

---

## 2. The area-effect primitive

The shared substrate both consumers call. It takes a **payload**, a **region**,
a **friend-or-foe rule**, and a **source**, and applies the payload to each
admitted creature.

### 2.1 The payload

A payload carries any of:

- **typed damage** — an amount and its damage type(s), run through the normal
  damage step ([combat](combat.md) §4.4) so per-type resistance
  ([armor-depth](armor-depth.md) §4) and mitigation apply per target;
- **a condition** — a [conditions](conditions.md) flag applied on a hit (e.g.
  caltrops' movement-halving), with its own duration and shake-off per that spec;
- **an avoidance save** — an optional Reflex save ([saves](saves.md)) each target
  rolls; on success the payload is **negated or halved** per the payload's policy
  (a grenade's splash half-on-save; caltrops' no-save-but-armor-ignoring rule are
  payload policy, §7).

A payload is **content data**, not code: acid is "1 type of damage, no
condition, half-on-save"; caltrops are "small damage + a movement condition."

### 2.2 The region

The region is the creatures **co-located** with the effect:

- for a **room hazard**, the room the hazard is attached to;
- for a **grenade**, the room resolved through the thrower's
  [ranged-combat](ranged-combat.md) engagement — same-room in v1; the per-
  engagement range band stands in for "near the landing point" if a finer split
  is ever wanted (§8), but v1 splash is **room-scoped**.

There is no measured radius. "Everyone within 5 ft of where it lands" is
rendered as "everyone in the region," because the engine has no intra-room
geometry (a deliberate simplification, §8).

### 2.3 The friend-or-foe rule

Every area effect names a **source** and applies a friend-or-foe rule:

- **direct vs. splash** (grenades, §3.2): the primary target takes the direct
  payload; everyone else admitted takes the splash payload (often smaller). A
  hazard has no "direct" target — every triggering creature takes the one payload.
- **source exemption**: whether the payload spares its source. A thrown grenade
  does **not** catch its thrower (they are not at the landing point); a placed
  hazard's layer **can** be caught if they later enter the field (the layer must
  remember where they put it) — but the policy is configurable (§7), and a hazard
  records its **placer** for attribution (kill credit, §6) regardless.
- **ally exemption is out of scope** (no grouping system); v1 admits every
  creature in the region except as the source-exemption above carves out.

### 2.4 Application and the multi-target announcement

To apply, the primitive:

1. emits a **cancellable `area_effect.before`** (§6) so a ward / content rule can
   abort the whole application; a cancel stops it with no damage, no condition,
   no message beyond the cancel's own;
2. resolves the **admitted target set** from the region and the friend-or-foe
   rule **once**, so creatures entering/leaving mid-resolution don't double- or
   half-count;
3. for **each** target independently: rolls the avoidance save (if any), runs the
   damage through the per-target resistance step, applies the condition (if any),
   and routes death to the normal death path ([combat](combat.md) §6) — so a
   grenade *can* kill, and the kill is credited to the source (§6);
4. **announces** the multi-target result coherently — the actor view, each
   victim's view, and a room view that reads as one event ("The flask bursts,
   spattering everyone nearby!") rather than N separate hit lines.

This is the engine's first attack that resolves a **set** of defenders in one
action; the round/turn accounting, threat, and kill-credit seams
([combat](combat.md)) must tolerate a one-to-many resolution.

**Acceptance criteria**

- [ ] A payload delivers typed damage and/or a condition to each admitted target,
      running each through the normal per-target resistance and condition systems.
- [ ] An avoidance save is rolled per target; success negates or halves per the
      payload policy.
- [ ] The admitted target set is resolved once; the friend-or-foe rule decides
      direct/splash and source exemption; ally exemption is not modeled.
- [ ] `area_effect.before` is cancellable and aborts the whole application cleanly.
- [ ] A target reduced to death dies through the normal death path with the kill
      credited to the source.
- [ ] The room sees one coherent multi-target announcement, not N separate lines.

---

## 3. Grenadelike weapons

A grenadelike weapon is a **thrown consumable** that fires the primitive once on
impact. It reuses [ranged-combat](ranged-combat.md)'s throw and
[economy-survival](economy-survival.md)'s consumable item.

### 3.1 The item and throwing it

A grenadelike weapon is an item a character **throws** at a target in range
([ranged-combat](ranged-combat.md)). Unlike an ordinary thrown weapon (which
lands recoverable, ranged-combat §3), a grenade is **consumed/destroyed on use**.
Throwing one requires **no weapon proficiency** unless the item's content says
otherwise (the fireworks-rocket exception, §8). A thrown grenade is an attack
action that resolves immediately on the tick it is thrown.

### 3.2 Direct hit and splash

On a throw the primitive (§2) runs with a **direct/splash** friend-or-foe rule:

- the **primary target** (what the thrower aimed at) takes the **direct** payload;
- **every other creature in the region** (§2.2), excluding the thrower, takes the
  **splash** payload — typically smaller, and typically a half-on-save Reflex
  payload (§7);
- a **miss** on the primary target still lands the grenade *somewhere* — content
  policy decides whether a missed throw still splashes the region or is simply
  wasted (the "grenade scatters" rule, §7).

### 3.3 Ignition and fuses

Some grenades have a **delayed or chance-based secondary effect**, modeled as a
**scheduled** follow-up on the tick scheduler (the same primitive
[scripting-and-packs](scripting-and-packs.md)'s `engine.schedule` /
[abilities-and-effects](abilities-and-effects.md)'s effect ticks use):

- **oil** has a chance to **ignite** on impact (a configured chance, §7); on
  ignition it leaves a **lit pool** (§3.4);
- a **firework rocket** detonates on a **delay** (a fuse) — it is thrown/planted
  one tick and the area effect resolves a configured number of ticks later, a
  window in which targets can act;
- a target *on fire* (or in a burning area) may attempt a **Reflex save**
  ([saves](saves.md)) to extinguish / escape, per the payload's policy.

Ignition state is the grenade half of the **bridge** to hazards.

### 3.4 The oil-pool bridge

A thrown oil flask that **ignites** (§3.3) does not just resolve once — it
**creates a room hazard** (§4): a lit floor-oil pool that fires a fire payload on
the primitive against creatures in the room each tick until it **burns out** (a
duration, §7). This is the canonical demonstration that the two consumers are one
system: the grenade *places* a hazard. A poured-but-unlit oil flask likewise can
become a (non-burning) hazard that a later spark ignites — content's call (§8).

**Acceptance criteria**

- [ ] Throwing a grenade consumes/destroys it, requires no proficiency by default,
      and resolves on the throw tick.
- [ ] The primary target takes the direct payload; others in the region (not the
      thrower) take the splash payload; a missed throw resolves per the scatter
      policy.
- [ ] An igniting/fused grenade schedules its secondary effect on the tick
      scheduler; a configured chance/delay governs it.
- [ ] An igniting oil flask creates a lit-pool **room hazard** (§4) that burns for
      a configured duration, exercising the grenade→hazard bridge.

---

## 4. Room hazards

A room hazard is a **placed, persistent** payload-emitter attached to a room. It
fires the primitive on a **trigger** until cleared or expired.

### 4.1 Placement

A hazard enters the world by:

- a **player action** — scattering caltrops, pouring oil — which consumes the
  placing item ([economy-survival](economy-survival.md)) and attaches a hazard to
  the actor's room, recording the actor as **placer** (§2.3);
- **content / scripting** — an authored or scripted hazard placed at load or by a
  trap-spring event (a content hook, not the full trap system, §8);
- the **grenade bridge** — an igniting oil flask (§3.4);
- **intrinsic to a biome / room** — not laid down at all, but declared on the
  location itself (§4.6): every room of a hazardous biome *is* the hazard, with no
  placer and no placing action.

A hazard is attached to a **room** (or a room + a footprint the content declares,
e.g. "the doorway"); v1 treats the footprint as the whole room unless content
narrows it (§8). Multiple hazards can coexist in one room.

### 4.2 The trigger model

A hazard declares one or both triggers:

- **on-enter** — a creature **entering** the room takes the payload once on
  arrival (caltrops bite the stepping foot);
- **on-tick-while-present** — a creature **lingering or fighting** in the room
  takes the payload on a recurring hazard tick (a burning pool, an acid mist),
  via a `hazard` tick handler (§7 cadence).

Each trigger fires the **primitive** (§2) against the triggering creature(s) —
on-enter against the one who entered, on-tick against everyone present the
friend-or-foe rule admits. A creature already present when a hazard is *placed*
is covered by the on-tick path (or an immediate first application — content
policy).

### 4.3 Lifetime: clearing and expiry

A hazard ends by:

- **expiry** — a configured **duration** (a burning pool burns out; some hazards
  are permanent until cleared — caltrops "until swept");
- **clearing** — an action that removes it (sweeping caltrops, smothering a fire),
  which may be a skill check ([skills](skills.md)) or a simple action, content's
  call;
- **depletion** — an optional **charge count** (a caltrop field degrades after N
  triggers), so a hazard isn't necessarily eternal.

Expiry/clearing is driven by the `hazard` tick handler and emits the hazard's
removal so observers see it end.

### 4.4 Concealment hook (visibility)

A hazard MAY be **concealed** — a hidden caltrop field, a covered pit — reusing
[visibility](visibility.md)'s perception/`search` mechanic: a concealed hazard
carries a search difficulty, is not announced to a creature that hasn't perceived
it, and is found via the `search` path the same way a hidden exit is
([hidden-exits](hidden-exits.md)). **v1 default: hazards are visible**; the
concealed variant is a *hook* the visibility system already supports, with the
full armed-trap experience (disarming, telegraphing, trap rooms) deferred (§8).
A concealed hazard still triggers on an unaware creature — concealment hides the
*warning*, not the *bite*.

### 4.5 Friend-or-foe and attribution

A hazard applies the §2.3 rule: by default it catches **anyone** the trigger
admits, **including its placer** if they re-enter/linger (the layer must avoid
their own field) — the source-exemption policy (§7) can flip this. A hazard
remembers its **placer** so a triggered death is credited correctly ([combat](combat.md)
§6), even after the placer has left or logged out. A hazard with **no** placer
(content/environmental) credits no one.

**Acceptance criteria**

- [ ] A hazard can be placed by a player action (consuming the item, recording the
      placer), by content/scripting, or by the grenade bridge.
- [ ] A hazard fires the primitive on on-enter and/or on-tick-while-present
      triggers, per its declaration.
- [ ] A hazard ends by duration expiry, a clearing action, or charge depletion,
      and its removal is observable.
- [ ] A concealed hazard is unannounced to an unaware creature, discoverable via
      `search`, and still triggers; v1 defaults to visible.
- [ ] A hazard catches anyone admitted (placer included by default; policy can
      exempt); a triggered death is credited to the recorded placer, or to no one
      if environmental.

### 4.6 Biome & ambient hazards (intrinsic, unplaced)

Some hazards are not laid down by anyone — they are **intrinsic to a place**: the
radiation of a `toxic` zone (Glow City), the pressure/anoxia of a `vacuum` zone
(a hull breach), an acid-fog district. Instead of an actor scattering caltrops,
the hazard is declared on the **biome** ([biomes](biomes.md)) — so every room of
that terrain inherits it — or on a **single room** to make one location dangerous
without a whole biome. It reuses the room-hazard machinery (§4.1–§4.5: the
primitive, the trigger model, attribution) with **three** differences.

**(a) Environmental — no placer.** An intrinsic hazard has no placer; a death it
causes is credited to no one (the §4.5 environmental case). It is never *cleared*
by an action — it is a property of the location, not an object on the floor. It
ends only when content changes the location (a decontaminated-room override, a
sealed breach), not via a sweep/smother action.

**(b) A protection / immunity gate.** An intrinsic hazard names a **protection
key** — a content-declared tag or property a creature can **carry or wear** to be
exempt (a sealed vacuum suit vs. `vacuum`, rad gear vs. `toxic`, a filter mask vs.
acid-fog). A creature that holds the protection takes **no** payload; everyone else
takes it. This is the "you can't go there without the right gear" rule, and it is
the biome-hazard addition — placed hazards have no immunity concept (caltrops bite
everyone). Immunity **composes with**, and is distinct from, the per-type
**resistance** already in the payload (§2.1): resistance *reduces* the damage
(a rad-adapted ghoul soaks some of Glow City), protection *negates* it entirely
(a sealed suit takes none). A creature with neither takes the full payload.

**(c) Derived, not persisted.** An intrinsic hazard is **recomputed from the
biome/room definition at load**, exactly like weather and biome ambience
([biomes](biomes.md) §6, which the README save surface lists as deliberately
*not* saved). It is therefore **not** part of the placed-hazard world store (§5):
there is nothing to persist, because re-reading the pack reconstructs it. This
cleanly partitions the two families — **placed** hazards are durable world state
(§5); **intrinsic** hazards are derived content.

Everything else is unchanged. Triggers reuse §4.2 — **on-tick-while-present** is
the norm (radiation ticks while a runner lingers), and **on-enter** may fire a
first jolt on arrival. The primitive (§2), the payload (§2.1, incl. its optional
avoidance save and condition — irradiation can also apply a [conditions](conditions.md)
flag, e.g. fatigued), and the multi-target announcement (§2.4) are the same; only
the room copy differs ("The air itself sears your lungs."). An intrinsic hazard
**composes with the biome's other properties**: a `toxic` / `vacuum` / `underground`
room already carries its light and movement-cost traits ([biomes](biomes.md),
[light-and-darkness](light-and-darkness.md), [movement-cost](movement-cost.md)) —
the ambient hazard is the *damage* layer on top, so a toxic room is dark **and**
poisonous, and a barrens step is costly **and** (in Glow City) irradiating.

**Acceptance criteria**

- [ ] A biome — or a single-room override — can declare an intrinsic ambient hazard
      (a payload + trigger) with **no placer**; every creature present takes it per
      the trigger, environmental death credited to no one (§4.5).
- [ ] A creature carrying or wearing the hazard's declared **protection key** is
      exempt; a creature without it is not; per-type **resistance** still reduces the
      payload independently (immunity negates, resistance mitigates).
- [ ] An intrinsic hazard is **derived from content and not persisted** — it
      reconstructs from the biome/room definition on load (like weather/ambience) and
      is absent from the placed-hazard world store (§5).
- [ ] An intrinsic hazard composes with the location's light and movement-cost
      properties rather than replacing them (a toxic room is dark, costly, and
      damaging at once).

---

## 5. Persistence

Grenades are ordinary **consumable items** and need no new persistence beyond the
item itself.

Room hazards are **new durable world state**. Today rooms persist **no** dynamic
state — weather, spawn tracking, and temporary exits are explicitly *not* saved
([world-rooms-movement](world-rooms-movement.md), README save surface). Placed
hazards are the exception the ask requires: a scattered caltrop field **survives a
restart**. So this feature adds a **placed-hazard world store** holding each live
hazard's room, payload, trigger model, remaining duration/charges, concealment,
and placer attribution — additive and versioned/migrated like other world stores
(the auction listing store is the nearest precedent). A hazard whose duration is
better treated as transient (a short-lived burning pool) MAY be content-flagged
**not** to persist, so only durable hazards (caltrops) pay the save cost; the
split is policy (§7).

The exact store shape (a per-room attachment in a world save vs. a standalone
hazard store) is an implementation choice constrained only by the durability +
attribution contracts above; see [persistence](persistence.md).

**Intrinsic biome/room hazards (§4.6) are the opposite case: they are never
stored.** Because they are declared on the biome/room content and reconstructed at
load — like weather and biome ambience, which the README save surface lists as
deliberately not persisted — the placed-hazard store holds only *placed* hazards.
Restarting the server loses nothing about a biome hazard: re-reading the pack
recreates it. This keeps the world store bounded to durable, player/scripted
placements and out of the (potentially vast) set of intrinsically-hazardous rooms.

**Acceptance criteria**

- [ ] A durable placed hazard (its room, payload, trigger, remaining lifetime,
      concealment, placer) round-trips across a restart.
- [ ] A hazard content-flagged transient is not persisted and is gone after a
      restart.
- [ ] An intrinsic biome/room hazard (§4.6) is not written to the hazard store; it
      reconstructs from content on load.
- [ ] Grenade items need no hazard-specific persistence.

---

## 6. Observable events

- **`area_effect.before`** — **cancellable** (§2.4 step 1), emitted once before a
  payload application (grenade impact or hazard trigger). A listener can abort the
  whole application (a ward, a content rule, an admin no-fly zone). Mirrors the
  cancellable-pre-event pattern of [recall](recall.md) §3.1 /
  [loot-and-corpses](loot-and-corpses.md) `corpse.creating`.
- **Hazard placement / removal** — non-cancellable signals so observers and a
  future client surface see a hazard appear and end (§4.3). Whether these are
  distinct bus events or ride existing room-change signals is an implementation
  detail; the **observable contract** is that placement and end are visible.
- Damage, condition application, and death ride their **existing** events
  ([combat](combat.md), [conditions](conditions.md)) per target — this feature
  adds no per-target damage event, only the one-to-many application around them.

---

## 7. Configuration surface

The following are externally configurable and not fixed by this spec.

| Policy | Where it applies |
|---|---|
| Grenade roster — direct/splash damage, type, condition, save | §2.1, §3.2 (content) |
| Splash save rule (negate vs. half on success) | §2.1 |
| Missed-throw scatter policy (still splashes vs. wasted) | §3.2 |
| Ignition chance (oil) and fuse delay (rockets) | §3.3 |
| Reflex extinguish/escape rule for burning targets | §3.3 |
| Lit-oil-pool payload and burn duration | §3.4 |
| Hazard roster — payload, trigger model, duration/charges | §4 (content) |
| Biome/room intrinsic hazard — payload + trigger, declared on the biome or room | §4.6 (content) |
| Hazard protection keys — the tag/property that grants immunity to a biome hazard | §4.6 (content) |
| Hazard `hazard`-tick cadence | §4.2 |
| Hazard clearing method (action vs. skill check) | §4.3 |
| Hazard concealment / search difficulty | §4.4 (content) |
| Source-exemption policy (does a hazard/grenade spare its source) | §2.3, §4.5 |
| Hazard persist-vs-transient flag | §5 |
| Fireworks-rocket proficiency requirement | §3.1 |
| User-facing copy (burst, scatter, caltrops bite, pool ignites) | §2.4, §3, §4 |

---

## 8. Open questions / future work

- **Region granularity.** v1 splash/trigger is **room-scoped** — everyone in the
  room. The [ranged-combat](ranged-combat.md) range bands (far/near/melee) could
  later refine "near the landing point" to a band rather than the whole room, if
  splash feels too broad. Deferred until play shows it matters.
- **Footprint within a room.** A hazard attaches to the whole room in v1. A
  sub-room footprint ("the doorway only," "the east half") needs intra-room
  positioning the engine lacks; deferred with the sub-room-geometry question.
- **The hidden-trap system.** §4.4 ships a *concealment hook* over
  [visibility](visibility.md), not the full experience: arming/disarming
  ([skills](skills.md) Disable Device), telegraphed trap rooms, reset traps, and
  trap-as-content-puzzle are a later system that would extend this hazard layer.
- **Rocket-stack demolition.** The fireworks-as-siege-charge rule (planting
  stacked rockets, an architecture-knowledge gate for a tripled structure
  multiplier, retreat windows) is out of v1; it needs a structure/object-damage
  model the engine lacks.
- **Cross-room lobbing.** Throwing a grenade into an **adjacent** room couples to
  [ranged-combat](ranged-combat.md) Model C and its deliberately-bounded
  cross-room rules; v1 grenades and hazards are single-room.
- **Ally / party friendly-fire.** Source-vs-everyone is v1 (§2.3). Real
  friend-or-foe (spare your party, hit your enemies) waits on a grouping system;
  until then an area effect is genuinely indiscriminate, which is itself a design
  statement.
- **Caltrops-style armor-ignoring math.** The source material's caltrops ignore
  armor/shield/deflection but footwear helps — whether the payload's save/
  resistance model expresses that exactly, or approximates it, is a content-
  tuning question (§7) left to balance.
- **Hazard density / cleanup pressure.** Nothing caps how many hazards a room (or
  the world) accumulates. A field of permanent caltrops everywhere is a growth /
  grief concern; a cap, a global sweep, or mandatory durations may be needed
  (parallel to corpse decay). (Intrinsic biome hazards §4.6 are exempt — they are
  derived, not accumulated, so they carry no store-growth cost.)
- **Graduated exposure / dose (biome hazards).** v1 fires a flat payload per tick
  while present. Radiation/toxins realistically **accumulate a dose** — a rising
  meter that lingers after you leave, or a save DC that climbs with time-in-zone —
  rather than a memoryless per-tick hit. A persisted per-character dose is a richer
  model deferred here; the flat per-tick form is the v1 baseline. Ties to the SR
  Essence/health track if that lands (`docs/BACKLOG.md` Shadowrun cluster).
- **Protection degradation.** v1 protection is binary and permanent — hold the
  sealed suit, take no damage, forever. Consumable/degrading protection (a suit that
  wears out, an air supply that depletes, a filter that saturates) is a natural
  follow-on that turns "have the gear" into "manage the gear," but needs a
  durability/charge model on the protection item; deferred.
- **Client surfacing.** A room's active hazards are not exposed to a structured
  client channel (GMCP) today; a future HUD/map may want "this room is dangerous."
- **Balance.** Every number — splash damage, ignition chance, durations, save DCs,
  caltrop bite — is policy (§7) tuned once the systems are playable, mirroring the
  movement-cost / mounts balance notes.
```
