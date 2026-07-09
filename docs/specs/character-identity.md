# Character Identity Across Worlds (world-locking)

**Status:** Draft · **Scope:** binding a character to the *world* (ruleset) that
defined it — the world stamp, its persistence + backfill, and the login/roster
gate that keeps a character out of a server running a different world · **Audience:**
anyone reimplementing the multi-ruleset hosting model. Resolves the
`docs/proposals/character-identity-across-packs.md` §7 decisions (all six adopted
2026-06-16). Builds on the pack manifest + dependency-closure loader
(`scripting-and-packs.md`), the versioned/migrated player save
(`persistence.md`), and the login flow (`login.md`).

This document describes *what* world-locking must do, not *how*. The fallback
world, the manifest flag's spelling, and the save version number are policy /
interoperability details called out where they matter.

---

## 1. Overview

A character's persisted state splits cleanly in two:

- **Portable** — identity and raw mechanical state: name, account, gender, roles,
  base stats, vitals, resource pools, gold, alignment. These would mean the same
  thing in any ruleset.
- **Content-bound** — everything that *names content*: race / class / background
  / track ids, abilities, known recipes and feats, location and recall rooms,
  inventory and equipment item ids, visited rooms. These are meaningful only
  inside the ruleset that defined them.

Loading a character into a **different ruleset** drives its content-bound fields
through the engine's within-world fail-soft restore (unknown race → raceless,
missing room → start room, unresolved item → not spawned), which **silently
strips and corrupts** the character — and overwrites the good save with the
degraded one. A Wheel-of-Time channeler logging into a Shadowrun server returns
raceless, classless, with vanished weaves and gear.

The fix is to **lock each character to a world**: stamp it with a **`WorldID`**,
and gate login + the roster by the server's **active worlds**. A character only
enters a server running its world; otherwise login is refused — visibly and
safely — rather than silently degrading the save. The cost is **one additive save
field plus a login gate + roster filter**; the within-world fail-soft net is
unchanged, just scoped to its real job (retiring content within a world).

### Core concepts

- **World** — a *leaf* content pack that, together with its dependency closure,
  defines a complete playable ruleset (e.g. a starter world, a WoT world).
- **Library pack** — a shared baseline pack that worlds *depend on* (the engine
  baseline). A library is never a world on its own.
- **World stamp (`WorldID`)** — the leaf world pack id a character belongs to,
  persisted on the character. Exactly one per character.
- **Active world set** — the world-flagged packs active on a running server
  (libraries in the closure are loaded but are not worlds).
- **Login gate / roster filter** — the runtime check that a character's `WorldID`
  is in the active world set before it may log in or be offered.

### Goals

1. Bind each character to exactly one world via an additive save field.
2. Refuse login for a character whose world isn't active — visibly, never
   destructively.
3. Keep the within-world fail-soft restore unchanged.
4. Support an active world *set* in the model while running one world per process
   today.

### Non-goals

- **Cross-world portability.** Carrying a character between rulesets is not a
  goal; the only honest cross-ruleset move is an explicit, lossy re-roll (§7).
- **Co-hosting multiple full rulesets in one process** — supported by the model
  but not shipped; it depends on a deferred prerequisite (§9).
- Any change to account credentials or the rest of the save format beyond the
  one new field.

---

## 2. Worlds and library packs

A pack's manifest (`scripting-and-packs.md`) declares whether the pack is a
**world** or a **library**:

- A **world** pack is a leaf ruleset a character may belong to.
- A **library** pack is a shared dependency (the engine baseline, common content)
  and is **never** a valid world stamp.
- The default for a pack that declares neither is **library** — a pack must
  *opt in* to being a world. This is the safe default: a new or baseline pack is
  never accidentally a world, and a leaf pack that forgets the flag fails loudly
  the first time a character would be stamped to it, rather than silently
  becoming a stampable world.

The **active world set** of a running server is the world-flagged packs among its
active packs. Library packs in the dependency closure are loaded (their content
is available) but are not worlds. A server with no world-flagged active pack
cannot host characters — a configuration error surfaced at boot.

**Acceptance criteria**

- [ ] Each pack manifest can be flagged world or library; absent ⇒ library.
- [ ] Only a world-flagged pack may be a character's world; a library pack id is
      never a valid `WorldID`.
- [ ] The active world set is the world-flagged subset of the active packs;
      libraries are loaded but excluded from it.
- [ ] A boot whose active packs contain no world is a clear configuration error,
      not a server that silently hosts no one.

---

## 3. The world stamp

Every character carries a **`WorldID`** — the id of the leaf world pack it belongs
to. It is assigned once, at **creation**, from the server's active world, and is
never reassigned by normal play (a class swap, a teleport, retiring content do not
change a character's world).

A character belongs to **exactly one** world. Multiple characters per world is
normal; one account may hold characters in several worlds at once (§5).

**Acceptance criteria**

- [ ] Character creation stamps `WorldID` from the active world.
- [ ] A character's `WorldID` is stable across normal play (only an explicit
      cross-world conversion, §7, could change it — and that is out of scope).
- [ ] Creating a character when no world is active is refused (it has nothing to
      stamp).

---

## 4. Persistence and backfill

`WorldID` is an **additive** field on the player save; adding it bumps the
player-save version (to v23 in this engine) with an **append-only** migration —
no new persistence machinery, atomic writes unchanged.

Existing saves predate the stamp and are backfilled **deterministically**, with no
operator input:

- Derive `WorldID` from the **namespace of the character's saved location room id**
  (`starter-world:town-square` → `starter-world`).
- Fall back to a **configured default world** when the location is empty or its
  namespace is unparseable.

The migration is pure (location → world id), runs in the established chain, and
introduces no nullable-`WorldID` edge case downstream — every loaded save has a
world after migration.

**Acceptance criteria**

- [ ] `WorldID` persists on the character save behind an append-only version bump.
- [ ] A pre-stamp save is backfilled by deriving the world from its location
      namespace, falling back to the configured default world.
- [ ] After migration every save has a non-empty `WorldID`; the login gate never
      sees an unstamped character.
- [ ] Account credentials and all other save fields are unchanged.

---

## 5. The login gate and roster

An account maps to **many characters**, which may span worlds. On a given server,
only characters whose `WorldID` is in the **active world set** may log in.

- A character whose world **is** active proceeds through the normal returning-player
  flow (`login.md`).
- A character whose world is **not** active: it may not be entered. Where a
  character roster is presented, an out-of-world character is **omitted from the
  selectable list** and instead surfaced as a one-line **awareness footnote** (a
  count of characters in other worlds) — so the player knows it still exists but
  isn't offered something it can't act on here. It is **never** auto-deleted and
  **never** loaded into a different world (no silent degrade). (Earlier this was
  a greyed, select-then-refuse row; `character-select.md` §3 replaced that with
  the hide + footnote to cut the clutter and the misleading numbering.)
- The gate runs at **character selection** (the name / returning-player phase,
  `login.md`) — before any content restore — so an out-of-world character's save
  is never touched, let alone rewritten.

**Acceptance criteria**

- [ ] A character whose `WorldID` is in the active world set logs in normally.
- [ ] A character whose world is not active is not a selectable roster entry
      (hidden from the list, surfaced as a footnote count) — not deleted, not
      loaded elsewhere.
- [ ] The gate fires before content restore: a refused character's save is read
      for its world stamp but otherwise untouched and never rewritten.
- [ ] One account may hold characters in multiple worlds simultaneously; the
      roster reflects all of them, gating availability per the active set.

---

## 6. Within-world fail-soft is unchanged

The existing fail-soft restore — unresolved location → start room, unknown
race/class → fallback, missing item → not spawned, retired recipe culled — is
**kept exactly as-is**. It is the within-world safety net that lets content be
edited (a room deleted, a recipe retired) without bricking saves.

World-locking only **narrows its scope**: fail-soft no longer doubles as a
cross-ruleset bridge (its silent stripping was the corruption source). Within a
world, fail-soft absorbs content drift; across worlds, the §5 gate refuses before
restore ever runs.

**Acceptance criteria**

- [ ] Within a character's own world, the fail-soft restore behaves exactly as
      before this feature (no regression for content edits).
- [ ] Fail-soft is never reached for a character whose world is not active — the
      gate stops it first.

---

## 7. Cross-world transfer (out of scope)

A character is **not** portable across worlds. A character's stats, class, and
gear cohere only inside the ruleset that defined them, so the only honest
cross-ruleset move is a **re-roll**, not a carry-over.

If a true cross-world transfer is ever wanted, it must be an **explicit, lossy,
opt-in conversion** that preserves only the portable column (§1 — identity + raw
mechanical state) and **re-rolls** every content-bound field against the target
world. It is never an implicit consequence of logging in. Such a conversion is a
separate feature, not specified here.

**Acceptance criteria**

- [ ] No login path ever changes a character's `WorldID` or carries its
      content-bound state into another world.

---

## 8. Hosting model

The roster and login gate are built to support an active world **set** (multiple
worlds), but a server **runs one world per process** for now. Co-hosting two full
rulesets in one process is **deferred** — it depends on the prerequisite in §9.
Nothing in the single-world path is blocked by that deferral.

**Acceptance criteria**

- [ ] The gate and roster operate over a *set* of active worlds (not a hardcoded
      single world), so co-hosting is an additive change later, not a redesign.
- [ ] A single-world-per-process server is fully functional under this model.

---

## 9. Deferred prerequisite — global-registry namespacing

Race, class, track, and background ids are stored **un-namespaced** and resolve
against a **global, last-pack-wins** registry (unlike rooms / items / abilities /
recipes, which are pack-qualified). This is safe when a server hosts **one
world** per process; it is **unsafe** the moment two full rulesets share one
process, where a bare id (e.g. `channeler`) could collide across worlds.

Co-hosting therefore requires **namespacing those global registries** — recorded
here as the **next domino**, explicitly **deferred**. World-locking ships first
and is safe for single-world servers; the namespacing work is sequenced when
co-hosting is actually wanted.

---

## 10. Configuration surface

The following are externally configurable and not fixed by this spec.

| Setting | Meaning | Default |
|---|---|---|
| Pack world/library flag | Whether a pack is a world or a library (§2). | library (a pack opts in to being a world) |
| Active world set | The world-flagged packs active on a server (§2). | derived from the active pack selection |
| Backfill fallback world | The world assigned to a pre-stamp save whose location namespace is unparseable (§4). | the default starter world |

---

## 11. Decisions (§7 — resolved 2026-06-16) and open questions

**Decided** (all in favor of the proposal's recommended defaults):

- **World stamp = leaf-pack id + a manifest world/library flag** (§2, §3). Not the
  full active-pack set (brittle) and not a separate ruleset field (extra
  indirection); a library pack can never be a stamp.
- **Support many worlds, run one per process** (§8). Co-hosting deferred to §9.
- **A character whose world isn't active: hidden from the roster (footnote
  count), never entered** (§5).
  Never auto-deleted, never silently degraded.
- **Backfill `WorldID` from the location namespace**, fallback to the default
  world (§4). Deterministic, no operator input.
- **A library/baseline pack can never be a character's world** (§2), enforced by
  the manifest flag.
- **Global-registry namespacing is deferred** (§9) — the co-host prerequisite;
  world-locking ships first.

**Still open (non-blocking):**

- **Co-hosting + registry namespacing** — the next domino (§9): namespacing
  race/class/track/background to let two full rulesets share one process. Scope it
  when co-hosting is a real goal.
- **Explicit cross-world conversion** — the lossy opt-in re-roll ritual (§7) is a
  separate future feature if cross-ruleset character transfer is ever wanted.

---

<!-- Scope: WorldID stamp + manifest world/library flag + v23 backfill-from-location migration + login/roster gate, fail-soft scope narrowed · Spec style: narrative + acceptance criteria · Detail level: behavior only · Resolves docs/proposals/character-identity-across-packs.md §7 -->
