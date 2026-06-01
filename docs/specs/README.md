# Feature Specifications

Language-agnostic specifications for every major engine subsystem in
AnotherMUD. Each spec describes *what* the feature must do, not *how*
to implement it. Specific values (timeouts, dice expressions, cap
tiers, color names) are policy and live outside the specs.

All specs use the same shape:

- **Overview** with core concepts and goals / non-goals.
- **Narrative sections** organized around the feature's operations.
- **Acceptance criteria** checklists per section, suitable for
  reading as tests.
- **Configuration surface** table of what's externally configurable.
- **Open questions** flagging design tensions worth deciding.

---

## Reading order

The specs can be read independently, but they form a layered stack
if you want to understand the engine from the bottom up:

### 1. Substrate

The pieces that everything else stands on.

- [time-and-clock](time-and-clock.md) — the tick loop, in-game
  hour clock, and tick-handler scheduling primitive.
- [persistence](persistence.md) — the property registry, account
  and player save shapes, atomic file I/O, autosave pipeline.
- [scripting-and-packs](scripting-and-packs.md) — pack
  discovery, two-phase loading, the sandboxed Lua runtime
  (gopher-lua), the bus bridge + engine API, hot reload.
- [networking-protocols](networking-protocols.md) — IConnection,
  telnet negotiation, GMCP, MSSP, WebSocket envelopes.
- [notifications](notifications.md) — per-entity priority queue
  for asynchronous addressed messages (tells, channel posts,
  system notices); offline routing and bounded growth.

### 2. World and entities

The simulated environment and the things in it.

- [world-rooms-movement](world-rooms-movement.md) — rooms,
  areas, exits, doors, temporary portals, weather, the entity
  tracking + tag-index layer.
- [tag-observers](tag-observers.md) — reactive `entity.tag_added` /
  `entity.tag_removed` bus events for systems other than the tag
  index; idempotency, payload, and the timing gotcha vs the
  double-buffered index. Substrate ahead of a consumer.
- [progression](progression.md) — stats, races, classes,
  tracks (XP / levels), alignment, training.
- [inventory-equipment-items](inventory-equipment-items.md) —
  item templates, slots, equip / unequip, container ops,
  stacking, keyword resolution.
- [mobs-ai-spawning](mobs-ai-spawning.md) — mob templates,
  area-driven spawning, AI behavior tick, disposition,
  mob-command queue, loot.

### 3. Action and interaction

The verbs players use and the systems that resolve them.

- [commands-and-dispatch](commands-and-dispatch.md) — command
  registration, resolution, arg typing, input parsing, ability-
  to-command bridge.
- [abilities-and-effects](abilities-and-effects.md) — ability
  registration, proficiency, action queue, validation pipeline,
  effects.
- [combat](combat.md) — engage / disengage, the combat round,
  hit / miss / damage, flee, death.
- [quests](quests.md) — definitions, prerequisites, stages,
  objectives, rewards, auto-tracking watcher, markers.
- [economy-survival](economy-survival.md) — currency, shops,
  sustenance, rest, consumables.
- [crafting-and-cooking](crafting-and-cooking.md) — recipes,
  crafting skills (proficiency), tiered crafting stations, the
  quality roll (output = a rarity tier), recipe acquisition; and
  cooking as the food specialization that feeds sustenance and
  grants quality-scaled well-fed effects. Permissive access,
  gated quality.
- [chat-channels-and-tells](chat-channels-and-tells.md) —
  multi-recipient channels (engine baseline + pack-defined),
  one-to-one private tells with offline inbox, per-channel
  global scrollback; consumer of the notifications substrate.
- [emotes](emotes.md) — table-driven and freeform room-scoped
  social actions with actor/target/room view substitution;
  uses the per-room broadcast path, not the notifications
  queue.
- [recall](recall.md) — per-character recall room bookmark;
  `set recall` / `recall` verbs; cancellable pre-event for
  content-layer cost/cooldown policies.
- [admin-verbs](admin-verbs.md) — the admin gate (commands marked
  admin, refused unless the actor holds the admin role), admin
  target resolution with visibility bypass, the baseline verb set
  (inspect / set / teleport / announce / restore / purge / reload),
  and the audit trail. Builds on roles-and-permissions.
- [who](who.md) — the connected-character roster verb; per-line
  columns, summary count, and which characters appear (all in v1;
  per-viewer hiding once visibility lands).

### 4. Player lifecycle

How a connection becomes a session becomes a character.

- [login](login.md) — name → email → password →
  Playing / Creating / takeover / link-dead reconnect.
- [character-creation](character-creation.md) — the wizard
  flow, validation, restart, atomic commit, spawn.
- [session-lifecycle](session-lifecycle.md) — PlayerSession,
  SessionManager, flood protection, idle timeouts, link-dead,
  takeover.
- [roles-and-permissions](roles-and-permissions.md) — per-character
  role set, the `HasRole` authorization check, grant/revoke,
  config seed/bootstrap. Consulted by admin verbs, the admin
  channel, and the §5 idle-sweep exemption.

### 5. Presentation

The output layer.

- [ui-rendering-help](ui-rendering-help.md) — color tags, theme
  registry, prompts, panels, help topics.
- [item-decorations](item-decorations.md) — rarity tiers (ordered,
  decorated, color/visibility) and essence (colored glyph) item
  markers; content registries, themed rendering (inline + padded),
  essence as stack identity.

---

## Cross-cutting topics

Some concerns surface in multiple specs. The summary view:

### Events

Every spec lists the engine-bus events it emits in its
**Observable events** section. A cancellable event is one
where a listener can flip a `cancel` field to abort the
operation. The set of cancellable events across the engine:

| Event | Emitted by |
|---|---|
| `alignment.shift.check` | [progression](progression.md) §6.4 |
| `entity.death.check` | [combat](combat.md) §6.1 |
| `entity.rest_state.changed` | [economy-survival](economy-survival.md) §5.3 |
| `container.item_adding` | [inventory-equipment-items](inventory-equipment-items.md) §4.5 |
| `item.consuming` | [economy-survival](economy-survival.md) §6.2 |
| `shop.buy`, `shop.sell` | [economy-survival](economy-survival.md) §3 |
| `recall.before` | [recall](recall.md) §3.1 |

### Registries and content

Most features are content-driven. The registries that pack
authors populate, in roughly the order packs touch them at
load time:

| Registry | Spec |
|---|---|
| Tag | [scripting-and-packs](scripting-and-packs.md) §4 |
| Property | [persistence](persistence.md) §2 |
| Slot | [inventory-equipment-items](inventory-equipment-items.md) §3.1 |
| WeatherZone | [world-rooms-movement](world-rooms-movement.md) §6 |
| Area | [world-rooms-movement](world-rooms-movement.md) §2.4 |
| Room (rooms live in `World` directly) | [world-rooms-movement](world-rooms-movement.md) §2 |
| Item template | [inventory-equipment-items](inventory-equipment-items.md) §2 |
| Theme | [ui-rendering-help](ui-rendering-help.md) §3 |
| Mob template, loot table, area-spawn | [mobs-ai-spawning](mobs-ai-spawning.md) §2, §3 |
| Ability | [abilities-and-effects](abilities-and-effects.md) §2 |
| Effect template | [abilities-and-effects](abilities-and-effects.md); applied by consumables [economy-survival](economy-survival.md) §6 |
| Race, class | [progression](progression.md) §3, §4 |
| Track | [progression](progression.md) §5 |
| Command | [commands-and-dispatch](commands-and-dispatch.md) §2 |
| Emote | [commands-and-dispatch](commands-and-dispatch.md) §7 |
| Quest | [quests](quests.md) §2 |
| Help topic | [ui-rendering-help](ui-rendering-help.md) §9 |
| Rarity tier, Essence | [item-decorations](item-decorations.md) §2, §3 *(spec; build pending)* |
| Recipe | [crafting-and-cooking](crafting-and-cooking.md) §3 *(spec; build pending)* |

Engine-vs-pack scope (engine-scope registrations are visible
to all packs without prefixing; pack-scope registrations are
namespaced) applies to tags, properties, and slots; see
[scripting-and-packs](scripting-and-packs.md) §4.

### Save / load surface

Each spec calls out what it persists. The aggregate view:

- **Account file** — id, email, password hash, character list,
  creation / verification timestamps.
- **Player file** — entity id, account id, name, location,
  tags, roles, stats (base + modifiers + vitals), properties,
  equipment, inventory, flat item list, **abilities +
  proficiencies**, **recall address**, **prompt template**.
- **Quest file** (sibling of player file) — active list,
  completed list.
- **Notifications file** (sibling of player file) — per-entity
  priority queue of undelivered messages awaiting drain on
  reconnect; see [notifications](notifications.md) §6.3.
- **Chat subscriptions file** (sibling of player file) — per-player
  channel subscription set (which channels the player is currently
  tuned in to); schema independent of `player.yaml`; see
  [chat-channels-and-tells](chat-channels-and-tells.md) §5.1.
- **Channel files** — global per-channel ring buffer of recent
  messages, shared scrollback across all players; lives under
  `saves/channels/`; see [chat-channels-and-tells](chat-channels-and-tells.md) §4.
- **Connection records** — content-defined, loaded by the pack
  pipeline after content load.
- **NOT persisted** — sessions, link-dead state, in-game time,
  weather, mob spawn tracking, temporary exits, active
  effects, rest state.

Details: [persistence](persistence.md), with feature-specific
sections in [quests](quests.md) §6, [progression](progression.md),
[session-lifecycle](session-lifecycle.md), [world-rooms-movement](world-rooms-movement.md) §6.6.

### Tick handlers

The handler set actually registered at boot (verified against the
composition root):

| Handler | Cadence | Spec |
|---|---:|---|
| pre-tick: world tag-buffer swap | per tick | [world-rooms-movement](world-rooms-movement.md) §3.4 |
| `ai-tick` | 1s | [mobs-ai-spawning](mobs-ai-spawning.md) §4 |
| `area-tick` (spawn scheduler) | 1s | [world-rooms-movement](world-rooms-movement.md) §6, [mobs-ai-spawning](mobs-ai-spawning.md) §3 |
| `game-clock` | 1 | [time-and-clock](time-and-clock.md) §3 |
| `combat-tick` (combat phases: ability / auto-attack / effects) | configured | [combat](combat.md) §3, [abilities-and-effects](abilities-and-effects.md) §4 |
| `effect-tick` (effect expiry) | configured | [abilities-and-effects](abilities-and-effects.md) |
| `sustenance-drain` | configured | [economy-survival](economy-survival.md) §4.4 |
| `vitals-regen` | configured | [session-lifecycle](session-lifecycle.md) (via game loop) |
| `prompt-flush` | 1 | [ui-rendering-help](ui-rendering-help.md) §7.3 |
| `scripting-schedule` | 1 | [scripting-and-packs](scripting-and-packs.md) (the `engine.schedule` primitive) |
| `gmcp-vitals-flush` / `-items-` / `-combat-` / `-effects-` / `-experience-` / `-charstatus-` | 1 each | [networking-protocols](networking-protocols.md) (GMCP package layer) |
| `autosave` | configured | [persistence](persistence.md) §6.2 |
| `idle-sweep` | configured | [session-lifecycle](session-lifecycle.md) §5 |
| `linkdead-cleanup` | configured | [session-lifecycle](session-lifecycle.md) §7.3 |

Cadence is in *ticks* (or "1s"/"configured" where derived from a
duration). With the default 100 ms tick rate, an interval of 10 fires
every second. (`mob-command-queue` and `corpse-decay` are specced but
not yet wired as standalone handlers.)

---

## Spec style

These specs intentionally take a **narrative + acceptance
criteria** form rather than RFC-style numbered requirements.
Trade-off:

- Narrative reads better for understanding intent.
- Acceptance criteria checkboxes drive test development.
- The "open questions" sections preserve design tensions that
  would otherwise be lost between spec and code.

The format is locked in; new specs should follow it.

The spec set is **behavior-only**: no specific values, no
library names, no implementation language. Where a value or
constant matters for interoperability (e.g. telnet option
codes, IAC byte values), the spec calls out the contract
explicitly. Otherwise everything numeric is in the
configuration-surface table.

---

## Open-question summary

Each spec carries its own open-questions section. The
highest-impact themes that recur across specs:

- **Hardcoded magic values.** Cap tiers (25/50/75/100), flee
  cooldown, sustenance cap, engine namespace (`tapestry-core`),
  Lua sandbox limits (timeout / instruction / memory), and
  several others are baked into source. Externalizing these
  is a cross-cutting cleanup.
- **Persistence gaps.** In-game time, weather state, link-dead
  recoverability across restart, active effects, temporary
  exits, and rest state are all lost on restart. Whether each
  *should* persist is a per-feature design call.
- **Order dependency in pack loading.** Several cross-pack
  references (door mirroring, fixture refs) work only because
  pack discovery is alphabetical. A topological sort over
  declared dependencies would make these explicit.
- **Stale event handling.** Several features have explicit
  "is this event stale?" guards (session takeover, combat
  death). A general staleness primitive (event versioning,
  generation counters) could replace ad-hoc guards.
- **Role enforcement not yet built.** The help-service role
  "tier" is a no-op stub — it doesn't actually elevate anyone.
  The real authorization model is now specced
  ([roles-and-permissions](roles-and-permissions.md): a flat
  `HasRole` capability check) and [admin-verbs](admin-verbs.md)
  gates on it, but neither is implemented yet, so no privilege
  gating is live today.
- **Unbounded growth.** Render cache, bad-input tracker,
  alignment history (this one is bounded), notification
  queues, and a few others have no eviction or cap.
  Memory-bounded production deployments need caps.

---

<!-- Updated: 2026-06-01 · 27 specs covering the engine substrate, world, action, lifecycle, and presentation layers. Some (roles-and-permissions, admin-verbs, item-decorations, tag-observers, who, crafting-and-cooking) are behavior contracts whose Go implementation is still pending. -->
