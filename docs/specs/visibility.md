# Visibility — Feature Specification

**Status:** Draft · **Scope:** The per-observer "can X see Y?"
rules layered behind the existing permissive visibility filter
(`world-rooms-movement.md` §7): the four concealment sources
(hide, sneak, darkness, magical/admin invisibility), the four
detection paths (passive auto-detect, see-invisible/detect
traits, the `search` verb, reveal-on-action), the perception
contest that resolves roll-based concealment, and the ephemeral
state all of this lives in · **Audience:** Anyone reimplementing
or porting this feature in any language.

This document describes *what* the visibility surface must do,
not *how* to implement it. Verb names, defaults, dice shapes,
and edge-case policy live in the configuration-surface table at
§8.

Visibility is the **keystone of the Gameplay Systems cluster**
(`BACKLOG.md` §2). It is written first because three shipped
specs already forward-reference it as a future consumer —
`who.md` §4 ("once visibility rules land"), `admin-verbs.md` §3
(the `bypass_visibility` argument property), and
`commands-and-dispatch.md` §5.4 (the resolver visibility
filter) — and because hidden/secret doors
(`BACKLOG.md` §2) reuse its detection mechanic. The engine
already ships the **seam**: the `CanSee` / visible-entity-list
primitive in `world-rooms-movement.md` §7 and the
`BypassVisibility` arg flag in `commands-and-dispatch.md` §5.4.
This spec fills in the **rules** behind that seam; it does not
move the seam.

---

## 1. Overview

The world exposes one question — **"can this observer see that
target right now?"** — and every renderer, roster, and target
resolver routes through it instead of reading a room's entity
list directly. Today the answer is unconditionally *yes*
(`world-rooms-movement.md` §7). This spec makes the answer
depend on **concealment** carried by the target and
**perception** brought by the observer.

### 1.1 The hybrid resolution model (PD-1)

Concealment resolves by one of two paths, chosen per source:

- **Flag-gated (binary).** Magical invisibility, admin
  invisibility, and darkness are pierced or not by a yes/no
  counter the observer either holds or lacks (see-invisible,
  admin rank, a light source / see-in-dark trait). No dice.
- **Roll-gated (opposed contest).** Hide and sneak produce a
  **concealment score** on the target; the observer pierces it
  by winning a **perception contest** against that score. This
  reuses the same hit-style contest machinery combat already
  has.

A target may carry **more than one** concealment layer at once
(a magically invisible rogue who is also hidden). The filter
composes them with **AND**: the observer sees the target iff it
pierces *every* active layer (§2.2). This is the single rule
that keeps an arbitrary stack of concealments unambiguous.

### 1.2 What visibility is *not*

- **Not security.** Visibility filters *rendering and target
  resolution*, exactly as `commands-and-dispatch.md` §11 warns
  about command visibility predicates. It is a perception
  model, not an authorization boundary. Authorization is roles
  (`roles-and-permissions.md`).
- **Not movement.** Concealment does not change where an entity
  may go, only who observes it going there. A sneaking mover
  still walks real exits and honors doors.
- **Not persisted.** Concealment state and detection memory are
  ephemeral; they drop on logout and on restart (§7), like
  active effects and rest state.
- **Not a combat bonus.** Whether attacking-from-hide grants an
  ambush/backstab benefit is a combat/abilities concern
  (`combat.md`, `abilities-and-effects.md`); this spec only
  defines what *is seen*, and that an attack *reveals* (§4.5).
- **One documented exception — quest-spawn ownership.** The v1
  concealment sources are perception layers that fail *open* (an
  unknown source ⇒ visible). `quest-spawns.md` Phase 2 reuses this
  predicate for an *existence* gate (a quest spawn does not exist
  for a non-owner), which fails *closed* by design. That is the
  sole non-perception source riding this seam; it is scoped to
  quest-owned entities and carries its own layer source, so the
  perception model above is otherwise unchanged.

### 1.3 Pre-decisions

| ID | Decision | Status |
|---|---|---|
| PD-1 | Hybrid model: flag-gated for magical/admin invis + darkness; roll-gated (opposed contest) for hide/sneak. | Decided |
| PD-2 | v1 concealment sources: hide, sneak, darkness, magical/admin invisibility. | Decided |
| PD-3 | v1 detection paths: passive auto-detect, see-invisible/detect traits, the `search` verb, reveal-on-action. | Decided |
| PD-4 | Concealment lives in ephemeral tags (`hidden`, `sneaking`, `invisible`) plus a snapshot **concealment score** for the roll-based sources; nothing persists (§7). | Defaulted — open to revisit |
| PD-5 | v1 light model is minimal: a static `dark` room property, a `light` item property that illuminates the whole room when carried lit, and a `see_in_dark` observer trait. Time-of-day / weather-driven darkness and light-fuel timers are deferred (§9). | Defaulted — open to revisit |
| PD-6 | Detection is **sticky** per observer: a successful pierce is remembered against that concealment *instance* until the concealment is re-established, dispelled, or the observer leaves the room — preventing per-render flicker (§4.1). | Defaulted — open to revisit |

---

## 2. The visibility filter primitive

The filter is the one integration point. It exposes two shapes
over the existing §7 seam:

- **`CanSee(observer, target) → bool`** — the per-pair check.
- **`VisibleEntities(observer, room) → []entity`** — the room's
  occupant list with everything the observer cannot see removed.
  Built on `CanSee`; the convenience callers use most.

### 2.1 Invariants

- **Self is always visible.** An observer always sees itself,
  regardless of its own concealment. (A hidden rogue still sees
  themselves in `look`/`who`.)
- **Callers go through the filter.** Renderers, the `who`
  roster, GMCP room-char lists, movement broadcast, and the §5.4
  command resolvers obtain occupants through `VisibleEntities` /
  `CanSee`, never by direct room-list access. A later policy
  change stays a single integration point — the §7 contract.
- **Bypass is explicit.** A caller may pass the
  `BypassVisibility` arg property (`commands-and-dispatch.md`
  §5.4) to skip the filter — used by admin verbs to reach
  hidden/invisible/sneaking targets (`admin-verbs.md` §3). The
  filter does not consult roles itself; bypass is a *caller*
  decision.

### 2.2 Composition over concealment layers

A target carries zero or more **concealment layers**, each with
a source type (hide, sneak, magical-invis, admin-invis) plus,
for the whole room, the darkness layer (§3.3) applied to every
non-luminous occupant.

`CanSee(observer, target)` returns true **iff the observer
pierces every active layer** on that target:

- A flag-gated layer is pierced iff the observer holds the
  matching counter (§3.3, §3.4).
- A roll-gated layer is pierced iff the observer wins the §4.2
  perception contest against the layer's concealment score — or
  has already pierced this layer instance (§4.1 sticky memory),
  or holds a detect trait that auto-pierces the layer's class
  (§4.3).

With no active layers, the result is the legacy permissive
*yes*. This is how the spec degrades to today's behavior for
any entity that is not concealed.

### 2.3 Acceptance — the primitive

- [ ] An entity with no concealment layers is visible to every
      observer (legacy parity).
- [ ] An observer always sees itself even while concealed.
- [ ] A target with two concealment layers is visible only to an
      observer that pierces *both*.
- [ ] `VisibleEntities` omits exactly the occupants for which
      `CanSee` is false, and never the observer itself.
- [ ] A caller with `BypassVisibility` set receives the
      unfiltered list / a true `CanSee` for any target.
- [ ] No renderer, roster, GMCP char-list, or §5.4 resolver
      reads a room's raw occupant list around the filter.

---

## 3. Concealment sources

### 3.1 Hide (stationary)

The `hide` verb attempts to conceal a **stationary** actor in
its current room.

- On dispatch the engine publishes a cancellable
  `concealment.before` event (source type = hide) so content
  may forbid hiding (no cover, full light, sanctuary). If
  cancelled, the actor sees a generic "can't hide here" line and
  no state changes.
- If uncancelled, the engine computes a **concealment score**
  from the actor's hide proficiency + the governing stat +
  situational modifiers (room cover, ambient light; policy in
  §8) and rolls/derives a value to **set** (it is the difficulty
  other observers must beat, not re-rolled per observer). On
  success it sets the `hidden` tag + stores the score and
  publishes `entity.concealed` (type = hide).
- A failed attempt leaves the actor unconcealed (actor-only
  "you fail to hide"); it does not broadcast.
- Hide **breaks** when the actor moves to another room (unless
  also sneaking — §3.2) or performs a revealing action (§4.5).

### 3.2 Sneak (moving)

The `sneak` verb toggles a **moving** concealment: the actor
stays concealed *across* room changes and their movement does
not broadcast normal enter/leave lines to occupants who fail to
detect them.

- Sneaking sets the `sneaking` tag + a concealment score
  (computed like hide, from sneak proficiency). Toggling it off
  is a plain actor-only action.
- On each move, the score is **re-evaluated against the
  occupants of the destination** (for arrival) and the **source**
  (for departure): occupants who pierce it (§4) see the normal
  movement line; those who do not see nothing. The mover's own
  arrival render is unaffected.
- Sneak can **fail per room** — a sharp-eyed occupant simply
  pierces it; this does not drop the `sneaking` tag (the actor
  keeps trying to be quiet), it only means that observer saw
  this move.
- A revealing action (§4.5) drops sneaking, same as hide.
- Sneak and hide may both be active; hide governs being seen
  while stationary, sneak governs being seen while moving.

### 3.3 Darkness and light

Darkness is an **environmental** concealment of a room's
contents from an observer who has no way to see in the dark. It
is the one layer that originates on the *room*, not the target,
yet it resolves per-observer through the same filter.

- A room is **dark** when its `dark` property is set (v1: a
  static, content-authored room property — PD-5). In a dark
  room the darkness layer is applied to **every non-luminous
  occupant and to the room description itself**.
- An observer **pierces darkness** iff it (a) carries or wears a
  **lit light source** (an item with the `light` property in a
  state that emits), or (b) has the `see_in_dark` trait
  (racial or effect-granted), or (c) the *target* is itself
  **luminous** (a lit torch on the ground, a glowing item, a
  fire is visible in the dark to anyone).
- **One light lights the room.** A carried lit light source
  illuminates the whole room: while any occupant holds one, the
  room is not dark for *anyone* in it. (Light is a room-level
  illuminant when present, not a personal cone in v1.)
- When an observer cannot pierce darkness, `look` yields the
  room's **dark description** (policy, §8), the visible-occupant
  list is empty save luminous things and the observer itself,
  and whether exits are listed is render policy (§8 default:
  exits hidden in the dark).

Darkness is flag-gated (PD-1): no contest, just "has light /
see-in-dark / target is luminous".

### 3.4 Magical and admin invisibility

Both are **flag-gated** and, unlike hide/sneak, **do not break
on action** (an invisible mage may still cast).

- **Magical invisibility** is an `invisible` effect/tag applied
  by an ability, potion, or other content. It is pierced only by
  the `see_invisible` counter (§4.3); never by a perception
  contest. It expires/dispels through the normal effect
  lifecycle (`abilities-and-effects.md`), which emits
  `entity.revealed` (reason = expired/dispelled).
- **Admin invisibility** ("wizinvis") is a role-tied concealment
  toggled by an admin command (`admin-verbs.md`). It is pierced
  only by an observer of **equal or greater admin rank**
  (`roles-and-permissions.md` `HasRole`), regardless of any
  see-invisible counter. An admin-invisible character is also
  excluded from `who` per-viewer (`who.md` §4) and from room
  renders for lower-rank observers.

### 3.5 Acceptance — concealment sources

- [ ] `hide` sets the `hidden` tag + a concealment score and
      emits `entity.concealed` (type = hide) when uncancelled.
- [ ] A cancelled `concealment.before` aborts hiding with a
      generic message and no state change.
- [ ] Moving rooms drops `hidden` unless `sneaking` is also set.
- [ ] `sneak` conceals across moves; an occupant who pierces the
      sneak sees the movement line, one who does not sees
      nothing.
- [ ] In a `dark` room, an observer with no light / `see_in_dark`
      sees no non-luminous occupants and gets the dark room
      description.
- [ ] A single carried lit `light` source un-darkens the room
      for every occupant.
- [ ] A luminous target is visible in the dark even to an
      observer without light.
- [ ] `invisible` is pierced only by `see_invisible`, never by a
      perception contest, and does not break on the bearer's
      actions.
- [ ] Admin invisibility is pierced only by equal/greater admin
      rank and removes the character from `who` for lower-rank
      viewers.

---

## 4. Detection

### 4.1 Passive auto-detect and sticky memory (PD-6)

Roll-based concealment is contested **lazily, by the filter
itself**: the first time `CanSee` is consulted for an
(observer, roll-based layer) pair that the observer has not yet
pierced, it runs the §4.2 perception contest.

To avoid per-render flicker (seen, then unseen, then seen on the
next `look`), the result is **sticky**:

- A **successful** pierce is remembered in the observer's
  ephemeral **detection set**, keyed to that concealment
  *instance*. Subsequent `CanSee` calls return true without
  re-rolling.
- The memory is **invalidated** when the concealment instance
  is re-established (the target re-hides → new instance), the
  target's concealment drops (revealed), or the **observer
  changes rooms** (you lose track when you leave).
- A **failed** pierce is not remembered as a permanent miss;
  it may be retried on a later passive check at the engine's
  cadence (policy, §8) or upgraded by an active `search` (§4.4).
  The spec does not require constant re-rolling every render —
  a single result may be cached for a short window (§8) to bound
  cost.

A concealment **instance** therefore needs an identity (a
generation/id bumped each time the source re-establishes) so the
detection set keys off the right thing. This is conceptual;
representation is implementation detail.

### 4.2 The perception contest

Resolves a roll-based layer (hide, sneak):

- The **concealment score** is the value snapshotted when the
  source was established (§3.1/§3.2) — stable difficulty.
- The **perception value** is the observer's perception stat +
  any awareness proficiency + situational modifiers (light
  helps; being the target's combat opponent helps; policy §8),
  combined with a randomized roll in the same style as the
  combat hit contest (`combat.md`).
- The observer pierces iff perception meets/beats the
  concealment score (comparator + tie rule are policy, §8).

### 4.3 See-invisible / see-in-dark / detect traits

Standing counters granted by race, ability, effect, item, or
admin rank. Each auto-pierces a **class** of concealment without
a contest:

- `see_invisible` — pierces magical invisibility (§3.4).
- `see_in_dark` — pierces darkness (§3.3).
- `detect_hidden` — auto-pierces (or grants a large perception
  bonus to — policy §8) hide and sneak.
- Admin rank — pierces admin invisibility of lower/equal rank
  (§3.4) and, by convention, all roll-based concealment for
  staff comfort (policy §8; defaults to "admins see all when
  using the bypass, not passively" to keep staff presence
  predictable).

Traits granted by **effects** are not persisted (the effect
isn't, §7); traits granted by **race/abilities** persist exactly
as those systems already persist.

### 4.4 The `search` verb

`search` is an **active, higher-effort** detection attempt in
the current room. Where passive detection is opportunistic,
`search` is intentional and stronger.

- It runs a §4.2 perception contest against **every** roll-based
  concealment in the room **with an active-search bonus**
  (policy §8), and it is the verb that reveals **hidden exits /
  secret doors** (the hidden-doors feature, `BACKLOG.md` §2,
  hooks here — searching tests concealed exits alongside
  concealed entities).
- Successful pierces are added to the detection set (§4.1) and
  announced to the searcher ("you spot …"); a search that finds
  nothing emits the empty result line.
- `search` may carry an action cost / short cooldown (policy
  §8) so it is not free spam; v1 default is a single round of
  action, no hard cooldown.
- `search` does **not** pierce flag-gated concealment (magical
  invisibility, admin invisibility) — those need their counters
  (§4.3), not effort.

### 4.5 Reveal on action

Roll-based concealment is **fragile**: it drops the instant its
bearer takes a **revealing action**. Flag-gated concealment
(magical/admin invisibility) does **not** drop on action.

- A command is marked `breaks_concealment` in its registration
  (a registry flag alongside the existing admin / hand-parsed
  flags, `commands-and-dispatch.md` §2). The revealing class by
  default includes attacking, casting an offensive ability,
  speaking aloud (say/yell), and loud manipulation (open/close,
  get/drop/give). Quiet actions (look, examine, whisper,
  inventory) do not break concealment. The exact membership is
  policy (§8).
- On dispatch, if the actor carries `hidden` or `sneaking` and
  the command breaks concealment, the engine drops those tags
  **before/at** the action resolves (so the action is observed)
  and publishes `entity.revealed` (reason = acted).
- Magical and admin invisibility are untouched by this rule.

### 4.6 Acceptance — detection

- [ ] A passive `CanSee` against an unpierced roll-based layer
      runs one perception contest; a success is remembered and
      not re-rolled until invalidated.
- [ ] Detection memory clears when the observer leaves the room,
      when the target re-establishes concealment, and when the
      target is revealed.
- [ ] `see_invisible` makes an invisible target visible with no
      contest; `see_in_dark` pierces darkness; `detect_hidden`
      pierces hide/sneak per §4.3.
- [ ] `search` reveals concealed entities (and concealed exits,
      per the hidden-doors feature) with the active-search bonus,
      and reports an empty result when nothing is found.
- [ ] `search` does not reveal magical/admin invisibility.
- [ ] A `breaks_concealment` command drops `hidden`/`sneaking`
      and emits `entity.revealed` (reason = acted); the action is
      then observable to the room.
- [ ] An invisible bearer's `breaks_concealment` action does
      **not** drop invisibility.

---

## 5. Consumers and integration points

Visibility is substrate; these are the systems that must route
through §2 once it lands. Each is a single seam.

| Consumer | What changes | Spec |
|---|---|---|
| Room render (`look`, auto-look on entry) | occupant list via `VisibleEntities`; dark-room description when darkness unpierced | `ui-rendering-help.md`, `world-rooms-movement.md` §7 |
| `who` roster | per-viewer exclusion of admin-invisible / invisible characters | `who.md` §4 |
| GMCP room char list | the same filtered occupant set sent to clients | `networking-protocols.md` (GMCP room package) |
| Command target resolution | §5.4 resolvers filter candidates through `CanSee` unless `BypassVisibility` | `commands-and-dispatch.md` §5.4 |
| Admin verbs | pass `BypassVisibility` to reach concealed/offline targets | `admin-verbs.md` §3 |
| Movement broadcast | a sneaking mover's enter/leave lines filtered per-observer (§3.2) | `world-rooms-movement.md` (movement layer) |
| Combat / abilities | attacking reveals (§4.5); ambush bonus is *their* call, not this spec | `combat.md`, `abilities-and-effects.md` |
| Mob AI | mobs may hide/sneak via the same filter; AI use is `mobs-ai-spawning.md` territory | `mobs-ai-spawning.md` |

### 5.1 Acceptance — integration

- [ ] `look` omits concealed occupants the actor cannot see and
      shows the dark description when darkness is unpierced.
- [ ] `who` omits characters concealed from the viewer (§3.4).
- [ ] The GMCP room char list matches the `VisibleEntities`
      result for that observer.
- [ ] `kill`/`look at`/`get` cannot target an entity the actor
      cannot see, except through a `BypassVisibility` verb.
- [ ] A non-admin cannot resolve an admin-invisible target;
      an equal/greater-rank admin can.

---

## 6. Observable events

| Event | Fields | When | Cancellable |
|---|---|---|---|
| `concealment.before` | actor, source_type, room | a `hide`/`sneak` attempt, before it commits | **yes** |
| `entity.concealed` | entity, source_type, room | hide/sneak/invis established | no |
| `entity.revealed` | entity, source_type, reason (moved / acted / detected / dispelled / expired) | a concealment layer drops | no |

Notes:

- `entity.detected` (an observer piercing a concealment) is
  intentionally **not** a bus event — it would be high-volume
  and per-pair. It is a `debug` log line only.
- `concealment.before` follows the engine's thin-substrate
  pattern (mirrors `recall.before`): the engine ships the hook
  and a generic refusal message; **packs** impose where hiding
  is allowed by subscribing and cancelling on room tags (lit,
  no-cover, sanctuary).
- Magical-invisibility expiry rides the effect lifecycle and
  surfaces as `entity.revealed` (reason = expired/dispelled),
  not a second event from this feature.

### 6.1 Acceptance — observability

- [ ] Each table event fires with the documented payload.
- [ ] A cancelled `concealment.before` produces no
      `entity.concealed`.
- [ ] `entity.revealed` carries the correct reason for each of:
      moved, acted, detected, dispelled/expired.
- [ ] No per-pair detection event is published on the bus.

---

## 7. Persistence

Visibility state is **entirely ephemeral** — nothing in this
feature is written to a save, and the README "NOT persisted"
list gains: concealment tags (`hidden`, `sneaking`,
`invisible`), the snapshot concealment scores, admin
invisibility, and per-observer detection memory.

- On logout / link-death / restart, all concealment drops; a
  character returns fully visible. (Consistent with active
  effects and rest state, which also do not persist.)
- **Room darkness is not feature state**: the `dark` room
  property is content-authored and loaded with the pack like any
  other room property; it is not a per-session toggle that needs
  saving. (Dynamic, time-driven darkness — when it lands, §9 —
  would derive from the game clock, still not from a save.)
- Trait-granted detection (`see_in_dark` from a race, etc.)
  persists only insofar as the granting **race/ability** already
  persists; effect-granted detection does not persist because
  effects do not.

### 7.1 Acceptance — persistence

- [ ] A character that hides, then logs out and back in, is
      visible on return (no `hidden` tag restored).
- [ ] No new field is added to the player or account save by
      this feature.
- [ ] Restarting the server clears all concealment and detection
      memory.
- [ ] A `see_in_dark` racial trait survives logout (via the race
      record), while a `see_invisible` *effect* does not.

---

## 8. Configuration surface

| Setting | Default | Meaning |
|---|---|---|
| `hide` / `sneak` / `search` verb names | `hide` / `sneak` / `search` | Canonical verbs; aliases are policy. |
| Hide concealment formula | proficiency + governing stat + room-cover/light mods | Inputs to the §3.1 score. |
| Sneak concealment formula | proficiency + governing stat + mods | Inputs to the §3.2 score. |
| Perception formula | perception stat + awareness proficiency + situational mods + roll | Inputs to the §4.2 contest. |
| Contest comparator / tie rule | perception ≥ concealment wins; ties to the observer | §4.2 resolution. |
| Active-search bonus | a positive modifier over passive perception | §4.4. |
| `search` action cost / cooldown | one round of action, no hard cooldown | §4.4. |
| Passive re-check cadence / cache window | engine default | §4.1 bound on re-rolling. |
| Revealing-action set (`breaks_concealment`) | attack, offensive cast, say/yell, open/close, get/drop/give | §4.5 membership. |
| Detect classes | `see_invisible`, `see_in_dark`, `detect_hidden` | §4.3 counters. |
| Admin-sees-all mode | bypass-only (not passive) | §4.3 staff sight default. |
| `dark` room property + dark description | content-authored | §3.3. |
| `light` item property (emits when lit) | content-authored | §3.3. |
| Exits shown in the dark | hidden | §3.3 render policy. |
| Hide-failed / can't-hide-here / search-empty / spotted messages | policy strings | actor-facing wording. |

All text above is policy the renderer or pack may override; the
spec only requires *some* message in each slot.

---

## 9. Open questions / future work

- **Light-source depth.** v1 light is on/off and roomwide
  (PD-5). Fuel/burn-out timers, lanterns, refueling, and
  per-source radius are deferred; they ride a future light/fuel
  slice and would tick on the game loop.
- **Dynamic darkness.** v1 darkness is a static room property.
  Time-of-day (outdoor night via `gameclock`) and weather-driven
  darkness (`world-rooms-movement.md` §6) are the obvious next
  step; both derive darkness rather than authoring it, and both
  stay non-persisted (§7).
- **Darkness tiers.** v1 is binary lit/dark. A dim tier (reduced
  perception, not blindness) is a future gradient.
- **Infravision nuance.** v1 `see_in_dark` is full sight. The
  classic "see only living/heat, not items or room text" variant
  is deferred.
- **Ambush / backstab.** A combat bonus for striking from hide
  belongs to `combat.md` / `abilities-and-effects.md`; this spec
  only guarantees the attack reveals (§4.5). Pin it in whichever
  combat slice adds opening strikes.
- **Group / party sneak.** A leader sneaking a whole group is
  deferred to the grouping/party feature (`BACKLOG.md` §2,
  Character/Survival cluster); this spec is per-actor.
- **Sneak movement cost.** Whether sneaking is slower or costs
  more movement is deferred to the mana/movement-pool slice
  (`BACKLOG.md` §2).
- **Mob concealment & AI.** The filter is symmetric, so mobs can
  hide/sneak/ambush; *whether and how AI uses it* is
  `mobs-ai-spawning.md` work, not this spec.
- **Hidden exits/doors detail.** This spec defines that `search`
  (§4.4) is the reveal mechanic and that detection memory is
  per-observer; the **exit-side** representation (a hidden flag
  on the exit/door vs. a hidden-exit tag, reveal messaging,
  whether "found" persists per character) is the hidden-doors
  spec's job and is settled there.

---

## Cross-references

- `world-rooms-movement` — §7 is the seam this spec fills in;
  the movement layer is where sneak's per-observer enter/leave
  filtering (§3.2) attaches; §6 weather and the game clock feed
  the deferred dynamic-darkness work (§9).
- `commands-and-dispatch` — §5.4 resolver visibility filter and
  the `BypassVisibility` arg property (§2.1); the
  `breaks_concealment` registration flag (§4.5) sits beside the
  existing admin / hand-parsed flags; §11 "visibility is not
  security" is the shared caveat (§1.2).
- `who` — §4 becomes a live consumer: per-viewer hiding of
  invisible / admin-invisible characters (§3.4, §5).
- `admin-verbs` — §3 visibility bypass and admin invisibility
  (wizinvis) integration (§3.4, §5).
- `abilities-and-effects` — magical invisibility and the detect
  traits are effects/abilities; their lifecycle emits
  `entity.revealed` on expiry (§3.4, §4.3).
- `roles-and-permissions` — `HasRole` decides admin-invisibility
  rank piercing (§3.4) and is the real authorization boundary
  visibility is *not* (§1.2).
- `combat` — the perception contest reuses the hit-contest style
  (§4.2); attacking reveals (§4.5); ambush bonuses are deferred
  to combat (§9).
- `mobs-ai-spawning` — mobs as concealment users / detectors via
  the symmetric filter (§5, §9).
- `persistence` — the "NOT persisted" surface this feature adds
  to (§7).
- `docs/specs/README.md` — reading-order placement (layer 2),
  the cancellable-events table (`concealment.before`), and the
  NOT-persisted list all need this spec folded in.
- `BACKLOG.md` — §2 Gameplay Systems cluster; this is the
  keystone that unblocks hidden-doors and the `who`/admin
  forward-references.
