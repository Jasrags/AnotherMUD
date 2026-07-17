<!-- Generated: 2026-07-17 | Persistence (YAML files) + content packs — no database | Token estimate: ~950 -->

# Data: Saves & Content

No database. Two data domains: **mutable player/account saves** (YAML on disk,
atomic writes) and **read-only content packs** (YAML + Lua, loaded at boot).

## Save surface (`<ANOTHERMUD_SAVE_DIR>`, default ./saves)
```
accounts/index.yaml              username → id (+ legacy email → id map)
accounts/<id>/account.yaml       bcrypt creds, username, created_at
players/<lname>/player.yaml      versioned char save (tags, roles, stats,
   ├ quest.yaml                  properties, equip/inventory, abilities+profs,
   ├ notifications.yaml          known recipes, feats, pools, gender,
   └ chat-subscriptions          WorldID, recall, prompt, roomdata toggle,
                                 madness, owned mounts, faction standing,
                                 reputation, heat/wanted, karma ledger)
channels/<id>.yaml               global channel scrollback
clock.yaml                       global in-game time (CurrentHour, DayCount)
trade-audit.yaml                 global escrow audit log (direct-trade + auction)
auctions.yaml                    global auction listings + pending coin ledger
```
- Writes via `internal/persistence`: tmp → bak → rename rotation, path-safety.
- **Login is account-first by username** (`character-select`): `index.yaml` keeps
  a username→id map alongside the legacy email→id map (backfilled on load); one
  account holds a roster of characters across worlds.
- `internal/player` — `player.yaml` carries `version`; **`CurrentVersion = 39`**
  with an **append-only migration chain** (never edit an old migration). Recent
  bumps: v19 backgrounds, v20 feats, v21 resource-pool currents, v22 gender,
  v23 `WorldID` (world-locking), v24 `movement_max` base stat, v25 `madness` (saidin
  taint accumulator), v26 `Mounts []MountRecord` (owned-mount list), v27 `PowerAttackActive`
  (combat stance), v28 `ChannelingGift` (channeler affinity), v29 `BackgroundFeat`/
  `BackgroundEquipmentChoice` (creation choices), v30 `KnownLanguages`, v31 `FactionStanding`
  (per-character standing map), v32 `Reputation` (renown score), v33 `Hirelings`
  (owned hireling contracts), v34 `AdminTags` (admin-applied gameplay tags),
  **v35 `Mods` in InventoryEntry** (installed armor mods + weapon accessories;
  capacity-based + mount-slot-based admission), **v36 no-op**, **v37 `Burned` in
  InventoryEntry** (SIN credential burned on scan; legality.md Slice 2),
  **v38 `Heat`/`WantedLevel` on Save** (security-response.md v2 patrol spawning),
  **v39 `Karma *Snapshot` on Save** (karma-ledger advancement, SR-M5).
  Boolean/string prefs with a safe zero-value (autoloot, auto-assist, wimpy,
  prompt, `show_room_data`, minimap, autoreload) are added `omitempty` **without**
  a version bump.
- **Equipment save shape**: one entry per equipped item, keyed by its TARGET
  slot key (`{Template, Entity}`). A spanning item (two-hander) is NOT
  duplicated across its footprint — companion keys are re-derived from the
  template on reload (`respawnEquipment`). Installed mods persist as IDs in the
  host's `Mods` list, re-seeded from templates at load (v35+). No version bump
  for per-instance state (legacy saves are structurally identical — one entry per item).
- **Autosave**: `session.Manager.SaveAll` writes actors with the `dirty` bit set
  (`SetRoom` flips it); final flush on SIGINT. Per-player errors isolated.
- **In-game time persists**: `gameclock.Store` writes `clock.yaml` (atomic),
  flushed on each in-game hour advance + clean shutdown, seeded at boot;
  missing/corrupt cold-starts at hour 0/day 0. Global, not per-player.
- **Trade audit log persists**: `escrow.AuditStore` → `<save-dir>/trade-audit.yaml`
  (atomic, versioned, append-only); every direct-trade + auction event. Global.
- **Auction listings persist**: `auction.Store` → `<save-dir>/auctions.yaml`
  (atomic, versioned/migratable, in-memory-authoritative, whole-file rewrite per
  mutation). Holds active/sold/expired/cancelled listings (each embeds the
  **serialized escrowed item** — template + property bag, so grade/decorations
  survive — never a live entity until collect), a pending-coin ledger (proceeds +
  refunds awaiting pickup), and a monotonic id counter. Reconciled on boot
  (lapsed-while-down listings expire on load). Global, not per-player.
- **Not persisted** (by design): sessions, weather, link-dead state, mob spawn
  tracking, temporary exits, active effects (incl. light source lit/fuel across
  restart), rest state, direct-trade sessions, transit car position, guard
  assignments. **Mounts**: only the durable `MountRecord` (template identity)
  persists on the player save; the live materialized `MobInstance` + the ride
  relationship are transient (on logout an owned mount resolves to a resting/stabled
  record, re-mounted after login). **Heat**: persists across relogin (security
  does not decay while offline in v1). **Karma**: persists only for karma-ledger
  characters (level-track characters hold a nil ledger, so Persist writes nothing).

## Content packs (`<ANOTHERMUD_CONTENT_DIR>`, default ./content)
`internal/pack` — manifest/discovery/dep-order/two-phase loader. Four packs ship:
`content/core/` (the `tapestry-core` engine baseline — slots/races/classes/tracks/
abilities/effects/rarity/essence/biomes/channels/feats/backgrounds/conditions/
theme/help; **no world**), `content/starter-world/` (the default-boot demo village,
depends on core), `content/shadowrun/` (Shadowrun MVP pack — a second world option with
`shadowrun-primaries` attribute set, street-samurai class, Stun+Essence pools,
metatypes, cyberware slot, legality market gate + SIN/licenses, security zones
+ patrol response, implants, depends on core), and `content/wot/` (a Wheel-of-Time
pack in progress, depends on core). Manifests declare `kind: world | library`
(`character-identity` world-locking). Registries (roughly load order):
```
theme · slots · races · classes · tracks · abilities · effects ·
conditions · feats · backgrounds · rarity · essence · channels/channel-map · pools ·
biomes · items · grades · mobs · loot_tables · forage/nodes · recipes ·
emotes · rooms · areas · weather_zones · quests · scripts(*.lua)
```
- Ids are namespaced (`starter-world:town-square`); unqualified ids resolve
  against the current pack, `other-pack:foo` crosses packs.
- **Equippable items** declare `eligible_slots` (which slots they fit) and
  optional `companion_slots` (footprint spanned when worn, e.g. a two-hander's
  `offhand`); a legacy single `properties.slot` is bridged to a one-element
  eligible set. Slot names are validated against the registry in a boot
  post-pass (`validateItemSlots`). Engine baseline slots: `wield`, `offhand`,
  `head`, `finger`(×2), `light`; packs add more (e.g. `cloak`). **Modifications**
  (v35+): armor items declare `mod_capacity` (flat or `[Rating]`-scaled budget);
  mods are items with `mod_type` + `mod_cost` that install into compatible hosts,
  live inside the host's `Mods` list, and fold their effects into equip on mount.
- **Rideable mounts** are ordinary mobs carrying an optional `mount:` block
  (`mob.MountSpec`: temperament + travel-pool config, mounts.md §2.1); its
  presence is what makes a mob a mount. No new registry — they load through the
  `mobs` registry. Demo content: `starter-world` riding-horse + village
  stablemaster at the village gate.
- **Shadowrun content growth** (v38+): legality tiers on items (`legal` / `restricted`
  / `forbidden`); `requires_license` shop gate; credential items (`properties.credential_rating`,
  SIN/license; `sin` verb alias). Security zones marked by area `security` tier
  (`AAA…Z`). New pool `stun` (alongside `hp`); `energy` + `matrix` deferred. Firearm
  `firing_modes`, `recoil_comp`, `ammo_type`/`ap`; ranged `ranged_style` for flavor.
- Pack load order is **dependency-ordered**: `internal/pack/order.go` runs a
  Kahn's-algorithm topological sort over declared deps (alphabetical
  tie-breaks; `ErrCycle`/`ErrUnknownDep`), wired into `pack.Load`.
- **Room coordinates are derived, not authored**: after the exit graph is
  assembled, `world.DeriveCoordinates` (room-coordinates spec) walks each
  area's directional exits to assign area-local `(x,y,z)`, honoring an
  optional per-room `coord:` pin. Recomputed every boot, **never persisted**;
  conflicts emit load-time warnings, never abort. Exposed over GMCP Room.Info.

## Scripting data
`content/<pack>/scripts/*.lua` → `internal/scripting` (sandboxed gopher-lua
per-instance LState; base/table/string/math only) + `internal/script` registry.
Bus bridge: `engine.subscribe`/`log`/`schedule`. Hot reload via admin `reload`.

## Key files
`internal/persistence/`, `internal/player/` (migrations + CurrentVersion),
`internal/account/`, `internal/pack/{loader,content}.go`, `internal/queststore/`,
`internal/questspawn/` (quest-scoped spawn visibility + lifecycle),
`internal/security/` (heat tracker store).
