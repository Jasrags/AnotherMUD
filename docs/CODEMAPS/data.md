<!-- Generated: 2026-06-07 | Persistence (YAML files) + content packs — no database | Token estimate: ~770 -->

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
- `internal/player` — `player.yaml` carries `version`; `CurrentVersion = 15`
  with an **append-only migration chain** (never edit an old migration).
  Boolean/string prefs with a safe zero-value (autoloot, wimpy, prompt,
  `show_room_data`) are added `omitempty` **without** a version bump.
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
- Load order relies on **alphabetical discovery** — no topological sort over
  declared deps yet (open item).
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
