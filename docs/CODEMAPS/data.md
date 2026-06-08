<!-- Generated: 2026-06-07 | Persistence (YAML files) + content packs — no database | Token estimate: ~800 -->

# Data: Saves & Content

No database. Two data domains: **mutable player/account saves** (YAML on disk,
atomic writes) and **read-only content packs** (YAML + Lua, loaded at boot).

## Save surface (`<ANOTHERMUD_SAVE_DIR>`, default ./saves)
```
accounts/index.yaml              email → account id
accounts/<id>/account.yaml       bcrypt creds, created_at
players/<lname>/player.yaml      versioned char save (tags, roles, stats,
   ├ quest.yaml                  properties, equip/inventory, abilities+profs,
   ├ notifications.yaml          recall, prompt, roomdata toggle)
   └ chat-subscriptions          per-player
channels/<id>.yaml               global channel scrollback
clock.yaml                       global in-game time (CurrentHour, DayCount)
```
- Writes via `internal/persistence`: tmp → bak → rename rotation, path-safety.
- `internal/player` — `player.yaml` carries `version`; `CurrentVersion = 16`
  with an **append-only migration chain** (never edit an old migration).
  Boolean/string prefs with a safe zero-value (autoloot, wimpy, prompt,
  `show_room_data`) are added `omitempty` **without** a version bump.
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
`internal/pack` (3.4k LOC) — manifest/discovery/dep-order/two-phase loader.
`content/core/` ships the `tapestry-core` starter pack (placeholder setting).
Registries (roughly load order):
```
theme · slots · races · classes · tracks · abilities · effects ·
items · mobs · loot_tables · rooms · areas · weather_zones ·
quests · help · scripts(*.lua)
```
- Ids are namespaced (`tapestry-core:town-square`); unqualified ids resolve
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
