# Scripting and Pack System — Feature Specification

**Status:** Draft · **Scope:** Pack discovery and manifest loading,
the two-phase declarations-then-content load pipeline, per-pack
namespace and entity-id enforcement, the tags / properties / slots
registries' scope rules, content file loading (rooms, items, mobs,
quests, areas, weather zones, themes, help, fixtures), script
execution via the sandboxed JS runtime, the engine API surface
exposed to scripts, and cross-reference validation · **Audience:**
Anyone reimplementing or porting this feature in any language.

This document describes *what* the pack and scripting feature must
do, not *how* to implement it. The specific JS runtime, YAML format,
file extensions, and module names are policy; the load pipeline and
namespace rules are contract.

---

## 1. Overview

A **pack** is a self-contained directory of game content (rooms,
items, mobs, areas, quests, scripts, themes, help text) packaged
with a manifest. The engine ships with zero hardcoded content;
**everything** the player interacts with comes from packs. The
engine's job is to load them in a defined order, register their
declarations, validate cross-references, and execute their scripts
inside a sandboxed JavaScript runtime that exposes a typed engine
API.

The feature has four responsibilities:

1. **Discovery and ordering.** Find pack directories under a known
   root, filter by server configuration, and produce a stable load
   order.
2. **Two-phase loading.** Load every pack's *declarations* (tags,
   properties, equipment slots, manifest, MOTD) before loading
   *content* (rooms, items, mobs, scripts, …) for any pack, so
   late-phase content can resolve early-phase declarations across
   packs.
3. **Scripting.** Execute pack JavaScript inside a single
   sandboxed engine, with the engine API exposed under a
   `tapestry.*` namespace built from registered API modules.
4. **Validation.** After all packs load, sweep cross-references
   (mob abilities, command verbs, tag and property registrations,
   weather-zone-to-area, room-to-area, etc.) and report problems
   per the pack's declared validation mode.

### Core concepts

- **Pack manifest** — `pack.yaml` (or `tapestry.yaml`) at the
  root of a pack directory. Declares name, version, dependencies,
  load order, validation mode, and content paths.
- **Pack namespace** — a normalized lowercase identifier derived
  from the pack name. Scoped names (`@scope/pkg`) become
  `scope-pkg`; bare names pass through unchanged.
- **Engine pack** — the conventional name `tapestry-core` (a
  reserved identifier). Tags / properties / equipment slots
  declared by this pack register at *engine* scope; every other
  pack registers at *pack* scope.
- **Declaration vs content.** Declarations (tags, properties,
  slots, the manifest itself) describe *what is allowed* and must
  be available before YAML files using them can be coerced.
  Content (rooms, items, mobs, scripts, …) is the actual world.
- **API module** — a small object implementing an interface that
  exposes a named slice of engine functionality to scripts (e.g.
  `tapestry.world.*`, `tapestry.commands.*`). Modules are
  registered with DI; the runtime walks all of them at startup.
- **Validation mode** — per-pack `strict` (default) or `lenient`.
  Strict packs throw on the first cross-reference failure;
  lenient packs log and count, allowing the server to start with
  warnings.

### Goals

1. Let content authors ship a self-contained directory and have
   the engine load it without code changes.
2. Make declarations (tags, properties, slots) available across
   packs before any pack's content YAML is parsed, so a
   downstream pack can reference an upstream pack's properties.
3. Enforce namespace discipline: every entity id declared by a
   pack must be namespaced with that pack's identifier.
4. Detect cross-reference problems before the server accepts
   players.
5. Run untrusted-by-default content scripts inside a CPU- and
   memory-bounded sandbox.
6. Expose a stable engine API surface to scripts that mirrors the
   engine's feature boundaries.

### Non-goals

- Pack discovery from the internet, signing, or installation.
  Packs are local directories under a configured root.
- Hot-reloading. The engine loads packs once at startup. Content
  changes require a restart.
- A package manager. The `dependencies` field is consulted for
  scope-resolution purposes but is NOT used to install missing
  dependencies — pack authors are expected to ship them.
- Per-script isolation. All pack scripts share a single JS
  runtime context.
- The shape of any specific content file (room YAML, mob YAML,
  …). The content schemas are policy that lives in YAML
  documentation; this spec only specifies the *pipeline*.

---

## 2. Pack manifest

### 2.1 Required fields

A manifest MUST carry:

- A **name**. Either bare (`my-pack`) or scoped
  (`@scope/my-pack`). Used to derive the pack namespace (§2.3).
- An **active** flag. Default true. Inactive packs are skipped
  entirely.

### 2.2 Optional fields

A manifest MAY carry:

- Version, display name, description, author, copyright,
  website, license, engine-version compatibility string. All
  cosmetic / informational.
- A **dependencies** map of `pack-name → version-constraint`.
  The engine consults the *keys* to wire dependency-aware
  resolution of tags and properties (§4.6); the version values
  are not interpreted (today).
- A **load order** integer (default 100). Used by the help
  service for topic precedence (higher loads later → wins);
  the pack loader itself processes packs in discovery order
  (alphabetical within the packs root), not load order.
- A **validation** mode of `strict` (default) or `lenient`.
- A **content** block enumerating the relative paths for each
  content category (rooms, items, equipment slots, recipes,
  scripts, strings, mobs, area definitions, weather zones,
  help, quests, motd, motd_color). Paths are globs interpreted
  against the pack directory.

### 2.3 Namespace derivation

Pack namespaces follow a fixed rule:

- A name containing `/` is stripped of any leading `@` and
  has `/` replaced with `-`. So `@anthropic/tapestry-core`
  becomes `anthropic-tapestry-core`.
- Otherwise the name passes through unchanged.

The namespace is the prefix every entity id declared in this
pack MUST carry before the `:` separator (§5.4).

The conventional engine namespace is **`tapestry-core`**.
Declarations from a pack with this namespace are treated as
engine-scope: their tags, properties, and equipment slots are
visible to all packs without a `tapestry-core:` prefix.

### 2.4 Discovery rules

The pack root (today `<binary-dir>/packs/`) is scanned for
direct subdirectories:

- Bare subdirectories (e.g. `packs/legends-forgotten/`) are
  candidate packs.
- Scoped subdirectories (`packs/@scope/`) contain candidate
  packs one level deeper (`packs/@scope/some-pack/`).

If the server config carries a non-empty packs list, the
discovered set is filtered to entries matching by relative
path (`@scope/name`), by namespace (`scope-name`), or by
folder name. Packs not in the list are silently skipped.

Discovery order is alphabetical at each level. The two-phase
loader processes packs in this order; relative load semantics
between packs depend on the alphabetical sort, not on the
manifest's `load_order` field (which only affects help-topic
precedence).

**Acceptance criteria**

- [ ] Manifest at `pack.yaml` or `tapestry.yaml` is loaded.
- [ ] Inactive manifests are skipped before any content is
      loaded.
- [ ] Namespace derivation produces `scope-name` for scoped
      names and identity for bare names.
- [ ] `tapestry-core` is the only engine-scope namespace.
- [ ] Discovery walks both bare and `@scope/`-nested
      directories.
- [ ] The server config's pack list, when non-empty, filters
      discovery.

---

## 3. The load pipeline

Packs load in two distinct phases, with a dependency-resolver
wiring step in between. After both phases complete, several
post-load steps run.

### 3.1 Phase 1: declarations

For each discovered pack, in discovery order:

1. Read the manifest. If `active = false`, return without
   touching the pack.
2. Set the loader's current pack-dir and namespace context.
3. Add the manifest to the loaded-packs list. (Listed packs
   are exposed via the `IPackManifestProvider` service so
   downstream features can iterate.)
4. Load **MOTD** files declared by the manifest's content
   paths into the loader's `PackMotd` / `PackMotdColor` slots
   if not already populated. First-wins across packs.
5. Load `tags.yml` if present (§4.4). Tags register at engine
   scope for `tapestry-core`; at pack scope for everyone else.
6. Load `properties.yml` if present (§4.5). Same scope rule.
7. Load equipment slots if the manifest declares an equipment-
   slots file. Same scope rule.

After Phase 1, every pack's tags, properties, and slot
declarations are in their respective registries. No content
YAML has been parsed yet.

### 3.2 Dependency resolver wiring

Between phases, the loader walks the manifests it loaded and
builds a dependency map: `packNamespace → [dep namespaces]`,
derived from the manifest's `dependencies` keys (each itself
namespaced).

The tag registry's and property registry's dependency-resolver
hooks (see `docs/specs/persistence.md` §2.4) are wired with
this map. From this point on, an unqualified tag or property
name encountered during Phase 2 YAML coercion is resolved as:

1. Engine name (direct match).
2. `currentPack:name`.
3. `dep:name` for each dependency in declaration order.

### 3.3 Phase 2: content

For each pack, in the same discovery order, the loader walks
its content paths and loads:

1. **Weather zones** before area definitions (area definitions
   reference zones).
2. **Area definitions** before rooms (rooms reference areas).
3. **Rooms.** Each room declares an id, its area, exits,
   doors, alignment range, optional weather-exposed and
   time-exposed flags, properties, spawns, fixture references,
   and a per-room reset-interval override. Spawns and fixtures
   are buffered for after the room is registered (§3.4, §3.6).
4. **Area validity check.** Every loaded room whose `area`
   field is non-null MUST reference a known area (otherwise
   `InvalidOperationException`). This is the first cross-pack
   check and is fatal regardless of validation mode.
5. **Items.** Each item file produces an item template
   registered by id.
6. **Fixture placement** (§3.6) — happens after items are
   loaded but before mobs, since fixture refs are item template
   ids.
7. **Strings / themes.** A theme file is detected by filename
   (`theme.yaml`) and its tag-to-color mappings register with
   the theme registry.
8. **Mobs.** Each mob file produces a mob template (with
   optional loot table, trainer config, shop config; see
   `docs/specs/mobs-ai-spawning.md`).
9. **Scripts.** §6.
10. **Help topics.** Loaded into the help service with the
    pack's load-order priority.
11. **Quests** (see `docs/specs/quests.md`).

After every pack's content is loaded, the loader runs a final
**area-weather-zone cross-check**: every area's declared
weather zone (if any) must be registered. This is fatal in
both validation modes.

### 3.4 Spawns and reset intervals

Room spawn rules declared inline in room YAML are buffered on
the room load. After the room is registered with the world,
the buffered spawn rules go to the spawn manager along with
the effective reset interval:

- Per-room override on the room YAML wins.
- Otherwise the room's area's reset interval is used.
- Otherwise a hardcoded fallback (today 300 ticks).

### 3.5 Door mirroring

After all rooms in a pack are loaded, the loader walks each
room's exits and ensures doors are mirrored on the reverse
side (so a door declared only on the north side is also
present on the south side of the target room). See
`docs/specs/world-rooms-movement.md` §5.

Mirroring runs per-pack, not globally. A pack that adds a door
to a room declared in another pack does NOT automatically
mirror through the door (the other room's pack already loaded
in Phase 2).

### 3.6 Fixtures

A room YAML may list **fixture item ids** — items that should
exist in the room at startup (signs, statues, fountains).
Fixtures are buffered during room loading and placed after
all items in the same pack are loaded:

1. Resolve the item id via the item registry. If unknown,
   log a warning and skip.
2. Resolve the room. If unknown (e.g. typo), log and skip.
3. Create an item instance from the template, place it in
   the room, track it.

Fixtures from one pack referring to items in another pack
work as long as the item-declaring pack loads first
(alphabetical discovery order). Cross-pack fixture refs that
load in the wrong order silently miss.

### 3.7 Post-load steps

After both phases complete, the bootstrapping module runs:

1. **AbilityCommandBridge.WireAll()** — every active ability
   registered during script execution is wired as a typeable
   command (see `docs/specs/commands-and-dispatch.md` §9).
2. **PackValidator.Validate()** — cross-reference sweep
   (§7).
3. **ConnectionLoader.Load()** — restore any persistent
   "connection" records from disk (a feature used by content
   to remember world-altering state across restarts).
4. **MOTD / MOTD-color selection** — server config's MOTD
   takes precedence, otherwise the first-pack MOTD wins.
5. **Auto-generate help for commands** — every command
   registered (engine + pack) with arg definitions gets a
   synthesized help topic (see `docs/specs/commands-and-
   dispatch.md` §8).
6. **Theme registry compile** — finalize the tag-to-ANSI map.

**Acceptance criteria**

- [ ] Phase 1 runs to completion for every pack before any
      pack's Phase 2 begins.
- [ ] Dependency resolvers are wired between phases.
- [ ] Phase 2 loads in the order: weather zones → areas →
      rooms → items → fixtures → themes → mobs → scripts →
      help → quests.
- [ ] Inactive packs are skipped at Phase 1 and absent from
      Phase 2.
- [ ] Cross-pack room→area and area→weather-zone references
      are fatal when broken.
- [ ] Door mirroring runs within each pack's room set.
- [ ] Fixtures are placed after items in the same pack are
      loaded, before mobs.
- [ ] Auto-generated command help runs after every script-
      driven command registration completes.

---

## 4. Declarations registries

### 4.1 Three registries

Three registries underpin the declaration phase:

- **Tag registry.** Strings entities may carry. Used for
  filtering (`safe`, `no_get`, `aggro`), classification
  (`weapon`, `armor`), and grouping.
- **Property registry.** Typed entity properties (see
  `docs/specs/persistence.md` §2).
- **Slot registry.** Equipment slots (see
  `docs/specs/inventory-equipment-items.md` §3.1).

All three follow the same engine-scope vs pack-scope
distinction.

### 4.2 Engine scope vs pack scope

A registration arriving from the engine namespace
(`tapestry-core`) registers at **engine scope**:

- The name is the bare identifier (e.g. `safe`).
- It is visible everywhere without prefixing.
- It cannot be shadowed by any pack.

A registration from any other pack registers at **pack scope**:

- The name is prefixed: `<packNamespace>:name`.
- The unqualified form resolves only inside the pack's own
  files and inside dependent packs' files (via the dependency
  resolver, §4.6).
- Two packs may register the same bare name; they get
  distinct full keys.

### 4.3 Scope assignment at load time

The pack loader inspects the *current* pack's namespace when
loading a declaration file (`tags.yml`, `properties.yml`,
equipment slots). If the namespace equals `tapestry-core`, it
calls the engine-scope registration method; otherwise the
pack-scope method. Pack authors cannot opt into engine scope
by accident.

### 4.4 tags.yml

A pack may carry a `tags.yml` at its root listing tag names
with description and an optional `applies_to` set:

```yaml
tags:
  safe:
    description: Marks a room as PvP/PvE-safe.
    applies_to: [room]
  weapon:
    description: An item that can be wielded.
    applies_to: [item]
```

`applies_to` is enforced at validation (§7.3). Tags applied
to entity types not in the list are fatal.

### 4.5 properties.yml

A pack may carry a `properties.yml` listing typed entity
properties:

```yaml
properties:
  level:
    description: Combat level.
    type: int
    applies_to: [player, npc]
  reputation:
    description: Standing with a faction.
    type: map_int
    applies_to: [player]
```

The `type` field draws from a fixed enumeration mirroring
the property-value-type set: `string`, `int`, `double`,
`bool`, `long`, `map_int`, `map_string`, `list_string`. An
unknown type is a fatal registration error.

### 4.6 Dependency resolution

After Phase 1, the loader installs a dependency resolver
into the tag and property registries (§3.2). Inside a pack's
files, an unprefixed tag or property name is resolved by:

1. Engine name.
2. Current pack's scoped name.
3. Each declared dependency's scoped name, in declaration
   order.

A pack that wants to use another pack's tag without prefixing
declares the other pack as a dependency. Without the
dependency, the unprefixed name fails resolution and the tag
or property is treated as unknown.

The resolution rule is identical between tag and property
registries because they share the dependency-resolver
contract.

**Acceptance criteria**

- [ ] `tapestry-core` is the only pack that registers at
      engine scope.
- [ ] Pack-scope registrations carry the `<ns>:` prefix in
      their full key.
- [ ] Bare-name resolution checks engine → current pack →
      dependencies.
- [ ] Cross-pack tag use without declaring a dependency
      fails resolution.
- [ ] Unknown property types in `properties.yml` are fatal
      at load.

---

## 5. Entity ids and namespaces

### 5.1 Id shape

Every entity (room, item, mob, quest) declared by a pack
MUST carry a stable string id. By convention, ids are
prefixed with the pack namespace:

```
legends-forgotten:goblin_warrior
legends-forgotten:rusty_dagger
legends-forgotten:goblin_camp
```

A pack MAY declare an id without a prefix (e.g.
`recall_room`); such ids are accepted but the engine cannot
enforce ownership.

### 5.2 Namespace enforcement

When an id IS prefixed (contains `:`), the prefix MUST equal
the loading pack's namespace. The loader rejects any other
prefix as a fatal error:

```
Namespace mismatch: pack 'legends-forgotten' declared ID
  'other-pack:thing' in /path/to/file.yaml
```

This prevents one pack from squatting on another pack's
namespace.

### 5.3 Duplicate-id detection

The loader maintains a per-load registry of every declared
entity id together with the file path it was declared in.
A second declaration of the same id from a different file
is a fatal error:

```
Duplicate entity ID 'legends-forgotten:smith': declared in
  .../mobs/smith.yaml
  .../mobs/smith_v2.yaml
```

Duplicate detection is cross-content-type (a room id and a
mob id may not share a name) and cross-pack (two packs may
not declare the same id).

### 5.4 Filename / id mismatch

A non-fatal warning fires when a file's stem (last path
component without extension) does not match the entity's
short id (suffix after the last `:` of the entity id). This
is purely a content-hygiene warning; the loader continues.

**Acceptance criteria**

- [ ] Prefixed ids must match the loading pack's namespace.
- [ ] Duplicate ids fail with a message naming both files.
- [ ] Filename / id mismatch logs at warn level and does
      not abort.

---

## 6. Scripts and the JavaScript runtime

### 6.1 The runtime

A single sandboxed JavaScript engine is created at startup
with these limits:

- **Execution timeout** of 5 seconds per top-level execute
  call.
- **Recursion limit** of 100.
- **Memory limit** of 50 MB.
- **Strict mode** enabled.

These limits apply to every script execution: pack init
scripts, command handlers invoked from scripts, quest
lifecycle hooks, etc. A handler that exceeds any limit
throws; the runtime caller catches and logs.

### 6.2 The API surface

The runtime exposes a single global, `tapestry`, whose
properties are named slices of engine functionality:

```
tapestry.commands.register(...)
tapestry.world.getRoom(id)
tapestry.items.create(templateId)
tapestry.stats.set(entityId, "strength", 20)
tapestry.dice.roll("2d6+1")
tapestry.events.subscribe("combat.kill", handler)
...
```

Each slice is built by an **API module** (an object
implementing a small interface that declares a `namespace`
and a `build(engine)` method returning the slice object).
Modules register through DI; the runtime walks them all at
construction and assigns each module's slice to
`tapestry[module.namespace]`.

The set of modules is engine-wide and shared by every script.
There is no per-pack module set; a pack's scripts see exactly
the same surface as any other pack's scripts.

### 6.3 Pack context globals

The runtime sets two engine-managed globals before each
script execution:

- **`__currentPack`** — the pack namespace of the script
  being executed. Used by API modules that need to know who
  is calling (e.g. `commands.register` records the pack
  name in the registration; flow registration captures it).
- **`__currentSource`** — the relative path of the script
  file within its pack (for diagnostics).

Scripts SHOULD NOT mutate these globals. Modules SHOULD
read them defensively (treat undefined / null as "no pack
context").

### 6.4 Script discovery and order

A pack's `scripts` content path is a glob. Matching files
are loaded in this order:

1. **`init.js`** first if present. By convention this is the
   pack's registration entry point — it calls
   `tapestry.commands.register(...)`, declares emotes, sets
   up event subscriptions.
2. Every other matched file in alphabetical order.

Within `init.js`, the script may load other resources
imperatively via API modules (e.g. `tapestry.fs.read(...)`).

Across packs, scripts execute in pack discovery order
(§2.4). Once a script has executed, anything it registered
is immediately visible to later scripts (event subscribers,
command registrations, helper functions hung off the
`tapestry` global by content convention).

### 6.5 Failure mode

A script that throws aborts the loader for its pack with the
exception. There is no `try/catch` around individual scripts
at the loader level; a bad pack stops the server start. This
is deliberate — broken content should not silently degrade
into a half-running world.

**Acceptance criteria**

- [ ] One JS runtime is constructed at engine startup with
      the configured sandbox limits.
- [ ] `tapestry.<namespace>` is populated from every
      registered API module.
- [ ] `__currentPack` and `__currentSource` are set before
      each script execution.
- [ ] `init.js` runs before any other script in the same
      pack.
- [ ] A throwing script aborts the loader for its pack.

---

## 7. Cross-reference validation

After all packs load, a separate validator sweeps the
populated registries and reports problems. The validator
runs once, produces a count of issues, and logs each.

### 7.1 Validation mode per pack

Each pack's manifest declares `validation: strict` (default)
or `validation: lenient`. The mode controls how violations
attributed to that pack are reported:

- **Strict.** Throw `InvalidOperationException` on the first
  violation. The server start fails.
- **Lenient.** Log a warning and increment the issue count.
  The server continues.

Validation mode is per-pack, not global. A strict pack's
violations are fatal even if a lenient pack would have only
warned.

### 7.2 Mob validation

For every registered mob template:

- A `skill_trainer`-tagged mob without a TrainerConfig or
  with an empty ability list is a violation.
- A shop-tagged mob without a ShopConfig (or with an empty
  sells list) is a violation.
- Battle commands that look like ability ids (single tokens
  with no space) on a mob with no abilities declared
  produces a warning ("they will fizzle").
- A mob with battle commands but `MaxHp = 0` produces a
  warning ("can't survive combat").
- Each declared ability id must resolve in the ability
  registry. Unknown ids are a violation.
- A spell ability requires `MaxResource > 0`.
- A skill ability requires `MaxMovement > 0`.
- Each battle command's verb must resolve in the command
  registry. Unknown verbs are a violation.

### 7.3 Tag validation

For every mob, item, and room, every tag on the entity is
resolved against the tag registry using the entity's pack
namespace context:

- Unknown tags fail per the entity's pack validation mode.
- Tags whose `applies_to` set does not include the entity's
  type fail unconditionally (even in lenient mode), because
  applying a room tag to an item is a semantic error.

A tag declared on an entity whose pack has no loaded
manifest (orphan entity from a stripped pack) defaults to
strict validation. The validator logs a warning explaining
the orphan.

### 7.4 Property validation

For every loaded entity, every property *key* is resolved
against the property registry:

- Unknown properties fail per the pack's validation mode.
- Known transient properties are skipped from value-type
  checking.
- Known non-transient properties have their value's runtime
  type checked against the registered type (string, int,
  long, double, bool, map / list shapes). Mismatches are
  fatal regardless of validation mode.
- Properties whose `applies_to` set does not include the
  entity's type are fatal regardless of validation mode.

### 7.5 Item and room validation

The validator visits item templates and rooms but currently
performs no per-entity checks beyond the tag and property
sweeps in §7.3 / §7.4. The scaffolding exists to add more.

**Acceptance criteria**

- [ ] Each pack's validation mode controls only that pack's
      violations.
- [ ] Type mismatches and `applies_to` violations are fatal
      even in lenient mode.
- [ ] Mob trainer / shop / ability / battle-command checks
      run for every template.
- [ ] Orphan entities (pack manifest missing) default to
      strict.
- [ ] The validator logs a final issue count.

---

## 8. The connection feature

The pack system also restores a small auxiliary state
artifact at startup: **persistent connection records**. These
are content-defined links / portals / world-altering changes
serialized to disk under the configured connections path.

The connection loader reads the serialized records and
applies them to the world (e.g. opening keyword exits,
setting flags on rooms). The exact schema and semantics are
content-defined; the engine just owns the load timing
(after content load, before validation).

This feature is intentionally minimal in the spec — it lives
here because the pack loader orchestrates it. Detailed
semantics belong to its own spec when needed.

---

## 9. Observable events

The pack system itself does not emit engine bus events. It is
a startup-only feature. Modules that *do* emit events
(combat, abilities, etc.) are populated by scripts during
script execution, and once registered they emit via the
normal event-bus path.

The only side effects observable from outside the loader are:

- Log messages at debug / info / warning / error level.
- The loaded-packs list available through
  `IPackManifestProvider`.

---

## 10. Configuration surface

The following are externally configurable and not fixed by
this spec.

| Policy | Where it applies |
|---|---|
| Packs root directory (today `<binary-dir>/packs/`) | §2.4 |
| Server config's pack-filter list | §2.4 |
| Per-pack validation mode | §7.1 |
| JS sandbox limits (timeout, recursion, memory) | §6.1 |
| Module catalog (the set of `IJintApiModule` registered with DI) | §6.2 |
| Default room-spawn reset interval fallback | §3.4 |
| Engine pack namespace (today `tapestry-core`) | §2.3, §4.2 |

---

## 11. Open questions / future work

- **Discovery order is alphabetical.** Phase ordering depends
  on alphabetical pack-directory discovery, not on the
  manifest's `dependencies` field or `load_order` field. Two
  packs with circular content references (door declared in
  pack A pointing to a room in pack B and vice versa) work
  only because rooms are loaded fully before doors mirror.
  A formal topological sort over `dependencies` would make
  the ordering explicit.
- **`load_order` is help-only.** The field affects help-topic
  precedence but not pack load order. This is surprising;
  authors expect it to control everything.
- **Door mirroring is per-pack.** A pack that adds a door to
  a foreign pack's room cannot rely on the other side being
  mirrored automatically. Workaround: declare the reverse
  door explicitly in the foreign room override. Better: a
  global mirror pass at end of all Phase 2.
- **Fixture cross-pack refs are order-dependent.** A fixture
  in pack A pointing at an item declared in pack B works
  only if pack B is alphabetically before pack A. No error
  if it's wrong — the fixture silently misses.
- **No script isolation.** All packs share one JS runtime.
  A pack that hangs a property on a shared global pollutes
  every other pack's namespace. The `__currentPack` global
  is the only signal of pack identity.
- **JS sandbox limits are hardcoded.** The 5s / 100-depth /
  50 MB triple is in the runtime constructor. Externalize.
- **No partial-failure recovery.** A throwing script aborts
  the entire server start. Operators who want "start with
  what works" need an opt-in `continue-on-error` mode.
- **Connection loader is opaque.** The persistent-connection
  side channel exists, has a path, and is loaded at a known
  point — but its semantics live entirely in content code.
  A first-class spec entry would help.
- **`dependencies` versions are ignored.** The values in
  `dependencies` (constraint strings) are not interpreted at
  all. Tooling could at least warn on missing-by-name
  dependencies.
- **`engine_version` is informational.** No compatibility
  check happens at load time. A version mismatch detector
  would fail-fast on incompatible content.
- **Phase 2 ordering within a pack is engine-baked.** The
  weather-zones-first-then-areas-then-rooms order is
  hardcoded in the loader. A pack that wants to load
  content in a different order has no path.
- **`tapestry-core` is a magic string.** Treat the engine
  namespace as configuration so a downstream fork can
  rename it without rewriting the loader.

---

<!-- Generated: 2026-05-21 · Scope: PackManifest + PackContentPaths + PackLoader + PackValidator + PackContext + JintRuntime + IJintApiModule + IPackManifestProvider + YamlContentLoader + TagsFileLoader + PropertiesFileLoader + ContentLoadingModule · Spec style: narrative + acceptance criteria · Detail level: behavior only -->
