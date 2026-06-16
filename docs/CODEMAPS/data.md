<!-- Generated: 2026-06-16 | Persistence (YAML files) + content packs — no database | Token estimate: ~820 -->

# Data: Saves & Content

No database. Two data domains: **mutable player/account saves** (YAML on disk,
atomic writes) and **read-only content packs** (YAML + Lua, loaded at boot).

## Save surface (`<ANOTHERMUD_SAVE_DIR>`, default ./saves)
```
accounts/index.yaml              username → id (+ legacy email → id map)
accounts/<id>/account.yaml       bcrypt creds, username, created_at
players/<lname>/player.yaml      versioned char save (tags, roles, stats,
   ├ quest.yaml                  properties, equip/inventory, abilities+profs,
   ├ notifications.yaml          known recipes, feats, resource pools, gender,
   └ chat-subscriptions          WorldID, recall, prompt, roomdata toggle)
channels/<id>.yaml               global channel scrollback
clock.yaml                       global in-game time (CurrentHour, DayCount)
```
- Writes via `internal/persistence`: tmp → bak → rename rotation, path-safety.
- **Login is account-first by username** (`character-select`): `index.yaml` keeps
  a username→id map alongside the legacy email→id map (backfilled on load); one
  account holds a roster of characters across worlds.
- `internal/player` — `player.yaml` carries `version`; `CurrentVersion = 24`
  with an **append-only migration chain** (never edit an old migration). Recent
  bumps: v19 backgrounds, v20 feats, v21 resource-pool currents, v22 gender,
  v23 `WorldID` (world-locking, backfilled from the location namespace),
  v24 `movement_max` base stat. Boolean/string prefs with a safe zero-value
  (autoloot, wimpy, prompt, `show_room_data`) are added `omitempty` **without** a
  version bump.
- **Equipment save shape**: one entry per equipped item, keyed by its TARGET
  slot key (`{Template, Entity}`). A spanning item (two-hander) is NOT
  duplicated across its footprint — companion keys are re-derived from the
  template on reload (`respawnEquipment`). No version bump (legacy saves are
  structurally identical — one entry per item).
- **Autosave**: `session.Manager.SaveAll` writes actors with the `dirty` bit set
  (`SetRoom` flips it); final flush on SIGINT. Per-player errors isolated.
- **In-game time persists**: `gameclock.Store` writes `clock.yaml` (atomic),
  flushed on each in-game hour advance + clean shutdown, seeded at boot;
  missing/corrupt cold-starts at hour 0/day 0. Global, not per-player.
- **Not persisted** (by design): sessions, weather, link-dead state, mob spawn
  tracking, temporary exits, active effects (incl. light source lit/fuel across
  restart), rest state, direct-trade sessions.

## Content packs (`<ANOTHERMUD_CONTENT_DIR>`, default ./content)
`internal/pack` — manifest/discovery/dep-order/two-phase loader. Three packs ship:
`content/core/` (the `tapestry-core` engine baseline — slots/races/classes/tracks/
abilities/effects/rarity/essence/biomes/channels/feats/backgrounds/conditions/
theme/help; **no world**), `content/starter-world/` (the default-boot demo village,
depends on core), and `content/wot/` (a Wheel-of-Time pack in progress, depends on
core). Manifests declare `kind: world | library` (`character-identity` world-locking).
Registries (roughly load order):
```
theme · slots · races · classes · tracks · abilities · effects ·
conditions · feats · backgrounds · rarity · essence · channels/channel-map ·
biomes · items · grades · mobs · loot_tables · forage/nodes · recipes ·
emotes · rooms · areas · weather_zones · quests · help · scripts(*.lua)
```
- Ids are namespaced (`starter-world:town-square`); unqualified ids resolve
  against the current pack, `other-pack:foo` crosses packs.
- **Equippable items** declare `eligible_slots` (which slots they fit) and
  optional `companion_slots` (footprint spanned when worn, e.g. a two-hander's
  `offhand`); a legacy single `properties.slot` is bridged to a one-element
  eligible set. Slot names are validated against the registry in a boot
  post-pass (`validateItemSlots`). Engine baseline slots: `wield`, `offhand`,
  `head`, `finger`(×2), `light`; packs add more (e.g. `cloak`).
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
`internal/persistence/`, `internal/player/` (migrations), `internal/account/`,
`internal/pack/{loader,content}.go`, `internal/queststore/`.
