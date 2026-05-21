# Persistence and Saves — Feature Specification

**Status:** Draft · **Scope:** The property registry, account and
player save data shapes, the serialize/deserialize pipeline, file-
based storage with atomic writes, account-service operations
(create, authenticate, password change), the autosave pipeline, and
version/migration scaffolding · **Audience:** Anyone reimplementing
or porting this feature in any language.

This document describes *what* the persistence layer must do, not
*how* to implement it. The specific on-disk format (YAML, JSON,
binary) and the hash algorithm (BCrypt today) are policy and live
outside this spec.

---

## 1. Overview

Persistence is the substrate that lets accounts, characters, and
their items survive process restarts. The feature has four
responsibilities:

1. A **property registry** that knows the type and scope of every
   entity property so the serializer can coerce values on load.
2. Two **stores** with stable contracts — accounts and players —
   plus a stable storage layout that other features may extend
   with their own supplemental files.
3. A **serializer** that converts a live player entity (plus its
   item tree) into a typed save record and back.
4. **Services** that wrap the stores with domain operations
   (account create / authenticate / password change; player save
   / load; full-server autosave with snapshot-then-write
   semantics).

Other features own their own state files within the player
directory (quests, flow, etc.) and are not driven by this spec
beyond convention. This feature owns the *core* player and
account records.

### Core concepts

- **Property registry** — typed metadata for entity properties.
  Each entry carries a name, scope (engine or pack), description,
  value type, optional `applies-to` filter, and a transient flag.
- **Player save data** — the on-disk record for a single
  character: version, ids, name, location, tags, roles, stats,
  properties, equipment, inventory, and a flat list of every item
  the character holds (transitively).
- **Account save data** — the on-disk record for a single
  account: id, email, password hash, character list, creation
  time, verification state.
- **Tagged value** — the wire shape for an *unknown* property
  value carried in a save: a `{type, value}` envelope that lets
  the loader recover the CLR type even when the property is not
  in the registry (e.g. a pack was uninstalled).
- **Supplemental file** — any non-core file in a player's
  directory, owned by another feature. The persistence feature
  exposes a "list supplemental file types" query so admin tools
  can enumerate them without coupling to the file format.

### Goals

1. Make every player-state mutation persistable by writing one
   self-contained save record.
2. Make load deterministic and tolerant of registry drift (pack
   removal, property additions / renames).
3. Make on-disk writes atomic against process crash and
   concurrent reads.
4. Hide the account password material behind one-way hashes.
5. Provide a snapshot-then-write autosave path so the autosave
   itself does not stall the game loop.
6. Provide a version field on every save so future format changes
   can migrate forward without losing existing data.

### Non-goals

- The on-disk format. YAML with snake_case fields is the current
  implementation; that is policy.
- Backup / restore tooling, ZFS snapshots, or off-host
  replication.
- Cryptographic identity beyond password hashing (no JWTs, no
  client certs, no PKI).
- Account recovery flows (password reset, email verification).
  The shape carries the verification fields but the workflow is
  not specified here.
- Item template persistence. Items are persisted by *instance
  state* on the holder; templates remain in content.
- World-level state persistence (rooms, doors, weather). Those
  are reloaded from content; the only world state that survives
  restart today is what is stored on player saves.

---

## 2. Property registry

### 2.1 Purpose

Entities carry a free-form property bag. Without metadata the
serializer would not know how to coerce loaded values — was
`level` a Dictionary&lt;string,int&gt; or an int? Did this property
ever exist? The property registry is the engine-wide source of
truth for that metadata.

### 2.2 Entry shape

A property registry entry carries:

- A **name** in `snake_case`. The registry MUST reject any name
  containing a hyphen or otherwise non-snake_case at registration
  time.
- A **scope**: either the engine itself, or a content pack
  identified by name.
- A **description** for diagnostics.
- A **value type** drawn from a fixed enumeration (string, int,
  long, double, bool, map-of-int, map-of-string, list-of-string).
  See §2.5 for why this set, not arbitrary types.
- An optional **applies-to** set of entity type strings.
  Diagnostic only — the serializer does not enforce this; it
  helps tooling surface "this property is only meaningful on
  NPCs", etc.
- A **transient** flag. Transient properties are read in memory
  but MUST NOT be serialized.

### 2.3 Engine vs pack scope

The registry distinguishes engine-owned properties from pack-
owned ones:

- **Engine properties** register with a bare name (e.g.
  `weight`). Duplicate engine registrations MUST raise at
  registration time.
- **Pack properties** register with a `(packName, name)` pair.
  Internally they are keyed `packName:name`. A pack property
  MUST NOT shadow an engine property — attempting to do so MUST
  raise. Two packs MAY register a property with the same bare
  name (they get distinct full keys).

The full key (`name` for engine, `packName:name` for pack) is
the canonical form for lookups in tools that need precise
identification.

### 2.4 Resolving a property name

A lookup takes a name and (optionally) a "current pack" context
used for shorthand resolution:

1. Direct match against the registered key (engine name or full
   `packName:name`).
2. If a current pack is supplied AND the name has no `:`, try
   `currentPack:name`.
3. If still unresolved AND a **dependency resolver** has been
   installed, walk the current pack's declared dependencies and
   try `<depPack>:name` for each, in declaration order. First hit
   wins.

The dependency-resolver hook is set by the pack loader so that
content can reference properties from packs it depends on
without typing the full prefix. Engine properties always resolve
without prefixes, so an engine name shadowing a pack name yields
the engine entry (per §2.3).

### 2.5 Why a closed type set

The value-type enumeration is intentionally narrow. Save formats
can serialize these eight shapes losslessly with no reflection
heuristics, and the deserializer can coerce a YAML / JSON value
into them with a small fixed coercion table. Properties that
need richer shapes (lists of mixed types, nested records, custom
classes) are handled via the tagged-value path (§4.4) — they
survive a save/load round-trip but lose type information from
the registry's point of view.

### 2.6 Queries

The registry exposes:

- "Is this name known (within the optional pack context)?"
- "Is this name transient?"
- "What value type does this name have?"
- "All entries" for tooling and diagnostics.

**Acceptance criteria**

- [ ] Engine property registration rejects duplicates.
- [ ] Pack property registration rejects shadowing engine names.
- [ ] Snake-case validation rejects hyphens and non-conforming
      names with a clear message.
- [ ] Resolution checks direct, then `currentPack:name`, then
      dependency-pack prefixes in order.
- [ ] Transient properties are flagged and are skipped by the
      serializer (§4.4).
- [ ] The closed type set is enforced at registration; unsupported
      types cannot be registered.

---

## 3. Storage contract

### 3.1 Stores

Two storage interfaces are part of the spec:

- **Account store** — `LoadById`, `LoadByEmail`, `Save`,
  `Delete`, and a synchronous `ExistsByEmail` cheap-check used
  during login.
- **Player store** — `Load(name)`, `Save(data)`, `Delete(name)`,
  a synchronous `Exists(name)`, and a
  `GetSupplementalFileTypes(name)` enumeration used by admin
  tooling.

Both interfaces are async-first (returning task-like values) so
implementations may use non-blocking I/O. Synchronous-existence
checks exist because they are called on the player input thread
during login (§5.1) and must be cheap.

### 3.2 Storage layout

The reference implementation lays out files like this. Other
implementations may choose other layouts but MUST preserve the
contract elements (atomicity, separation, path safety):

```
<savePath>/
  accounts/
    index.<format>              ← email → account-id map
    <accountGuid>/
      account.<format>          ← AccountSaveData
  players/
    <lowercased name>/
      player.<format>           ← PlayerSaveData (this feature)
      <other>.<format>          ← supplemental files (other features)
```

Player directories are keyed by **lower-cased name**. The store
MUST resolve any input name to lower case before computing the
filesystem path. Names with non-ASCII characters are out of
scope.

### 3.3 Atomic writes

Save MUST be atomic against:

- Process crash mid-write (the prior file remains valid).
- Concurrent readers (load never sees a half-written file).

The required pattern for player saves:

1. Write the new content to a sibling `.tmp` file.
2. Move the prior canonical file to a sibling `.bak` (if one
   exists).
3. Move the `.tmp` to the canonical filename.
4. Delete the `.bak`.

An interrupted process leaves either the prior file intact (step
2 not yet done) or the `.bak` next to the new file (step 4 not
yet done). Implementations MAY add a recovery sweep at startup
that promotes `.bak` files back to canonical when canonical is
missing — this spec does not require it but recommends it.

Account saves and the email index file MUST use the same write-
through-tmp-then-rename pattern. The email index MUST be updated
atomically as part of every account save.

### 3.4 Path safety

Player and account directories are derived from caller-supplied
identifiers (player name; account id). The implementation MUST
guard against path traversal: every resolved path MUST be a
descendant of its base directory. Names containing `..`, leading
slashes, or other escape characters MUST be rejected (or
sanitized to a safe form) before path construction.

### 3.5 Supplemental files

Other features (quests, flow, scripting) may write their own
state files into a player's directory using their own file
naming convention. The player store's
`GetSupplementalFileTypes(name)` query enumerates the names of
those files (without the format extension), so admin tools can
audit "what's saved for this player" without coupling to each
feature's format.

The player store MUST NOT interpret supplemental files; it only
lists them. Deletion of a player (§3.6) removes the entire
player directory including supplementals.

### 3.6 Delete

Player delete removes the player's directory recursively
(including supplemental files). Account delete removes the
account directory and updates the email index to drop the
account's email mapping. Neither delete is reversible at the
storage layer; callers wanting soft-delete behavior layer it on
top.

**Acceptance criteria**

- [ ] Player names are lower-cased before path computation.
- [ ] Saves are atomic: an interrupted save leaves either the
      prior or the new file intact, never a partial file.
- [ ] Path traversal attempts on either store throw before any
      filesystem access.
- [ ] The email index is updated atomically alongside an account
      save.
- [ ] Player delete also removes supplemental files.

---

## 4. Player serialization

### 4.1 Save data shape

A `PlayerSaveData` carries:

- A **version** integer (§7).
- The player's **id** (as a stable string), **account id**, and
  **type** (typically `player`).
- The player's **name** and **location** (room id, or empty
  string if not placed).
- A **tags** list and a **roles** list.
- A **stats** block (§4.2).
- A **properties** map keyed by registered name (§4.4).
- An **equipment** map from slot key to item id.
- An **inventory** list of top-level item ids carried by the
  player.
- A flat **items** list (§4.3) holding the save records for
  every item the player owns transitively.

The items list is FLAT — both top-level inventory items and
nested container contents appear in the same list. Each item
save record carries its own id and may declare a `container`
field referencing the parent item id. The reconstruction pass
(§4.5) wires the tree from those references.

### 4.2 Stats

Stats serialize as three sub-blocks:

- **Base** — the six attributes and three vital maxima.
- **Vitals** — current HP, resource (mana), and movement.
- **Modifiers** — a list of `{source, stat, value}` entries
  representing every active stat modifier on the player. Source
  keys MUST be stable across save/load (see equipment modifier
  source-keying in `docs/specs/inventory-equipment-items.md`).

On load, base stats are restored first, then modifiers are
added, then vitals are set. This order ensures that vital
clamping (e.g. HP capped at MaxHp) uses the post-modifier
maximum, so a player with a +20 HP buff at save time loads with
the correct effective max.

### 4.3 Item save data

An `ItemSaveData` carries id, name, type, an optional container
id (the id of the parent item when the item lives inside a
container), tags, keywords, and a properties map (§4.4).

Items do NOT serialize their own stats block — items are not
combatants. They MAY carry stat-modifier metadata via properties
(the inventory feature's `modifiers` property), but that
property is **transient** (§2.2) and is rebuilt from the
template at load time.

### 4.4 Property serialization

For each property on the entity:

1. If the registry marks the property as **transient**, skip it.
2. If the registry **knows** the property:
   - Pass the value through after normalizing the well-known
     container shapes (map-of-int, map-of-string, list-of-string)
     to the wire-friendly object representation. The exact value
     is preserved.
3. If the registry **does not know** the property:
   - Wrap the value in a tagged envelope: `{type: "<primitive>",
     value: <value>}`. The `type` field captures the runtime
     primitive (int, long, float, double, bool, string) so the
     loader can coerce back exactly even when the registry has
     since lost the entry.

Tagged-value envelopes only carry primitives; complex unknown
shapes fall back to whatever the underlying format serializes
naturally.

### 4.5 Property deserialization

For each property in the loaded record:

1. **Normalize.** The on-disk format may produce maps keyed by
   object (e.g. YAML's default behavior). Recursively normalize
   them to string-keyed maps.
2. **Tagged envelope path.** If the value is a normalized map
   with `type` and `value` keys, coerce the inner value to the
   declared primitive. The loader MUST tolerate accidental
   nested tagged envelopes (a value that was double-wrapped by a
   prior bug); it recursively unwraps until it finds a non-
   tagged inner value, using the *deepest* type tag.
3. **Known-property path.** Otherwise, look up the property in
   the registry. If the registry knows it, coerce the value to
   the registered CLR shape. Coercion failures fall back to a
   default of the registered type (e.g. an empty dictionary, an
   empty list, the zero value of a primitive).
4. **Unknown-untagged path.** If the value is neither tagged nor
   known, pass it through verbatim. The runtime will receive
   whatever the underlying format produced.

The nested-tag self-healing in step 2 is deliberate: it absorbs
a category of historical bugs where a save was re-serialized
without first deserializing tagged values, resulting in
`{type, value: {type, value: ...}}` nesting. The loader
collapses these silently so a long-lived save file does not
accumulate fossilized wrappers.

### 4.6 Item tree reconstruction

On load:

1. Build a fresh entity for the player from the top-level fields
   (id, name, type, location, tags, roles).
2. Restore stats per §4.2.
3. Restore properties per §4.5.
4. Build entity instances for every item in the flat items list,
   indexed by item id. Restore each item's tags, keywords, and
   properties. Track each item's container id (if any).
5. **Wire container relationships** first: for each item with a
   container id, add the item to the container's contents.
6. **Wire the player's top-level inventory** next: for each id
   in the inventory list, add the corresponding item to the
   player's contents. (Items already inside a container in step
   5 are not in the inventory list — only the outermost item
   appears.)
7. **Wire equipment**: for each `(slot, itemId)` in the
   equipment map, set the player's equipment at that slot to the
   referenced item.

Container relationships MUST be wired before inventory and
equipment so that "the chest inside my backpack" survives a
save/load cycle with the right shape.

### 4.7 Collecting items for save

When saving, the serializer collects items by:

1. Walking the player's contents recursively, including nested
   container contents.
2. Appending each equipped item that is not already in the
   recursive walk.

The resulting flat list goes into the save record. Each item
save record's `container` field references the item's *direct*
container only when that container is itself another item;
items directly held by the player (top-level inventory) have a
null/empty container field and are listed by id in the
inventory map instead.

**Acceptance criteria**

- [ ] Transient properties never appear in saves.
- [ ] Known properties round-trip without the tagged envelope.
- [ ] Unknown properties round-trip via the tagged envelope and
      preserve their primitive type.
- [ ] Nested tagged envelopes on load self-heal to a single
      coercion.
- [ ] Items load in the order container-wiring → inventory →
      equipment.
- [ ] Stat loading order is base → modifiers → vitals.
- [ ] An item appearing in equipment is also present in the
      items list.
- [ ] Saving and immediately reloading a player produces an
      equivalent entity (modulo runtime ids that may differ for
      transient artifacts).

---

## 5. Account service

### 5.1 Identity model

An account carries a globally-unique id, a normalized email
(lowercased, trimmed), a one-way password hash, a character-name
list, a creation timestamp, and verification fields (verified
flag + timestamp) that this spec acknowledges but does not
drive (§1 non-goals).

Email is the human-facing key and is enforced unique through
the store's index. The id is the durable key — character saves
reference the account id, not the email, so an email rename
does not orphan characters.

### 5.2 Operations

- **Create account.** Generate a new id, normalize the email,
  hash the password, save the account, return the new record.
  Saving an account MUST update the email index atomically with
  the account file (§3.3).
- **Authenticate by email.** Look up the account by normalized
  email; verify the password against the stored hash; return
  the record on success, null on failure.
- **Authenticate by id.** Same as above but starts from a known
  account id.
- **Add / remove character.** Append (idempotent on
  case-insensitive name match) or remove a character name from
  the account's list and save.
- **Change password.** Re-verify the old password, hash the new
  one, save.

### 5.3 Hashing

Stored passwords MUST be one-way hashes with per-account salt.
The implementation MAY choose its hash family but MUST NOT store
plaintext or reversible representations anywhere — including
logs, traces, and any debug surface. Verify operations MUST use
the hash family's constant-time comparison routine.

### 5.4 Online-entity tracking

The service maintains an in-memory map from entity id to
account id, used by features (combat, alignment, persistence)
that want to know "which account owns this in-world entity right
now". The map is populated and torn down by the session layer:

- **Track** on login completion / character spawn.
- **Untrack** on disconnect / character despawn.

The map is purely runtime; it does not persist across restart
and is not authoritative — the durable mapping lives in the
character's `AccountId` field.

**Acceptance criteria**

- [ ] Emails are normalized (trim + lowercase) on every entry
      point (create, authenticate, exists check).
- [ ] Password hashes use a one-way hash with per-account salt.
- [ ] Authenticate (by email or by id) returns null on any
      mismatch — including missing account — without revealing
      which condition failed.
- [ ] Adding a character to an account is idempotent on
      case-insensitive name.
- [ ] Online-entity tracking is purely in-memory and not
      consulted by the load path.

---

## 6. Player persistence service

The player persistence service is the domain wrapper around the
raw store, used by the rest of the engine.

### 6.1 Save / load

- **Save player session.** Build a save record from the live
  entity (and its item tree), then write through the store.
  Used after every meaningful state change (item picked up,
  combat ended, level up, etc.; the engine wires the trigger
  points).
- **Load player.** Read from the store and run the deserializer.
  Returns either a load result (entity + account id + flat item
  list) or null when missing.
- **Save new player.** Used by character creation commit. Same
  pipeline as save but doesn't require a session — it operates
  on the entity directly with an explicit account id.
- **Player save exists.** Cheap query routed straight to the
  store; used by login (§3.1).

### 6.2 Snapshot-then-write autosave

A naive autosave that serializes every player from the game
thread blocks the tick loop. The service provides a two-phase
path:

1. **Snapshot.** Iterate every Playing-or-LinkDead session,
   build a save record for each on the game thread. Snapshots
   are pure-data records that no longer touch live entity
   state.
2. **Write.** Hand the snapshot list to an async writer that
   loops `store.SaveAsync(record)` for each, swallowing and
   logging per-record exceptions so one corrupt save doesn't
   abort the batch.

The snapshot step holds whatever lock or thread the game loop
needs; the write step does not. The engine schedules them
together at the configured autosave cadence (in ticks).

A simpler one-shot "save all players" entry point also exists
for synchronous use (admin commands, shutdown). It runs both
phases in one call.

### 6.3 Error handling

The service treats per-player save failures as **isolated**:
exceptions on one player's save are logged with the player name
and do NOT propagate up to abort other players' saves. The
write loop continues. This is the operationally correct stance
for a long-running server.

Per-player load failures return null and log; the calling
feature (login flow) treats null as "no save exists" and falls
through to the new-player flow.

**Acceptance criteria**

- [ ] SavePlayer collects the item tree recursively before
      handing to the serializer.
- [ ] Autosave snapshots only Playing and LinkDead sessions.
- [ ] Snapshot phase touches live entity state; write phase
      does not.
- [ ] Per-player save errors are logged and do not abort the
      batch.

---

## 7. Versioning and migration

Every player save carries a numeric **version** field. The
loader MUST tolerate older versions by:

1. Reading the file as a generic dictionary.
2. Applying the registered migration for `version → version+1`,
   then `version+1 → version+2`, etc., in sequence until the
   file is at the current version.
3. Binding the migrated dictionary into the structured
   `PlayerSaveData` shape.

A migration is a function from the dictionary shape of version
`N` to the dictionary shape of version `N+1`. Migrations are
registered in a single table keyed by source version. The
current version is a single constant.

Today the table is empty (the file format has not changed since
version 1). The scaffolding exists to be ready when it does.

The loader MUST log when it encounters a save at an older
version — this gives operators a heads-up before the migration
runs. Saves at a NEWER version than the loader knows MUST be
treated as a load failure, not silently downgraded.

Account saves do not currently carry a version field; the same
pattern is recommended for parity but is not required.

**Acceptance criteria**

- [ ] Loading a save logs its version vs the loader's current
      version when they differ.
- [ ] Migrations apply in sequence, never skipping a step.
- [ ] Loading a save newer than the current version fails
      cleanly (does not produce a corrupt entity).

---

## 8. Observable events

This feature does not emit engine events for save / load
operations. Other features that *depend on* persistence emit
their own events — for example, quest persistence subscribes to
a `player.login` event emitted by the login flow to trigger its
load.

Conventionally, callers may observe:

- A `player.login` event emitted by the login flow after a
  successful authentication and entity restore. Persistence is
  NOT the emitter; it consumes the event in features that need
  to react.
- A `character.created` event emitted by the character-creation
  feature after a successful commit. Persistence is the
  mechanism by which the commit becomes durable; the event is
  emitted by character creation.

The persistence feature's silence is intentional: persistence is
a side effect of state changes already announced by other
features. Re-announcing them would be noise.

---

## 9. Configuration surface

The following are externally configurable and not fixed by this
spec.

| Policy | Where it applies |
|---|---|
| Save path (root directory) | §3.2 |
| On-disk format (YAML, JSON, etc.) | §3 |
| Hash family (BCrypt, Argon2, …) | §5.3 |
| Autosave cadence (ticks between snapshot+write cycles) | §6.2 |
| Per-property value type registrations (engine + pack) | §2 |
| Path-safety policy (rejection vs sanitization for unsafe names) | §3.4 |
| Whether to perform a startup sweep for orphan `.bak` / `.tmp` files | §3.3 |

---

## 10. Open questions / future work

- **Account version field.** Player saves carry a version;
  account saves do not. A symmetric version on accounts would
  let future schema changes follow the same migration path.
- **No transactional cross-feature save.** Each feature writes
  its own file in the player directory. A player save and a
  quest save are not transactional — a crash between them
  leaves a partially-updated player. A directory-level
  transaction (write all sibling files, then atomically rotate)
  would close the window.
- **Email rename has no path.** The account record's email is
  effectively immutable (the index would need re-keying). A
  documented rename flow is missing.
- **Soft delete.** Account and player delete are unconditional
  filesystem removes. Operators who want a recoverable trash
  bucket must layer it on top.
- **Path safety policy.** Current implementations reject names
  with traversal characters by throwing. A documented allow-
  list of acceptable characters (and a server-startup check
  against existing save directories) would be more robust.
- **Migration registration is engine-static.** Migrations live
  in a single in-engine table. A pack that introduces new
  required properties has no migration hook to seed defaults on
  legacy saves; the deserializer's coerce-to-default path
  handles missing values, but explicit pack migrations would
  reduce surprises.
- **Synchronous existence checks during login.** The
  `Exists(name)` query is called on the input thread because
  login blocks on it. If the store is a remote backend, that
  becomes a network round-trip on the input thread. A cache
  layer or an async-only login flow would mitigate.
- **Item template drift.** Item save records carry name, tags,
  keywords, and properties — *not* the template id. The
  template id IS in the properties map (under a known
  registered key), so it survives, but a template that gets
  removed leaves items with a dangling template id. The
  inventory feature treats them as functional but ungenerated
  ("no template"), which is correct but easy to overlook.
- **Tagged-value silent loss.** Tagged values capture only
  primitives. A pack that registers a custom complex type for
  a property, then is uninstalled, loses that property's shape
  on next load — the value either passes through as the
  format's default deserialization or coerces to a default.
  Tooling that detects this drift would help.

---

<!-- Generated: 2026-05-21 · Scope: PropertyRegistry + PlayerSaveData + AccountSaveData + PlayerSerializer + PlayerPersistenceService + AccountService + IPlayerStore + IAccountStore + FilePlayerStore + FileAccountStore + SaveMigrations + FlowPersistenceAdapter · Spec style: narrative + acceptance criteria · Detail level: behavior only -->
