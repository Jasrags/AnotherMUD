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
- [faction](faction.md) — per-character **standing** with
  content-defined factions: a signed standing int per
  (character, faction), named ranks mirrored as tags, bounded
  history, the cancellable shift pipeline, and the `ResolveRanks`
  gating helper. A parallel sibling that generalizes alignment's
  architecture (`progression` §6) to N axes without touching it.
- [reputation](reputation.md) — per-character single-axis **renown**
  (fame ↔ infamy): a signed score with content-defined tier bands
  mirrored as tags, the shared cancellable shift pipeline, and a
  recognition check. Distinct from faction's per-faction *standing*
  (`reputation` §1.1) — the axis the WoT Fame/Infamy/Low Profile
  feats need. Substrate ahead of a consumer.
- [inventory-equipment-items](inventory-equipment-items.md) —
  item templates, slots, equip / unequip, container ops,
  stacking, keyword resolution.
- [mobs-ai-spawning](mobs-ai-spawning.md) — mob templates,
  area-driven spawning, AI behavior tick, disposition,
  mob-command queue, loot.
- [visibility](visibility.md) — the per-observer "can X see Y?"
  rules behind the permissive `world-rooms-movement` §7 filter:
  hide / sneak / darkness / magical+admin invisibility, the four
  detection paths (passive, see-invisible/detect traits,
  `search`, reveal-on-action), the hybrid flag+contest model.
  Keystone of the Gameplay Systems cluster; substrate ahead of
  its consumers (`who`, admin verbs, hidden doors).
- [hidden-exits](hidden-exits.md) — secret doors and secret
  passages: a `hidden` + `search_difficulty` flag on the Exit,
  discovery via visibility's `search` mechanic, knowledge-gated
  traversal (an undiscovered hidden exit is unwalkable, not just
  unlisted), per-character ephemeral discovery. Built on
  visibility; extends `world-rooms-movement`'s exit model.
- [biomes](biomes.md) — the ecological classification behind the
  existing room `terrain` property: a registered Biome definition
  carrying weather shielding (generalizing `world-rooms-movement`
  §6.4), idle ambience, an optional mob spawn table, and the
  forage / node resource tables gathering consumes. Richer
  terrain, one axis, fully backward-compatible. Designed with
  gathering.
- [movement-cost](movement-cost.md) — the movement-point pool a
  character spends to travel: the per-step cost gate the
  player-volition layer adds over the (still unconditional) move
  primitive, terrain-weighted step cost via [biomes](biomes.md), the
  never-strand safety rule, and the difficulty hint. Owns the cost
  concern `world-rooms-movement` §3.3 declares a non-goal; spends the
  generalized pool ([progression](progression.md) max, regen-tick
  heartbeat with [economy-survival](economy-survival.md) multipliers).
- [room-coordinates](room-coordinates.md) — area-local integer
  `(x, y, z)` **derived from the exit graph** at load: the
  derivation walk, the collision / non-square-loop / unplaced-room
  conflict policy (all non-fatal warnings), and the optional
  `Room.Info` x/y/z exposure a client mapper or a future telnet
  `map` verb consumes. No authored data, no movement change, no
  save change; a pure projection of `world-rooms-movement`'s
  exits. Substrate ahead of its consumers.

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
- [weapon-identity](weapon-identity.md) — weapon categories /
  proficiency tiers / damage types, class-granted proficiency + the
  non-proficient to-hit penalty, and per-weapon critical threat range +
  multiplier. Layers on `combat` §4.4–§4.5; EPIC sub-epic S1
  *(shipped)*.
- [size-and-wielding](size-and-wielding.md) — size-relative wielding: a
  creature size + a weapon size, the wield mode (light / one-handed /
  two-handed / too-large) derived from their ordered distance, and the two
  consequences — the equip footprint (two-handed ties up the off hand) and a
  two-handed Strength bonus to melee damage. Layers on `weapon-identity` +
  `inventory-equipment-items` §3.3 + `combat` §4.5; EPIC sub-epic S1
  increment F *(slices 1–3 shipped 2026-06-17: substrate + two-handed Strength,
  size-derived footprint, mob relativity; §4.3 off-hand eligibility consumed by
  `two-weapon-fighting`)*.
- [two-weapon-fighting](two-weapon-fighting.md) — dual wield: a second weapon in
  the off-hand slot grants an extra **off-hand attack** (its own dice/crit/type),
  at a to-hit penalty on both hands and reduced (½×) Strength on the off-hand
  damage. The off-hand weapon must resolve **light** for its wielder — the off-hand
  eligibility `size-and-wielding` §4.3 reserved. Open to all; the feats
  (Two-Weapon Fighting / Ambidexterity reduce the penalty, Improved TWF adds a
  second off-hand strike at a cumulative penalty) *improve* it. Mobs dual-wield
  too (content-driven, un-feated). Reuses `combat` §4.2 swing count + the `HitMod`
  adjustment seam + the Strength-to-damage step. EPIC sub-epic S1 increment K
  *(COMPLETE — slices 1-4 shipped 2026-06-17: off-hand substrate, the feats,
  Improved TWF, mob dual-wield)*.
- [armor-depth](armor-depth.md) — armor's depth split across the two
  defensive channels: a **single** armor class on `defense` (decompose-and-cap
  — armor bonus + a max-Dex cap on the Dex term, shields stack) and **per-type
  damage resistance** on the `mitigation` channel (already subtracted from
  damage as a scalar; this slice makes it per-type and is its first real source —
  the cross-ruleset soak primitive where physical *and* elemental resistance
  live), plus armor proficiency, the check penalty, and don/doff timers.
  Consumes the damage type `weapon-identity` recorded inertly. Layers on
  `combat` §4.4–§4.5 + the channel map; EPIC sub-epic S1 increments E+D
  *(spec; build pending)*.
- [masterwork](masterwork.md) — item quality grades (masterwork / masterpiece /
  power-wrought): a grade-scaled bonus delivered through existing seams — weapon
  to-hit, power-wrought `damage_bonus`, armor check-penalty, tool skill-check —
  plus the power-wrought unbreakable flag (a forward hook; no durability system
  yet). The mechanical grade stays independent of the cosmetic rarity/essence
  decoration (`item-decorations` §1.1). EPIC sub-epic S1 increment H
  *(shipped 2026-06-16)*.
- [item-modification](item-modification.md) — **capacity + installed mods**: a
  host item carries a bounded modification budget; **modifications are items**
  that install into a compatible host, consume capacity (flat or `[Rating]`-
  scaled), and, while the host is equipped, fold their effects into the host's
  contribution through the existing equip modifier pipeline. Installed mods are
  durable per-instance state (save-version bump), generalizing the inserted-ammo-
  holder precedent (`ammo-and-reloading`). Carved out of
  `inventory-equipment-items`' item-modification non-goal; ruleset-agnostic,
  Shadowrun armor the reference consumer. Scoped to **Core-source armor + armor
  mods** for now *(Slice A — capacity + install/remove + equip aggregation +
  save v35 — SHIPPED; the mount-slot rule for weapons remains pending)*.
- [weapon-accessories](weapon-accessories.md) — the **second admission rule** of
  item modification: a weapon exposes a fixed set of **named mount points**
  (barrel / under-barrel / side / top / stock / internal), each holding one
  accessory that declares which mount(s) it fits — **slot occupancy, not a
  capacity budget**. Reuses `item-modification`'s substrate (mod-is-an-item /
  install-remove / instance persistence / equip aggregation / presentation) and
  its `Mods` save field with **no new version bump** (mounts re-derived at load);
  owns only the mount-slot admission test. Scoped to **Core-source weapons +
  accessories**; the smartgun↔smartlink pairing is a flagged follow-on
  *(SHIPPED 2026-07-14 — `modify`/`unmodify` verbs unified across both admission
  rules)*.
- [ranged-combat](ranged-combat.md) — thrown/projectile weapons via **abstract
  per-engagement range bands** (far → near → melee) *within one room* — an archer
  opens at range and gets shots while a melee opponent closes band by band, with
  advance/withdraw (kiting). Plus ammo as consumables (thrown lands recoverable,
  projectile consumes matching ammo, masterwork ammo destroyed on use) and the
  thrown/projectile Strength rules. Built A-first (same-room ranged mechanics)
  then B (bands), then **Model C** — cross-room targeting as an opportunistic
  adjacent-room action (the `shoot` verb + a shot mob's retaliation pursuit;
  sustained cross-room combat + multi-room LoS/pursuit deferred). All inside
  `internal/combat` + `internal/command` + `internal/ai`; EPIC sub-epic S1
  increment G *(shipped — Slice A + B + Model C)*.
- [ammo-and-reloading](ammo-and-reloading.md) — the physical-ammunition model
  extending `ranged-combat §3` from loose-per-shot to **holder-fed**: loose
  rounds → **ammunition holders** (clip/magazine/belt/drum/speed-loader) →
  weapons, the unified **`reload`** verb ("top up the target from the tier
  below" — load a weapon with a holder, load a holder with rounds, feed an
  internal weapon's cylinder), spent holders **ejected** to the room and
  decaying, and ammo **grade carried through the holder** (unblocking masterwork
  ammo for holder-fed weapons). Ruleset-agnostic; Shadowrun is the reference
  consumer. *(draft — internally-fed / abstract-magazine precursor shipped in the
  Shadowrun pack; holder-fed model planned)*.
- [autoreload](autoreload.md) — a per-character **preference** that, reactively at
  fire time, runs the standard timed [ammo-and-reloading](ammo-and-reloading.md)
  `reload` on a dry wielded weapon instead of a dry attempt — removing the manual
  keystroke while keeping the cost (same holder selection, busy window, ejection).
  `autoreload on|off`, default off (opt-in), persisted toggle; two-weapon = main-hand
  first then off-hand (deferred if the action budget is short); out-of-ammo = report
  only, rate-limited + ephemeral. A thin decision layer over `ammo-and-reloading` +
  `action-economy` + `ranged-combat §3`; Shadowrun firearms the reference consumer.
  *(shipped 2026-07-10 — toggle verb + preference, the firearm-reload peek, and the
  OnRangedDry trigger that delegates to `reload`)*.
- [saves](saves.md) — saving throws (Fortitude / Reflex / Will): three
  derived save values (class strong/weak base + governing-ability
  modifier), the `d20 + bonus vs DC` resolve primitive + the
  `SaveResolved` event, and the first consumer (the massive-damage
  Fortitude save). Layers on `combat` §4.4 + `progression`; EPIC sub-epic
  S6 *(shipped 2026-06-10)*.
- [conditions](conditions.md) — status conditions (the Core 5:
  stunned / prone / blinded / frightened / fatigued) as flagged effects, the
  combat hooks (incapacitation skip-swing, defender vulnerability, attacker +
  save penalties, frightened forced-flee), entry + per-tick shake-off saves
  (consumes `saves`), and the inflict path (`afflict`/`cure` admin verbs +
  save-gated `trip`/`bash` abilities). Layers on `abilities-and-effects` +
  `combat` §4–§5; EPIC sub-epic S5 *(shipped 2026-06-10)*.
- [subdual-damage](subdual-damage.md) — nonlethal damage / the
  **knock-out** mode (the `subdual` weapon field's consumer; the
  special-weapons whip prerequisite). Knock-out-at-zero: a subdual
  finishing blow drops a foe to **unconscious** (a new incapacitating +
  helpless condition, the first member adjacent to the HP-state family
  `conditions` deferred) instead of killing, via the cancellable
  `entity.death.check` seam (`combat` §6.1). Layers on `combat` §6 +
  `conditions` + `abilities-and-effects` §5; EPIC sub-epic S1 J.
  *(Complete — shipped 2026-06-21: unconscious condition, the knock-out, sap +
  whip content + a live walkthrough, plus the tail (whip anti-armor,
  unarmed-subdual, mob-attacker subdual). Deferred: intrinsic natural armor for
  the whip gate, and the separate-nonlethal-pool/coup-de-grace variants per §8.)*
- [action-economy](action-economy.md) — the generic per-actor **timed-action /
  busy-state** substrate: one in-flight occupation per actor (busy doing `Kind`
  until tick `ReadyAt`), a begin-refuse-if-busy gate, a completion sweep that
  routes due actions to their consumer by kind, optional interruption
  (movement / manual cancel), transient (no persistence). Generalizes the ad-hoc
  flee-cooldown / cast-warmup / timed-craft occupation trackers into one primitive;
  the prerequisite the **crossbow load actions** + **don/doff timers** tail has
  been blocked on (`special-weapons.md`, `armor-depth.md` §7). EPIC Decision 0
  (no d20 action grid). *(Substrate + the don/doff-timers consumer shipped
  2026-06-21: `internal/action`, the `IsAction` busy-gate, the action-complete
  replay sweep, `stop`/movement interruption, two-phase slow-armor don/doff.
  Crossbow load shipped too 2026-06-21 — `reload_ticks`, the `load` verb, and
  the loaded-gate on the round loop + `shoot`.)*
- [skills](skills.md) — skills as use-based proficiencies + the
  `ResolveSkillCheck` primitive (`d20 + bonus vs DC`, mirroring saves), with
  the first consumer: lockpicking (`pick` vs a door's pick difficulty) + the
  Open Lock skill + a `skills` listing. Layers on `progression` proficiency +
  the door lock system; EPIC sub-epic S3 *(substrate shipped 2026-06-10)*.
- [feats](feats.md) — player-chosen passive perks: the global feat
  registry, pure prerequisite/eligibility evaluation, the three multi-take
  rules, banked feat credits earned on a level cadence, and the grant bridge
  that confers bonuses through the source-keyed modifier surface (recomputed
  from known feats on load). Layers on `progression` §2.4 + the saves / skills /
  weapon-identity / abilities consumers; EPIC sub-epic S4
  *(shipped 2026-06-11)*.
- [loot-and-corpses](loot-and-corpses.md) — the death → drop path:
  corpse creation on the mob-killed event, coin drops, the killer-
  first looting-rights window, the loot / get-from verbs, the
  autoloot toggle, and corpse decay. Consumes combat's mob-killed
  signal + the spawn-time loot of mobs-ai-spawning §6.3.
- [quests](quests.md) — definitions, prerequisites, stages,
  objectives, rewards (auto-grant or turn-in at the giver),
  giver interaction for discovery/turn-in, auto-tracking watcher,
  markers.
- [npc-dialogue](npc-dialogue.md) — the `ask <npc> about <topic>`
  verb: free-form, content-authored NPC dialogue keyed by topic
  (single line or a rotating list), an optional fallback topic,
  and delegation back to the quest `talk` verb when no topic is
  given. Flavor conversation, no quest state.
- [quest-spawns](quest-spawns.md) — runtime creation of a quest's
  mobs/items when a player reaches the stage that needs them
  (instead of pre-placing them at boot), owned per-player and
  cleaned up when the quest ends. Phase 1: shared-world,
  stage-triggered; per-observer visibility is a deferred Phase 2.
  Design ahead of code.
- [economy-survival](economy-survival.md) — currency, shops,
  sustenance, rest, consumables.
- [crafting-and-cooking](crafting-and-cooking.md) — recipes,
  crafting skills (proficiency), tiered crafting stations, the
  quality roll (output = a rarity tier), recipe acquisition; and
  cooking as the food specialization that feeds sustenance and
  grants quality-scaled well-fed effects. Permissive access,
  gated quality.
- [gathering](gathering.md) — the non-vendor ingredient source
  crafting §8 wants: ambient `forage` (rolls the room biome's
  resource table) and discrete respawning `harvest` nodes, a
  gathering proficiency + rarity-tier quality roll, and the
  scarcity controls (cooldown, node charges/respawn) that keep
  crafting a gold sink. Designed with biomes; consumes its
  resource tables.
- [mounts](mounts.md) — rideable **mount** creatures a character owns:
  the owner/ride relationship, `mount`/`dismount`, and **mounted travel**
  where the mount *becomes the metered mover* (re-pointing
  [movement-cost](movement-cost.md)'s pool/gate from rider → mount, the move
  primitive unchanged). Barding is mount-worn armor reusing
  [armor-depth](armor-depth.md); saddlebags are a container; stabling/feed are
  economy gold sinks; combat is a conservative v1 boundary (fight-from-saddle,
  temperament-gated danger entry, killable mount — charge/Ride-contest deferred).
  Greenfield from the `equipment.md` review *(spec; build pending)*.
- [transit](transit.md) — a **conveyance** that carries riders between a fixed,
  ordered set of **stops** faster than walking the graph: a **car you ride
  inside** (an orphan room whose **doorway re-binds** to the current stop's
  landing — reusing the temporary keyword-exit / "portal" retarget primitive),
  driven by a tick through a board→ride→alight state machine. **Elevator** =
  short vertical line, one car, **on-demand** call policy; **subway/monorail** =
  the same machine with a **scheduled** timetable over a horizontal line. Riding
  is **free** of [movement-cost](movement-cost.md) (stairs stay the slow, metered
  path); state is **derived-not-persisted** (like weather/portals) with a
  never-strand deposit at shutdown (cf. [mounts](mounts.md) §6). Fares, express
  service, multi-car lines, and a crush hazard are §11 open questions.
  **Elevator + subway SHIPPED** 2026-07-15 (`internal/transit` — car state
  machine + both call policies: the ACHE express **elevator** (on-demand,
  `press <code>`/`call`, real directional doors) and the Downtown Metro
  **subway** (scheduled — a self-running timetable train, ping-pong route);
  `axis`/`car_noun` reskins the motion prose. Also wired the previously-unused
  keyword-exit traversal, so portals are now usable too). Fares + multi-car +
  the crush hazard pending.
- [area-effects](area-effects.md) — the engine's first **multi-target attack**: a
  shared *area-effect primitive* (a payload of typed damage and/or a condition
  applied to everyone in a region, with a friend-or-foe rule) and its three
  consumers — **grenadelike weapons** (thrown acid/oil/fireworks with direct +
  splash and an ignition state), **room hazards** (placed, persistent
  caltrops/oil pools that trigger on whoever enters or lingers), and **biome
  hazards** (§4.6 — intrinsic, unplaced environmental damage a biome declares:
  `toxic` radiation, `vacuum` pressure, gated by carried/worn protection; derived
  from content, not persisted). Reuses
  [combat](combat.md)/[armor-depth](armor-depth.md) (damage + resistance),
  [conditions](conditions.md), [saves](saves.md), [ranged-combat](ranged-combat.md)
  (the throw), [visibility](visibility.md) (the hidden-hazard hook), and
  [biomes](biomes.md) (the intrinsic-hazard host); the igniting oil flask *becomes*
  a hazard, the bridge between the placed forms. Adds a durable placed-hazard world
  store (intrinsic biome hazards stay derived/unsaved). Greenfield from the
  `equipment.md` review + the Shadowrun gazetteer *(spec; build pending)*.
- [trade-escrow](trade-escrow.md) — the shared escrow / atomic-
  transaction primitive (stage value → cancellable commit → all-or-
  nothing or make-whole rollback → audit log). Built once, consumed
  by the two trade systems below.
- [direct-trade](direct-trade.md) — synchronous same-room two-party
  swap; offers, the confirm-then-reset anti-bait-and-switch rule,
  atomic swap via trade-escrow; transient, zero-sum.
- [auction-house](auction-house.md) — asynchronous marketplace;
  persisted listing store, access point, browse/search, buyout, tick
  expiry, fees as the gold sink; consumes trade-escrow; pickup
  delivery in v1.
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
- [follow](follow.md) — the move-with-leader primitive: `follow` /
  `unfollow` / `lose` verbs; when a leader steps to an adjacent room each
  follower attempts the same move through the normal path (all gates apply),
  failing-to-keep-up breaks the follow; chains + cycle-prevention; transient
  (no persistence). The shared substrate under **grouping**, **hireable mobs**,
  and the **onboarding guide** (each a future consumer). *(Spec ahead of code —
  build pending.)*
- [grouping](grouping.md) — parties: invite/accept roster (`group` / `join` /
  `leave` / `disband`), **shared kill-XP** (introduces combat kill-XP itself,
  party-aware — solo = a party of one; even split among same-room members),
  **shared loot** (fills the corpse owner-set with the party), and a `gtell`
  party channel. The consensual counterpart to `follow` (resolves follow's parked
  consent fork). Layers on `combat` §10 kill credit, `progression`,
  `loot-and-corpses` §4, and `chat`. *(Spec ahead of code — build pending,
  sliced: roster → kill-XP → loot.)*
- [hireable-mobs](hireable-mobs.md) — NPCs a character hires to follow, fight
  for, and obey them: an **owned, world-resident creature** (mounts-but-it-fights)
  reusing the [mounts](mounts.md) owner relationship, materialize/dematerialize,
  owned-record persistence, and logout teardown; `hire` / `dismiss` / `order`; a
  hireling trails its owner (the consumer that brings the **mob-move signal**
  `follow` §1 deferred); combat assist with owner-routed loot and
  participation-gated XP; recurring upkeep as a gold sink. Layers on
  `mobs-ai-spawning`, `follow`, `combat`/`grouping`, `loot-and-corpses` §4, and
  `economy-survival`. Design at `docs/proposals/hireable-mobs.md`. *(All four
  slices shipped — substrate + `hire`/`dismiss`/`hirelings` + save v33
  persistence/logout/login; bound move-with-owner; combat assist + owner loot +
  participation XP; upkeep + death-ends-contract. Core feature complete.)*
- [admin-verbs](admin-verbs.md) — the admin gate (commands marked
  admin, refused unless the actor holds the admin role), admin
  target resolution with visibility bypass, the baseline verb set
  (inspect / set / teleport / announce / restore / purge / reload),
  and the audit trail. Builds on roles-and-permissions.
- [who](who.md) — the connected-character roster verb; per-line
  columns, summary count, and which characters appear (all in v1;
  per-viewer hiding once visibility lands).
- [tab-completion](tab-completion.md) — the transport-agnostic completion
  query over the command registry and the §5 typed-arg scopes, candidate
  disambiguation, and the information-leak visibility rule (Phase 0); the
  line-mode `suggest` stopgap; and both shipped surfaces — GMCP
  `Input.Complete` request/response (§13, Phase 1) and char-mode real TAB
  on raw telnet (§14, Phase 2). Remaining is client integration + char-mode
  editor polish — see `docs/proposals/tab-completion.md`.
- [light-and-darkness](light-and-darkness.md) — a per-viewer effective
  light level (`black`/`gloom`/`dim`/`lit`) from time-of-day, the
  `world-rooms-movement` §6.4 terrain sky-gate, a per-room `light`
  override, lit source items (held slot + fuel burn), and a darkvision
  floor; the real-friction consequences (obscured/suppressed room view,
  blocked examination, combat to-hit penalty, movement risk + the escape
  invariant); and **persisted in-game time** so a restart doesn't black
  out the world (resolves [time-and-clock](time-and-clock.md) §3.6).
  Shipped (`internal/light` resolver + sources/fuel + render/combat/
  movement gating + period transitions + GMCP/probe); design at
  `docs/proposals/light-and-darkness.md`.

How a connection becomes a session becomes a character.

- [login](login.md) — name → email → password →
  Playing / Creating / takeover / link-dead reconnect.
- [character-creation](character-creation.md) — the wizard
  flow, validation, restart, atomic commit, spawn.
- [character-select](character-select.md) — account-first login + a
  character **roster**: authenticate the account (email + password), then
  pick from its **in-world** characters or create a new one. Other-world
  characters are hidden from the list and surfaced as a footnote count (the
  surface for `character-identity` §5's world gate); selection/create reuse login's
  concurrency + Creating handoff. Revises `login.md`'s name-first entry;
  the account gains a unique username (login key; email demoted to optional)
  + the roster is derived from `account.Characters` + each save's WorldID +
  the active world set *(shipped 2026-06-16)*.
- [session-lifecycle](session-lifecycle.md) — PlayerSession,
  SessionManager, flood protection, idle timeouts, link-dead,
  takeover.
- [roles-and-permissions](roles-and-permissions.md) — per-character
  role set, the `HasRole` authorization check, grant/revoke,
  config seed/bootstrap. Consulted by admin verbs, the admin
  channel, and the §5 idle-sweep exemption.
- [character-identity](character-identity.md) — world-locking: a
  character is stamped with a `WorldID` (its leaf ruleset pack) and may
  only log in on a server running that world; an out-of-world character
  is hidden from the roster (footnote count), never entered, never deleted
  or silently degraded. One additive save field (v23, backfilled from the location
  namespace) + a login/roster gate; the within-world fail-soft restore is
  unchanged. Co-hosting multiple worlds (and the global-registry
  namespacing it needs) is deferred. Resolves
  `docs/proposals/character-identity-across-packs.md` §7 *(shipped 2026-06-16 —
  manifest `kind` flag + save v23 WorldID + login gate)*.
- [languages](languages.md) — content-driven tongues a character speaks:
  a language registry (id + name + comprehension **family** + flavor), a
  per-character known-language set (save v30), and a background-granted
  **home language** at creation, shown on `score` + a `languages` listing.
  Comprehension gating (garble unknown tongues) and Intelligence-budget
  bonus-language picks are deferred to the spec's open questions *(identity
  substrate shipped 2026-06-19)*.

### 5. Presentation

The output layer.

- [ui-rendering-help](ui-rendering-help.md) — color tags, theme
  registry, prompts, panels, help topics, the look/consider
  appearance-vs-tactical lenses.
- [item-decorations](item-decorations.md) — rarity tiers (ordered,
  decorated, color/visibility) and essence (colored glyph) item
  markers; content registries, themed rendering (inline + padded),
  essence as stack identity.
- [player-maps](player-maps.md) — the active toggleable minimap + the
  `map` verb (full current-area map), persisted fog of war (visited
  set), the shared local-window query, and the Mudlet GMCP surface, all
  over the [room-coordinates](room-coordinates.md) substrate.

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
| `entity.equipping` | [inventory-equipment-items](inventory-equipment-items.md) §3.4 |
| `container.item_adding` | [inventory-equipment-items](inventory-equipment-items.md) §4.5 |
| `item.consuming` | [economy-survival](economy-survival.md) §6.2 |
| `shop.buy`, `shop.sell` | [economy-survival](economy-survival.md) §3 |
| `recall.before` | [recall](recall.md) §3.1 |
| `corpse.creating` | [loot-and-corpses](loot-and-corpses.md) §2.1 |
| `concealment.before` | [visibility](visibility.md) §3.1 |
| `faction.shift.check` *(spec; build pending)* | [faction](faction.md) §4 |
| `reputation.shift.check` *(spec; build pending)* | [reputation](reputation.md) §4 |
| `resource.gathering` | [gathering](gathering.md) §6 |
| `mount.before` | [mounts](mounts.md) §4.1 |
| `area_effect.before` *(spec; build pending)* | [area-effects](area-effects.md) §2.4 |
| `trade.committing` *(spec; build pending)* | [trade-escrow](trade-escrow.md) §3 |

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
| Mount template (mob `mount:` block: temperament/travel/impassable) | [mounts](mounts.md) §2 |
| Hazard template (payload/trigger/duration) *(spec; build pending)* | [area-effects](area-effects.md) §4 |
| Ability | [abilities-and-effects](abilities-and-effects.md) §2 |
| Channel map (derived-stat formulas) | [combat](combat.md) §4.4 |
| Effect template | [abilities-and-effects](abilities-and-effects.md); applied by consumables [economy-survival](economy-survival.md) §6 |
| Race, class | [progression](progression.md) §3, §4 |
| Background | [backgrounds](backgrounds.md) §2 |
| Feat | [feats](feats.md) §2 |
| Track | [progression](progression.md) §5 |
| Faction | [faction](faction.md) §2 *(spec; build pending)* |
| Biome | [biomes](biomes.md) §2 |
| Resource node template | [gathering](gathering.md) §3 |
| Command | [commands-and-dispatch](commands-and-dispatch.md) §2 |
| Emote | [commands-and-dispatch](commands-and-dispatch.md) §7 |
| Quest | [quests](quests.md) §2 |
| Help topic | [ui-rendering-help](ui-rendering-help.md) §9 |
| Rarity tier, Essence | [item-decorations](item-decorations.md) §2, §3 |
| Recipe | [crafting-and-cooking](crafting-and-cooking.md) §3 |

Engine-vs-pack scope (engine-scope registrations are visible
to all packs without prefixing; pack-scope registrations are
namespaced) applies to tags, properties, and slots; see
[scripting-and-packs](scripting-and-packs.md) §4.

### Save / load surface

Each spec calls out what it persists. The aggregate view:

- **Account file** — id, email, password hash, character list,
  creation / verification timestamps.
- **Player file** — entity id, account id, name, location,
  tags, roles, stats (base + modifiers + vitals),
  **class set** (an ordered list of class ids — one in v1, the list shape
  so a second class needs no migration; [progression](progression.md) §4.7)
  + **per-track progression** (level + xp per track;
  [progression](progression.md) §5.2), properties,
  equipment, inventory, flat item list, **abilities +
  proficiencies**, **resource pools** (current values only — pools at
  full are omitted and re-seeded from the attribute-derived maximum on
  load, so rebalancing a pool's max needs no migration;
  [progression](progression.md) §2.6), **known feats + banked feat
  credits** (the conferred bonuses are derived, not stored;
  [feats](feats.md) §8), **recall address**, **prompt template**,
  **autoloot preference** ([loot-and-corpses](loot-and-corpses.md) §6),
  **autoreload preference** (an additive `omitempty` boolean toggle — no schema
  bump, the Autoloot/AutoAssist precedent; absent on older saves loads at the
  configured default; [autoreload](autoreload.md) §6),
  **faction standing bag + history** ([faction](faction.md) §8 *(spec; build pending)*),
  **renown score** (the single-axis reputation/fame value; the tier tag is
  derived on load, not stored; [reputation](reputation.md) §10 *(spec; build pending)*),
  **world stamp** (`WorldID` — the leaf ruleset pack a character belongs
  to; an additive v23 field backfilled from the location namespace;
  [character-identity](character-identity.md) §4).
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
- **Game-time** — global in-game clock (`CurrentHour`, `DayCount`),
  one per world, written to `saves/clock.yaml` (atomic, flushed on
  every in-game hour advance and at clean shutdown) and restored at
  boot so a restart resumes the time-of-day instead of resetting to
  night. Sub-hour position is not preserved; missing/corrupt time
  cold-starts at hour 0, day 0. Not part of any player save. See
  [light-and-darkness](light-and-darkness.md) §7 (resolving
  [time-and-clock](time-and-clock.md) §3.6).
- **Connection records** — content-defined, loaded by the pack
  pipeline after content load.
- **Auction listing store** *(spec; build pending)* — long-lived world
  data (active listings + escrowed items), versioned/migrated and
  atomic like player saves; see [auction-house](auction-house.md) §4.
- **Trade audit log** *(spec; build pending)* — append-only,
  tamper-evident record of every committed transaction; see
  [trade-escrow](trade-escrow.md) §5.
- **Owned mounts** — durable per-character mount ownership on the **player
  save** (`Save.Mounts []MountRecord`, save v26): each owned mount's identity
  (its template id; barding/tack/saddlebag/upkeep are additive later fields).
  The live ride relationship is NOT persisted — on logout every mount resolves
  to its stabled record. See [mounts](mounts.md) §10.
- **Placed hazards** *(spec; build pending)* — durable world store of live room
  hazards (caltrops, oil pools): each hazard's room, payload, trigger model,
  remaining duration/charges, concealment, and placer attribution. Additive and
  versioned/migrated; the **first** dynamic room state to persist (weather/spawn/
  temporary-exits deliberately do not). A hazard may be content-flagged transient
  to skip the save; see [area-effects](area-effects.md) §5.
- **NOT persisted** — sessions, link-dead state,
  weather, mob spawn tracking, temporary exits, active
  effects, rest state, **direct-trade sessions** (transient by design),
  **corpses + their unlooted loot** (transient; removed by the decay sweep or a restart — [loot-and-corpses](loot-and-corpses.md) §7),
  **concealment + detection state** (the `hidden` / `sneaking` /
  `invisible` tags, snapshot concealment scores, admin invisibility,
  and per-observer detection memory — all ephemeral, dropped on
  logout/restart — [visibility](visibility.md) §7),
  **biome ambience state** ([biomes](biomes.md) §6) and
  **gathering node/forage state** (node charges + respawn timing,
  per-room forage depletion — transient, respawn fresh on restart —
  [gathering](gathering.md) §7), and the **autoreload no-ammo suppression
  window** (the per-character/weapon last-notified timestamp behind the
  rate-limited "nothing to reload with" notice — ephemeral combat state, resets
  on relogin — [autoreload](autoreload.md) §6).

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
| `fuel-burn` (lit light-source fuel) | configured | [light-and-darkness](light-and-darkness.md) §3.2 |
| `vitals-regen` | configured | [session-lifecycle](session-lifecycle.md) (via game loop) |
| `mount-travel-regen` | configured | [mounts](mounts.md) §5.4 |
| `prompt-flush` | 1 | [ui-rendering-help](ui-rendering-help.md) §7.3 |
| `scripting-schedule` | 1 | [scripting-and-packs](scripting-and-packs.md) (the `engine.schedule` primitive) |
| `gmcp-vitals-flush` / `-items-` / `-combat-` / `-effects-` / `-experience-` / `-charstatus-` | 1 each | [networking-protocols](networking-protocols.md) (GMCP package layer) |
| `biome-ambience` | configured | [biomes](biomes.md) §4 |
| `biome-hazard` (intrinsic ambient environmental damage) | configured | [area-effects](area-effects.md) §4.6 |
| `node-respawn` / `forage-regen` *(spec; build pending)* | configured | [gathering](gathering.md) §3, §5 |
| `corpse-decay` | configured | [loot-and-corpses](loot-and-corpses.md) §7 |
| `hazard` (room-hazard on-tick trigger + expiry/decay) *(spec; build pending)* | configured | [area-effects](area-effects.md) §4.2–§4.3 |
| `campfire-decay` | configured | [crafting-and-cooking](crafting-and-cooking.md) §4 |
| `craft-complete` | configured | [crafting-and-cooking](crafting-and-cooking.md) §5 |
| `ability-idle-tick` | configured | [abilities-and-effects](abilities-and-effects.md) §4 |
| `autosave` | configured | [persistence](persistence.md) §6.2 |
| `idle-sweep` | configured | [session-lifecycle](session-lifecycle.md) §5 |
| `linkdead-cleanup` | configured | [session-lifecycle](session-lifecycle.md) §7.3 |

Cadence is in *ticks* (or "1s"/"configured" where derived from a
duration). With the default 100 ms tick rate, an interval of 10 fires
every second. (`mob-command-queue` is specced but not yet wired as a
standalone handler.)

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
- **Role enforcement — landed.** The authorization model is
  implemented and live: [roles-and-permissions](roles-and-permissions.md)
  (a flat `HasRole` capability check) plus [admin-verbs](admin-verbs.md),
  with the command registry refusing admin verbs unless the actor holds
  the admin role (`internal/command/registry.go`), live `grant`/`revoke`,
  and role-change events. Downstream gates (e.g. auction moderation) can
  rely on it.
- **Unbounded growth.** Render cache, bad-input tracker,
  alignment history (this one is bounded), notification
  queues, and a few others have no eviction or cap.
  Memory-bounded production deployments need caps.

---

<!-- Updated: 2026-07-15 · 61 specs covering the engine substrate, world, action, lifecycle, and presentation layers. Behavior contract added ahead of code: transit (elevators/subways — a ride-inside conveyance over a line of stops, the car doorway reusing the temporary keyword-exit retarget, on-demand vs. scheduled call policy, free ride vs. metered stairs, derived-not-persisted with a never-strand deposit; greenfield, build pending, 2026-07-15). Since-shipped: npc-dialogue (the `ask <npc> about <topic>` free-form dialogue verb — content-authored topic→line tables, rotating lists, fallback topic, no-topic delegation to `talk`; first users are the Shadowrun Cocktail bartenders, 2026-07-13). Behavior contracts still ahead of code: tag-observers, area-effects (grenades §3 + placed room hazards §4.1–4.5 + the placed-hazard world store — **§4.6 biome ambient hazards SHIPPED** 2026-07-13, see below), ammo-and-reloading (holder-fed reloading — draft; internally-fed/abstract-magazine precursor shipped in the Shadowrun pack). Since-shipped: area-effects §4.6 biome ambient hazards (intrinsic environmental damage — Glow City radiation, Puyallup ash-flat toxins — via `internal/biome` HazardService; wear-only protection-key immunity — a deliberate narrowing of §4.6(b)'s carry-or-wear; attacker-less environmental death through the existing death pipeline; derived-not-persisted per §5; players-only + raw damage in v1). Since-shipped: quest-spawns Phase 1 (shared-world, stage-triggered quest-scoped mob/item spawns with per-player ownership, load-time validation, and the full session lifecycle — login re-derivation + logout/reap cleanup; first user is the Mr. Johnson run. Per-observer visibility + collected-item-in-inventory cleanup remain Phase 2 / 1b-tail, 2026-07-13). Since-shipped: autoreload (per-character reload-on-dry toggle over ammo-and-reloading — the toggle verb, the firearm-reload peek, and the OnRangedDry trigger delegating to `reload`, 2026-07-10). Since-shipped: roles-and-permissions, admin-verbs, item-decorations (M19/M20), loot-and-corpses (M22), tab-completion Phase 0–2, who, light-and-darkness, room-coordinates (M23), player-maps (M24 — Mudlet GMCP wire-shape pending live-client validation), biomes, gathering, crafting-and-cooking (M27), weapon-identity (WoT EPIC S1), masterwork (WoT EPIC S1.H), ranged-combat (WoT EPIC S1.G — Slice A+B + Model C cross-room), armor-depth (WoT EPIC S1.E+D), size-and-wielding (WoT EPIC S1.F), two-weapon-fighting (WoT EPIC S1.K — slices 1-4: off-hand attack, the feats, Improved TWF, mob dual-wield), saves (WoT EPIC S6), conditions (WoT EPIC S5), skills (WoT EPIC S3, substrate), feats (WoT EPIC S4), backgrounds, visibility + hidden-exits (M28), movement-cost (flat→biome-weighted cost gate + encumbrance), character-select (account-first login), character-identity (world-locking, save v23), mounts (core v1 — substrate/persist save v26/acquire+stablemaster/ride+co-located travel/the mount as metered mover; barding + temperament-combat + lead pending). -->
