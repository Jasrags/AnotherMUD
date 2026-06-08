<!-- Generated: 2026-06-07 | Engine core: tick, eventbus, command, services | Token estimate: ~950 -->

# Engine & Command Flow

The "backend" of a MUD = the tick loop, the event bus, and command dispatch into
services. No HTTP routes — the route analog is `verb → handler → service → store`.

## Command dispatch (the "route table")
`internal/command` (9.0k LOC, largest pkg). Player line → `Registry.Dispatch`:
```
raw line ─▶ Fields() ─▶ resolveRegistration(verb)   (exact match, else
                         lowest-registration-order prefix; admin gate)
         ─▶ if Args declared && !HandParsed: ResolveArgsWithContext  (§5)
              └ scope from BuildResolveContext (inventory/room/entities/doors)
         ─▶ Handler(ctx, *Context)   reads c.Resolved (typed) or raw c.Args
```
- **§5 typed args** (`argresolve*.go`, `argdef.go`): types keyword/text/number/
  inventory/room_item/entity/player/npc/container/visible/findable/door. Resolvers
  run keyword rules (`internal/keyword`: exact→prefix→name-substring, ordinals,
  `all.`/bulk).
- **HandParsed** verbs declare Args for completion/help but parse raw Args
  themselves (get/take, kill, look, consider). Aliases inherit primary's args.
- **Tab-completion** (`complete.go`): read-only query over the registry + §5
  scopes. Surfaces: player `suggest <partial>` verb (line-mode), admin `complete`
  debug verb, and the **`Input.Complete` GMCP request/response** (client→server,
  via `Registry.CompleteLine` + `session/gmcp_complete.go`, rate-limited). Spec
  `docs/specs/tab-completion.md`.
- Builtins registered in `builtins.go`; per-channel/emote/movement verbs wired in
  `main.go`.
- **Equip footprint/contention** (`equipment.go`, inventory-equipment-items §3):
  items declare `eligible_slots` + `companion_slots`; `EquipHandler` checks
  eligibility, computes a footprint via `slot.{IsEligible,FreeKey,Footprint}`,
  displaces every conflicting occupant, then publishes the cancellable
  `entity.equipping` veto (the policy/feature-module seam) before mutating. A
  spanning item (two-hander) lives under several `connActor.equipment` keys but
  one `footprints[id]` entry → modifiers once, one save entry, whole-footprint
  unequip. Mobs reuse the same helpers in `entities.EquipMobAtSpawn` (no
  auto-swap; non-fitting gear carried-as-loot, not equipped).

## Tick loop
`internal/tick` — `Loop.Register(name, cadence, fn)`, default 100ms tick.
Handlers wired in `main.go`: combat round (`_COMBAT_CADENCE`), `autosave`
(`Manager.SaveAll` of dirty actors), `idle-sweep`, effect ticks, vitals regen,
`fuel-burn` (lit light-source fuel → gutter), GMCP flushers
(Char.Items/Combat/Effects/Vitals — cadence-1 poll-and-diff), prompt render.
In-game clock (`gameclock`) is tick-driven, not wall-clock, and **persists**
(`gameclock.Store` → `saves/clock.yaml`, seeded at boot, flushed on hour
advance + clean shutdown) so darkness doesn't reset to night on restart.

## Event bus
`internal/eventbus` — typed bus, cancellable + non-cancellable events.
`Publish` / `PublishCancellable` (veto). Producers = handlers/services after a
mutation; consumers = questwatch, ai disposition, gmcp flushers, scripting bridge.
Cancellable-event index lives in `docs/specs/README.md`.

## Services (called by handlers)
| Service | Pkg | Role |
|---|---|---|
| combat.Manager | combat | engage/round/flee/death |
| progression.Manager + Training/Ability/Proficiency/ActionQueue | progression (6.2k) | XP, tracks, abilities, effects-into-combat |
| economy.{Currency,Shop,Rest,Consumable}Service | economy | gold, shops, sustenance/rest |
| quest.Service + queststore + questwatch | quest* | accept/advance/turn-in |
| effect.Manager | effect | buffs/debuffs over ticks |
| light.Resolver | light | per-viewer effective light; gated at render/look/combat/move chokepoints via `command.EffectiveLight` (held source + room luminous items + darkvision); drives §6 transitions + fuel burn |
| entities.{Store,Placement,Contents} | entities | items/mobs, room placement, containers |
| session.Manager | session (7.1k) | actors, flood/idle/link-dead/takeover, SaveAll |

## Key files
- `internal/command/registry.go` (dispatch + Registry/Command/Context, ~920 LOC)
- `internal/command/argresolve*.go` (§5 resolvers), `complete.go` (completion)
- `internal/session/` (actor + Manager; largest after command)
- `internal/combat/`, `internal/progression/`, `internal/eventbus/`
